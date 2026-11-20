package whisk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/cli/internal/release"
)

// A release this computer will not start (Windows Smart App Control, an antivirus) never
// replaces the whisk that works: the old binary stays and nothing is left beside it.
func TestUpdateKeepsTheOldBinaryWhenTheNewOneWillNotStart(t *testing.T) {
	withoutReleaseKey(t)
	binary := []byte("new binary bytes")
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)
	srv := releaseServer(t, "v9.9.9", binary, release.Digest(binary)+"  "+name+"\n", "")
	defer srv.Close()
	exe := fakeExe(t)
	var tried string
	blocked := func(ctx context.Context, path string, args ...string) ([]byte, error) {
		tried = path
		return []byte("An Application Control policy has blocked this file.\r\n"), errors.New("exit status 1")
	}

	r := runUpdateTrying(t, false, blocked, exe, srv.URL+"/dl", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout, "UPDATE_FAILED") || !strings.Contains(r.Stdout, "Application Control policy") {
		t.Fatalf("update: %+v", r)
	}
	if filepath.Dir(tried) != filepath.Dir(exe) || tried == exe {
		t.Fatalf("tried %q, want a new file beside %q", tried, exe)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatalf("the old binary was replaced: %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	for _, e := range entries {
		if e.Name() != filepath.Base(exe) {
			t.Fatalf("leftover %s", e.Name())
		}
	}
}

// The trial run is the downloaded file itself: a release that prints its version goes in, one
// that fails or prints something else does not.
func TestUpdateRunsTheNewFileBeforeSwapping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts stand in for the release")
	}
	withoutReleaseKey(t)
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)
	for _, c := range []struct {
		name   string
		script string
		ok     bool
	}{
		{"answers as whisk", "#!/bin/sh\n[ \"$1\" = version ] && echo 'whisk v9.9.9 (api 1, conventions 1)'\n", true},
		{"exits with an error", "#!/bin/sh\necho 'blocked' >&2\nexit 1\n", false},
		{"is not whisk", "#!/bin/sh\necho hello\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			binary := []byte(c.script)
			srv := releaseServer(t, "v9.9.9", binary, release.Digest(binary)+"  "+name+"\n", "")
			defer srv.Close()
			exe := fakeExe(t)
			r := runUpdateTrying(t, false, nil, exe, srv.URL+"/dl", "--json")
			got, _ := os.ReadFile(exe)
			if c.ok != (r.Code == 0) || c.ok != (string(got) == c.script) {
				t.Fatalf("update: %+v, binary now %q", r, got)
			}
		})
	}
}

func TestTriedOutput(t *testing.T) {
	for _, c := range []struct {
		name string
		out  string
		err  error
		want string // "" when the run is good
	}{
		{"version line", "whisk 2026.11.19-a987712 (api 1, conventions 3)\n", nil, ""},
		{"blocked with a message", "An Application Control policy has blocked this file.\r\nmore", errors.New("exit status 1"), "exit status 1: An Application Control policy has blocked this file."},
		{"blocked silently", "", errors.New("fork/exec: operation not permitted"), "fork/exec: operation not permitted"},
		{"something else", "hello\n", nil, `it printed "hello" instead of its version`},
		{"nothing", "", nil, `it printed "" instead of its version`},
	} {
		err := triedOutput([]byte(c.out), c.err)
		if (c.want == "") != (err == nil) || (err != nil && err.Error() != c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// On Windows the refusal says Windows would not start it and to try again later; elsewhere it
// asks for a report. Both say the old whisk keeps working.
func TestRefusedToRun(t *testing.T) {
	for _, c := range []struct{ goos, says string }{
		{"windows", "Windows would not start whisk v2"},
		{"linux", "whisk v2 would not start on this computer"},
	} {
		var e *output.Error
		if !errors.As(refusedToRun("v2", "v1", c.goos, errors.New("blocked")), &e) || e.Code != "UPDATE_FAILED" ||
			!strings.Contains(e.Message, c.says) || !strings.Contains(e.Message, "whisk v1 stays installed") || e.Fix == "" {
			t.Errorf("%s: %+v", c.goos, e)
		}
	}
}
