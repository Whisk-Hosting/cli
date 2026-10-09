//go:build windows

package stub

import (
	"os/exec"
	"time"
)

// Windows has no process groups to signal; the child is killed outright when the stub stops.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Kill() }
}

func terminate(c *exec.Cmd, grace time.Duration) {
	if c.Process == nil {
		return
	}
	_ = c.Process.Kill()
	waitAtMost(c, grace)
}
