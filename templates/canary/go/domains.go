package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// diagDomains is POST /diag/domains {op, hostname?, id?, app?}: the app manages its own custom
// domains with its service token, through the domains helper in whisk.go, the way a white-label
// app lets the businesses it serves bring their own domain (H93). op is list, add, verify or
// remove. With app, the same token calls that other app's domain list instead, which the
// platform refuses. The answer is {status, domain | items | error}: the platform's status and
// what it answered. Private, Go canary only, and safe to delete in your own app.
func diagDomains(w http.ResponseWriter, req *http.Request) {
	var in struct {
		Op       string `json:"op"`
		Hostname string `json:"hostname"`
		ID       string `json:"id"`
		App      string `json:"app"`
	}
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "a JSON body with an op is required"})
		return
	}
	ctx := req.Context()
	if in.App != "" {
		writeJSON(w, 200, foreignDomains(ctx, in.App))
		return
	}
	var out map[string]any
	var err error
	switch in.Op {
	case "list":
		var items []Domain
		items, err = domains.List(ctx)
		out = map[string]any{"status": 200, "items": items}
	case "add":
		var d Domain
		d, err = domains.Add(ctx, in.Hostname)
		out = map[string]any{"status": 201, "domain": d}
	case "verify":
		var d Domain
		d, err = domains.Verify(ctx, in.ID)
		out = map[string]any{"status": 200, "domain": d}
	case "remove":
		err = domains.Remove(ctx, in.ID)
		out = map[string]any{"status": 204}
	default:
		writeJSON(w, 400, map[string]string{"error": "op is list, add, verify or remove"})
		return
	}
	var pe *PlatformError
	switch {
	case errors.As(err, &pe):
		writeJSON(w, 200, map[string]any{"status": pe.Status, "error": pe})
	case err != nil:
		writeJSON(w, 502, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, 200, out)
	}
}

// foreignDomains lists another app's domains with this app's own token.
func foreignDomains(ctx context.Context, app string) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	own := platformURL()
	base := own[:strings.LastIndex(own, "/")+1] + app
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/domains", nil)
	if err != nil {
		return map[string]any{"status": 0, "error": err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+env("WHISK_SERVICE_TOKEN", ""))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{"status": 0, "error": err.Error()}
	}
	defer resp.Body.Close()
	var body struct {
		Error PlatformError `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return map[string]any{"status": resp.StatusCode, "error": body.Error}
}
