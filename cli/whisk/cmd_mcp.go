package whisk

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// mcpCmd serves the CLI as an MCP server on stdio (CLI.md §5.13, CONTRACT.md §12).
func mcpCmd(s *session) *cobra.Command {
	var list bool
	c := &cobra.Command{
		Use:   "mcp [--list]",
		Short: "Serve the contract as MCP resources and every command as an MCP tool, over stdio",
		Long: `Runs an MCP server on stdin and stdout (JSON-RPC 2.0, one message per line, protocol
2025-06-18) for agents that prefer it to a shell: the contract documents and every error code
are resources, and every whisk command is a tool that runs with --json. Point a client at
"whisk mcp" as a stdio server. Nothing else is written to stdout while it runs.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			server := newMCPServer(s.env)
			if list {
				names := make([]string, len(server.tools))
				for i, t := range server.tools {
					names[i] = t.Name
				}
				uris := make([]string, len(server.resources))
				for i, r := range server.resources {
					uris[i] = r.URI
				}
				s.printer.Result(map[string]any{"protocol": mcpProtocolVersion, "tools": names, "resources": uris}, func(w io.Writer) {
					fmt.Fprintf(w, "MCP %s: %d tools, %d resources\n", mcpProtocolVersion, len(names), len(uris))
					for _, n := range names {
						fmt.Fprintln(w, "  tool     "+n)
					}
					for _, u := range uris {
						fmt.Fprintln(w, "  resource "+u)
					}
				})
				return nil
			}
			return server.serve(s.ctx, s.env.Stdin, s.env.Stdout)
		},
	}
	c.Flags().BoolVar(&list, "list", false, "print the tools and resources instead of serving")
	return c
}
