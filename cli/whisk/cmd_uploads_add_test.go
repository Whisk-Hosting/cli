package whisk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/contract/apitypes"
)

// uploadsAPI is the control plane and the object store for whisk uploads add: the form is
// authorised for the file's size and type, the store takes the post, and the record moves from
// authorised to scanned and converted as it is read.
func uploadsAPI(t *testing.T, authorised *map[string]any, stored *string, storedType *string, status string) *httptest.Server {
	mux := http.NewServeMux()
	var srv *httptest.Server
	var reads atomic.Int32
	app := "/v1/orgs/acme/apps/crm"
	record := func(stage int32) map[string]any {
		u := map[string]any{"id": "01UP", "key": "app/a1/01UP-hero.mp4", "filename": "hero.mp4", "bytes": 9,
			"content_type": "video/mp4", "status": "authorised", "visibility": "public", "authorised_by": "u1",
			"authorised_at": "2026-10-19T01:00:00Z",
			"media": map[string]any{"status": "pending", "kind": "video", "outputs": []any{}, "path": "/.whisk/media/01UP",
				"url": "https://crm--acme.whisk.page/.whisk/media/01UP", "html": `<whisk-video media="01UP"></whisk-video>`}}
		if stage > 0 {
			u["status"] = status
			u["media"].(map[string]any)["status"] = "ready"
		}
		return u
	}
	mux.HandleFunc("POST "+app+"/uploads", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(authorised)
		writeJSON(w, 201, map[string]any{"id": "01UP", "key": "app/a1/01UP-hero.mp4", "url": srv.URL + "/store/bucket", "method": "POST",
			"fields":    map[string]any{"key": "app/a1/01UP-hero.mp4", "policy": "p", "Content-Type": (*authorised)["content_type"]},
			"max_bytes": (*authorised)["max_bytes"], "expires_at": "2026-10-19T02:00:00Z"})
	})
	mux.HandleFunc("POST /store/bucket", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("the store was sent the token")
		}
		if r.ContentLength <= 0 {
			t.Error("the post's length was not known up front")
		}
		*storedType = r.FormValue("Content-Type")
		f, h, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		if h.Filename != "hero.mp4" {
			t.Errorf("the file part is named %q", h.Filename)
		}
		b, _ := io.ReadAll(f)
		*stored = string(b)
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET "+app+"/uploads/01UP", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, record(reads.Add(1)-1))
	})
	mux.HandleFunc("GET "+app+"/uploads", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []any{record(1)}})
	})
	mux.HandleFunc("PATCH "+app+"/uploads/01UP", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		u := record(1)
		u["path"] = in["path"]
		(*authorised)["patched_path"] = in["path"]
		writeJSON(w, 200, u)
	})
	srv = httptest.NewServer(mux)
	return srv
}

// TestUploadsAdd: a video is authorised as video/mp4 for its exact size, posted straight to the
// store with the form's fields, and --wait follows it until it plays.
func TestUploadsAdd(t *testing.T) {
	defer func(old time.Duration) { uploadPoll = old }(uploadPoll)
	uploadPoll = time.Millisecond
	var authorised map[string]any
	var stored, storedType string
	srv := uploadsAPI(t, &authorised, &stored, &storedType, "unscanned")
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.WriteFile(filepath.Join(dir, "hero.mp4"), []byte("\x00\x00\x00\x18ftyp"), 0o600))

	r := runRemote(t, dir, srv.URL, nil, "uploads", "add", "hero.mp4", "--public", "--wait", "--json")
	if r.Code != 0 {
		t.Fatalf("uploads add: %+v", r)
	}
	if authorised["filename"] != "hero.mp4" || authorised["content_type"] != "video/mp4" || authorised["max_bytes"] != float64(8) || authorised["visibility"] != "public" {
		t.Errorf("authorised %v", authorised)
	}
	if stored != "\x00\x00\x00\x18ftyp" || storedType != "video/mp4" {
		t.Errorf("the store got %q as %q", stored, storedType)
	}
	ups := r.JSON["uploads"].([]any)
	got := ups[0].(map[string]any)
	if len(ups) != 1 || got["id"] != "01UP" || got["media"].(map[string]any)["status"] != "ready" {
		t.Errorf("result %v", r.JSON)
	}
	if strings.Contains(r.Stderr, "upload: 01UP is") {
		t.Errorf("--json printed the stages: %q", r.Stderr)
	}

	// Without --json, the person reads that the file was stored without a scan.
	r = runRemote(t, dir, srv.URL, nil, "uploads", "add", "hero.mp4", "--public", "--wait")
	if r.Code != 0 || !strings.Contains(r.Stdout, "stored without a virus scan, ready to play") {
		t.Errorf("the stage = %+v", r)
	}
}

// TestUploadsAddOverTheScanLimit: a file over 100 MB is named on stderr before it goes up as one
// the virus scan does not read.
func TestUploadsAddOverTheScanLimit(t *testing.T) {
	uploadPoll = time.Millisecond
	var authorised map[string]any
	var stored, storedType string
	srv := uploadsAPI(t, &authorised, &stored, &storedType, "unscanned")
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	f, err := os.Create(filepath.Join(dir, "archive.tar"))
	must(t, err)
	must(t, f.Truncate(180<<20))
	must(t, f.Close())
	files, err := openUploads(dir, []string{"archive.tar"}, "", false)
	must(t, err)
	defer files[0].file.Close()
	if files[0].size <= scanMaxBytes {
		t.Fatalf("size %d", files[0].size)
	}
	if !strings.Contains(uploadScanNote(files[0], 0), "archive.tar is 180 MB, over the 100 MB the virus scan reads, so it is stored without a scan") {
		t.Errorf("note = %q", uploadScanNote(files[0], 0))
	}
	if uploadScanNote(uploadFile{name: "small.jpg", size: scanMaxBytes}, 0) != "" || uploadScanNote(uploadFile{name: "big.jpg", size: 60 << 20}, 50<<20) == "" {
		t.Error("a file at the limit got a note")
	}
}

// TestUploadsAddPrivateByDefault: without --public the upload is private, and the person is
// told where it plays and how to follow the conversion.
func TestUploadsAddPrivateByDefault(t *testing.T) {
	var authorised map[string]any
	var stored, storedType string
	srv := uploadsAPI(t, &authorised, &stored, &storedType, "unscanned")
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.WriteFile(filepath.Join(dir, "hero.mp4"), []byte("video-bit"), 0o600))

	r := runRemote(t, dir, srv.URL, nil, "uploads", "add", "hero.mp4")
	if r.Code != 0 || authorised["visibility"] != "private" {
		t.Fatalf("uploads add: %+v %v", r, authorised)
	}
	for _, want := range []string{"01UP  hero.mp4", "plays at https://crm--acme.whisk.page/.whisk/media/01UP", `<whisk-video media="01UP">`, "whisk uploads show 01UP --wait"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("stdout lacks %q: %s", want, r.Stdout)
		}
	}
	if !strings.Contains(r.Stderr, "upload: 100%") {
		t.Errorf("no progress: %q", r.Stderr)
	}
}

// TestUploadsAddRefusesBeforeSending: a missing file, a folder and an empty file are refused
// before anything is authorised, even alongside a good one.
func TestUploadsAddRefusesBeforeSending(t *testing.T) {
	var authorised map[string]any
	var stored, storedType string
	srv := uploadsAPI(t, &authorised, &stored, &storedType, "clean")
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.WriteFile(filepath.Join(dir, "hero.mp4"), []byte("video-bit"), 0o600))
	must(t, os.WriteFile(filepath.Join(dir, "empty.png"), nil, 0o600))
	for _, bad := range []string{"missing.mp4", ".", "empty.png"} {
		r := runRemote(t, dir, srv.URL, nil, "uploads", "add", "hero.mp4", bad, "--json")
		if r.Code == 0 || errCode(r) != "INVALID_REQUEST" {
			t.Errorf("%s: %+v", bad, r)
		}
	}
	if authorised != nil || stored != "" {
		t.Errorf("something was sent: %v %q", authorised, stored)
	}
}

// TestUploadsShowAndList: show reads one upload and list every one, for a person and as JSON.
func TestUploadsShowAndList(t *testing.T) {
	var authorised map[string]any
	var stored, storedType string
	srv := uploadsAPI(t, &authorised, &stored, &storedType, "clean")
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "uploads", "show", "01UP", "--json")
	if r.Code != 0 || r.JSON["upload"].(map[string]any)["status"] != "authorised" {
		t.Errorf("show: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "uploads", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "01UP  hero.mp4  public") || !strings.Contains(r.Stdout, "scanned, ready to play") {
		t.Errorf("list: %+v", r)
	}
}

// TestUploadsShowQuarantined: an upload the virus scan rejected is OBJECT_QUARANTINED.
func TestUploadsShowQuarantined(t *testing.T) {
	var authorised map[string]any
	var stored, storedType string
	srv := uploadsAPI(t, &authorised, &stored, &storedType, "infected")
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	defer func(old time.Duration) { uploadPoll = old }(uploadPoll)
	uploadPoll = time.Millisecond
	r := runRemote(t, dir, srv.URL, nil, "uploads", "show", "01UP", "--wait", "--json")
	if r.Code == 0 || errCode(r) != "OBJECT_QUARANTINED" {
		t.Errorf("show: %+v", r)
	}
}

func TestUploadType(t *testing.T) {
	mp4 := []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
	for _, c := range []struct {
		name string
		head []byte
		want string
	}{
		{"hero.mp4", nil, "video/mp4"},
		{"HERO.MOV", nil, "video/quicktime"},
		{"song.mp3", nil, "audio/mpeg"},
		{"photo.HEIC", nil, "image/heic"},
		{"logo.png", nil, "image/png"},
		{"Veetu-Seismic-Brace-Kit-3mm.step", []byte("ISO-10303-21;"), "model/step"},
		{"trapeze.glb", nil, "model/gltf-binary"},
		{"price-list.xlsx", nil, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"no-extension", mp4, "video/mp4"},
		{"no-extension", []byte("%PDF-1.7"), "application/pdf"},
		{"no-extension", []byte("plain words"), "text/plain; charset=utf-8"},
	} {
		if got := uploadType(c.name, c.head); got != c.want {
			t.Errorf("uploadType(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestUploadSettled(t *testing.T) {
	media := func(s apitypes.MediaStatus) *apitypes.UploadMedia { return &apitypes.UploadMedia{Status: s} }
	for _, c := range []struct {
		u    api.Upload
		want bool
	}{
		{api.Upload{Status: "authorised"}, false},
		{api.Upload{Status: "missing"}, false},
		{api.Upload{Status: "infected"}, true},
		{api.Upload{Status: "clean"}, true},
		{api.Upload{Status: "unscanned", Media: media("pending")}, false},
		{api.Upload{Status: "clean", Media: media("converting")}, false},
		{api.Upload{Status: "clean", Media: media("ready")}, true},
		{api.Upload{Status: "clean", Media: media("failed")}, true},
		{api.Upload{Status: "unscanned", Media: media("held")}, true},
	} {
		if got := uploadSettled(c.u); got != c.want {
			t.Errorf("uploadSettled(%s, %+v) = %v", c.u.Status, c.u.Media, got)
		}
	}
}

// --path names the site path of the one file given, sent once the file is stored.
func TestUploadsAddWithAPath(t *testing.T) {
	var authorised map[string]any
	var stored, storedType string
	srv := uploadsAPI(t, &authorised, &stored, &storedType, "clean")
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.WriteFile(filepath.Join(dir, "hero.mp4"), []byte("video-bit"), 0o600))
	r := runRemote(t, dir, srv.URL, nil, "uploads", "add", "hero.mp4", "--public", "--path", "/videos/hero.mp4", "--json")
	if r.Code != 0 || authorised["patched_path"] != "/videos/hero.mp4" || stored != "video-bit" {
		t.Fatalf("uploads add --path: %+v %v", r, authorised)
	}
	if got := r.JSON["uploads"].([]any)[0].(map[string]any)["path"]; got != "/videos/hero.mp4" {
		t.Errorf("result path %v", got)
	}
	must(t, os.MkdirAll(filepath.Join(dir, "media"), 0o700))
	if r := runRemote(t, dir, srv.URL, nil, "uploads", "add", "media", "--path", "/x", "--json"); r.Code == 0 || errCode(r) != "INVALID_REQUEST" {
		t.Errorf("--path with a folder: %+v", r)
	}
}

// --keep-paths gives every file of a folder its place under it on the site, leaving out hidden
// files, and opens each only when its turn comes.
func TestOpenUploadsKeepsPaths(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"public/posters/vlock install.jpg": "jpeg", "public/specs/bracket.step": "ISO-10303-21;",
		"public/.DS_Store": "x", "public/.git/HEAD": "ref", "loose.pdf": "%PDF",
	} {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o700))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	files, err := openUploads(dir, []string{"public", "loose.pdf"}, "", true)
	must(t, err)
	got := map[string]string{}
	for _, f := range files {
		got[f.sitePath] = f.contentType
		if f.file != nil {
			t.Errorf("%s is held open", f.path)
		}
	}
	want := map[string]string{"/posters/vlock install.jpg": "image/jpeg", "/specs/bracket.step": "model/step", "/loose.pdf": "application/pdf"}
	if len(got) != len(want) {
		t.Errorf("site paths %v, want %v", got, want)
	}
	for p, ct := range want {
		if got[p] != ct {
			t.Errorf("%s: %q, want %q (all: %v)", p, got[p], ct, got)
		}
	}
	if _, err := openUploads(dir, []string{"public"}, "", false); err == nil {
		t.Error("a folder without --keep-paths was taken")
	}
}

// TestAlreadyThere: a file whose path already answers with a whole upload of its size, seen by
// the same people, is not sent again; anything else is.
func TestAlreadyThere(t *testing.T) {
	held := []api.Upload{
		{ID: "02", Path: "/a.pdf", Bytes: 10, Status: apitypes.UploadClean, Visibility: "public"},
		{ID: "01", Path: "/a.pdf", Bytes: 99, Status: apitypes.UploadClean, Visibility: "public"},
		{ID: "03", Path: "/big.pdf", Bytes: 10, Status: apitypes.UploadUnscanned, Visibility: "public"},
		{ID: "04", Path: "/half.pdf", Bytes: 10, Status: apitypes.UploadAuthorised, Visibility: "public"},
		{ID: "05", Path: "/bad.pdf", Bytes: 10, Status: apitypes.UploadInfected, Visibility: "public"},
	}
	for _, c := range []struct {
		name       string
		f          uploadFile
		visibility string
		want       string
	}{
		{"the newest upload at the path, the same size", uploadFile{sitePath: "/a.pdf", size: 10}, "public", "02"},
		{"another size goes up", uploadFile{sitePath: "/a.pdf", size: 99}, "public", ""},
		{"another visibility goes up", uploadFile{sitePath: "/a.pdf", size: 10}, "private", ""},
		{"stored without a scan counts", uploadFile{sitePath: "/big.pdf", size: 10}, "public", "03"},
		{"one that never finished goes up", uploadFile{sitePath: "/half.pdf", size: 10}, "public", ""},
		{"a quarantined one goes up", uploadFile{sitePath: "/bad.pdf", size: 10}, "public", ""},
		{"a new path goes up", uploadFile{sitePath: "/new.pdf", size: 10}, "public", ""},
		{"a file with no path always goes up", uploadFile{size: 10}, "public", ""},
	} {
		u, ok := alreadyThere(c.f, c.visibility, held)
		if got := map[bool]string{true: u.ID}[ok]; got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestUploadsAddSkipsWhatIsThere: a folder sent again sends only what is not already at its path, the
// listing is read page by page, and a file the store did not take is sent again.
func TestUploadsAddSkipsWhatIsThere(t *testing.T) {
	defer func(old time.Duration) { uploadRetry = old }(uploadRetry)
	uploadRetry = time.Millisecond
	mux := http.NewServeMux()
	var srv *httptest.Server
	app := "/v1/orgs/acme/apps/crm"
	var posts, fails atomic.Int32
	pages := map[string]any{
		"":   map[string]any{"items": []any{map[string]any{"id": "09", "path": "/docs/a.pdf", "bytes": 3, "status": "clean", "visibility": "public"}}, "next_cursor": "09"},
		"09": map[string]any{"items": []any{map[string]any{"id": "08", "path": "/docs/b.pdf", "bytes": 3, "status": "clean", "visibility": "public"}}, "next_cursor": ""},
	}
	mux.HandleFunc("GET "+app+"/uploads", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, pages[r.URL.Query().Get("cursor")])
	})
	mux.HandleFunc("POST "+app+"/uploads", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 201, map[string]any{"id": "10", "key": "app/a1/10-c.pdf", "url": srv.URL + "/store", "method": "POST",
			"fields": map[string]any{"key": "app/a1/10-c.pdf"}, "max_bytes": 3, "expires_at": "2026-10-19T02:00:00Z"})
	})
	mux.HandleFunc("POST /store", func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		if fails.Add(1) == 1 {
			w.WriteHeader(502)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET "+app+"/uploads/10", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "10", "status": "clean", "bytes": 3, "visibility": "public"})
	})
	mux.HandleFunc("PATCH "+app+"/uploads/10", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "10", "status": "clean", "bytes": 3, "visibility": "public", "path": "/docs/c.pdf"})
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.MkdirAll(filepath.Join(dir, "site", "docs"), 0o700))
	for _, f := range []string{"a.pdf", "b.pdf", "c.pdf"} {
		must(t, os.WriteFile(filepath.Join(dir, "site", "docs", f), []byte("pdf"), 0o600))
	}
	r := runRemote(t, dir, srv.URL, nil, "uploads", "add", "site", "--public", "--keep-paths", "--json")
	if r.Code != 0 {
		t.Fatalf("uploads add: %+v", r)
	}
	if n := posts.Load(); n != 2 {
		t.Errorf("the store was sent %d posts, want 2: c.pdf, again after the store failed", n)
	}
	ids := []string{}
	for _, u := range r.JSON["uploads"].([]any) {
		ids = append(ids, u.(map[string]any)["id"].(string))
	}
	if strings.Join(ids, ",") != "09,08,10" {
		t.Errorf("uploads %v", ids)
	}
}
