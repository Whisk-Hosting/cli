package whisk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// A shop (whisk.yaml commerce: medusa) runs on Whisk's commerce engine everywhere, on this
// computer too (COMMERCE-ENGINE.md §6). The engine is Whisk's own program: whisk dev downloads it
// for this platform as the person signed in, keeps it beside the CLI's settings, and checks its
// digest before running it. A kept engine still served is not downloaded again, and one kept
// from before runs when the platform cannot be reached.

// engineDownload bounds one download of the engine, tens of megabytes on a slow link.
const engineDownload = 10 * time.Minute

// engineName is the engine's file on this platform. Pure.
func engineName(goos string) string {
	if goos == "windows" {
		return "engine.exe"
	}
	return "engine"
}

// keptEngine is the digest of the engine kept at path, "" when none is kept or it does not match
// what was recorded for it.
func keptEngine(path string) string {
	recorded, err := os.ReadFile(path + ".sha256")
	if err != nil {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	if got := hex.EncodeToString(h.Sum(nil)); got == strings.TrimSpace(string(recorded)) {
		return got
	}
	return ""
}

// devEngine is the path of the commerce engine for this computer, downloaded when it is not
// kept or the platform serves another.
func (s *session) devEngine() (string, error) {
	dir := filepath.Join(s.cfgDir, "engine")
	path := filepath.Join(dir, engineName(runtime.GOOS))
	have := keptEngine(path)
	client, _, err := s.client()
	if err != nil {
		if have != "" {
			return path, nil
		}
		return "", output.New("AUTH_REQUIRED", "A shop runs on Whisk's commerce engine, which whisk dev downloads for people signed in to Whisk.",
			"Run whisk login (or set WHISK_TOKEN), then run whisk dev again.", nil)
	}
	ctx, cancel := context.WithTimeout(s.ctx, engineDownload)
	defer cancel()
	got, err := client.DownloadEngine(ctx, runtime.GOOS, runtime.GOARCH, have)
	if err != nil {
		var down *api.Unavailable
		if have != "" && errors.As(err, &down) {
			s.printer.Progress("the platform could not be reached; running the commerce engine kept from before")
			return path, nil
		}
		return "", wrap(err)
	}
	if got.Current {
		return path, nil
	}
	defer got.Body.Close()
	s.printer.Progress("downloading Whisk's commerce engine for %s/%s", runtime.GOOS, runtime.GOARCH)
	if err := keepEngine(dir, path, got); err != nil {
		return "", err
	}
	return path, nil
}

// keepEngine writes a downloaded engine beside the CLI's settings, executable, once its digest
// is the one the platform named.
func keepEngine(dir, path string, got api.Engine) error {
	failed := func(why string) error {
		return output.New("PLATFORM_UNAVAILABLE", "Whisk's commerce engine could not be downloaded: "+why, "Run whisk dev again; if it fails the same way, report it with whisk feedback --kind bug.", nil)
	}
	if got.Digest == "" {
		return failed("the platform named no digest for it.")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return failed(err.Error())
	}
	tmp, err := os.CreateTemp(dir, ".engine-")
	if err != nil {
		return failed(err.Error())
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), got.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return failed(err.Error())
	}
	if hex.EncodeToString(h.Sum(nil)) != got.Digest {
		return failed("what arrived is not what the platform named.")
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return failed(err.Error())
	}
	_ = os.Remove(path) // Windows renames onto no existing file
	if err := os.Rename(tmp.Name(), path); err != nil {
		return failed(err.Error())
	}
	if err := os.WriteFile(path+".sha256", []byte(got.Digest+"\n"), 0o600); err != nil {
		return failed(err.Error())
	}
	return nil
}
