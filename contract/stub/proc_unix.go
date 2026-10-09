//go:build !windows

package stub

import (
	"os/exec"
	"syscall"
	"time"
)

// Each child runs in its own process group so that stopping the stub stops everything the
// child started (sh -c, npm, the app), as the platform's SIGTERM reaches the whole container.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return signalGroup(cmd, syscall.SIGTERM) }
}

func signalGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, sig)
}

func terminate(c *exec.Cmd, grace time.Duration) {
	if c.Process == nil {
		return
	}
	_ = signalGroup(c, syscall.SIGTERM)
	if !waitAtMost(c, grace) {
		_ = signalGroup(c, syscall.SIGKILL)
	}
}
