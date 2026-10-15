package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// The connection diagnostics the platform's canary drives to prove the broker (HARNESS.md
// H105, BROKER.md). Private like every /diag route, and safe to delete in your own app. Only the
// go canary carries them, since H105 deploys only it.

// connectionName is a connection's name as whisk.yaml declares it (CONTRACT.md §3.1).
var connectionName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,29}$`)

// envName is a secret's or variable's name.
var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// connectionMethods are the methods the diagnostic sends.
var connectionMethods = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// connectionTarget is the address one call to a connection goes to: the connection's own
// WHISK_CONNECTION_<NAME>_URL with the path below it, kept exactly as given (still escaped),
// so a path the broker must refuse reaches it unchanged. Pure.
func connectionTarget(lookup func(string) string, name, path string) (string, error) {
	if !connectionName.MatchString(name) {
		return "", errors.New("name is a connection name, such as erp")
	}
	if !strings.HasPrefix(path, "/") {
		return "", errors.New("path starts with /")
	}
	base := lookup("WHISK_CONNECTION_" + strings.ToUpper(name) + "_URL")
	if base == "" {
		return "", errors.New("WHISK_CONNECTION_" + strings.ToUpper(name) + "_URL is not set: whisk.yaml declares no connection " + name)
	}
	return strings.TrimSuffix(base, "/") + path, nil
}

// envPresence reports, for each valid name in a comma-separated list, whether the variable is
// set in the process; never its value. Pure.
func envPresence(names string, lookup func(string) (string, bool)) (map[string]bool, error) {
	out := map[string]bool{}
	for _, n := range strings.Split(names, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if !envName.MatchString(n) {
			return nil, errors.New("names are upper-case variable names separated by commas")
		}
		_, set := lookup(n)
		out[n] = set
	}
	if len(out) == 0 {
		return nil, errors.New("names is required: the variables to look for, separated by commas")
	}
	return out, nil
}

// noRedirects is the client the diagnostic calls with: a redirect the broker hands back is
// reported as it came, never followed.
var noRedirects = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// diagConnection is GET /diag/connection?name=<connection>&method=<GET|POST|…>&path=<path>
// [&body=<text>][&token=none]: one call through the broker to the connection's address,
// carrying this app's service token in Whisk-Service-Token (or none). It answers 200 with what
// came back: {status, broker (the Whisk-Broker header), location, code (the platform error
// code, when the broker refused), body}, or {status: 0, error} when the call never completed.
func diagConnection(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	method := strings.ToUpper(q.Get("method"))
	if method == "" {
		method = http.MethodGet
	}
	if !connectionMethods[method] {
		writeJSON(w, 400, map[string]string{"error": "method is one of GET, HEAD, POST, PUT, PATCH, DELETE"})
		return
	}
	target, err := connectionTarget(os.Getenv, q.Get("name"), q.Get("path"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 20*time.Second)
	defer cancel()
	var body io.Reader
	if b := q.Get("body"); b != "" {
		body = strings.NewReader(b)
	}
	out, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if body != nil {
		out.Header.Set("Content-Type", "application/json")
	}
	out.Header.Set("Accept", "application/json")
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
	writeJSON(w, 200, map[string]any{
		"status":   resp.StatusCode,
		"broker":   resp.Header.Get("Whisk-Broker"),
		"location": resp.Header.Get("Location"),
		"code":     errorCode(answer),
		"body":     string(answer),
	})
}

// diagEnv is GET /diag/env?names=A,B: whether each variable is set inside the container, never
// its value. The harness uses it to show a connection's secrets never reach the app.
func diagEnv(w http.ResponseWriter, req *http.Request) {
	set, err := envPresence(req.URL.Query().Get("names"), os.LookupEnv)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"set": set})
}
