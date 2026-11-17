package whisk

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
	"github.com/whisk-run/contract/mimetypes"
)

// maxUploadBytes is the most one upload form takes (CONTROL-PLANE.md §6.13).
const maxUploadBytes = 5 << 30

// scanMaxBytes is the largest object the virus scan reads when the platform does not say; a
// larger one is stored without a scan, status unscanned (CONTROL-PLANE.md §6.13 "Scanning"). The
// platform says in each authorisation (scan_max_bytes).
const scanMaxBytes = 100 << 20

// uploadPoll is how often --wait asks how an upload's scan and conversion are going.
var uploadPoll = 5 * time.Second

// uploadsAddCmd uploads files from this computer into the app's storage (CLI.md §5.9): each is
// authorised with this sign-in, posted straight to the store and then scanned and, for video,
// audio and pictures, made ready to show, exactly as a file a visitor uploads through the app.
func uploadsAddCmd(s *session) *cobra.Command {
	var public, wait, keepPaths bool
	var contentType, sitePath string
	c := &cobra.Command{
		Use:   "add <file|folder>... [--public] [--keep-paths | --path <path>] [--content-type <type>] [--wait]",
		Short: "Upload files from this computer to the app's storage, up to 5 GB each",
		Long: `Uploads files from this computer into the app's storage, the way a visitor's upload through the
app arrives: straight to the store, then virus-scanned, and video and audio converted for every
browser. Each prints its id and the address it shows or plays at. Put a video in a page with
<whisk-video media="<id>">, a picture with <img src="/.whisk/img/<id>?w=800">, and link any other
file, such as a PDF, at /.whisk/file/<id>.

Uploads are private (people signed in to the app) unless --public (anyone, embeddable, such as a
product video on a website).

--keep-paths gives each file a path on the app's site: every file in a folder given keeps its
place under it, so whisk uploads add public/ --public --keep-paths makes public/posters/a.jpg
answer at /posters/a.jpg, whenever the app itself has nothing there. A website links its files
by those paths, and old links to them keep working. Uploading to a path again replaces the file
there once the new one is scanned. --path names the path of a single file. The type comes from the file's name and contents; --content-type
sets it for every file given. --wait follows each until its scan and conversion are done;
whisk uploads show <id> --wait does it later.

The virus scan reads files up to 100 MB. A larger file is stored without a scan (its status is
unscanned): the app can still use it, and a file that must be scanned is split into pieces under
100 MB before it is uploaded. With --json, only the JSON goes to stdout and the progress lines
are left out.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := uploadsToAdd(s.env.Dir, args, contentType, keepPaths, sitePath)
			if err != nil {
				return err
			}
			defer closeUploads(files)
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			visibility := "private"
			if public {
				visibility = "public"
			}
			log := s.env.Stderr
			if s.quiet || s.json {
				log = io.Discard
			}
			out := make([]api.Upload, 0, len(files))
			for i := range files {
				u, err := addOne(s, client, org, app, &files[i], visibility, wait, log)
				if err != nil {
					return err
				}
				out = append(out, u)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "uploads": out}, func(w io.Writer) {
				for _, u := range out {
					printUpload(w, u)
				}
				for _, u := range out {
					if !uploadSettled(u) {
						fmt.Fprintf(w, "The scan and conversion run in the background; follow them with whisk uploads show %s --wait.\n", u.ID)
						break
					}
				}
			})
			return nil
		},
	}
	c.Flags().BoolVar(&public, "public", false, "anyone may see or play it, and other websites may embed it")
	c.Flags().BoolVar(&keepPaths, "keep-paths", false, "give each file a path on the site: its place under the folder given")
	c.Flags().StringVar(&sitePath, "path", "", "the path on the site of the one file given, such as /posters/install.jpg")
	c.MarkFlagsMutuallyExclusive("keep-paths", "path")
	c.Flags().StringVar(&contentType, "content-type", "", "the type of every file given, such as video/mp4 (default: from each file's name and contents)")
	c.Flags().BoolVar(&wait, "wait", false, "follow each upload until its scan and conversion are done")
	return c
}

// uploadsToAdd opens the files given, a folder's with keepPaths, giving the one file sitePath
// when it is named.
func uploadsToAdd(dir string, args []string, contentType string, keepPaths bool, sitePath string) ([]uploadFile, error) {
	if sitePath != "" && len(args) != 1 {
		return nil, output.New("INVALID_REQUEST", "--path names the path of one file.",
			"Pass one file with --path, or a folder with --keep-paths to keep each file's place under it.", nil)
	}
	files, err := openUploads(dir, args, contentType, keepPaths)
	if err != nil {
		return nil, err
	}
	if sitePath != "" {
		if len(files) != 1 || files[0].file == nil {
			closeUploads(files)
			return nil, output.New("INVALID_REQUEST", "--path names the path of one file, and "+args[0]+" is a folder.",
				"Pass one file with --path, or the folder with --keep-paths.", nil)
		}
		files[0].sitePath = sitePath
	}
	return files, nil
}

func closeUploads(files []uploadFile) {
	for _, f := range files {
		if f.file != nil {
			f.file.Close()
		}
	}
}

// addOne uploads one file, names its site path, and follows it with wait.
func addOne(s *session, client *api.Client, org, app string, f *uploadFile, visibility string, wait bool, log io.Writer) (api.Upload, error) {
	if f.file == nil {
		var err error
		if f.file, err = os.Open(f.path); err != nil {
			return api.Upload{}, output.New("INVALID_REQUEST", "Cannot upload "+f.path+": "+err.Error()+".", "Pass the path of a file on this computer.", map[string]any{"file": f.path})
		}
	}
	u, err := addUpload(s, client, org, app, *f, visibility, log)
	f.file.Close()
	f.file = nil
	if err != nil {
		return api.Upload{}, err
	}
	if f.sitePath != "" {
		p := f.sitePath
		if u, err = api.Send(s.ctx, client, endpoints.PatchUpload, apitypes.PatchUploadRequest{Path: &p}, org, app, u.ID); err != nil {
			return api.Upload{}, wrap(err)
		}
	}
	if wait && !uploadSettled(u) {
		if u, err = followUpload(s.ctx, client, org, app, u.ID, log, uploadPoll); err != nil {
			return api.Upload{}, wrap(err)
		}
	}
	if u.Status == "infected" {
		return api.Upload{}, uploadQuarantined(u)
	}
	return u, nil
}

// uploadFile is one file to upload, opened and measured before anything is authorised.
type uploadFile struct {
	// file is open while the file is inspected and uploaded; a file of a folder is opened again
	// when its turn comes, so a large folder never holds every file open.
	file       *os.File
	path, name string
	// sitePath is the path on the app's site the file answers at, "" for none.
	sitePath    string
	size        int64
	contentType string
}

// openUploads opens every file given and works out its type, refusing the lot before anything
// is uploaded when one cannot be read, is a folder, is empty or is over 5 GB.
func openUploads(dir string, args []string, contentType string, keepPaths bool) ([]uploadFile, error) {
	files := make([]uploadFile, 0, len(args))
	fail := func(path, why, fix string) ([]uploadFile, error) {
		for _, f := range files {
			if f.file != nil {
				f.file.Close()
			}
		}
		return nil, output.New("INVALID_REQUEST", "Cannot upload "+path+": "+why+".", fix, map[string]any{"file": path})
	}
	each, folder, err := uploadTargets(dir, args, keepPaths)
	if err != nil {
		return fail(err.path, err.why, err.fix)
	}
	for _, w := range each {
		path := w.path
		f, err := os.Open(path)
		if err != nil {
			return fail(path, err.Error(), "Pass the path of a file on this computer.")
		}
		info, err := f.Stat()
		switch {
		case err != nil:
			f.Close()
			return fail(path, err.Error(), "Pass the path of a file on this computer.")
		case info.IsDir():
			f.Close()
			return fail(path, "it is a folder", "Pass each file in it, such as whisk uploads add media/*.mp4, or the folder with --keep-paths to give each file its path on the site.")
		case info.Size() == 0:
			f.Close()
			return fail(path, "it is empty", "Pass a file with something in it.")
		case info.Size() > maxUploadBytes:
			f.Close()
			return fail(path, fmt.Sprintf("it is %s and one upload may be at most 5 GB", sizeMB(info.Size())),
				"Make the file smaller, such as a video exported at a lower bitrate, then upload that.")
		}
		head := make([]byte, 512)
		n, _ := io.ReadFull(f, head)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			f.Close()
			return fail(path, err.Error(), "Pass the path of a file on this computer.")
		}
		t := contentType
		if t == "" {
			t = uploadType(filepath.Base(path), head[:n])
		}
		u := uploadFile{file: f, path: path, name: filepath.Base(path), size: info.Size(), contentType: t, sitePath: w.sitePath}
		if folder {
			f.Close()
			u.file = nil
		}
		files = append(files, u)
	}
	return files, nil
}

// uploadTarget is one file to upload and its site path.
type uploadTarget struct{ path, sitePath string }

// targetError is why a path given cannot be uploaded.
type targetError struct{ path, why, fix string }

// uploadTargets is every file the paths given name, a folder's files with keepPaths, and whether
// any came from a folder.
func uploadTargets(dir string, args []string, keepPaths bool) ([]uploadTarget, bool, *targetError) {
	var each []uploadTarget
	folder := false
	for _, path := range args {
		if !filepath.IsAbs(path) && dir != "" {
			path = filepath.Join(dir, path)
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() || !keepPaths {
			site := ""
			if keepPaths {
				site = "/" + filepath.Base(path)
			}
			each = append(each, uploadTarget{path, site})
			continue
		}
		found, err := folderFiles(path)
		if err != nil {
			return nil, false, &targetError{path, err.Error(), "Pass a folder this computer can read."}
		}
		if len(found) == 0 {
			return nil, false, &targetError{path, "it holds no files", "Pass a folder with the files to upload in it."}
		}
		folder = true
		for _, rel := range found {
			each = append(each, uploadTarget{filepath.Join(path, rel), "/" + filepath.ToSlash(rel)})
		}
	}
	return each, folder, nil
}

// folderFiles is every file under root, by its path relative to root, leaving out hidden files
// and folders (.DS_Store, .git) and anything that is not a regular file.
func folderFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, rel)
		return nil
	})
	return out, err
}

// uploadScanNote is the line said before a file the virus scan will not read goes up, "" for
// one it reads: limit is what the platform's authorisation says the scan reads, scanMaxBytes
// when it says nothing. Pure.
func uploadScanNote(f uploadFile, limit int64) string {
	if limit <= 0 {
		limit = scanMaxBytes
	}
	if f.size <= limit {
		return ""
	}
	return fmt.Sprintf("upload: %s is %s, over the %s the virus scan reads, so it is stored without a scan", f.name, sizeMB(f.size), sizeMB(limit))
}

// addUpload authorises one file, posts it to the store and reads back its record. A file
// larger than one part goes up in parts, and carries on where an earlier run stopped.
func addUpload(s *session, client *api.Client, org, app string, f uploadFile, visibility string, log io.Writer) (api.Upload, error) {
	if f.size > partsFrom {
		return addInParts(s, client, org, app, f, visibility, log)
	}
	auth, err := client.AuthoriseUpload(s.ctx, org, app, f.name, f.contentType, visibility, f.size, false)
	if err != nil {
		return api.Upload{}, wrap(err)
	}
	if note := uploadScanNote(f, auth.ScanMaxBytes); note != "" {
		fmt.Fprintln(log, note)
	}
	quiet := s.quiet || s.json
	if !quiet {
		fmt.Fprintf(s.env.Stderr, "upload: %s (%s, %s)\n", f.name, f.contentType, sizeMB(f.size))
	}
	if err := client.PostUpload(s.ctx, auth, f.name, f.contentType, f.file, f.size, uploadProgress(s.env.Stderr, f.size, quiet)); err != nil {
		return api.Upload{}, output.New("PLATFORM_UNAVAILABLE", f.name+" did not upload: "+err.Error()+".",
			"Run the same whisk uploads add again; the file was not stored.", map[string]any{"file": f.path, "upload": auth.ID})
	}
	u, err := api.Call(s.ctx, client, endpoints.GetUpload, org, app, auth.ID)
	if err != nil {
		return api.Upload{}, wrap(err)
	}
	return u, nil
}

// uploadsListCmd lists what the app has authorised, newest first.
func uploadsListCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "The app's uploads, newest first: scan, conversion and where each shows or plays",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			list, err := client.ListUploads(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "uploads": list}, func(w io.Writer) {
				if len(list) == 0 {
					fmt.Fprintln(w, "No uploads yet. Add one with whisk uploads add <file>.")
				}
				for _, u := range list {
					printUpload(w, u)
				}
			})
			return nil
		},
	}
}

// uploadsShowCmd reads one upload, following it with --wait until its scan and conversion are
// done.
func uploadsShowCmd(s *session) *cobra.Command {
	var wait bool
	c := &cobra.Command{
		Use:   "show <id> [--wait]",
		Short: "One upload: its scan, its conversion and where it shows or plays",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			u, err := api.Call(s.ctx, client, endpoints.GetUpload, org, app, args[0])
			if err == nil && wait && !uploadSettled(u) {
				log := s.env.Stderr
				if s.quiet || s.json {
					log = io.Discard
				}
				u, err = followUpload(s.ctx, client, org, app, args[0], log, uploadPoll)
			}
			if err != nil {
				return wrap(err)
			}
			if u.Status == "infected" {
				return uploadQuarantined(u)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "upload": u}, func(w io.Writer) {
				printUpload(w, u)
				if u.URL != "" {
					fmt.Fprintf(w, "  download (15 minutes): %s\n", u.URL)
				}
			})
			return nil
		},
	}
	c.Flags().BoolVar(&wait, "wait", false, "follow the upload until its scan and conversion are done")
	return c
}

// followUpload asks how an upload is going every so often, naming each change on log, until its
// scan and any conversion are done.
func followUpload(ctx context.Context, client *api.Client, org, app, id string, log io.Writer, every time.Duration) (api.Upload, error) {
	last := ""
	for {
		u, err := api.Call(ctx, client, endpoints.GetUpload, org, app, id)
		if err != nil {
			return u, err
		}
		if now := uploadStage(u); now != last {
			fmt.Fprintf(log, "upload: %s is %s\n", id, now)
			last = now
		}
		if uploadSettled(u) {
			return u, nil
		}
		select {
		case <-ctx.Done():
			return u, ctx.Err()
		case <-time.After(every):
		}
	}
}

// uploadSettled is whether nothing more will happen to an upload by itself: its scan has a
// verdict and, for video and audio, the conversion is ready, failed or held for the month. A
// file not seen yet (missing) is still on its way. Pure.
func uploadSettled(u api.Upload) bool {
	switch u.Status {
	case "infected":
		return true
	case "clean", "unscanned":
		if u.Media == nil {
			return true
		}
		switch u.Media.Status {
		case "ready", "failed", "held":
			return true
		case apitypes.MediaPending, apitypes.MediaConverting:
			// Nothing to do for these.
		}
	case apitypes.UploadAuthorised, apitypes.UploadMissing:
		// Nothing to do for these.
	}
	return false
}

// uploadStage is an upload's state in a few words: the scan's, then the conversion's. Pure.
func uploadStage(u api.Upload) string {
	stage := map[string]string{
		"authorised": "waiting for its scan", "missing": "waiting for the file to arrive",
		"clean": "scanned", "unscanned": "stored without a virus scan", "infected": "quarantined",
	}[string(u.Status)]
	if stage == "" {
		stage = string(u.Status)
	}
	if u.Media != nil && u.Status != "infected" {
		stage += ", " + map[string]string{
			"pending": "conversion queued", "converting": "converting", "ready": "ready to play",
			"failed": "conversion failed", "held": "conversion held until the month turns",
		}[string(u.Media.Status)]
	}
	return strings.TrimSuffix(stage, ", ")
}

// uploadQuarantined is the error for a file the virus scan rejected. Pure.
func uploadQuarantined(u api.Upload) error {
	why := u.Detail
	if why == "" {
		why = "a virus scanner flagged it"
	}
	return output.New("OBJECT_QUARANTINED", u.Filename+" was quarantined: "+why+".",
		"Check the file on this computer with a virus scanner, then upload a clean copy.",
		map[string]any{"upload": u.ID, "key": u.Key})
}

// printUpload is one upload for a person: what it is, how it stands and where it shows.
func printUpload(w io.Writer, u api.Upload) {
	fmt.Fprintf(w, "%s  %s  %s  %s  %s\n", u.ID, u.Filename, u.Visibility, sizeMB(u.Bytes), uploadStage(u))
	if u.Detail != "" {
		fmt.Fprintf(w, "  %s\n", u.Detail)
	}
	if m := u.Media; m != nil && u.Status != "infected" {
		if m.Detail != "" {
			fmt.Fprintf(w, "  %s\n", m.Detail)
		}
		fmt.Fprintf(w, "  plays at %s\n", firstOf(m.URL, m.Path))
		if m.HTML != "" {
			fmt.Fprintf(w, "  %s\n", m.HTML)
		}
	}
	if im := u.Image; im != nil && u.Status != "infected" {
		if im.Detail != "" {
			fmt.Fprintf(w, "  %s\n", im.Detail)
		} else {
			fmt.Fprintf(w, "  shows at %s?w=800\n", firstOf(im.URL, im.Path))
		}
	}
	if u.Media == nil && u.Image == nil && u.Status != "infected" && firstOf(u.FileURL, u.FilePath) != "" {
		fmt.Fprintf(w, "  opens at %s\n", firstOf(u.FileURL, u.FilePath))
	}
	if u.Path != "" && u.Status != "infected" {
		fmt.Fprintf(w, "  on the site at %s\n", u.Path)
	}
}

// uploadType is a file's content type from its name, by mime-db's list (contract/mimetypes), or
// failing that from its first bytes. Pure.
func uploadType(name string, head []byte) string {
	if t := mimetypes.ByName(name); t != "" {
		return t
	}
	return http.DetectContentType(head)
}

// sizeMB is a size for a person, in MB, or GB from a thousand MB. Pure.
func sizeMB(n int64) string {
	mb := float64(n) / (1 << 20)
	if mb >= 1000 {
		return fmt.Sprintf("%.1f GB", mb/1024)
	}
	if mb < 1 {
		return fmt.Sprintf("%d KB", (n+1023)/1024)
	}
	return fmt.Sprintf("%.0f MB", mb)
}

// firstOf is the first of the strings that is not empty. Pure.
func firstOf(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
