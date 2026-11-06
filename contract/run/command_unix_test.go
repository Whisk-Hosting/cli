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
	// The deadline leaves the shell time to record its child's pid before the group is killed.
	cmd := Command(context.Background(), time.Second, "sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait")
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
	deadline := time.Now().Add(5 * time.Second)
	for running(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("the child %d outlived the command", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// running reports whether pid is a live process. A killed child is reparented and stays a
// zombie until whatever process adopted it reaps it, which can take seconds; a zombie has
// stopped running, so it counts as gone.
func running(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	state := string(stat)[strings.LastIndex(string(stat), ")")+1:]
	return !strings.HasPrefix(strings.TrimSpace(state), "Z")
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
