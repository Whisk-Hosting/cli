package whisk

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// maxUploadBytes is the most one upload form takes (CONTROL-PLANE.md §6.13).
const maxUploadBytes = 5 << 30

// scanMaxBytes is the largest object the virus scan reads; a larger one is stored without a scan,
// status unscanned (CONTROL-PLANE.md §6.13 "Scanning").
const scanMaxBytes = 100 << 20

// uploadPoll is how often --wait asks how an upload's scan and conversion are going.
var uploadPoll = 5 * time.Second

// uploadsAddCmd uploads files from this computer into the app's storage (CLI.md §5.9): each is
// authorised with this sign-in, posted straight to the store and then scanned and, for video,
// audio and pictures, made ready to show, exactly as a file a visitor uploads through the app.
func uploadsAddCmd(s *session) *cobra.Command {
	var public, wait bool
	var contentType string
	c := &cobra.Command{
		Use:   "add <file>... [--public] [--content-type <type>] [--wait]",
		Short: "Upload files from this computer to the app's storage, up to 5 GB each",
		Long: `Uploads files from this computer into the app's storage, the way a visitor's upload through the
app arrives: straight to the store, then virus-scanned, and video and audio converted for every
browser. Each prints its id and the address it shows or plays at. Put a video in a page with
<whisk-video media="<id>"> and a picture with <img src="/.whisk/img/<id>?w=800">.

Uploads are private (people signed in to the app) unless --public (anyone, embeddable, such as a
product video on a website). The type comes from the file's name and contents; --content-type
sets it for every file given. --wait follows each until its scan and conversion are done;
whisk uploads show <id> --wait does it later.

The virus scan reads files up to 100 MB. A larger file is stored without a scan (its status is
unscanned): the app can still use it, and a file that must be scanned is split into pieces under
100 MB before it is uploaded. With --json, only the JSON goes to stdout and the progress lines
are left out.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := openUploads(s.env.Dir, args, contentType)
			if err != nil {
				return err
			}
			defer func() {
				for _, f := range files {
					f.file.Close()
				}
			}()
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			visibility := "private"
			if public {
				visibility = "public"
			}
			quiet := s.quiet || s.json
			log := s.env.Stderr
			if quiet {
				log = io.Discard
			}
			out := make([]api.Upload, 0, len(files))
			for _, f := range files {
				if note := uploadScanNote(f); note != "" {
					fmt.Fprintln(log, note)
				}
				u, err := addUpload(s, client, org, app, f, visibility)
				if err != nil {
					return err
				}
				if wait && !uploadSettled(u) {
					if u, err = followUpload(s.ctx, client, org, app, u.ID, log, uploadPoll); err != nil {
						return wrap(err)
					}
				}
				if u.Status == "infected" {
					return uploadQuarantined(u)
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
	c.Flags().StringVar(&contentType, "content-type", "", "the type of every file given, such as video/mp4 (default: from each file's name and contents)")
	c.Flags().BoolVar(&wait, "wait", false, "follow each upload until its scan and conversion are done")
	return c
}

// uploadFile is one file to upload, opened and measured before anything is authorised.
type uploadFile struct {
	file        *os.File
	path, name  string
	size        int64
	contentType string
}

// openUploads opens every file given and works out its type, refusing the lot before anything
// is uploaded when one cannot be read, is a folder, is empty or is over 5 GB.
func openUploads(dir string, args []string, contentType string) ([]uploadFile, error) {
	files := make([]uploadFile, 0, len(args))
	fail := func(path, why, fix string) ([]uploadFile, error) {
		for _, f := range files {
			f.file.Close()
		}
		return nil, output.New("INVALID_REQUEST", "Cannot upload "+path+": "+why+".", fix, map[string]any{"file": path})
	}
	for _, path := range args {
		if !filepath.IsAbs(path) && dir != "" {
			path = filepath.Join(dir, path)
		}
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
			return fail(path, "it is a folder", "Pass each file in it, such as whisk uploads add media/*.mp4.")
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
		files = append(files, uploadFile{file: f, path: path, name: filepath.Base(path), size: info.Size(), contentType: t})
	}
	return files, nil
}

// uploadScanNote is the line said before a file the virus scan will not read goes up, "" for
// one it reads. Pure.
func uploadScanNote(f uploadFile) string {
	if f.size <= scanMaxBytes {
		return ""
	}
	return fmt.Sprintf("upload: %s is %s, over the 100 MB the virus scan reads, so it is stored without a scan", f.name, sizeMB(f.size))
}

// addUpload authorises one file, posts it to the store and reads back its record. A file
// larger than one part goes up in parts, and carries on where an earlier run stopped.
func addUpload(s *session, client *api.Client, org, app string, f uploadFile, visibility string) (api.Upload, error) {
	if f.size > partsFrom {
		return addInParts(s, client, org, app, f, visibility)
	}
	auth, err := client.AuthoriseUpload(s.ctx, org, app, f.name, f.contentType, visibility, f.size, false)
	if err != nil {
		return api.Upload{}, wrap(err)
	}
	quiet := s.quiet || s.json
	if !quiet {
		fmt.Fprintf(s.env.Stderr, "upload: %s (%s, %s)\n", f.name, f.contentType, sizeMB(f.size))
	}
	if err := client.PostUpload(s.ctx, auth, f.name, f.contentType, f.file, f.size, uploadProgress(s.env.Stderr, f.size, quiet)); err != nil {
		return api.Upload{}, output.New("PLATFORM_UNAVAILABLE", f.name+" did not upload: "+err.Error()+".",
			"Run the same whisk uploads add again; the file was not stored.", map[string]any{"file": f.path, "upload": auth.ID})
	}
	u, err := client.GetUpload(s.ctx, org, app, auth.ID)
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
			u, err := client.GetUpload(s.ctx, org, app, args[0])
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
		u, err := client.GetUpload(ctx, org, app, id)
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
		}
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
}

// uploadTypes are the types of files a website brings that Go's own table leaves out or names
// differently from one system to the next; the store keeps the type it was given and the platform
// converts only video/*, audio/* and image/* (CONTROL-PLANE.md §6.13).
var uploadTypes = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
	".mkv": "video/x-matroska", ".avi": "video/x-msvideo", ".mpeg": "video/mpeg", ".mpg": "video/mpeg",
	".ogv": "video/ogg", ".3gp": "video/3gpp",
	".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".aac": "audio/aac", ".wav": "audio/wav",
	".ogg": "audio/ogg", ".oga": "audio/ogg", ".opus": "audio/opus", ".flac": "audio/flac",
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".gif": "image/gif",
	".webp": "image/webp", ".avif": "image/avif", ".heic": "image/heic", ".heif": "image/heif",
	".svg": "image/svg+xml", ".tif": "image/tiff", ".tiff": "image/tiff", ".bmp": "image/bmp",
	".ico": "image/x-icon", ".pdf": "application/pdf",
}

// uploadType is a file's content type from its name, or failing that from its first bytes.
// Pure but for the system's own type table.
func uploadType(name string, head []byte) string {
	ext := strings.ToLower(filepath.Ext(name))
	if t, ok := uploadTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); ext != "" && t != "" {
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
