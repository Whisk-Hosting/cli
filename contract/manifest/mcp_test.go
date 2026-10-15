package manifest

import (
	"strings"
	"testing"
)

// The app's MCP server (CONTRACT.md §3, §4 "A chat assistant in an app"): one private path of
// the app's own, and the tools that only read.
func TestMCP(t *testing.T) {
	m, err := Parse([]byte("whisk: 1\nname: ledger\nroutes:\n  public: ['/**']\nmcp:\n  path: /mcp\n  read_tools: [list_invoices, get.invoice]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.MCP == nil || m.MCP.Path != "/mcp" || strings.Join(m.MCP.ReadTools, ",") != "list_invoices,get.invoice" {
		t.Fatalf("mcp = %+v", m.MCP)
	}
	if bare, err := Parse([]byte("whisk: 1\nname: ledger\nmcp: {path: /api/mcp}\n")); err != nil || bare.MCP.ReadTools != nil {
		t.Fatalf("no read tools = %+v, %v", bare.MCP, err)
	}
	if none, _ := Parse([]byte("whisk: 1\nname: ledger\n")); none.MCP != nil {
		t.Fatalf("no mcp = %+v", none.MCP)
	}
	for _, tc := range []struct{ name, yaml, problem string }{
		{"no path", "mcp: {read_tools: [a]}", "path"},
		{"another host", "mcp: {path: //evil.example}", "/mcp/path"},
		{"a backslash", `mcp: {path: '/\evil.example'}`, "/mcp/path"},
		{"a glob", "mcp: {path: /mcp/*}", "/mcp/path"},
		{"a query", "mcp: {path: '/mcp?x=1'}", "mcp"},
		{"listed as public", "routes: {public: [/mcp]}\nmcp: {path: /mcp}", "/mcp/path"},
		{"the queue", "mcp: {path: /.whisk/inngest}", "/mcp/path"},
		{"the metadata", "mcp: {path: /.well-known/mcp}", "/mcp/path"},
		{"an apps route", "routes: {apps: [/mcp]}\nmcp: {path: /mcp}", "/mcp/path"},
		{"a webhook", "webhooks: [{name: s, preset: token, handler: /mcp}]\nmcp: {path: /mcp}", "/mcp/path"},
		{"a tool name with a space", "mcp: {path: /mcp, read_tools: ['list invoices']}", "read_tools"},
		{"a tool twice", "mcp: {path: /mcp, read_tools: [a, a]}", "read_tools"},
		{"something else", "mcp: {path: /mcp, write_tools: [a]}", "mcp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte("whisk: 1\nname: ledger\n" + tc.yaml + "\n"))
			if err == nil || !strings.Contains(err.Error(), tc.problem) {
				t.Fatalf("Parse = %v, want a problem naming %s", err, tc.problem)
			}
		})
	}
}
