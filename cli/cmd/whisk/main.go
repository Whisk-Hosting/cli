// whisk is the command coding agents drive and humans occasionally type. See docs/CLI.md.
package main

import (
	"context"
	"os"

	"golang.org/x/term"

	"github.com/whisk-run/cli/whisk"
)

func main() {
	dir, _ := os.Getwd()
	env := whisk.Env{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Getenv:     os.Getenv,
		Dir:        dir,
		IsTerminal: term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())),
	}
	os.Exit(whisk.Run(context.Background(), os.Args[1:], env))
}
