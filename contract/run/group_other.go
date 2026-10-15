//go:build !unix

package run

import "os/exec"

// group leaves the command as it is where there are no process groups: cancelling kills the
// process itself.
func group(*exec.Cmd) {}
