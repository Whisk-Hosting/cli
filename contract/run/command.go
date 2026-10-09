package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// MaxOutput is how much of a command's output Output and CombinedOutput keep: the start, which
// says what went wrong. The rest is dropped.
const MaxOutput = 1 << 20

// Cmd is a command with a deadline. It is an exec.Cmd, so Dir, Env, Stdin and the pipes are set
// as usual; when the deadline passes or ctx ends the command's whole process group is killed,
// not only its first process, and Wait returns at most a few seconds later even if a child kept
// the output open.
type Cmd struct {
	*exec.Cmd
	cancel context.CancelFunc
	once   sync.Once
}

// Command is name with args, to run within timeout.
func Command(ctx context.Context, timeout time.Duration, name string, args ...string) *Cmd {
	if timeout <= 0 {
		panic("run.Command: a command needs a deadline")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	cmd := exec.CommandContext(ctx, name, args...)
	group(cmd)
	cmd.WaitDelay = 5 * time.Second
	return &Cmd{Cmd: cmd, cancel: cancel}
}

func (c *Cmd) done() { c.once.Do(c.cancel) }

// Run starts the command and waits for it.
func (c *Cmd) Run() error {
	defer c.done()
	return c.Cmd.Run()
}

// Wait waits for a started command.
func (c *Cmd) Wait() error {
	defer c.done()
	return c.Cmd.Wait()
}

// Start starts the command; Wait must follow, or Release when the command is left running in
// the care of something else.
func (c *Cmd) Start() error {
	if err := c.Cmd.Start(); err != nil {
		c.done()
		return err
	}
	return nil
}

// Release gives up the deadline's resources without waiting; the command is killed now if it is
// still running.
func (c *Cmd) Release() { c.done() }

// Output runs the command and answers its standard output, the first MaxOutput bytes of it. A
// failure's ExitError carries the start of standard error.
func (c *Cmd) Output() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("run: Stdout already set")
	}
	var out, stderr capped
	c.Stdout = &out
	if c.Stderr == nil {
		c.Stderr = &stderr
	}
	err := c.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		exit.Stderr = stderr.Bytes()
	}
	return out.Bytes(), err
}

// CombinedOutput runs the command and answers its standard output and error together, the first
// MaxOutput bytes of them.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil || c.Stderr != nil {
		return nil, errors.New("run: Stdout or Stderr already set")
	}
	var out capped
	c.Stdout, c.Stderr = &out, &out
	err := c.Run()
	return out.Bytes(), err
}

// String is the command as it would be typed, for logs.
func (c *Cmd) String() string { return fmt.Sprint(c.Cmd) }

// capped keeps the first MaxOutput bytes written to it and drops the rest, while telling the
// writer everything was taken so the command is never blocked or failed by it.
type capped struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *capped) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := MaxOutput - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (c *capped) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return bytes.Clone(c.buf.Bytes())
}

// Process is name with args for a process meant to run as long as ctx lasts, such as an app a
// developer is running locally or a server the program supervises: it has no deadline, but when
// ctx ends its whole process group is killed, and Wait returns at most a few seconds later.
func Process(ctx context.Context, name string, args ...string) *Cmd {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, name, args...)
	group(cmd)
	cmd.WaitDelay = 5 * time.Second
	return &Cmd{Cmd: cmd, cancel: cancel}
}
