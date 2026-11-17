package whisk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/output"
)

func TestEngineName(t *testing.T) {
	if engineName("windows") != "engine.exe" || engineName("darwin") != "engine" {
		t.Error("the engine is engine, engine.exe on Windows")
	}
}

// whisk dev downloads the engine as the person signed in, checks it, keeps it, and downloads it
// again only when the platform serves another; a kept one runs when the platform cannot be
// reached, and nothing that does not match its digest is kept.
func TestDevEngine(t *testing.T) {
	program := "\x7fELF engine one"
	var downloads atomic.Int32
	var lie atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/commerce-engine/"+runtime.GOOS+"/"+runtime.GOARCH || r.Header.Get("Authorization") != "Bearer whsk_agent_test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		sum := sha256.Sum256([]byte(program))
		digest := hex.EncodeToString(sum[:])
		w.Header().Set("X-Whisk-Sha256", digest)
		if r.Header.Get("If-None-Match") == `"`+digest+`"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		downloads.Add(1)
		body := program
		if lie.Load() {
			body += " changed on the way"
		}
		_, _ = io.WriteString(w, body)
	}))
	dir := t.TempDir()
	env := map[string]string{"WHISK_TOKEN": "whsk_agent_test"}
	s := &session{ctx: context.Background(), cfgDir: dir, cfg: config.Config{API: srv.URL},
		env: Env{Getenv: func(k string) string { return env[k] }}, printer: output.Printer{Err: io.Discard}}

	path, err := s.devEngine()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != program {
		t.Fatalf("kept %q %v", b, err)
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm()&0o100 == 0 {
		t.Errorf("the engine is executable: %v", st.Mode())
	}
	if _, err := s.devEngine(); err != nil || downloads.Load() != 1 {
		t.Errorf("the engine still served is not downloaded again: %v, %d downloads", err, downloads.Load())
	}

	program = "\x7fELF engine two"
	if _, err := s.devEngine(); err != nil || downloads.Load() != 2 {
		t.Fatalf("a new engine is downloaded: %v, %d downloads", err, downloads.Load())
	}
	if b, _ := os.ReadFile(path); string(b) != program {
		t.Errorf("the new engine is kept: %q", b)
	}

	program = "\x7fELF engine three"
	lie.Store(true)
	if _, err := s.devEngine(); err == nil || !strings.Contains(err.Error(), "not what the platform named") {
		t.Errorf("an engine that does not match its digest is refused: %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "\x7fELF engine two" {
		t.Errorf("the engine kept before stays: %q", b)
	}

	srv.Close()
	if got, err := s.devEngine(); err != nil || got != path {
		t.Errorf("the kept engine runs when the platform cannot be reached: %q %v", got, err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.devEngine(); err == nil {
		t.Error("a kept engine that no longer matches its digest is not run")
	}
	env["WHISK_TOKEN"] = ""
	s.store = &memStore{m: map[string]config.Credential{}}
	if _, err := s.devEngine(); err == nil || !strings.Contains(err.Error(), "signed in") {
		t.Errorf("without a sign-in or a kept engine: %v", err)
	}
}

// A site's content engine comes from its own route and is kept in a folder of its own, so the
// two engines never take each other's place.
func TestDevContentEngine(t *testing.T) {
	program := "\x7fELF content engine"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/content-engine/"+runtime.GOOS+"/"+runtime.GOARCH {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		sum := sha256.Sum256([]byte(program))
		w.Header().Set("X-Whisk-Sha256", hex.EncodeToString(sum[:]))
		_, _ = io.WriteString(w, program)
	}))
	defer srv.Close()
	dir := t.TempDir()
	env := map[string]string{"WHISK_TOKEN": "whsk_agent_test"}
	s := &session{ctx: context.Background(), cfgDir: dir, cfg: config.Config{API: srv.URL},
		env: Env{Getenv: func(k string) string { return env[k] }}, printer: output.Printer{Err: io.Discard}}
	path, err := s.devContentEngine()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != program || !strings.Contains(path, "content-engine") || !strings.HasSuffix(strings.TrimSuffix(path, ".exe"), "content") {
		t.Fatalf("kept %q at %s", b, path)
	}
	if _, err := s.devEngine(); err == nil {
		t.Error("the commerce engine was taken from the content engine's route")
	}
}
