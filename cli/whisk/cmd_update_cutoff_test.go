package whisk

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/whisk-run/cli/internal/release"
)

// A download cut off half way is the platform's side, not a CLI fault, and installs nothing.
func TestUpdateCutOffDownloadIsPlatformUnavailable(t *testing.T) {
	withoutReleaseKey(t)
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dl/VERSION", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("v9.9.9\n")) })
	mux.HandleFunc("GET /dl/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(release.Digest([]byte("whole")) + "  " + name + "\n"))
	})
	mux.HandleFunc("GET /dl/SHA256SUMS.sig", http.NotFound)
	mux.HandleFunc("GET /dl/"+name, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("half"))
		w.(http.Flusher).Flush()
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	exe := fakeExe(t)
	r := runUpdate(t, exe, srv.URL+"/dl", "--json")
	if r.Code != 5 || r.JSON["error"].(map[string]any)["code"] != "PLATFORM_UNAVAILABLE" {
		t.Fatalf("cut off: %+v", r)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatal("a cut-off download must not install")
	}
}
