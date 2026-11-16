package apitypes

import (
	"encoding/json"
	"testing"
)

// A CLI from before repositories had no size limit reads limits.repo_bytes and falls back to
// the Free plan's 100 MB without it. Every validation answers it, above any limit a plan ever
// had (Business's 2 GB), so dropping a plan's limit never shrinks an older client's.
func TestOlderCLIsNeverReadASmallerRepositoryLimit(t *testing.T) {
	raw, err := json.Marshal(NewValidation())
	if err != nil {
		t.Fatal(err)
	}
	var old struct {
		Limits struct {
			RepoBytes int64 `json:"repo_bytes"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	const largestEverAllowed = 2 << 30
	if old.Limits.RepoBytes <= largestEverAllowed {
		t.Errorf("limits.repo_bytes is %d, want more than %d: an older CLI would refuse what the platform takes", old.Limits.RepoBytes, largestEverAllowed)
	}
	// A JavaScript client reads it exactly.
	if old.Limits.RepoBytes > 1<<53 {
		t.Errorf("limits.repo_bytes %d is past what a JSON number holds exactly", old.Limits.RepoBytes)
	}
}
