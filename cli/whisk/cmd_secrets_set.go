package whisk

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// secretsSetCmd is `whisk secrets set`: it stores a value for each name, read from a hidden
// prompt on a terminal, from stdin, or from a dotenv file, so no value is ever an argument (and
// so in shell history or a process list). The answer names the version, never the value.
func secretsSetCmd(s *session, scope *string) *cobra.Command {
	var previews bool
	var note, envFile string
	set := &cobra.Command{
		Use:   "set NAME [NAME...] [--from-env FILE] [--note TEXT]",
		Short: "Set secret values, read from a hidden prompt, stdin or a dotenv file",
		Long: `Stores a new version of each secret, declaring the name first when it has none. Apps that read
it restart and take the new value; a deploy waiting on it goes ahead.

The value never goes on the command line. On a terminal it is typed at a prompt that does not
echo. Otherwise it is read from stdin, so pipe it in from wherever it is kept:

  op read op://vault/stripe/key | whisk secrets set STRIPE_SECRET_KEY

One trailing newline is dropped. With --from-env every NAME=VALUE line of a dotenv file (- for
stdin) is set; names given as well limit it to those. The CLI never prints a value: the answer
names each secret and its new version.

Previews receive a secret only when it is shared with previews. With --previews (or
--previews=false) the command instead asks an owner or admin to turn "Share with previews" on
(or off) for the names on the secrets page, since a preview runs a branch's code.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.secretScope(*scope)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("previews") {
				if len(args) == 0 {
					return output.New("INVALID_REQUEST", "--previews needs the names to share.", "Pass the names: whisk secrets set NAME --previews.", nil)
				}
				return previewsNeedsHuman(secretsURL(s.dashboard(), org, app), org, app, args, previews)
			}
			values, err := s.secretValues(args, envFile)
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			done := make([]api.Secret, 0, len(values))
			for _, v := range values {
				sec, err := api.Send(s.ctx, client, endpoints.SetSecretValue, apitypes.SecretValueRequest{Value: v.Value, AppID: app, Note: note}, org, v.Name)
				if err != nil {
					if len(done) > 0 {
						s.printer.Progress("Set before the failure: %s", strings.Join(secretNames(done), ", "))
					}
					return wrap(err)
				}
				done = append(done, sec)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "secrets": done}, func(w io.Writer) {
				for _, sec := range done {
					fmt.Fprintf(w, "Set %s (%s scope) to version %d; apps that read it restart.\n", sec.Name, sec.Scope, sec.Version)
				}
			})
			return nil
		},
	}
	set.Flags().BoolVar(&previews, "previews", false, "ask a person to share the secrets with previews (--previews=false to stop sharing)")
	set.Flags().StringVar(&note, "note", "", "a note kept with the new version, such as where the value came from")
	set.Flags().StringVar(&envFile, "from-env", "", "a dotenv file of NAME=VALUE lines to set, or - for stdin")
	return set
}

// secretValue is one name and the value to store under it.
type secretValue struct {
	Name, Value string
}

// secretNames lists the secrets' names, in order.
func secretNames(all []api.Secret) []string {
	out := make([]string, len(all))
	for i, sec := range all {
		out[i] = sec.Name
	}
	return out
}

// secretValues reads the value for each name: from the dotenv file when there is one, else from
// a hidden prompt on a terminal, else from stdin (one name only).
func (s *session) secretValues(names []string, envFile string) ([]secretValue, error) {
	if envFile != "" {
		raw, err := s.readEnvFile(envFile)
		if err != nil {
			return nil, err
		}
		values, err := parseEnvFile(raw)
		if err != nil {
			return nil, err
		}
		return pickSecretValues(values, names)
	}
	if len(names) == 0 {
		return nil, output.New("INVALID_REQUEST", "No secret named.", "Pass the name: whisk secrets set NAME, with the value on stdin or typed at the prompt, or pass --from-env FILE.", nil)
	}
	if s.env.ReadHidden != nil {
		out := make([]secretValue, 0, len(names))
		for _, name := range names {
			fmt.Fprintf(s.env.Stderr, "Value for %s (not shown): ", name)
			raw, err := s.env.ReadHidden()
			fmt.Fprintln(s.env.Stderr)
			if err != nil {
				return nil, output.New("CLI_ERROR", "The value could not be read: "+err.Error(), "Type the value at the prompt, or pipe it in on stdin.", nil)
			}
			if len(raw) == 0 {
				return nil, emptyValue(name)
			}
			out = append(out, secretValue{Name: name, Value: string(raw)})
		}
		return out, nil
	}
	if len(names) > 1 {
		return nil, output.New("INVALID_REQUEST", "Stdin holds one value, and several names were given.",
			"Set one name at a time with its value on stdin, or put NAME=VALUE lines in a file and pass --from-env FILE (or --from-env - for stdin).", map[string]any{"names": names})
	}
	raw, err := io.ReadAll(io.LimitReader(s.env.Stdin, 1<<20))
	if err != nil {
		return nil, output.New("CLI_ERROR", "Stdin could not be read: "+err.Error(), "Pipe the value in on stdin.", nil)
	}
	value := stdinValue(raw)
	if value == "" {
		return nil, emptyValue(names[0])
	}
	return []secretValue{{Name: names[0], Value: value}}, nil
}

func emptyValue(name string) error {
	return output.New("INVALID_REQUEST", "No value came for "+name+".",
		"Pipe the value in on stdin (for example: printenv "+name+" | whisk secrets set "+name+"), or type it at the prompt on a terminal.", map[string]any{"name": name})
}

// readEnvFile reads a dotenv file, or stdin for -.
func (s *session) readEnvFile(path string) ([]byte, error) {
	if path == "-" {
		raw, err := io.ReadAll(io.LimitReader(s.env.Stdin, 4<<20))
		if err != nil {
			return nil, output.New("CLI_ERROR", "Stdin could not be read: "+err.Error(), "Pipe NAME=VALUE lines in on stdin.", nil)
		}
		return raw, nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.env.Dir, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, output.New("INVALID_REQUEST", "The file could not be read: "+err.Error(), "Pass the path of a file of NAME=VALUE lines, relative to the working directory, or - for stdin.", map[string]any{"file": path})
	}
	return raw, nil
}

// stdinValue is a value read from stdin with the one newline echo or a file ends it with
// dropped. Pure.
func stdinValue(raw []byte) string {
	raw = bytes.TrimSuffix(raw, []byte("\n"))
	raw = bytes.TrimSuffix(raw, []byte("\r"))
	return string(raw)
}

// parseEnvFile reads NAME=VALUE lines: blank lines and # comments are skipped, an export prefix
// is allowed, and a value wrapped in matching single or double quotes loses them. A name given
// twice keeps its last value, in the place it first appeared. Pure.
func parseEnvFile(raw []byte) ([]secretValue, error) {
	var out []secretValue
	at := map[string]int{}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, output.New("INVALID_REQUEST", fmt.Sprintf("Line %d is not NAME=VALUE.", n), "Write one NAME=VALUE per line; lines starting with # are comments.", map[string]any{"line": n})
		}
		value = unquote(strings.TrimSpace(value))
		if i, seen := at[name]; seen {
			out[i].Value = value
			continue
		}
		at[name] = len(out)
		out = append(out, secretValue{Name: name, Value: value})
	}
	if err := sc.Err(); err != nil {
		return nil, output.New("INVALID_REQUEST", "The file could not be read: "+err.Error(), "Keep each line under 1 MB.", nil)
	}
	if len(out) == 0 {
		return nil, output.New("INVALID_REQUEST", "The file has no NAME=VALUE lines.", "Write one NAME=VALUE per line.", nil)
	}
	return out, nil
}

// unquote drops one pair of matching quotes around a value. Pure.
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// pickSecretValues keeps the file's values for the names given, in the order given, or all of
// them when none is. A name the file lacks, or whose value is empty, is refused before anything
// is set. Pure.
func pickSecretValues(values []secretValue, names []string) ([]secretValue, error) {
	picked := values
	if len(names) > 0 {
		byName := map[string]string{}
		for _, v := range values {
			byName[v.Name] = v.Value
		}
		var missing []string
		picked = make([]secretValue, 0, len(names))
		for _, name := range names {
			value, ok := byName[name]
			if !ok {
				missing = append(missing, name)
				continue
			}
			picked = append(picked, secretValue{Name: name, Value: value})
		}
		if len(missing) > 0 {
			return nil, output.New("INVALID_REQUEST", "The file has no line for "+strings.Join(missing, ", ")+".", "Add NAME=VALUE lines for them, or leave the names out to set every line.", map[string]any{"names": missing})
		}
	}
	var empty []string
	for _, v := range picked {
		if v.Value == "" {
			empty = append(empty, v.Name)
		}
	}
	if len(empty) > 0 {
		return nil, output.New("INVALID_REQUEST", strings.Join(empty, ", ")+" has no value in the file.", "Give each name its value, or leave it out.", map[string]any{"names": empty})
	}
	return picked, nil
}
