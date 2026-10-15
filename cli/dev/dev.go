package dev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/whisk-run/cli/internal/safefile"
	"github.com/whisk-run/cli/scaffold"
	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/run"
	"github.com/whisk-run/contract/stub"
)

// Dir is where the generated files live, relative to the app.
const Dir = ".whisk/dev"

// Options for one `whisk dev` run.
type Options struct {
	AppDir   string
	Manifest manifest.Manifest
	Port     int // the edge; api is Port+1, internal Port+2
	Identity struct {
		As, Name, Audience string
		Groups, Roles      []string
	}
	Log io.Writer
}

// Stack is a running local stack.
type Stack struct {
	Plan  Plan
	Ports Ports
	File  string
}

// FreePort asks the kernel for an unused TCP port on the loopback interface.
func FreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func freePorts(n int) ([]int, error) {
	out := make([]int, 0, n)
	listeners := make([]net.Listener, 0, n)
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()
	for len(out) < n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, l)
		out = append(out, l.Addr().(*net.TCPAddr).Port)
	}
	return out, nil
}

// How long docker may take: a question to the daemon, starting the stack (which pulls its
// images on first use), and stopping it.
const (
	dockerQuick = time.Minute
	composeUp   = 30 * time.Minute
	composeDown = 5 * time.Minute
)

// DockerDesktop reports whether the daemon is Docker Desktop, where host.docker.internal
// reaches the host's loopback listeners.
func DockerDesktop(ctx context.Context) bool {
	out, err := run.Command(ctx, dockerQuick, "docker", "info", "--format", "{{.OperatingSystem}}").Output()
	return err == nil && strings.Contains(string(out), "Docker Desktop")
}

// CheckDocker makes sure docker and the compose plugin answer.
func CheckDocker(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker is not installed or not on PATH; whisk dev needs Docker with the compose plugin")
	}
	if err := run.Command(ctx, dockerQuick, "docker", "compose", "version").Run(); err != nil {
		return errors.New("docker compose does not answer; install the compose plugin or start Docker")
	}
	return nil
}

// Up writes the compose file (reusing the running stack's only when it is exactly the file
// the CLI would write), makes sure .gitignore keeps .whisk/dev/ out of commits, and starts the
// services, waiting for health.
func Up(ctx context.Context, o Options, log io.Writer) (Stack, error) {
	// The app directory may be a cloned repository: .whisk or .whisk/dev committed as a link
	// must not send the generated files somewhere else.
	dir, err := safefile.Inside(o.AppDir, Dir)
	if err != nil {
		return Stack{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Stack{}, err
	}
	file := filepath.Join(dir, "compose.yaml")
	project := "whisk-" + o.Manifest.Name
	internal := o.Port + 2
	hostNet := runtime.GOOS == "linux" && !DockerDesktop(ctx)

	ports, reuse := runningPorts(ctx, file, project, o.Manifest, internal, hostNet)
	if !reuse {
		got, err := freePorts(6)
		if err != nil {
			return Stack{}, err
		}
		ports = usedPorts(o.Manifest, Ports{Postgres: got[0], PgBouncer: got[1], Inngest: got[2], Valkey: got[3], Storage: got[4], Console: got[5]})
	}
	plan := BuildPlan(o.Manifest, ports, internal, hostNet)
	if !reuse {
		if err := safefile.WriteFile(file, []byte(plan.Compose), 0o644); err != nil {
			return Stack{}, err
		}
	}
	// secrets.env holds values: the directory must be ignored before the file exists, also
	// in a repository bound with whisk use or whisk clone, which write no .gitignore.
	added, err := EnsureIgnored(o.AppDir)
	if err != nil {
		return Stack{}, err
	}
	if len(added) > 0 {
		fmt.Fprintf(log, "dev: added %s to .gitignore\n", strings.Join(added, ", "))
	}
	secrets := filepath.Join(dir, "secrets.env")
	if _, err := os.Lstat(secrets); errors.Is(err, os.ErrNotExist) {
		header := "# Values for the names under secrets: in whisk.yaml, one NAME=value per line.\n# This file is git-ignored; whisk dev reads it on start.\n"
		for _, n := range o.Manifest.Secrets {
			header += n + "=\n"
		}
		_ = safefile.WriteFile(secrets, []byte(header), 0o600)
	}
	fmt.Fprintf(log, "dev: starting %s (postgres, pgbouncer, inngest%s%s)\n", project, when(o.Manifest.KV, ", valkey"), when(o.Manifest.Storage, ", storage"))
	cmd := run.Command(ctx, composeUp, "docker", "compose", "-f", file, "-p", project, "up", "-d", "--wait", "--quiet-pull")
	cmd.Dir = o.AppDir
	var errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = io.Discard, &errOut
	if err := cmd.Run(); err != nil {
		return Stack{}, fmt.Errorf("docker compose up: %v\n%s", err, strings.TrimSpace(errOut.String()))
	}
	if err := waitTCP(ctx, ports.PgBouncer, 60*time.Second); err != nil {
		return Stack{}, fmt.Errorf("pgbouncer did not answer on port %d: %v", ports.PgBouncer, err)
	}
	return Stack{Plan: plan, Ports: ports, File: file}, nil
}

func when(cond bool, s string) string {
	if cond {
		return s
	}
	return ""
}

// runningPorts reads the ports of an already running stack whose compose file is the CLI's
// own, so restarts keep their addresses.
func runningPorts(ctx context.Context, file, project string, m manifest.Manifest, internal int, hostNet bool) (Ports, bool) {
	src, err := os.ReadFile(file)
	if err != nil {
		return Ports{}, false
	}
	ports, ok := reusable(string(src), m, internal, hostNet)
	if !ok {
		return Ports{}, false
	}
	out, err := run.Command(ctx, dockerQuick, "docker", "compose", "-f", file, "-p", project, "ps", "--format", "json", "--status", "running").Output()
	if err != nil || len(bytes.TrimSpace(out)) == 0 {
		return Ports{}, false
	}
	return ports, true
}

// reusable reports whether an on-disk compose file may be run as it is: only when it is,
// byte for byte, the file the CLI would write now for the ports it names. A file that differs
// in any way (an edited service, a mount, a forged inputs line committed to the repository) is
// regenerated, never run. Pure.
func reusable(src string, m manifest.Manifest, internal int, hostNet bool) (Ports, bool) {
	ports, ok := portsFromCompose(src)
	if !ok || BuildPlan(m, ports, internal, hostNet).Compose != src {
		return Ports{}, false
	}
	return ports, true
}

// usedPorts keeps only the ports of services the manifest runs, so the file written from them
// reads back to the same ports and a restart can recognise it. Pure.
func usedPorts(m manifest.Manifest, p Ports) Ports {
	if !m.KV {
		p.Valkey = 0
	}
	if !m.Storage {
		p.Storage, p.Console = 0, 0
	}
	return p
}

var rePort = regexp.MustCompile(`127\.0\.0\.1:(\d+):(\d+)`)
var reInngestHost = regexp.MustCompile(`"--port", "(\d+)"`)

func portsFromCompose(src string) (Ports, bool) {
	var p Ports
	for _, m := range rePort.FindAllStringSubmatch(src, -1) {
		host, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "5432":
			if p.Postgres == 0 {
				p.Postgres = host
			} else {
				p.PgBouncer = host
			}
		case "8288":
			p.Inngest = host
		case "6379":
			p.Valkey = host
		case "9000":
			p.Storage = host
		case "9001":
			p.Console = host
		}
	}
	if p.Inngest == 0 && strings.Contains(src, "network_mode: host") {
		if m := reInngestHost.FindStringSubmatch(src); m != nil {
			p.Inngest, _ = strconv.Atoi(m[1])
		}
	}
	return p, p.Postgres != 0 && p.PgBouncer != 0 && p.Inngest != 0
}

func waitTCP(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			c.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("timeout")
}

// Down stops the stack; volumes go too when wipe is set. The compose file on disk is not run:
// it may have come with the repository. Down runs a file it generates itself for the project,
// declaring every service and volume whisk dev can create, so --remove-orphans and --volumes
// reach all of them.
func Down(ctx context.Context, appDir string, m manifest.Manifest, wipe bool, log io.Writer) error {
	file := filepath.Join(appDir, filepath.FromSlash(Dir), "compose.yaml")
	if _, err := os.Lstat(file); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(log, "dev: nothing to stop; no .whisk/dev/compose.yaml")
		return nil
	}
	tmp, err := os.MkdirTemp("", "whisk-dev-down-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	own := filepath.Join(tmp, "compose.yaml")
	if err := os.WriteFile(own, []byte(DownPlan(m).Compose), 0o600); err != nil {
		return err
	}
	args := []string{"compose", "-f", own, "-p", "whisk-" + m.Name, "down", "--remove-orphans"}
	if wipe {
		args = append(args, "--volumes")
	}
	cmd := run.Command(ctx, composeDown, "docker", args...)
	cmd.Dir = tmp
	cmd.Stdout, cmd.Stderr = log, log
	return cmd.Run()
}

// DownPlan is the stack Down names: the manifest's project with every optional service on, so
// a stack started before kv or storage was taken out of whisk.yaml is stopped whole. Ports do
// not matter to down. Pure.
func DownPlan(m manifest.Manifest) Plan {
	m.KV, m.Storage = true, true
	return BuildPlan(m, Ports{Postgres: 1, PgBouncer: 2, Inngest: 3, Valkey: 4, Storage: 5, Console: 6}, 7, false)
}

// EnsureIgnored adds the entries every app repository needs (scaffold.GitignoreLines,
// .whisk/dev/ among them) to the app's .gitignore when they are missing, and returns what it
// added. A .gitignore that is a symbolic link is refused rather than followed.
func EnsureIgnored(appDir string) ([]string, error) {
	path, err := safefile.Inside(appDir, ".gitignore")
	if err != nil {
		return nil, err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	merged, added := scaffold.MergeGitignore(string(existing))
	if len(added) == 0 {
		return nil, nil
	}
	return added, safefile.WriteFile(path, []byte(merged), 0o644)
}

// Status describes the stack for `whisk dev --status` and the banner.
func (s Stack) Status() map[string]any {
	out := map[string]any{
		"project":      s.Plan.Project,
		"database_url": s.Plan.DatabaseURL,
		"postgres_url": s.Plan.DirectURL,
		"inngest_url":  s.Plan.InngestURL,
		"compose_file": s.File,
	}
	for _, kv := range s.Plan.ExtraEnv {
		k, v, _ := strings.Cut(kv, "=")
		out[strings.ToLower(k)] = v
	}
	return out
}

// RunStub starts the stub against the stack and blocks until ctx ends.
func RunStub(ctx context.Context, o Options, s Stack, appCmd []string) error {
	opts := stub.Options{
		ManifestPath: filepath.Join(o.AppDir, "whisk.yaml"),
		Listen:       fmt.Sprintf("127.0.0.1:%d", o.Port),
		API:          fmt.Sprintf("127.0.0.1:%d", o.Port+1),
		Internal:     fmt.Sprintf("127.0.0.1:%d", o.Port+2),
		As:           o.Identity.As,
		Name:         o.Identity.Name,
		Groups:       o.Identity.Groups,
		Roles:        o.Identity.Roles,
		Audience:     o.Identity.Audience,
		DatabaseURL:  s.Plan.DatabaseURL,
		SecretsFile:  filepath.Join(o.AppDir, filepath.FromSlash(Dir), "secrets.env"),
		InngestURL:   s.Plan.InngestURL,
		ExtraEnv:     s.Plan.ExtraEnv,
		Migrate:      true,
		Log:          o.Log,
	}
	return stub.Run(ctx, opts, appCmd)
}

// JSON renders a status map.
func JSON(v any) string {
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}
