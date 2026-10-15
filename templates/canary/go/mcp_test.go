package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type memoryNotes struct{ notes []Note }

func (m *memoryNotes) ListNotes(_ context.Context, c Caller) ([]Note, error) {
	if c.UserID == "" {
		return []Note{}, nil
	}
	return m.notes, nil
}

func (m *memoryNotes) InsertNote(_ context.Context, _ Caller, authorID, authorEmail, body string) (Note, error) {
	n := Note{ID: int64(len(m.notes) + 1), AuthorID: authorID, AuthorEmail: authorEmail, Body: body}
	m.notes = append(m.notes, n)
	return n, nil
}

func mcpPost(t *testing.T, h http.Handler, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("X-Whisk-Audience", "team")
	req.Header.Set("X-Whisk-User-Id", "01JUSER")
	req.Header.Set("X-Whisk-Email", "ann@example.com")
	req.Header.Set("X-Whisk-Agent", "Claude")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// The canary's MCP server answers each message on its own, as the person the platform names.
func TestMCPServer(t *testing.T) {
	store := &memoryNotes{}
	h := mcpServer(store)
	if code, out := mcpPost(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`); code != 200 ||
		out["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize: %d %v", code, out)
	}
	if code, _ := mcpPost(t, h, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != http.StatusAccepted {
		t.Fatalf("a notification: %d", code)
	}
	_, out := mcpPost(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := out["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 || tools[0].(map[string]any)["name"] != "list_notes" {
		t.Fatalf("tools: %v", tools)
	}
	_, out = mcpPost(t, h, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"add_note","arguments":{"body":"hello"}}}`)
	note := out["result"].(map[string]any)["structuredContent"].(map[string]any)
	if note["author_id"] != "01JUSER" || note["body"] != "hello" {
		t.Fatalf("add_note: %v", out)
	}
	_, out = mcpPost(t, h, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_notes"}}`)
	listed := out["result"].(map[string]any)["structuredContent"].(map[string]any)
	if len(listed["notes"].([]any)) != 1 || listed["as"].(map[string]any)["x-whisk-agent"] != "Claude" {
		t.Fatalf("list_notes: %v", listed)
	}
	if _, out := mcpPost(t, h, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"add_note","arguments":{"body":" "}}}`); out["result"].(map[string]any)["isError"] != true {
		t.Fatalf("an empty note: %v", out)
	}
	if _, out := mcpPost(t, h, `{"jsonrpc":"2.0","id":6,"method":"resources/list"}`); out["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatalf("an unknown method: %v", out)
	}
	if _, out := mcpPost(t, h, `not json`); out["error"].(map[string]any)["code"] != float64(-32700) {
		t.Fatalf("not JSON: %v", out)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}
}
