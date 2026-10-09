package whisk

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/whisk-run/cli/internal/config"
)

// A tool call whose handler panics still gets one answer, an internal error with CLI_ERROR,
// and the server goes on to answer the next request.
func TestMCPPanicAnswersTheCall(t *testing.T) {
	var armed atomic.Bool
	var out, errb bytes.Buffer
	env := Env{Stdout: &out, Stderr: &errb, Dir: t.TempDir(), ConfigDir: t.TempDir(), Store: &memStore{m: map[string]config.Credential{}}, Getenv: func(string) string {
		if armed.Load() {
			panic("getenv broke")
		}
		return ""
	}}
	srv := newMCPServer(env)
	armed.Store(true)
	in := strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"whoami","arguments":{}}}` + "\n" +
		`{"jsonrpc":"2.0","id":8,"method":"ping"}` + "\n")
	if err := srv.serve(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	answers := byID(t, out.String())
	if len(answers) != 2 {
		t.Fatalf("want two answers, got %v", answers)
	}
	var call map[string]any
	for _, a := range answers {
		if a["id"] == float64(7) {
			call = a
		}
	}
	e, _ := call["error"].(map[string]any)
	data, _ := e["data"].(map[string]any)
	if e == nil || e["code"] != float64(rpcInternalError) || !strings.Contains(e["message"].(string), "getenv broke") || data["code"] != "CLI_ERROR" || data["fix"] == "" {
		t.Fatalf("want an internal error for id 7, got %v", call)
	}
}
