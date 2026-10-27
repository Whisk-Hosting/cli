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
	files, err := openUploads(dir, []string{"archive.tar"}, "")
	must(t, err)
	defer files[0].file.Close()
	if files[0].size <= scanMaxBytes {
		t.Fatalf("size %d", files[0].size)
	}
	if !strings.Contains(uploadScanNote(files[0]), "archive.tar is 180 MB, over the 100 MB the virus scan reads, so it is stored without a scan") {
		t.Errorf("note = %q", uploadScanNote(files[0]))
	}
	if uploadScanNote(uploadFile{name: "small.jpg", size: scanMaxBytes}) != "" {
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
