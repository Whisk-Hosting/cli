package whisk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"
)

// An app's setup list (the manifest's admin.setup, CONTRACT.md §3.5): what the app still needs
// from the business before it works for its customers, which whisk deploy prints once the deploy
// is live so the agent walks the person through it.

// setupItem is one thing on the list. Done is nil for what the app cannot check itself.
type setupItem struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Required bool     `json:"required"`
	Done     *bool    `json:"done"`
	Fix      string   `json:"fix,omitempty"`
	Steps    []string `json:"steps,omitempty"`
}

// setupLeft is what the app's answer says is left: ready, and each item not done, required
// ones first, in the app's own order otherwise. ok is false for an answer that is not a setup
// list. Pure.
func setupLeft(body []byte) (ready bool, left []setupItem, ok bool) {
	var list struct {
		Ready *bool       `json:"ready"`
		Items []setupItem `json:"items"`
	}
	if json.Unmarshal(body, &list) != nil || list.Ready == nil {
		return false, nil, false
	}
	left = []setupItem{}
	for _, it := range list.Items {
		if it.Label != "" && (it.Done == nil || !*it.Done) {
			left = append(left, it)
		}
	}
	slices.SortStableFunc(left, func(a, b setupItem) int {
		switch {
		case a.Required == b.Required:
			return 0
		case a.Required:
			return -1
		}
		return 1
	})
	return *list.Ready, left, true
}

// setupLines is the list for a person. Pure.
func setupLines(path string, ready bool, left []setupItem) []string {
	if len(left) == 0 {
		return nil
	}
	head := "Setup: the app needs these from the business before it works for its customers."
	if ready {
		head = "Setup: recommended, for the business to finish."
	}
	out := []string{head}
	for _, it := range left {
		mark := "recommended"
		switch {
		case it.Required:
			mark = "required"
		case it.Done == nil:
			mark = "check"
		}
		out = append(out, fmt.Sprintf("  [%s] %s: %s", mark, it.Label, it.Fix))
		for _, step := range it.Steps {
			out = append(out, "      - "+step)
		}
	}
	return append(out, "Walk the person through each, required ones first; whisk call GET "+path+" shows the list again.")
}

// setupOf reads the live app's setup list as the person the agent acts for, or nothing when the
// app keeps none, no credential reaches it, or it does not answer in time: the deploy is live
// either way.
func (s *session) setupOf(appURL string) (map[string]any, []string) {
	m, err := loadManifest(s.env.Dir)
	if err != nil || m.Admin == nil || m.Admin.Setup == "" || appURL == "" {
		return nil, nil
	}
	token, key, err := s.callCredential()
	if err != nil {
		return nil, nil
	}
	u, err := callTarget(appURL, m.Admin.Setup)
	if err != nil {
		return nil, nil
	}
	a, err := s.sendCall(http.MethodGet, u, http.Header{}, nil, token, key, 30*time.Second)
	if err != nil || a.Status != http.StatusOK || a.Truncated {
		return nil, nil
	}
	ready, left, ok := setupLeft(a.Body)
	if !ok {
		return nil, nil
	}
	return map[string]any{"path": m.Admin.Setup, "ready": ready, "left": left}, setupLines(m.Admin.Setup, ready, left)
}

func printSetup(w io.Writer, lines []string) {
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
}
