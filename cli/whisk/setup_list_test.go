package whisk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSetupLeft(t *testing.T) {
	no := false
	body := `{"ready": false, "items": [
		{"key": "domain", "label": "Own domain", "required": false, "done": false, "fix": "Add it."},
		{"key": "orders", "label": "Who hears of orders", "required": true, "done": false, "fix": "Enter it.", "steps": ["Ask.", "Send it."]},
		{"key": "policies", "label": "Policies", "required": false, "done": null, "fix": "Write them."},
		{"key": "name", "label": "Name", "required": true, "done": true, "fix": "x"},
		{"key": "pay", "label": "Payment", "required": true, "done": false, "fix": "Connect Stripe."}],
		"values": {"orders_email": ""}}`
	ready, left, ok := setupLeft([]byte(body))
	keys := make([]string, 0, len(left))
	for _, it := range left {
		keys = append(keys, it.Key)
	}
	if !ok || ready || !reflect.DeepEqual(keys, []string{"orders", "pay", "domain", "policies"}) {
		t.Fatalf("setupLeft = %v %v %v", ready, keys, ok)
	}
	if left[0].Done == nil || *left[0].Done != no || len(left[0].Steps) != 2 || left[3].Done != nil {
		t.Fatalf("items = %+v", left)
	}

	for _, bad := range []string{``, `[]`, `{"items": []}`, `{"ready": "no"}`, `<html>`} {
		if _, _, ok := setupLeft([]byte(bad)); ok {
			t.Errorf("setupLeft(%q) took it for a setup list", bad)
		}
	}
	if ready, left, ok := setupLeft([]byte(`{"ready": true, "items": []}`)); !ok || !ready || len(left) != 0 {
		t.Errorf("all done = %v %v %v", ready, left, ok)
	}
}

func TestSetupLines(t *testing.T) {
	no := false
	left := []setupItem{
		{Key: "orders", Label: "Who hears of orders", Required: true, Done: &no, Fix: "Enter it.", Steps: []string{"Ask."}},
		{Key: "domain", Label: "Own domain", Done: &no, Fix: "Add it."},
		{Key: "policies", Label: "Policies", Fix: "Write them."},
	}
	got := strings.Join(setupLines("/admin/whisk/setup", false, left), "\n")
	want := `Setup: the app needs these from the business before it works for its customers.
  [required] Who hears of orders: Enter it.
      - Ask.
  [recommended] Own domain: Add it.
  [check] Policies: Write them.
Walk the person through each, required ones first; whisk call GET /admin/whisk/setup shows the list again.`
	if got != want {
		t.Fatalf("setupLines =\n%s\nwant\n%s", got, want)
	}
	if lines := setupLines("/s", true, left[1:]); !strings.HasPrefix(lines[0], "Setup: recommended") {
		t.Errorf("ready heading = %q", lines[0])
	}
	if lines := setupLines("/s", true, []setupItem{}); lines != nil {
		t.Errorf("nothing left prints %q", lines)
	}
}

// whisk deploy reads the live app's setup list as the person the agent acts for, and prints
// nothing when the app keeps none or does not answer with one.
func TestSetupOfTheLiveApp(t *testing.T) {
	var auth, path string
	answer := `{"ready": false, "items": [{"key": "orders", "label": "Who hears of orders", "required": true, "done": false, "fix": "Enter it."}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		if r.URL.Path != "/admin/whisk/setup" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	session := func(yaml string) *session {
		dir := t.TempDir()
		must(t, os.WriteFile(filepath.Join(dir, "whisk.yaml"), []byte(yaml), 0o644))
		env := map[string]string{"WHISK_TOKEN": "whsk_agent_test"}
		return &session{ctx: context.Background(), env: Env{Dir: dir, Getenv: func(k string) string { return env[k] }}}
	}
	shop := "whisk: 1\nname: shop\nadmin:\n  path: /app\n  setup: /admin/whisk/setup\n"

	got, lines := session(shop).setupOf(srv.URL)
	if got == nil || got["ready"] != false || got["path"] != "/admin/whisk/setup" || len(got["left"].([]setupItem)) != 1 || len(lines) != 3 {
		t.Fatalf("setupOf = %v %q", got, lines)
	}
	if auth != "Bearer whsk_agent_test" || path != "/admin/whisk/setup" {
		t.Errorf("sent %q to %q", auth, path)
	}

	for name, yaml := range map[string]string{
		"no admin":      "whisk: 1\nname: site\n",
		"no setup list": "whisk: 1\nname: site\nadmin:\n  path: /app\n",
		"not answered":  "whisk: 1\nname: shop\nadmin:\n  path: /app\n  setup: /missing\n",
	} {
		if got, lines := session(yaml).setupOf(srv.URL); got != nil || lines != nil {
			t.Errorf("%s: setupOf = %v %q", name, got, lines)
		}
	}
	answer = `<html>not a list</html>`
	if got, _ := session(shop).setupOf(srv.URL); got != nil {
		t.Errorf("an answer that is no list: %v", got)
	}
}
