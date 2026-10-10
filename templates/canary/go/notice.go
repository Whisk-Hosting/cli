package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// The notice diagnostic the platform's canary drives to prove a managed app's notices
// (HARNESS.md H107, MANAGED-APPS.md §6.1). Private like every /diag route, and safe to delete in
// your own app: only an app Whisk runs as a managed product sends notices. Only the go canary
// carries it, since H107 deploys only it.

// noticeURL is the platform's notify route, on the broker's address.
const noticeURL = "http://connect.internal.whisk:8443/.whisk/notify"

// noticeBody is the JSON a notice posts, from the diagnostic's query: code and occurrence as
// given, and detail when there is one. Pure.
func noticeBody(code, occurrence, detail string) []byte {
	in := map[string]string{"code": code, "occurrence": occurrence}
	if detail != "" {
		in["detail"] = detail
	}
	b, _ := json.Marshal(in)
	return b
}

// diagNotice is GET /diag/notify?code=<CODE>&occurrence=<key>[&detail=<text>][&token=none]: one
// notice to the platform's notify route, carrying this app's service token in
// Whisk-Service-Token (or none). It answers 200 with what came back: {status, broker (the
// Whisk-Broker header), code (the platform error code, when refused), outcome, body}, or
// {status: 0, error} when the call never completed.
func diagNotice(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	out, err := http.NewRequestWithContext(ctx, http.MethodPost, noticeURL, bytes.NewReader(noticeBody(q.Get("code"), q.Get("occurrence"), q.Get("detail"))))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	out.Header.Set("Content-Type", "application/json")
	if q.Get("token") != "none" {
		out.Header.Set("Whisk-Service-Token", os.Getenv("WHISK_SERVICE_TOKEN"))
	}
	resp, err := noRedirects.Do(out)
	if err != nil {
		writeJSON(w, 200, map[string]any{"status": 0, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var got struct {
		Outcome string `json:"outcome"`
	}
	_ = json.Unmarshal(answer, &got)
	writeJSON(w, 200, map[string]any{
		"status":  resp.StatusCode,
		"broker":  resp.Header.Get("Whisk-Broker"),
		"code":    errorCode(answer),
		"outcome": got.Outcome,
		"body":    strings.TrimSpace(string(answer)),
	})
}
