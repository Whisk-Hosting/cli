// whisk is the command coding agents drive and humans occasionally type. See docs/CLI.md.
package main

import (
	"context"
	"os"

	"golang.org/x/term"

	"github.com/whisk-run/cli/whisk"
)

func main() {
	whisk.TrustSystemStoreToo()
	dir, _ := os.Getwd()
	env := whisk.Env{
		Stdin:        os.Stdin,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		Getenv:       os.Getenv,
		Dir:          dir,
		IsTerminal:   term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())),
		ReportMisses: true,
		// A build from source (go run, a test binary) may live in a temporary directory that
		// is gone by the next git push, so only a release becomes git's helper.
		SetupGit: whisk.Version != "dev",
	}
	os.Exit(whisk.Run(context.Background(), os.Args[1:], env))
}
