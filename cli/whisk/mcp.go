package whisk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/whisk-run/contract"
	"github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/run"
)

// The CLI as an MCP server (CONTRACT.md §12): the contract documents as resources, every
// command as a tool. JSON-RPC 2.0 over stdio, one message per line, MCP 2025-06-18.

const mcpProtocolVersion = "2025-06-18"

// mcpResource is one readable document.
type mcpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType"`
	text        string
}

// mcpTool is one callable command.
type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	path        []string       // the command words, e.g. runs replay
	flagTypes   map[string]string
}

// mcpResources is the contract as resources: the nine documents and one entry per error code.
func mcpResources() []mcpResource {
	docs := []mcpResource{
		{URI: "whisk://contract/SKILL.md", Name: "SKILL.md", Description: "The skill: what an agent reads first to build and deploy on Whisk", MimeType: "text/markdown", text: contract.Skill},
		{URI: "whisk://contract/errors.md", Name: "errors.md", Description: "The error catalogue: every code, when it occurs and its fix", MimeType: "text/markdown", text: contract.ErrorsDoc},
		{URI: "whisk://contract/doctor-rules.md", Name: "doctor-rules.md", Description: "Every rule whisk doctor checks, with its level and fix", MimeType: "text/markdown", text: contract.DoctorRulesDoc},
		{URI: "whisk://contract/headers.md", Name: "headers.md", Description: "The identity headers the platform injects", MimeType: "text/markdown", text: contract.HeadersDoc},
		{URI: "whisk://contract/environment.md", Name: "environment.md", Description: "The environment variables an app receives", MimeType: "text/markdown", text: contract.EnvironmentDoc},
		{URI: "whisk://contract/whisk.schema.json", Name: "whisk.schema.json", Description: "The whisk.yaml JSON schema", MimeType: "application/json", text: string(contract.ManifestSchema)},
		{URI: "whisk://contract/graph.schema.json", Name: "graph.schema.json", Description: "The workflow graph JSON schema", MimeType: "application/json", text: string(contract.GraphSchema)},
		{URI: "whisk://contract/webhook-presets.yaml", Name: "webhook-presets.yaml", Description: "The webhook presets: how each provider signs its deliveries", MimeType: "application/yaml", text: string(contract.PresetsYAML)},
		{URI: "whisk://contract/workflows.md", Name: "workflows.md", Description: "What the workflow engine supports, with its limits", MimeType: "text/markdown", text: contract.WorkflowsDoc},
	}
	for _, code := range errors.Codes() {
		entry, _ := errors.Lookup(code)
		section, _ := errors.Section(code)
		docs = append(docs, mcpResource{URI: "whisk://errors/" + code, Name: code, Description: firstSentence(entry.When), MimeType: "text/markdown", text: section})
	}
	return docs
}

func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

// mcpExcluded are commands that are not tools: the server itself, shell helpers, and the ones
// that run until stopped or need a terminal.
var mcpExcluded = map[string]bool{"mcp": true, "completion": true, "help": true, "dev": true, "db shell": true}

// mcpTools lists every runnable command of the tree as a tool, sorted by name.
func mcpTools(root *cobra.Command) []mcpTool {
	var tools []mcpTool
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		if cmd.Hidden || mcpExcluded[strings.Join(path, " ")] {
			return
		}
		// A command that runs on its own is a tool even when it has subcommands (logs and logs
		// forwarding, traces and traces show).
		if cmd.Runnable() && len(path) > 0 {
			tools = append(tools, toolOf(cmd, path))
		}
		for _, sub := range cmd.Commands() {
			walk(sub, append(append([]string{}, path...), sub.Name()))
		}
	}
	walk(root, nil)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

// toolOf describes one command: its flags as properties, its positional words as `args`.
func toolOf(cmd *cobra.Command, path []string) mcpTool {
	props := map[string]any{
		"args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "positional arguments, as in: whisk " + cmd.UseLine()},
	}
	types := map[string]string{}
	add := func(f *pflag.Flag) {
		if f.Hidden || f.Name == "json" || f.Name == "help" || f.Name == "quiet" {
			return
		}
		if f.Name == "follow" {
			return // a stream never ends; a tool call must
		}
		typ, schema := flagSchema(f)
		types[f.Name] = typ
		props[f.Name] = schema
	}
	cmd.LocalFlags().VisitAll(add)
	cmd.InheritedFlags().VisitAll(add)
	desc := cmd.Short
	if cmd.Long != "" {
		desc += "\n\n" + cmd.Long
	}
	if mcpNoWait[strings.Join(path, "_")] {
		desc += "\n\nThrough MCP this returns as soon as the deploy is queued, with its deploy_id; follow it with deploys_info <deploy_id> until its status is live or failed. Pass no-wait: false to wait for the end in this call instead (deploys can take several minutes)."
	}
	return mcpTool{
		Name:        strings.Join(path, "_"),
		Description: desc,
		InputSchema: map[string]any{"type": "object", "properties": props, "additionalProperties": false},
		path:        path, flagTypes: types,
	}
}

// flagSchema maps a pflag type to a JSON schema type.
func flagSchema(f *pflag.Flag) (string, map[string]any) {
	switch f.Value.Type() {
	case "bool":
		return "bool", map[string]any{"type": "boolean", "description": f.Usage}
	case "int", "int64", "int32", "uint", "uint64":
		return "int", map[string]any{"type": "integer", "description": f.Usage}
	case "stringSlice", "stringArray":
		return "list", map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": f.Usage}
	}
	return "string", map[string]any{"type": "string", "description": f.Usage}
}

// toolArgv turns a tool call into the argv the CLI runs, always with --json. Unknown
// properties are an error so a misspelt flag is reported rather than ignored. Every value is
// attached to its flag and the positional words come after "--", so no value an MCP client
// sends can become a flag of its own.
func toolArgv(t mcpTool, arguments map[string]any) ([]string, error) {
	argv := append([]string{}, t.path...)
	keys := make([]string, 0, len(arguments))
	for k := range arguments {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var positional []string
	for _, k := range keys {
		v := arguments[k]
		if k == "args" {
			list, ok := v.([]any)
			if !ok {
				return nil, fmt.Errorf("args must be an array of strings")
			}
			for _, item := range list {
				positional = append(positional, fmt.Sprint(item))
			}
			continue
		}
		typ, ok := t.flagTypes[k]
		if !ok {
			return nil, fmt.Errorf("%s is not a flag of whisk %s", k, strings.Join(t.path, " "))
		}
		switch typ {
		case "bool":
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("%s must be true or false", k)
			}
			if b {
				argv = append(argv, "--"+k)
			}
		case "list":
			list, ok := v.([]any)
			if !ok {
				return nil, fmt.Errorf("%s must be an array of strings", k)
			}
			for _, item := range list {
				argv = append(argv, "--"+k+"="+fmt.Sprint(item))
			}
		default:
			argv = append(argv, "--"+k+"="+fmt.Sprint(v))
		}
	}
	if _, chosen := arguments["no-wait"]; mcpNoWait[strings.Join(t.path, "_")] && t.flagTypes["no-wait"] == "bool" && !chosen {
		argv = append(argv, "--no-wait")
	}
	// The positional words follow "--", so none is read as a flag (--yes, --api=...).
	argv = append(argv, "--json", "--")
	return append(argv, positional...), nil
}

// ---- JSON-RPC --------------------------------------------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603
)

// panicResponse is the answer to a request whose handler panicked: a JSON-RPC internal error
// carrying the CLI's CLI_ERROR code and its fix, so the agent can report it. Pure.
func panicResponse(id json.RawMessage, value any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{
		Code:    rpcInternalError,
		Message: fmt.Sprintf("The CLI failed while running this tool call: %v.", value),
		Data: map[string]any{
			"code": "CLI_ERROR",
			"fix":  "Run the same command with the whisk CLI directly; if it fails the same way, report it with whisk feedback --kind bug --code CLI_ERROR and the message.",
		},
	}}
}

// mcpServer answers one client over a pair of streams.
type mcpServer struct {
	env       Env
	resources []mcpResource
	tools     []mcpTool
}

func newMCPServer(env Env) *mcpServer {
	return &mcpServer{env: env, resources: mcpResources(), tools: mcpTools(newRoot(&session{env: env}))}
}

// serve reads messages until EOF. Notifications get no answer; requests get exactly one. A
// tool call runs on its own goroutine, so a long one (login --resume, export --wait, a deploy
// asked to wait) never holds up ping or another call, and notifications/cancelled stops the
// call it names. Answers and progress notifications share one writer, one message per line.
func (m *mcpServer) serve(ctx context.Context, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	var mu sync.Mutex
	var failed error
	send := func(v any) {
		mu.Lock()
		defer mu.Unlock()
		if err := enc.Encode(v); err != nil && failed == nil {
			failed = err
		}
	}
	calls := &mcpCalls{cancel: map[string]context.CancelFunc{}}
	var wg sync.WaitGroup
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var peek struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				RequestID json.RawMessage `json:"requestId"`
			} `json:"params"`
		}
		_ = json.Unmarshal(line, &peek)
		switch {
		case peek.Method == "notifications/cancelled":
			calls.stop(string(peek.Params.RequestID))
			continue
		case peek.Method == "tools/call" && len(peek.ID) > 0 && string(peek.ID) != "null":
			id := string(peek.ID)
			callCtx, cancel := context.WithCancel(ctx)
			calls.start(id, cancel)
			msg := append([]byte{}, line...)
			wg.Add(1)
			run.Spawn(callCtx, "mcp tools/call", func(callCtx context.Context) (err error) {
				defer wg.Done()
				defer cancel()
				// A call that panics still gets its one answer, and run.Spawn reports the panic.
				defer func() {
					if r := recover(); r != nil {
						if cancelled := calls.end(id); !cancelled {
							send(panicResponse(json.RawMessage(id), r))
						}
						err = run.Panic{Value: r, Stack: string(debug.Stack())}
					}
				}()
				resp, ok := m.handle(callCtx, msg, send)
				// A cancelled request gets no answer (MCP, cancellation).
				if cancelled := calls.end(id); ok && !cancelled {
					send(resp)
				}
				return nil
			})
			continue
		}
		if resp, ok := m.handle(ctx, line, send); ok {
			send(resp)
		}
	}
	wg.Wait()
	if failed != nil {
		return failed
	}
	return sc.Err()
}

// mcpCalls is the tool calls in flight, by request id, so a cancellation can stop one.
type mcpCalls struct {
	mu        sync.Mutex
	cancel    map[string]context.CancelFunc
	cancelled map[string]bool
}

func (c *mcpCalls) start(id string, cancel context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancel[id] = cancel
}

func (c *mcpCalls) stop(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cancel, ok := c.cancel[id]; ok {
		cancel()
		if c.cancelled == nil {
			c.cancelled = map[string]bool{}
		}
		c.cancelled[id] = true
	}
}

// end forgets a finished call and says whether it was cancelled.
func (c *mcpCalls) end(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cancel, id)
	was := c.cancelled[id]
	delete(c.cancelled, id)
	return was
}

// handle answers one message; ok is false for a notification. send carries the progress
// notifications of a tool call made with a progress token.
func (m *mcpServer) handle(ctx context.Context, line []byte, send func(any)) (rpcResponse, bool) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: rpcParseError, Message: "The message is not JSON: " + err.Error()}}, true
	}
	notification := len(req.ID) == 0 || string(req.ID) == "null"
	if req.JSONRPC != "2.0" || req.Method == "" {
		if notification {
			return rpcResponse{}, false
		}
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: rpcInvalidRequest, Message: "A request needs jsonrpc \"2.0\" and a method."}}, true
	}
	if strings.HasPrefix(req.Method, "notifications/") {
		return rpcResponse{}, false
	}
	result, rpcErr := m.dispatch(ctx, req.Method, req.Params, send)
	if notification {
		return rpcResponse{}, false
	}
	return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr}, true
}

func (m *mcpServer) dispatch(ctx context.Context, method string, params json.RawMessage, send func(any)) (any, *rpcError) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"resources": map[string]any{"listChanged": false}, "tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "whisk", "version": Version},
			"instructions":    "Read whisk://contract/SKILL.md first. Every tool runs the whisk CLI with --json and returns its output; exit codes: 0 ok, 1 error (read code and fix), 2 a human must act (show them the URL), 3 validation, 4 not signed in or not allowed (read code: AUTH_REQUIRED means call login, AGENT_CATEGORY or FORBIDDEN_ROLE means ask the human), 5 unavailable, 6 deploy failed. login answers at once with the URL and code for the human (exit 2); once they approve, call login with resume set to the device_code. deploy, rollback and envs_start return once the deploy is queued: follow it with deploys_info. Calls run side by side, and a call made with a progressToken reports what it is doing as it goes. Whenever Whisk gets in your way, call feedback_send: the Whisk team reads every piece.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "resources/list":
		return map[string]any{"resources": m.resources}, nil
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": []any{}}, nil
	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(params, &p); err != nil || p.URI == "" {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "resources/read needs a uri."}
		}
		for _, r := range m.resources {
			if r.URI == p.URI {
				return map[string]any{"contents": []map[string]any{{"uri": r.URI, "mimeType": r.MimeType, "text": r.text}}}, nil
			}
		}
		return nil, &rpcError{Code: -32002, Message: "No resource at " + p.URI + ".", Data: map[string]any{"uri": p.URI}}
	case "tools/list":
		return map[string]any{"tools": m.tools}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
			Meta      struct {
				ProgressToken any `json:"progressToken"`
			} `json:"_meta"`
		}
		if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{Code: rpcInvalidParams, Message: "tools/call needs a name."}
		}
		for _, t := range m.tools {
			if t.Name == p.Name {
				var progress func(int, string)
				if token := p.Meta.ProgressToken; token != nil && send != nil {
					progress = func(n int, message string) {
						send(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress", "params": map[string]any{"progressToken": token, "progress": n, "message": message}})
					}
				}
				return m.call(ctx, t, p.Arguments, progress), nil
			}
		}
		return nil, &rpcError{Code: rpcInvalidParams, Message: "No tool named " + p.Name + ".", Data: map[string]any{"name": p.Name}}
	}
	return nil, &rpcError{Code: rpcMethodNotFound, Message: "Unknown method " + method + "."}
}

// call runs one command in the server's environment with its own streams and reports the
// CLI's JSON as the tool's text; a non-zero exit marks the result an error. With a progress
// token, every line the command writes while it runs (a deploy's events, what it waits for)
// is also sent as notifications/progress, so a long call is never silent.
func (m *mcpServer) call(ctx context.Context, t mcpTool, arguments map[string]any, progress func(n int, message string)) map[string]any {
	argv, err := toolArgv(t, arguments)
	if err != nil {
		return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}}
	}
	var stdout, stderr bytes.Buffer
	env := m.env
	env.Stdin = strings.NewReader("")
	env.Stdout, env.Stderr = &stdout, &stderr
	if progress != nil {
		tap := &lineTap{progress: progress}
		env.Stdout, env.Stderr = tap.writer(&stdout), tap.writer(&stderr)
	}
	env.IsTerminal = false
	code := Run(ctx, argv, env)
	text := strings.TrimSpace(stdout.String())
	if text == "" {
		text = strings.TrimSpace(stderr.String())
	}
	return map[string]any{
		"isError": code != 0,
		"content": []map[string]any{{"type": "text", "text": text}},
		"_meta":   map[string]any{"exit_code": code, "argv": append([]string{"whisk"}, argv...)},
	}
}

// lineTap copies a command's output into its buffer and reports each complete line, numbered,
// to progress. The command's streams may be written from more than one goroutine.
type lineTap struct {
	mu       sync.Mutex
	n        int
	progress func(n int, message string)
}

type tapWriter struct {
	tap     *lineTap
	buf     *bytes.Buffer
	pending []byte
}

func (t *lineTap) writer(buf *bytes.Buffer) io.Writer { return &tapWriter{tap: t, buf: buf} }

func (w *tapWriter) Write(p []byte) (int, error) {
	w.tap.mu.Lock()
	defer w.tap.mu.Unlock()
	w.buf.Write(p)
	w.pending = append(w.pending, p...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			return len(p), nil
		}
		line := strings.TrimSpace(string(w.pending[:i]))
		w.pending = w.pending[i+1:]
		if line != "" {
			w.tap.n++
			w.tap.progress(w.tap.n, clip(line, 500))
		}
	}
}

// mcpNoWait are the tools that would otherwise wait for a deploy to finish, which can take many
// minutes. Through MCP they return as soon as the deploy is queued, unless the call passes
// no-wait: false; deploys_info follows it.
var mcpNoWait = map[string]bool{"deploy": true, "rollback": true, "envs_start": true}
