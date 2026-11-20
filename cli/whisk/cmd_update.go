package whisk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/cli/internal/release"
	"github.com/whisk-run/contract/run"
	"github.com/whisk-run/contract/run/runhttp"
)

// updateCmd replaces the running binary with the release the platform serves (CLI.md §5.1):
// VERSION decides, SHA256SUMS is checked, SHA256SUMS.sig is verified under the embedded
// release key when the build has one, and the file is swapped in atomically.
func updateCmd(s *session) *cobra.Command {
	var check, force bool
	c := &cobra.Command{
		Use:   "update [--check] [--force]",
		Short: "Self-update from the platform, with checksum and signature verification",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			base := s.releaseBase()
			served, err := s.fetchText(base + "/VERSION")
			if err != nil {
				return err
			}
			served = strings.TrimSpace(served)
			exe, err := s.executable()
			if err != nil {
				return err
			}
			result := map[string]any{"current": Version, "latest": served, "path": exe, "signature": signatureState()}
			if !release.Newer(Version, served) && !force {
				result["updated"] = false
				s.printer.Result(result, func(w io.Writer) {
					fmt.Fprintf(w, "whisk %s is up to date (%s).\n", Version, signatureState())
				})
				return nil
			}
			if check {
				result["updated"] = false
				result["available"] = true
				s.printer.Result(result, func(w io.Writer) {
					fmt.Fprintf(w, "whisk %s is available (running %s). Run whisk update to install it.\n", served, Version)
				})
				return nil
			}
			if sourceBuild {
				return output.New("UPDATE_FAILED", "This whisk was built from Whisk's own source; the platform serves the public build, which would replace it.",
					"Build it again from the repository with make cli.", map[string]any{"served": served, "current": Version})
			}
			sums, err := s.fetchText(base + "/SHA256SUMS")
			if err != nil {
				return err
			}
			signed, err := s.verifySums(base, []byte(sums))
			if err != nil {
				return err
			}
			stamp, stamped := release.ParseStamp(sums)
			switch release.CheckStamp(releasedAt(), signed, stamp, stamped, force) {
			case release.RefuseUnstamped:
				return output.New("UPDATE_FAILED", "The signed release does not say which version it is or when it was built, so it could be an old release served again.", "Do not install it; the whisk you have keeps working. Report it with whisk feedback --kind bug --code UPDATE_FAILED so the release is rebuilt.", map[string]any{"url": base + "/SHA256SUMS"})
			case release.RefuseOlder:
				return output.New("UPDATE_FAILED", fmt.Sprintf("The platform serves whisk %s, built before this one (%s); installing it would go back to an older version.", stamp.Version, Version),
					"Keep this version. If you mean to go back, run whisk update --force.", map[string]any{"served": stamp.Version, "current": Version})
			}
			if stamped {
				// VERSION is not signed; the stamp is.
				served = stamp.Version
				result["latest"] = served
			}
			name := release.AssetName(runtime.GOOS, runtime.GOARCH)
			want, ok := release.ParseSums(sums)[name]
			if !ok {
				return output.New("UPDATE_FAILED", fmt.Sprintf("SHA256SUMS at %s has no entry for %s.", base, name), "The release was built without this platform; the whisk you have keeps working. Report it with whisk feedback --kind bug --code UPDATE_FAILED.", map[string]any{"asset": name})
			}
			s.printer.Progress("downloading %s %s", name, served)
			data, err := s.fetchBytes(base + "/" + name)
			if err != nil {
				return err
			}
			if got := release.Digest(data); got != want {
				return output.New("UPDATE_FAILED", fmt.Sprintf("The downloaded %s does not match SHA256SUMS.", name), "Run whisk update again; if it fails the same way the release on the platform is inconsistent: do not install it, and report it with whisk feedback --kind bug --code UPDATE_FAILED.", map[string]any{"asset": name, "want": want, "got": got})
			}
			if err := s.install(exe, data, served); err != nil {
				return err
			}
			result["updated"] = true
			result["signature_verified"] = signed
			s.printer.Result(result, func(w io.Writer) {
				fmt.Fprintf(w, "Updated whisk %s → %s at %s (checksum ok, %s).\n", Version, served, exe, signatureWords(signed))
			})
			return nil
		},
	}
	c.Flags().BoolVar(&check, "check", false, "report whether an update exists without installing it")
	c.Flags().BoolVar(&force, "force", false, "reinstall even when the versions match, or go back to an older release")
	return c
}

// releasedAt is when this build's commit was made (Released, set by make cli-dist); 0 for a
// development build.
func releasedAt() int64 {
	var t int64
	if _, err := fmt.Sscan(Released, &t); err != nil {
		return 0
	}
	return t
}

// releaseBase is where the platform serves releases: WHISK_RELEASE_URL, else <dashboard>/dl.
func (s *session) releaseBase() string {
	if v := s.env.Getenv("WHISK_RELEASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return s.dashboard() + "/dl"
}

// executable is the running binary's path, symlinks resolved (Env.Executable in tests).
func (s *session) executable() (string, error) {
	path := s.env.Executable
	if path == "" {
		var err error
		if path, err = os.Executable(); err != nil {
			return "", output.New("UPDATE_FAILED", "The running binary's path is unknown: "+err.Error(), "Reinstall with the install script from CLI.md §2.", nil)
		}
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path, nil
}

func signatureState() string {
	if ReleasePublicKey == "" {
		return "signatures unverified: this build embeds no release key"
	}
	return "signatures verified"
}

func signatureWords(signed bool) string {
	if signed {
		return "signature verified"
	}
	return "signature unverified: this build embeds no release key"
}

// verifySums checks SHA256SUMS.sig when the build embeds a key. Without a key it says so once
// and carries on with the checksum alone.
func (s *session) verifySums(base string, sums []byte) (bool, error) {
	if ReleasePublicKey == "" {
		s.printer.Progress("this build embeds no release key; verifying the checksum only")
		return false, nil
	}
	sig, err := s.fetchText(base + "/SHA256SUMS.sig")
	if err != nil {
		var e *output.Error
		if errors.As(err, &e) && e.Code == "NOT_FOUND" {
			return false, output.New("UPDATE_FAILED", "The platform serves no SHA256SUMS.sig, and this build requires a signed release.", "Do not install an unsigned release; the whisk you have keeps working. Report it with whisk feedback --kind bug --code UPDATE_FAILED.", map[string]any{"url": base + "/SHA256SUMS.sig"})
		}
		return false, err
	}
	ok, err := release.Verify(ReleasePublicKey, sums, sig)
	if err != nil {
		return false, output.New("UPDATE_FAILED", "The release is not signed by the key this build trusts: "+err.Error(), "Do not install it. If the key was rotated, reinstall with the install script from https://github.com/Whisk-Hosting/cli/releases/latest/download/install.sh (install.ps1 on Windows); otherwise report it with whisk feedback --kind bug --code UPDATE_FAILED.", map[string]any{"url": base + "/SHA256SUMS.sig"})
	}
	return ok, nil
}

// releaseAddress is where a release may come from: https, or http to the machine itself for a
// local stack. A binary fetched in the clear could be swapped on the way.
func releaseAddress(raw string) bool { return webAddress(raw) }

func (s *session) fetchText(url string) (string, error) {
	b, err := s.fetchBytes(url)
	return string(b), err
}

// releaseFetch bounds one release download, a binary of tens of megabytes on a slow link.
const releaseFetch = 5 * time.Minute

// fetchBytes downloads one release file; a 404 is NOT_FOUND, anything else that is not 200
// is UPDATE_FAILED, and a transport failure PLATFORM_UNAVAILABLE.
func (s *session) fetchBytes(url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "whisk-cli/"+Version+" ("+runtime.GOOS+")")
	if !releaseAddress(url) {
		return nil, output.New("UPDATE_FAILED", "Releases are fetched only over https, and "+url+" is not.", "Point WHISK_RELEASE_URL (or WHISK_API) at an https address, then run whisk update again.", map[string]any{"url": url})
	}
	client := runhttp.Client(releaseFetch)
	client.CheckRedirect = api.SafeRedirect
	resp, err := client.Do(req)
	if err != nil {
		return nil, output.New("PLATFORM_UNAVAILABLE", "The release could not be fetched from "+url+": "+err.Error(), "Retry with backoff. WHISK_RELEASE_URL overrides where releases come from.", nil)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, output.New("NOT_FOUND", url+" is not served by the platform.", "No CLI release is served there yet; the whisk you have keeps working. Report it with whisk feedback --kind bug --code UPDATE_FAILED.", map[string]any{"url": url})
	}
	if resp.StatusCode != http.StatusOK {
		return nil, output.New("UPDATE_FAILED", fmt.Sprintf("%s answered HTTP %d.", url, resp.StatusCode), "Retry; if it persists, report it with whisk feedback --kind bug --code UPDATE_FAILED.", map[string]any{"url": url, "status": resp.StatusCode})
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return nil, output.New("PLATFORM_UNAVAILABLE", "The download of "+url+" was cut off: "+err.Error(), "Retry with backoff. WHISK_RELEASE_URL overrides where releases come from.", map[string]any{"url": url})
	}
	return b, nil
}

// tryRun bounds the trial run of a new release: Windows may look the file up online before it
// lets it start the first time.
const tryRun = 60 * time.Second

// install puts the downloaded release in place of exe. The new file runs before it replaces this
// one, so a build the computer refuses to start (Windows Smart App Control, an antivirus) never
// becomes the CLI.
func (s *session) install(exe string, data []byte, served string) error {
	unwritable := func(err error) error {
		return output.New("UPDATE_FAILED", "The new binary could not be put in place: "+err.Error(), "Check that "+filepath.Dir(exe)+" is writable, or reinstall with the install script from CLI.md §2.", map[string]any{"path": exe})
	}
	staged, err := stageExecutable(exe, data, runtime.GOOS)
	if err != nil {
		return unwritable(err)
	}
	s.printer.Progress("checking whisk %s runs on this computer", served)
	if err := s.tryBinary(staged); err != nil {
		_ = os.Remove(staged)
		return refusedToRun(served, Version, runtime.GOOS, err)
	}
	if err := swapExecutable(exe, staged, runtime.GOOS); err != nil {
		_ = os.Remove(staged)
		return unwritable(err)
	}
	return nil
}

// tryBinary runs the staged release as `whisk version` and checks it answers as whisk.
func (s *session) tryBinary(path string) error {
	try := s.env.TryBinary
	if try == nil {
		try = func(ctx context.Context, path string, args ...string) ([]byte, error) {
			return run.Command(ctx, tryRun, path, args...).CombinedOutput()
		}
	}
	out, err := try(s.ctx, path, "version")
	return triedOutput(out, err)
}

// triedOutput judges a trial run: it must exit cleanly and print the version line.
func triedOutput(out []byte, err error) error {
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%w: %s", err, shortLine(msg))
		}
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "whisk ") {
		return fmt.Errorf("it printed %q instead of its version", shortLine(strings.TrimSpace(string(out))))
	}
	return nil
}

// shortLine is a trial run's first line, cut to a length an error can carry.
func shortLine(s string) string {
	s = strings.TrimSpace(firstLine(s))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// refusedToRun is the error when the downloaded release would not start here. On Windows that
// is almost always Smart App Control or another application control policy, which lets a new
// build start only once Microsoft knows it.
func refusedToRun(served, current, goos string, err error) error {
	details := map[string]any{"served": served, "current": current, "reason": err.Error()}
	if goos == "windows" {
		return output.New("UPDATE_FAILED", fmt.Sprintf("Windows would not start whisk %s (%s), so whisk %s stays installed and keeps working.", served, err.Error(), current),
			"Nothing to undo. Run whisk update again later; Windows usually allows a new release within a day. Report it with whisk feedback --kind bug --code UPDATE_FAILED if it keeps happening.", details)
	}
	return output.New("UPDATE_FAILED", fmt.Sprintf("whisk %s would not start on this computer (%s), so whisk %s stays installed and keeps working.", served, err.Error(), current),
		"Nothing to undo. Report it with whisk feedback --kind bug --code UPDATE_FAILED, and run whisk update again once a new release is out.", details)
}

// stageExecutable writes data beside path, ready to run and to rename into place, and returns
// its name. On Windows it ends in .exe, so Windows will start it.
func stageExecutable(path string, data []byte, goos string) (string, error) {
	dir := filepath.Dir(path)
	pattern := ".whisk-update-*"
	if goos == "windows" {
		pattern += ".exe"
	}
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return "", err
	}
	// The bytes reach the disk before the rename makes them the CLI, so a power cut cannot
	// leave a truncated binary in its place.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		cleanup()
		return "", err
	}
	return tmpName, nil
}

// swapExecutable renames the staged file into place. On Windows the running file cannot be
// overwritten, but it can be renamed, so the old file is moved aside first (`whisk.exe.old`)
// and removed on the next update.
func swapExecutable(path, staged, goos string) error {
	old := path + ".old"
	_ = os.Remove(old) // a previous update's leftover, if any
	if goos == "windows" {
		if err := os.Rename(path, old); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(staged, path); err != nil {
			_ = os.Rename(old, path)
			return err
		}
		return nil
	}
	return os.Rename(staged, path)
}
