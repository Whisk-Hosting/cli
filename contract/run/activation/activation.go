// Package activation serves on the listening socket systemd holds for a service (socket
// activation), so a restart of the service leaves its port open: connections wait in the
// socket's queue for the new process instead of being refused. Without one it listens itself.
package activation

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// firstFD is the first descriptor systemd passes (SD_LISTEN_FDS_START).
const firstFD = 3

// Env is what systemd's socket activation tells a process: LISTEN_PID, LISTEN_FDS and
// LISTEN_FDNAMES as set, and the process's own pid.
type Env struct {
	PID, FDs, Names string
	Self            int
}

// Pick is the descriptor of the socket named name among those systemd passed, and whether it
// passed any to this process at all. Sockets passed to another process (LISTEN_PID names
// another, as in a program this one started) or none count as not passed; passed sockets without
// name among them, or a count and names that disagree, are an error, since listening anew would
// race the socket systemd holds. With no names given, a single socket is the one. Pure.
func Pick(e Env, name string) (int, bool, error) {
	if e.FDs == "" {
		return 0, false, nil
	}
	if pid, err := strconv.Atoi(e.PID); err != nil || pid != e.Self {
		return 0, false, nil
	}
	n, err := strconv.Atoi(e.FDs)
	if err != nil || n < 0 {
		return 0, false, fmt.Errorf("LISTEN_FDS %q is not a count", e.FDs)
	}
	if n == 0 {
		return 0, false, nil
	}
	if e.Names == "" {
		if n == 1 {
			return firstFD, true, nil
		}
		return 0, false, fmt.Errorf("systemd passed %d sockets without names, so none is known to be %q", n, name)
	}
	names := strings.Split(e.Names, ":")
	if len(names) != n {
		return 0, false, fmt.Errorf("LISTEN_FDS is %d but LISTEN_FDNAMES names %d", n, len(names))
	}
	for i, got := range names {
		if got == name {
			return firstFD + i, true, nil
		}
	}
	return 0, false, fmt.Errorf("systemd passed the sockets %q and none is named %q", e.Names, name)
}

// Listen is the TCP listener systemd passed under name (the socket unit's FileDescriptorName),
// or a new one on addr when it passed none. Programs the process starts ignore the passed
// variables, which name this process's pid; the listener's descriptor closes on exec.
func Listen(name, addr string) (net.Listener, error) {
	fd, ok, err := Pick(Env{PID: os.Getenv("LISTEN_PID"), FDs: os.Getenv("LISTEN_FDS"), Names: os.Getenv("LISTEN_FDNAMES"), Self: os.Getpid()}, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return net.Listen("tcp", addr)
	}
	f := os.NewFile(uintptr(fd), name)
	// FileListener duplicates the descriptor with close-on-exec; systemd's copy stays its own.
	defer f.Close()
	lis, err := net.FileListener(f)
	if err != nil {
		return nil, fmt.Errorf("socket %q from systemd: %w", name, err)
	}
	return lis, nil
}
