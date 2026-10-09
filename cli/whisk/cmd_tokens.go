package whisk

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/output"
)

// agentTokenCmd mints an agent token for CI or a second agent (CLI.md §5.12). A human runs
// it: the platform refuses an agent token minting another.
func agentTokenCmd(s *session) *cobra.Command {
	root := &cobra.Command{Use: "agent-token", Short: "Agent tokens for CI or a second agent"}
	var label string
	var scopes []string
	create := &cobra.Command{
		Use:   "create --label \"claude on laptop\" [--scopes deploy,logs:read,secrets:declare]",
		Short: "Mint an agent token acting for you; its value is shown once",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(label) == "" {
				return output.New("INVALID_REQUEST", "An agent token needs a label.", `Pass --label with what it is for, for example --label "github actions".`, nil)
			}
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			t, err := client.CreateAgentToken(s.ctx, org, label, scopes)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "token": t.Token, "value": t.Value}, func(w io.Writer) {
				fmt.Fprintf(w, "Agent token %s (%s), shown once:\n%s\n", t.Token.ID, t.Token.Label, t.Value)
				fmt.Fprintf(w, "Scopes: %s. Set it as WHISK_TOKEN where the agent runs; revoke it with whisk tokens revoke %s.\n", strings.Join(t.Token.Scopes, ", "), t.Token.ID)
			})
			return nil
		},
	}
	create.Flags().StringVar(&label, "label", "", "what the token is for")
	create.Flags().StringSliceVar(&scopes, "scopes", nil, "grant scopes from deploy, logs:read, secrets:declare; all three when omitted. An operator may also name feedback:read and feedback:resolve")
	root.AddCommand(create)
	return root
}

// tokensCmd lists and revokes the org's agent and deploy tokens (CLI.md §5.12).
func tokensCmd(s *session) *cobra.Command {
	tokens := &cobra.Command{Use: "tokens", Short: "The org's agent and deploy tokens"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List live tokens: kind, label, scopes, last use, expiry (never their values)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			all, err := client.ListTokens(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "tokens": all}, func(w io.Writer) {
				if len(all) == 0 {
					fmt.Fprintln(w, "No live tokens. whisk login mints one; whisk agent-token create mints one for CI.")
					return
				}
				rows := make([][]string, len(all))
				for i, t := range all {
					rows[i] = []string{t.ID, t.Kind, orDash(t.Label), strings.Join(t.Scopes, ","), when(t.LastUsedAt), when(t.ExpiresAt)}
				}
				s.printer.Table(w, []string{"ID", "KIND", "LABEL", "SCOPES", "LAST USED", "EXPIRES"}, rows)
			})
			return nil
		},
	}

	revoke := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke a token; every call with it fails from now on",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			if err := client.RevokeToken(s.ctx, org, args[0]); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "id": args[0], "revoked": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Revoked %s.\n", args[0])
			})
			return nil
		},
	}

	tokens.AddCommand(list, revoke)
	return tokens
}
