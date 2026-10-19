package whisk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/run"
)

// Uploading in parts (CLI.md §5.9, CONTROL-PLANE.md §6.13 "Uploading in parts"): a file larger
// than one part goes up as parts, a few at once, each retried on its own, and the upload is
// remembered on this computer so the same command carries on from the parts already stored.

var (
	// partsFrom is the size from which a file goes up in parts: larger than one part.
	partsFrom int64 = 16 << 20
	// partRetry is how long a part waits before each new attempt after a dropped connection.
	partRetry = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
)

// partWorkers is how many parts go at once: enough to fill a long, thin line to the store.
const partWorkers = 3

// partsState is an upload in parts this computer started, kept until the store joins it.
type partsState struct {
	ID       string    `json:"id"`
	UploadID string    `json:"upload_id"`
	Org      string    `json:"org"`
	App      string    `json:"app"`
	File     string    `json:"file"`
	Bytes    int64     `json:"bytes"`
	Modified time.Time `json:"modified"`
}

// partsStatePath is where the upload of this file, as it is now, to this app is remembered: a
// file changed since is a new upload. Pure.
func partsStatePath(cfgDir, org, app, file string, size int64, modified time.Time) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%s\x00%d\x00%d", org, app, file, size, modified.UnixNano()))
	return filepath.Join(cfgDir, "uploads", hex.EncodeToString(sum[:16])+".json")
}

// readPartsState is the upload remembered at path, if any.
func readPartsState(path string) (partsState, bool) {
	var st partsState
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &st) != nil || st.ID == "" || st.UploadID == "" {
		return partsState{}, false
	}
	return st, true
}

// writePartsState remembers an upload; failing to is no reason to stop it, only to lose the
// chance to carry it on.
func writePartsState(path string, st partsState) {
	b, _ := json.Marshal(st)
	if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
		_ = os.WriteFile(path, b, 0o600)
	}
}

// addInParts uploads one file in parts, carrying on an upload of it this computer started.
func addInParts(s *session, client *api.Client, org, app string, f uploadFile, visibility string) (api.Upload, error) {
	info, err := f.file.Stat()
	if err != nil {
		return api.Upload{}, output.New("INVALID_REQUEST", "Cannot read "+f.path+": "+err.Error()+".", "Pass the path of a file on this computer.", nil)
	}
	statePath := partsStatePath(s.cfgDir, org, app, f.path, f.size, info.ModTime())
	quiet := s.quiet || s.json
	say := func(format string, a ...any) {
		if !quiet {
			fmt.Fprintf(s.env.Stderr, "upload: "+format+"\n", a...)
		}
	}
	var id string
	var parts api.UploadParts
	if st, ok := readPartsState(statePath); ok {
		parts, err = client.UploadPartsOf(s.ctx, org, app, st.ID, st.UploadID)
		var ae *api.Error
		switch {
		case err == nil:
			id = st.ID
			say("carrying on %s: %d of %d parts already stored", f.name, storedParts(parts), parts.Count)
		case errors.As(err, &ae) && (ae.Code == "UPLOAD_NOT_RESUMABLE" || ae.Code == "NOT_FOUND"):
			_ = os.Remove(statePath)
			say("starting %s again: its earlier upload is no longer kept", f.name)
		default:
			return api.Upload{}, wrap(err)
		}
	}
	if id == "" {
		auth, err := client.AuthoriseUpload(s.ctx, org, app, f.name, f.contentType, visibility, f.size, true)
		if err != nil {
			return api.Upload{}, wrap(err)
		}
		if auth.Parts == nil {
			return api.Upload{}, output.New("PLATFORM_UNAVAILABLE", "The platform answered a form where parts were asked for.",
				"Update the CLI with whisk update, then run the same whisk uploads add again.", map[string]any{"upload": auth.ID})
		}
		id, parts = auth.ID, *auth.Parts
		writePartsState(statePath, partsState{ID: id, UploadID: parts.UploadID, Org: org, App: app, File: f.path, Bytes: f.size, Modified: info.ModTime()})
		say("%s (%s, %s) in %d parts", f.name, f.contentType, sizeMB(f.size), parts.Count)
	}
	if !parts.Complete {
		refresh := func(ctx context.Context) (api.UploadParts, error) {
			return client.UploadPartsOf(ctx, org, app, id, parts.UploadID)
		}
		if err := sendParts(s.ctx, client, f.file, parts, refresh, uploadProgress(s.env.Stderr, f.size, quiet)); err != nil {
			var ae *api.Error
			if errors.As(err, &ae) {
				return api.Upload{}, wrap(err)
			}
			return api.Upload{}, output.New("PLATFORM_UNAVAILABLE", f.name+" stopped part way: "+err.Error()+".",
				"Run the same whisk uploads add again; it carries on from the parts already stored.", map[string]any{"file": f.path, "upload": id})
		}
	}
	u, err := client.CompleteUpload(s.ctx, org, app, id, parts.UploadID)
	if err != nil {
		return api.Upload{}, wrap(err)
	}
	_ = os.Remove(statePath)
	return u, nil
}

// storedParts is how many parts the store already holds. Pure.
func storedParts(p api.UploadParts) int {
	n := 0
	for _, pt := range p.Parts {
		if pt.ETag != "" {
			n++
		}
	}
	return n
}

// sendParts puts every part not yet stored, a few at once. A part whose connection drops is
// tried again after a wait; URLs about to expire, or refused as expired, are fetched afresh
// with refresh. It stops at the first part that will not go, leaving the rest for next time.
func sendParts(ctx context.Context, client *api.Client, file io.ReaderAt, parts api.UploadParts, refresh func(context.Context) (api.UploadParts, error), progress func(int64)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	current := parts
	sent := int64(0)
	todo := []int{}
	for _, p := range parts.Parts {
		if p.ETag != "" {
			sent += p.Bytes
		} else {
			todo = append(todo, p.Number)
		}
	}
	if progress != nil && sent > 0 {
		progress(sent)
	}
	// urlFor is the part's URL, fetched afresh when the ones held are expired or about to be.
	urlFor := func(number int, force bool) (api.UploadPart, error) {
		mu.Lock()
		defer mu.Unlock()
		if force || (!current.URLsExpireAt.IsZero() && time.Until(current.URLsExpireAt) < 5*time.Minute) {
			fresh, err := refresh(ctx)
			if err != nil {
				return api.UploadPart{}, err
			}
			current = fresh
		}
		for _, p := range current.Parts {
			if p.Number == number {
				return p, nil
			}
		}
		return api.UploadPart{}, fmt.Errorf("the platform did not name part %d", number)
	}
	send := func(number int) error {
		force := false
		for attempt := 0; ; attempt++ {
			p, err := urlFor(number, force)
			if err != nil {
				return err
			}
			if p.URL == "" {
				return nil // stored meanwhile
			}
			_, err = client.PutPart(ctx, p.URL, io.NewSectionReader(file, int64(number-1)*parts.PartSize, p.Bytes), p.Bytes)
			if err == nil {
				mu.Lock()
				sent += p.Bytes
				if progress != nil {
					progress(sent)
				}
				mu.Unlock()
				return nil
			}
			var pe *api.PartError
			status := 0
			if errors.As(err, &pe) {
				status = pe.Status
			}
			retryable := status == 0 || status == 403 || status == 408 || status == 429 || status >= 500
			if !retryable || attempt >= len(partRetry) || ctx.Err() != nil {
				return fmt.Errorf("part %d of %d: %w", number, parts.Count, err)
			}
			force = status == 403
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(partRetry[attempt]):
			}
		}
	}
	// The first part that will not go stops the others, and is the one reported.
	var first error
	var once sync.Once
	err := run.Each(ctx, partWorkers, todo, func(ctx context.Context, number int) error {
		if ctx.Err() != nil {
			return nil
		}
		err := send(number)
		if err != nil {
			once.Do(func() { first = err; cancel() })
		}
		return err
	})
	if first != nil {
		return first
	}
	return err
}
