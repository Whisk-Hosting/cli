package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// The canary's MCP server (whisk.yaml mcp, CONTRACT.md §4 "A chat assistant in an app"): a chat
// assistant adds /mcp as a connector and the platform signs it in, so every request here
// carries the person's identity headers and X-Whisk-Agent. It is stateless: each JSON-RPC
// message is answered on its own, and no Mcp-Session-Id is kept. list_notes only reads and is
// listed under mcp.read_tools; add_note changes data.

// noteStore is what the MCP server needs of the database.
type noteStore interface {
	ListNotes(ctx context.Context, c Caller) ([]Note, error)
	InsertNote(ctx context.Context, c Caller, authorID, authorEmail, body string) (Note, error)
}

var mcpVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

var mcpTools = []map[string]any{
	{
		"name": "list_notes", "title": "List notes",
		"description": "Use to read the notes the signed-in person may see, newest first, and who the app sees as the caller.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		"annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false, "openWorldHint": false},
	},
	{
		"name": "add_note", "title": "Add a note",
		"description": "Use to write a note as the signed-in person.",
		"inputSchema": map[string]any{"type": "object", "required": []string{"body"}, "properties": map[string]any{"body": map[string]any{"type": "string"}}},
		"annotations": map[string]any{"readOnlyHint": false, "destructiveHint": false, "openWorldHint": false},
	},
}

// mcpServer answers POST /mcp: one JSON-RPC request or notification per POST.
func mcpServer(store noteStore) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var in rpcRequest
		if err := json.NewDecoder(io.LimitReader(req.Body, 1<<20)).Decode(&in); err != nil || in.JSONRPC != "2.0" || in.Method == "" {
			rpcReply(w, nil, nil, &rpcError{Code: -32700, Message: "the body is not a JSON-RPC request"})
			return
		}
		if len(in.ID) == 0 {
			// A notification: nothing to answer.
			w.WriteHeader(http.StatusAccepted)
			return
		}
		result, rerr := mcpAnswer(req.Context(), store, identityFrom(req.Header), whiskHeaders(req.Header), in)
		rpcReply(w, in.ID, result, rerr)
	}
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// mcpAnswer is the result of one request, or its error.
func mcpAnswer(ctx context.Context, store noteStore, id Identity, headers map[string]string, in rpcRequest) (any, *rpcError) {
	switch in.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(in.Params, &p)
		version := mcpVersions[0]
		for _, v := range mcpVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "whisk-go canary", "version": "1"},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools}, nil
	case "tools/call":
		var p struct {
			Name      string `json:"name"`
			Arguments struct {
				Body string `json:"body"`
			} `json:"arguments"`
		}
		if err := json.Unmarshal(in.Params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: "params are not a tool call"}
		}
		return mcpCall(ctx, store, id, headers, p.Name, p.Arguments.Body)
	}
	return nil, &rpcError{Code: -32601, Message: "no method " + in.Method}
}

// mcpCall runs one tool as the person the request is from.
func mcpCall(ctx context.Context, store noteStore, id Identity, headers map[string]string, name, body string) (any, *rpcError) {
	var out any
	var err error
	switch name {
	case "list_notes":
		var notes []Note
		notes, err = store.ListNotes(ctx, callerOf(id))
		out = map[string]any{"notes": notes, "as": headers}
	case "add_note":
		if id.UserID == "" {
			return toolResult(map[string]any{"error": "a signed-in person is required"}, true), nil
		}
		if strings.TrimSpace(body) == "" {
			return toolResult(map[string]any{"error": "body is required"}, true), nil
		}
		out, err = store.InsertNote(ctx, callerOf(id), id.UserID, id.Email, strings.TrimSpace(body))
	default:
		return nil, &rpcError{Code: -32602, Message: "no tool " + name}
	}
	if err != nil {
		return toolResult(map[string]any{"error": err.Error()}, true), nil
	}
	return toolResult(out, false), nil
}

func toolResult(structured any, isError bool) map[string]any {
	text, _ := json.Marshal(structured)
	return map[string]any{"content": []map[string]any{{"type": "text", "text": string(text)}}, "structuredContent": structured, "isError": isError}
}

func rpcReply(w http.ResponseWriter, id json.RawMessage, result any, rerr *rpcError) {
	out := map[string]any{"jsonrpc": "2.0", "id": id}
	if id == nil {
		out["id"] = nil
	}
	if rerr != nil {
		out["error"] = rerr
	} else {
		out["result"] = result
	}
	writeJSON(w, http.StatusOK, out)
}
