package whisk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/config"
)

// mcpSession feeds one message per line to the server and returns one decoded answer per
// request (notifications get none).
func mcpSession(t *testing.T, lines ...string) []map[string]any {
	t.Helper()
	var out, errb bytes.Buffer
	env := Env{Stdout: &out, Stderr: &errb, Dir: t.TempDir(), ConfigDir: t.TempDir(), Store: &memStore{m: map[string]config.Credential{}}, Getenv: func(string) string { return "" }}
	srv := newMCPServer(env)
	if err := srv.serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	return byID(t, out.String())
}

func rpcResult(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	if m["error"] != nil {
		t.Fatalf("error answer: %v", m["error"])
	}
	r, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", m)
	}
	return r
}

func TestMCPInitializeAndResources(t *testing.T) {
	answers := mcpSession(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"whisk://contract/SKILL.md"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"whisk://errors/NEEDS_HUMAN"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"whisk://nope"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"no/such"}`,
		`this is not json`,
	)
	if len(answers) != 7 {
		t.Fatalf("expected 7 answers (the notification gets none), got %d: %v", len(answers), answers)
	}
	init := rpcResult(t, answers[0])
	if init["protocolVersion"] != mcpProtocolVersion || init["serverInfo"].(map[string]any)["name"] != "whisk" {
		t.Fatalf("initialize: %v", init)
	}
	if answers[0]["id"] != float64(1) {
		t.Fatalf("id echoed: %v", answers[0]["id"])
	}
	list := rpcResult(t, answers[1])["resources"].([]any)
	uris := map[string]bool{}
	for _, r := range list {
		uris[r.(map[string]any)["uri"].(string)] = true
	}
	for _, want := range []string{"whisk://contract/SKILL.md", "whisk://contract/errors.md", "whisk://contract/doctor-rules.md", "whisk://contract/headers.md", "whisk://contract/environment.md", "whisk://contract/whisk.schema.json", "whisk://contract/graph.schema.json", "whisk://contract/webhook-presets.yaml", "whisk://contract/workflows.md", "whisk://errors/NEEDS_HUMAN", "whisk://errors/BUILD_FAILED"} {
		if !uris[want] {
			t.Errorf("resource %s missing", want)
		}
	}
	skill := rpcResult(t, answers[2])["contents"].([]any)[0].(map[string]any)
	if skill["mimeType"] != "text/markdown" || !strings.Contains(skill["text"].(string), "whisk") {
		t.Fatalf("SKILL.md: %v", skill)
	}
	needs := rpcResult(t, answers[3])["contents"].([]any)[0].(map[string]any)
	if !strings.Contains(needs["text"].(string), "Status: - · Surface: cli") {
		t.Fatalf("NEEDS_HUMAN section: %v", needs)
	}
	if answers[4]["error"] == nil || answers[5]["error"].(map[string]any)["code"] != float64(rpcMethodNotFound) {
		t.Fatalf("errors: %v %v", answers[4], answers[5])
	}
	if answers[6]["error"].(map[string]any)["code"] != float64(rpcParseError) {
		t.Fatalf("parse error: %v", answers[6])
	}
}

func TestMCPTools(t *testing.T) {
	answers := mcpSession(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"version","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"errors","arguments":{"args":["NEEDS_HUMAN"]}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"whoami","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"version","arguments":{"bogus":1}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"no_such_tool"}}`,
	)
	tools := rpcResult(t, answers[0])["tools"].([]any)
	byName := map[string]map[string]any{}
	for _, tl := range tools {
		m := tl.(map[string]any)
		byName[m["name"].(string)] = m
	}
	for _, want := range []string{"version", "doctor", "deploy", "runs_replay", "runs_cancel", "customers_invite", "db_url", "restore", "tokens_revoke", "update"} {
		if byName[want] == nil {
			t.Errorf("tool %s missing", want)
		}
	}
	for _, no := range []string{"mcp", "completion", "help", "dev", "db_shell", "dev_tunnel"} {
		if byName[no] != nil {
			t.Errorf("tool %s should not be offered", no)
		}
	}
	props := byName["deploy"]["inputSchema"].(map[string]any)["properties"].(map[string]any)
	if props["args"] == nil || props["env"].(map[string]any)["type"] != "string" || props["force"].(map[string]any)["type"] != "boolean" || props["org"] == nil {
		t.Fatalf("deploy schema: %v", props)
	}
	if props["json"] != nil {
		t.Fatal("--json is not a tool argument; the server always passes it")
	}
	scopes := byName["agent-token_create"]["inputSchema"].(map[string]any)["properties"].(map[string]any)["scopes"].(map[string]any)
	if scopes["type"] != "array" {
		t.Fatalf("scopes schema: %v", scopes)
	}

	version := rpcResult(t, answers[1])
	text := version["content"].([]any)[0].(map[string]any)["text"].(string)
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil || body["whisk_cli"] != "dev" || version["isError"] != false {
		t.Fatalf("version call: %v (%v)", text, err)
	}
	errs := rpcResult(t, answers[2])
	if !strings.Contains(errs["content"].([]any)[0].(map[string]any)["text"].(string), `"code":"NEEDS_HUMAN"`) {
		t.Fatalf("errors call: %v", errs)
	}
	whoami := rpcResult(t, answers[3])
	if whoami["isError"] != true || !strings.Contains(whoami["content"].([]any)[0].(map[string]any)["text"].(string), "AUTH_REQUIRED") || whoami["_meta"].(map[string]any)["exit_code"] != float64(4) {
		t.Fatalf("whoami without a token: %v", whoami)
	}
	bogus := rpcResult(t, answers[4])
	if bogus["isError"] != true || !strings.Contains(bogus["content"].([]any)[0].(map[string]any)["text"].(string), "bogus") {
		t.Fatalf("unknown argument: %v", bogus)
	}
	if answers[5]["error"] == nil {
		t.Fatalf("unknown tool: %v", answers[5])
	}
}

func TestToolArgv(t *testing.T) {
	tl := mcpTool{path: []string{"runs", "list"}, flagTypes: map[string]string{"function": "string", "since": "string", "verbose": "bool", "scopes": "list"}}
	argv, err := toolArgv(tl, map[string]any{"function": "nightly", "verbose": true, "scopes": []any{"a", "b"}, "args": []any{"x"}})
	must(t, err)
	if strings.Join(argv, " ") != "runs list --function=nightly --scopes=a --scopes=b --verbose --json -- x" {
		t.Fatalf("argv %q", strings.Join(argv, " "))
	}
	if _, err := toolArgv(tl, map[string]any{"verbose": "yes"}); err == nil {
		t.Fatal("a string for a bool should be refused")
	}
}

func TestMCPListFlag(t *testing.T) {
	r := runCLI(t, t.TempDir(), nil, &memStore{}, "mcp", "--list", "--json")
	if r.Code != 0 || r.JSON["protocol"] != mcpProtocolVersion || len(r.JSON["tools"].([]any)) < 30 || len(r.JSON["resources"].([]any)) < 8 {
		t.Fatalf("mcp --list: %+v", r)
	}
}

// byID decodes the server's output into its answers in request-id order (tool calls answer
// as they finish), with an answer to an unreadable message (id null) last. Notifications the
// server sends (progress) are left out.
func byID(t *testing.T, out string) []map[string]any {
	t.Helper()
	var answers []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %q", line)
		}
		if _, isNotification := m["method"]; isNotification {
			continue
		}
		answers = append(answers, m)
	}
	key := func(m map[string]any) float64 {
		if id, ok := m["id"].(float64); ok {
			return id
		}
		return 1 << 50
	}
	sort.SliceStable(answers, func(i, j int) bool { return key(answers[i]) < key(answers[j]) })
	return answers
}

// A tool call that waits (here login --resume, for an approval that never comes) runs beside
// everything else: ping is answered meanwhile, the call reports what it waits for as progress,
// and notifications/cancelled stops it with no answer.
func TestMCPLongCallDoesNotBlock(t *testing.T) {
	polled := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(polled) })
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, 202, map[string]any{"status": "pending"})
	}))
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{config.PendingProfile("default"): {PrivateKey: "kept", PendingCode: codeHash("dc-x"), ExpiresAt: time.Now().Add(5 * time.Minute)}}}
	var out, errb bytes.Buffer
	env := Env{Stdout: &out, Stderr: &errb, Dir: t.TempDir(), ConfigDir: t.TempDir(), Store: store, Getenv: func(k string) string {
		if k == "WHISK_API" {
			return srv.URL
		}
		return ""
	}}
	in, feed := io.Pipe()
	done := make(chan error)
	go func() { done <- newMCPServer(env).serve(context.Background(), in, &out) }()
	write := func(s string) {
		if _, err := io.WriteString(feed, s+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"login","arguments":{"resume":"dc-x"},"_meta":{"progressToken":"p1"}}}`)
	select {
	case <-polled:
	case <-time.After(10 * time.Second):
		t.Fatal("the call never started polling")
	}
	write(`{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	write(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`)
	_ = feed.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled call kept the server running")
	}
	answers := byID(t, out.String())
	if len(answers) != 1 || answers[0]["id"] != float64(2) {
		t.Fatalf("want only ping's answer, got %v", answers)
	}
	if !strings.Contains(out.String(), `"method":"notifications/progress"`) || !strings.Contains(out.String(), "Waiting for the human") || !strings.Contains(out.String(), `"progressToken":"p1"`) {
		t.Fatalf("no progress about the wait: %s", out.String())
	}
}

// Through MCP a deploy returns once it is queued unless the call asks to wait, and says so.
func TestMCPDeployDoesNotWaitByDefault(t *testing.T) {
	tools := mcpTools(newRoot(&session{env: Env{}}))
	var deploy mcpTool
	for _, tl := range tools {
		if tl.Name == "deploy" {
			deploy = tl
		}
	}
	argv, err := toolArgv(deploy, map[string]any{})
	must(t, err)
	if !strings.Contains(strings.Join(argv, " "), "--no-wait") || !strings.Contains(deploy.Description, "deploys_info") {
		t.Fatalf("deploy through MCP: %q, %q", argv, deploy.Description)
	}
	argv, err = toolArgv(deploy, map[string]any{"no-wait": false})
	must(t, err)
	if strings.Contains(strings.Join(argv, " "), "--no-wait") {
		t.Fatalf("no-wait: false still added --no-wait: %q", argv)
	}
}
