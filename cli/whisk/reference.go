package whisk

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RefGroup is one top-level command and every command under it, as the public CLI reference at
// /docs/cli lists them. The reference is the command tree itself (reference.json, written by
// TestReferenceIsTheCommandTree), so the docs never list a command the CLI lacks or miss one it has.
type RefGroup struct {
	Name     string       `json:"name"`
	Short    string       `json:"short"`
	Commands []RefCommand `json:"commands"`
}

// RefCommand is one command a person or an agent can run.
type RefCommand struct {
	Usage string    `json:"usage"`
	Short string    `json:"short"`
	Flags []RefFlag `json:"flags,omitempty"`
}

// RefFlag is one of a command's own flags.
type RefFlag struct {
	Name  string `json:"name"`
	Usage string `json:"usage"`
}

// refLeftOut are top-level commands the public reference does not list: the operator's, and the
// shell's completion scripts cobra adds.
var refLeftOut = map[string]bool{"operator": true, "completion": true, "help": true}

// Reference is the public CLI reference from the command tree: every visible top-level command in
// name order with each runnable command under it, its usage line, its words and its own flags.
// Pure: it reads the tree and changes nothing.
func Reference(root *cobra.Command) []RefGroup {
	var groups []RefGroup
	for _, top := range root.Commands() {
		if top.Hidden || refLeftOut[top.Name()] {
			continue
		}
		groups = append(groups, RefGroup{Name: top.Name(), Short: top.Short, Commands: refCommands(top)})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups
}

// refCommands is cmd and every visible command under it that runs, in name order.
func refCommands(cmd *cobra.Command) []RefCommand {
	var out []RefCommand
	if cmd.Runnable() {
		out = append(out, RefCommand{Usage: strings.TrimSuffix(cmd.UseLine(), " [flags]"), Short: cmd.Short, Flags: refFlags(cmd)})
	}
	subs := cmd.Commands()
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
	for _, sub := range subs {
		if !sub.Hidden && sub.Name() != "help" {
			out = append(out, refCommands(sub)...)
		}
	}
	return out
}

// refFlags is the command's own visible flags, without the ones every command has.
func refFlags(cmd *cobra.Command) []RefFlag {
	var out []RefFlag
	cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		name := "--" + f.Name
		if t := f.Value.Type(); t != "bool" {
			name += " " + t
		}
		out = append(out, RefFlag{Name: name, Usage: f.Usage})
	})
	return out
}
