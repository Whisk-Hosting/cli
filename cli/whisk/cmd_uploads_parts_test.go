package whisk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/config"
)

// partsAPI is the control plane and the store for an upload in parts, with parts of 4 bytes:
// the store keeps what each part URL is sent, fails the parts named in failing (once, or for
// good when the count is negative), and joins the parts on complete.
type partsAPI struct {
	t         *testing.T
	srv       *httptest.Server
	mu        sync.Mutex
	stored    map[int][]byte
	failing   map[int]int
	puts      []int
	authorise int
	listed    int
	joined    []byte
	size      int64
}

func newPartsAPI(t *testing.T) *partsAPI {
	p := &partsAPI{t: t, stored: map[int][]byte{}, failing: map[int]int{}}
	mux := http.NewServeMux()
	app := "/v1/orgs/acme/apps/crm"
	mux.HandleFunc("POST "+app+"/uploads", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["parts"] != true {
			t.Errorf("a large file was not asked for in parts: %v", in)
		}
		p.mu.Lock()
		p.authorise++
		p.size = int64(in["max_bytes"].(float64))
		p.mu.Unlock()
		writeJSON(w, 201, map[string]any{"id": "01UP", "key": "app/a1/01UP-hero.mp4", "max_bytes": in["max_bytes"],
			"expires_at": "2026-10-22T02:00:00Z", "parts": p.state()})
	})
	mux.HandleFunc("POST "+app+"/uploads/01UP/parts", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["upload_id"] != "up-1" {
			t.Errorf("parts of %v", in)
		}
		p.mu.Lock()
		p.listed++
		p.mu.Unlock()
		writeJSON(w, 200, p.state())
	})
	mux.HandleFunc("PUT /store/{n}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("the store was sent the token")
		}
		n, _ := strconv.Atoi(r.PathValue("n"))
		b, _ := io.ReadAll(r.Body)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.puts = append(p.puts, n)
		if f := p.failing[n]; f != 0 {
			if f > 0 {
				p.failing[n]--
			}
			http.Error(w, "<Error><Code>InternalError</Code></Error>", 500)
			return
		}
		p.stored[n] = b
		w.Header().Set("ETag", fmt.Sprintf(`"e%d"`, n))
	})
	mux.HandleFunc("POST "+app+"/uploads/01UP/complete", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		var whole []byte
		for n := 1; n <= p.count(); n++ {
			b, ok := p.stored[n]
			if !ok {
				writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "UPLOAD_PARTS_MISSING", "message": "missing", "fix": "send them"}})
				return
			}
			whole = append(whole, b...)
		}
		p.joined = whole
		writeJSON(w, 200, map[string]any{"id": "01UP", "filename": "hero.mp4", "status": "authorised", "visibility": "private", "bytes": p.size})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *partsAPI) count() int { return int((p.size + 3) / 4) }

// state is the parts answer: stored parts with their ETag, the rest with a URL.
func (p *partsAPI) state() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	parts := []any{}
	for n := 1; n <= p.count(); n++ {
		size := min(4, p.size-int64(n-1)*4)
		part := map[string]any{"number": n, "bytes": size}
		if _, ok := p.stored[n]; ok {
			part["etag"] = fmt.Sprintf(`"e%d"`, n)
		} else {
			part["url"] = p.srv.URL + "/store/" + strconv.Itoa(n)
		}
		parts = append(parts, part)
	}
	return map[string]any{"upload_id": "up-1", "part_size": 4, "count": p.count(), "parts": parts,
		"urls_expire_at": time.Now().Add(time.Hour).Format(time.RFC3339)}
}

func smallParts(t *testing.T) {
	oldFrom, oldRetry := partsFrom, partRetry
	partsFrom, partRetry = 8, []time.Duration{time.Millisecond}
	t.Cleanup(func() { partsFrom, partRetry = oldFrom, oldRetry })
}

// TestUploadsAddInParts: a file larger than one part goes up in parts, a dropped part is sent
// again, and the store joins them in order.
func TestUploadsAddInParts(t *testing.T) {
	smallParts(t)
	p := newPartsAPI(t)
	p.failing[2] = 1
	dir := boundDir(t, p.srv.URL)
	content := "0123456789abcdefXY"
	must(t, os.WriteFile(filepath.Join(dir, "hero.mp4"), []byte(content), 0o600))

	r := runRemote(t, dir, p.srv.URL, nil, "uploads", "add", "hero.mp4")
	if r.Code != 0 {
		t.Fatalf("uploads add: %+v", r)
	}
	if string(p.joined) != content {
		t.Errorf("the store joined %q", p.joined)
	}
	if !strings.Contains(r.Stderr, "in 5 parts") || !strings.Contains(r.Stderr, "upload: 100%") {
		t.Errorf("stderr: %q", r.Stderr)
	}
}

// TestUploadsAddCarriesOn: a run that stops on a part leaves the upload to carry on, and the
// next run of the same command sends only the parts the store does not hold.
func TestUploadsAddCarriesOn(t *testing.T) {
	smallParts(t)
	p := newPartsAPI(t)
	p.failing[3] = -1
	dir := boundDir(t, p.srv.URL)
	content := "0123456789abcdefXY"
	must(t, os.WriteFile(filepath.Join(dir, "hero.mp4"), []byte(content), 0o600))

	cfg := t.TempDir()
	r := runWithConfig(t, cfg, dir, p.srv.URL, "uploads", "add", "hero.mp4", "--json")
	if r.Code == 0 || errCode(r) != "PLATFORM_UNAVAILABLE" || !strings.Contains(r.Stdout+r.Stderr, "carries on") {
		t.Fatalf("a part that would not go: %+v", r)
	}
	p.mu.Lock()
	delete(p.failing, 3)
	before := len(p.puts)
	held := len(p.stored)
	p.mu.Unlock()

	r = runWithConfig(t, cfg, dir, p.srv.URL, "uploads", "add", "hero.mp4")
	if r.Code != 0 || string(p.joined) != content {
		t.Fatalf("carrying on: %+v, joined %q", r, p.joined)
	}
	if p.authorise != 1 || p.listed == 0 {
		t.Errorf("authorised %d times, listed %d", p.authorise, p.listed)
	}
	if again := len(p.puts) - before; again != 5-held {
		t.Errorf("sent %d parts again, want %d", again, 5-held)
	}
	if !strings.Contains(r.Stderr, fmt.Sprintf("carrying on hero.mp4: %d of 5 parts already stored", held)) {
		t.Errorf("stderr: %q", r.Stderr)
	}

	// Done, the upload is forgotten: the same file again is a new upload.
	runWithConfig(t, cfg, dir, p.srv.URL, "uploads", "add", "hero.mp4")
	if p.authorise != 2 {
		t.Errorf("a finished upload was carried on: authorised %d times", p.authorise)
	}
}

func TestPartsStatePath(t *testing.T) {
	at := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	a := partsStatePath("/c", "acme", "crm", "/v/hero.mp4", 10, at)
	for _, b := range []string{
		partsStatePath("/c", "acme", "site", "/v/hero.mp4", 10, at),
		partsStatePath("/c", "acme", "crm", "/v/hero.mp4", 11, at),
		partsStatePath("/c", "acme", "crm", "/v/hero.mp4", 10, at.Add(time.Second)),
	} {
		if a == b {
			t.Errorf("another upload shares %s", a)
		}
	}
	if a != partsStatePath("/c", "acme", "crm", "/v/hero.mp4", 10, at) || !strings.HasPrefix(a, filepath.Join("/c", "uploads")) {
		t.Errorf("partsStatePath = %s", a)
	}
}

// runWithConfig is runRemote keeping the CLI's configuration folder from one run to the next.
func runWithConfig(t *testing.T, cfg, dir, apiURL string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	env := map[string]string{"WHISK_API": apiURL, "WHISK_TOKEN": "whsk_agent_test", "WHISK_AGENT": "claude"}
	e := Env{Stdout: &out, Stderr: &errb, Stdin: strings.NewReader(""), Dir: dir, ConfigDir: cfg,
		Store: &memStore{m: map[string]config.Credential{}}, Getenv: func(k string) string { return env[k] }}
	r := result{Code: Run(context.Background(), args, e), Stdout: out.String(), Stderr: errb.String()}
	if lines := jsonLines(r.Stdout); len(lines) > 0 {
		r.JSON = lines[len(lines)-1]
	}
	return r
}
