package whisk

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/release"
)

// releaseServer serves a release directory: VERSION, SHA256SUMS, an optional signature and one
// binary for this platform.
func releaseServer(t *testing.T, version string, binary []byte, sums string, sig string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)
	mux.HandleFunc("GET /dl/VERSION", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(version + "\n")) })
	mux.HandleFunc("GET /dl/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(sums)) })
	mux.HandleFunc("GET /dl/SHA256SUMS.sig", func(w http.ResponseWriter, r *http.Request) {
		if sig == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(sig + "\n"))
	})
	mux.HandleFunc("GET /dl/"+name, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(binary) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	return httptest.NewServer(mux)
}

func runUpdate(t *testing.T, exe, base string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	env := map[string]string{"WHISK_RELEASE_URL": base}
	e := Env{Stdout: &out, Stderr: &errb, Dir: t.TempDir(), ConfigDir: t.TempDir(), Store: &memStore{m: map[string]config.Credential{}}, Executable: exe, Getenv: func(k string) string { return env[k] }}
	code := Run(context.Background(), append([]string{"update"}, args...), e)
	r := result{Code: code, Stdout: out.String(), Stderr: errb.String()}
	if lines := jsonLines(r.Stdout); len(lines) > 0 {
		r.JSON = lines[len(lines)-1]
	}
	return r
}

func fakeExe(t *testing.T) string {
	dir := t.TempDir()
	path := filepath.Join(dir, "whisk")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	must(t, os.WriteFile(path, []byte("old binary"), 0o755))
	return path
}

func TestUpdateReplacesTheBinaryAfterChecksum(t *testing.T) {
	withoutReleaseKey(t)
	binary := []byte("new binary bytes")
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)
	srv := releaseServer(t, "v9.9.9", binary, release.Digest(binary)+"  "+name+"\n", "")
	defer srv.Close()
	exe := fakeExe(t)

	r := runUpdate(t, exe, srv.URL+"/dl", "--json")
	if r.Code != 0 || r.JSON["updated"] != true || r.JSON["latest"] != "v9.9.9" || r.JSON["signature_verified"] != false {
		t.Fatalf("update: %+v", r)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new binary bytes" {
		t.Fatalf("binary not replaced: %q", got)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(exe); info.Mode()&0o111 == 0 {
			t.Fatal("the new binary is not executable")
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	for _, e := range entries {
		if e.Name() != filepath.Base(exe) && e.Name() != filepath.Base(exe)+".old" {
			t.Fatalf("leftover %s", e.Name())
		}
	}
}

func TestUpdateChecksAndUpToDate(t *testing.T) {
	withoutReleaseKey(t)
	binary := []byte("new")
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)
	srv := releaseServer(t, "v9.9.9", binary, release.Digest(binary)+"  "+name+"\n", "")
	defer srv.Close()
	exe := fakeExe(t)

	r := runUpdate(t, exe, srv.URL+"/dl", "--check", "--json")
	if r.Code != 0 || r.JSON["updated"] != false || r.JSON["available"] != true {
		t.Fatalf("--check: %+v", r)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatal("--check must not install")
	}

	same := releaseServer(t, Version, binary, release.Digest(binary)+"  "+name+"\n", "")
	defer same.Close()
	r = runUpdate(t, exe, same.URL+"/dl", "--json")
	if r.Code != 0 || r.JSON["updated"] != false || r.JSON["latest"] != Version {
		t.Fatalf("up to date: %+v", r)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatal("an up-to-date binary must not be touched")
	}
	r = runUpdate(t, exe, same.URL+"/dl", "--force", "--json")
	if r.Code != 0 || r.JSON["updated"] != true {
		t.Fatalf("--force: %+v", r)
	}
}

func TestUpdateRefusesABadChecksum(t *testing.T) {
	withoutReleaseKey(t)
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)
	srv := releaseServer(t, "v9.9.9", []byte("evil"), release.Digest([]byte("good"))+"  "+name+"\n", "")
	defer srv.Close()
	exe := fakeExe(t)
	r := runUpdate(t, exe, srv.URL+"/dl", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "UPDATE_FAILED" {
		t.Fatalf("bad checksum: %+v", r)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatal("a bad checksum must not install")
	}

	missing := releaseServer(t, "v9.9.9", []byte("x"), "abcd  whisk_plan9_mips\n", "")
	defer missing.Close()
	r = runUpdate(t, exe, missing.URL+"/dl", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "UPDATE_FAILED" {
		t.Fatalf("missing asset: %+v", r)
	}
}

func TestUpdateVerifiesTheSignatureWhenTheBuildHasAKey(t *testing.T) {
	kp, err := release.GenerateKey()
	must(t, err)
	old := ReleasePublicKey
	ReleasePublicKey = kp.Public
	defer func() { ReleasePublicKey = old }()

	binary := []byte("signed binary")
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)
	oldReleased := Released
	Released = "1700000000"
	defer func() { Released = oldReleased }()
	stamped := func(version string, at int64) string {
		return release.Digest(binary) + "  " + name + "\n" + release.StampLine(release.Stamp{Version: version, Released: at}) + "\n"
	}
	sums := stamped("v9.9.9", 1700000100)
	sig, err := release.Sign(kp.Private, []byte(sums))
	must(t, err)

	good := releaseServer(t, "v9.9.9", binary, sums, sig)
	defer good.Close()
	exe := fakeExe(t)
	r := runUpdate(t, exe, good.URL+"/dl", "--json")
	if r.Code != 0 || r.JSON["updated"] != true || r.JSON["signature_verified"] != true {
		t.Fatalf("signed update: %+v", r)
	}

	other, err := release.GenerateKey()
	must(t, err)
	wrongSig, _ := release.Sign(other.Private, []byte(sums))
	bad := releaseServer(t, "v9.9.10", binary, sums, wrongSig)
	defer bad.Close()
	r = runUpdate(t, exe, bad.URL+"/dl", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "UPDATE_FAILED" {
		t.Fatalf("wrong key: %+v", r)
	}

	unsigned := releaseServer(t, "v9.9.11", binary, sums, "")
	defer unsigned.Close()
	r = runUpdate(t, exe, unsigned.URL+"/dl", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "UPDATE_FAILED" {
		t.Fatalf("unsigned release with a keyed build: %+v", r)
	}

	// A genuine but older release, served again, is refused unless --force.
	olderSums := stamped("v9.9.1", 1690000000)
	olderSig, _ := release.Sign(kp.Private, []byte(olderSums))
	older := releaseServer(t, "v9.9.12", binary, olderSums, olderSig)
	defer older.Close()
	must(t, os.WriteFile(exe, []byte("old binary"), 0o755))
	r = runUpdate(t, exe, older.URL+"/dl", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "UPDATE_FAILED" {
		t.Fatalf("older release: %+v", r)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatal("an older release must not install")
	}
	r = runUpdate(t, exe, older.URL+"/dl", "--force", "--json")
	if r.Code != 0 || r.JSON["updated"] != true || r.JSON["latest"] != "v9.9.1" {
		t.Fatalf("older release with --force, named by its signed stamp: %+v", r)
	}

	// A signed release without its stamp could be any release; it is refused.
	bare := release.Digest(binary) + "  " + name + "\n"
	bareSig, _ := release.Sign(kp.Private, []byte(bare))
	unstamped := releaseServer(t, "v9.9.13", binary, bare, bareSig)
	defer unstamped.Close()
	r = runUpdate(t, exe, unstamped.URL+"/dl", "--force", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "UPDATE_FAILED" {
		t.Fatalf("unstamped signed release: %+v", r)
	}
}

// withoutReleaseKey runs a test as a build that embeds no release key, which verifies
// checksums only; the pinned key would otherwise require every fake release to be signed.
func withoutReleaseKey(t *testing.T) {
	old := ReleasePublicKey
	ReleasePublicKey = ""
	t.Cleanup(func() { ReleasePublicKey = old })
}
