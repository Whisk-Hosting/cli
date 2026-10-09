//go:build unix

package run

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCommandKillsTheWholeGroupAtItsDeadline(t *testing.T) {
	pidFile := t.TempDir() + "/child"
	cmd := Command(context.Background(), 200*time.Millisecond, "sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait")
	start := time.Now()
	err := cmd.Run()
	if err == nil || time.Since(start) > 8*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
	raw, rerr := os.ReadFile(pidFile)
	if rerr != nil {
		t.Fatal(rerr)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("the child %d outlived the command", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestOutputIsCappedAndCarriesStderr(t *testing.T) {
	out, err := Command(context.Background(), 10*time.Second, "sh", "-c", "head -c 3000000 /dev/zero").Output()
	if err != nil || len(out) != MaxOutput {
		t.Fatalf("len %d err %v", len(out), err)
	}
	_, err = Command(context.Background(), 10*time.Second, "sh", "-c", "echo oops >&2; exit 3").Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 || strings.TrimSpace(string(exit.Stderr)) != "oops" {
		t.Fatalf("err %v", err)
	}
	both, err := Command(context.Background(), 10*time.Second, "sh", "-c", "echo a; echo b >&2").CombinedOutput()
	if err != nil || !strings.Contains(string(both), "a") || !strings.Contains(string(both), "b") {
		t.Fatalf("%q %v", both, err)
	}
}
