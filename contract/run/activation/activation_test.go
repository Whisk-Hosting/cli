package activation

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestPick(t *testing.T) {
	cases := []struct {
		name   string
		env    Env
		want   int
		passed bool
		err    bool
	}{
		{"nothing passed", Env{Self: 7}, 0, false, false},
		{"another process's sockets", Env{PID: "8", FDs: "1", Names: "http", Self: 7}, 0, false, false},
		{"no pid", Env{FDs: "1", Names: "http", Self: 7}, 0, false, false},
		{"none passed", Env{PID: "7", FDs: "0", Self: 7}, 0, false, false},
		{"the one named", Env{PID: "7", FDs: "1", Names: "http", Self: 7}, 3, true, false},
		{"the second named", Env{PID: "7", FDs: "2", Names: "grpc:http", Self: 7}, 4, true, false},
		{"a single unnamed", Env{PID: "7", FDs: "1", Self: 7}, 3, true, false},
		{"several unnamed", Env{PID: "7", FDs: "2", Self: 7}, 0, false, true},
		{"not among them", Env{PID: "7", FDs: "1", Names: "push", Self: 7}, 0, false, true},
		{"count and names disagree", Env{PID: "7", FDs: "2", Names: "http", Self: 7}, 0, false, true},
		{"count unreadable", Env{PID: "7", FDs: "two", Names: "http", Self: 7}, 0, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, passed, err := Pick(c.env, "http")
			if (err != nil) != c.err || got != c.want || passed != c.passed {
				t.Fatalf("Pick = %d, %v, %v; want %d, %v, error %v", got, passed, err, c.want, c.passed, c.err)
			}
		})
	}
}

func TestListenWithoutSystemdListensItself(t *testing.T) {
	t.Setenv("LISTEN_FDS", "")
	lis, err := Listen("http", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	if lis.Addr().(*net.TCPAddr).Port == 0 {
		t.Fatalf("listening on %s", lis.Addr())
	}
}

// A process started as systemd starts it, with the socket at descriptor 3 and its own pid in
// LISTEN_PID, serves on that socket rather than binding the address, which the socket holds.
func TestListenServesOnThePassedSocket(t *testing.T) {
	if os.Getenv("ACTIVATION_CHILD") == "1" {
		lis, err := Listen("http", "not-an-address")
		if err != nil {
			os.Exit(2)
		}
		c, err := lis.Accept()
		if err != nil {
			os.Exit(3)
		}
		_, _ = c.Write([]byte("served"))
		c.Close()
		os.Exit(0)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	f, err := held.(*net.TCPListener).File()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// exec keeps the shell's pid, so LISTEN_PID names the test binary's process.
	cmd := exec.Command(sh, "-c", `LISTEN_PID=$$ exec "$0" -test.run=TestListenServesOnThePassedSocket`, os.Args[0])
	cmd.Env = append(os.Environ(), "ACTIVATION_CHILD=1", "LISTEN_FDS=1", "LISTEN_FDNAMES=http")
	cmd.ExtraFiles = []*os.File{f}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", held.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	got, _ := io.ReadAll(conn)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child: %v", err)
	}
	if string(got) != "served" {
		t.Fatalf("read %q from the passed socket", got)
	}
}

func TestDrainWaitsForAnAcceptedConnection(t *testing.T) {
	lis, err := Listen("none", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	client, err := net.Dial("tcp", lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := lis.Accept()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		close bool
		ok    bool
	}{
		{"an open connection holds the stop until its time runs out", false, false},
		{"a closed one lets it go", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.close {
				_ = server.Close()
				_ = server.Close() // a second close counts once
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if err := lis.Drain(ctx); (err == nil) != tc.ok {
				t.Fatalf("Drain: %v, want ok %v", err, tc.ok)
			}
		})
	}
}
