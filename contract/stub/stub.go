// Package stub is a local stand-in for the Whisk platform, so an app can be run and tested
// against the contract before any platform exists: the edge (identity headers, public routes,
// CSRF, challenge, security headers), the queue and workflow API (events, approvals) in front
// of the Inngest dev server, and the webhook ingress (presets, storage, delivery, replay).
//
// cmd/whisk-stub is the command-line wrapper; the CLI's whisk dev embeds this package with
// Postgres and the dev server managed. The app receives the same environment it has in
// production (environment.md) and the same headers (headers.md). Nothing here is production
// code; it is the contract, executable.
package stub

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/run"
	"github.com/whisk-run/contract/run/runhttp"
	"github.com/whisk-run/contract/webhook"
)

// Options configure one run. Zero values take the defaults documented on each field.
type Options struct {
	ManifestPath string // whisk.yaml
	Listen       string // edge listener, the app hostname: 127.0.0.1:3000
	API          string // api listener, queue, approvals, webhooks, inngest proxy: 127.0.0.1:3001
	Internal     string // internal listener, platform deliveries with service identity: 127.0.0.1:3002
	Upstream     string // where the app listens: http://127.0.0.1:8080

	As       string   // sign in as this email (team identity); empty means nobody can sign in
	Name     string   // display name for As
	Groups   []string // groups for As
	Roles    []string // org roles for As; defaults to owner
	Audience string   // team or customer; defaults to team

	OrgID string // generated when empty
	AppID string // generated when empty

	DatabaseURL string            // DATABASE_URL for the app; required unless database: none
	SecretsFile string            // NAME=value lines for declared secrets: .whisk/dev/secrets.env
	Secrets     map[string]string // secrets in addition to (and over) the file
	ExtraEnv    []string          // NAME=value pairs the caller injects, for storage and kv in whisk dev

	InngestURL  string // a running Inngest dev server; empty starts one on InngestPort
	InngestPort string // 8288
	InngestCmd  string // npx --yes inngest-cli@1.46.0
	NoInngest   bool   // run without a workflow engine
	Migrate     bool   // run the manifest's migrate command before starting the app

	Log io.Writer // where the stub's own lines go; os.Stderr when nil
}

// Defaults are the values cmd/whisk-stub offers on its flags.
var Defaults = Options{
	ManifestPath: "whisk.yaml",
	Listen:       "127.0.0.1:3000",
	API:          "127.0.0.1:3001",
	Internal:     "127.0.0.1:3002",
	Upstream:     "http://127.0.0.1:8080",
	Audience:     "team",
	Roles:        []string{"owner"},
	SecretsFile:  ".whisk/dev/secrets.env",
	InngestPort:  "8288",
	InngestCmd:   "npx --yes inngest-cli@1.46.0",
	Migrate:      true,
}

func (o Options) withDefaults() Options {
	def := Defaults
	pick := func(v, d string) string {
		if v == "" {
			return d
		}
		return v
	}
	o.ManifestPath = pick(o.ManifestPath, def.ManifestPath)
	o.Listen = pick(o.Listen, def.Listen)
	o.API = pick(o.API, def.API)
	o.Internal = pick(o.Internal, def.Internal)
	o.Upstream = pick(o.Upstream, def.Upstream)
	o.Audience = pick(o.Audience, def.Audience)
	o.SecretsFile = pick(o.SecretsFile, def.SecretsFile)
	o.InngestPort = pick(o.InngestPort, def.InngestPort)
	o.InngestCmd = pick(o.InngestCmd, def.InngestCmd)
	if o.Roles == nil {
		o.Roles = def.Roles
	}
	if o.Log == nil {
		o.Log = os.Stderr
	}
	return o
}

type stub struct {
	manifest manifest.Manifest
	identity *identity
	orgID    string
	appID    string

	edgeURL     string
	apiURL      string
	internalURL string
	upstream    *url.URL
	inngestURL  *url.URL
	eventKey    string
	signingKey  string
	// deliveryKey is WHISK_DELIVERY_KEY, decoded: random per run, the app's own, what the
	// stub signs webhook deliveries with. deliveryMarker is a private token the stub's own
	// posts carry so the internal listener can tell them from any other caller.
	deliveryKey    []byte
	deliveryMarker string
	// mediaKey signs and checks media links (media.go): random per run, never handed to the app,
	// as the platform's key is not.
	mediaKey []byte

	serviceToken string
	sessionValue string
	sessionID    string
	secrets      map[string]string
	extraEnv     []string
	urlTokens    map[string]string
	bodyLimit    int64
	altcha       altcha
	log          io.Writer

	store *store
	hooks *hooks
}

func (s *stub) logf(format string, args ...any) {
	w := s.log
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "stub: "+format+"\n", args...)
}

// build reads the manifest and secrets and assembles the stub without starting anything.
func build(o Options) (*stub, manifest.Manifest, error) {
	src, err := os.ReadFile(o.ManifestPath)
	if err != nil {
		return nil, manifest.Manifest{}, fmt.Errorf("W001: %v", err)
	}
	m, err := manifest.Parse(src)
	if err != nil {
		return nil, m, fmt.Errorf("W002: %v", err)
	}
	upstream, err := url.Parse(o.Upstream)
	if err != nil {
		return nil, m, err
	}
	s := &stub{
		manifest:       m,
		orgID:          orULID(o.OrgID),
		appID:          orULID(o.AppID),
		edgeURL:        "http://" + o.Listen,
		apiURL:         "http://" + o.API,
		internalURL:    "http://" + o.Internal,
		upstream:       upstream,
		eventKey:       "whisk-stub-event-key",
		signingKey:     "signkey-test-" + strings.Repeat("0", 64),
		serviceToken:   "whsk_service_" + randomToken(32),
		deliveryKey:    randomBytes(32),
		deliveryMarker: randomToken(32),
		mediaKey:       randomBytes(32),
		sessionValue:   randomToken(32),
		sessionID:      newULID(),
		extraEnv:       o.ExtraEnv,
		urlTokens:      map[string]string{},
		bodyLimit:      8 << 20,
		altcha:         altcha{key: []byte(randomToken(32)), maxNumber: 50000},
		store:          newStore(),
		log:            o.Log,
	}
	s.hooks = newHooks(s)
	if o.As != "" {
		if o.Audience != "team" && o.Audience != "customer" {
			return nil, m, errors.New("audience must be team or customer")
		}
		s.identity = &identity{UserID: newULID(), Email: strings.ToLower(o.As), Name: o.Name, Groups: o.Groups, Roles: o.Roles, Audience: o.Audience}
	}
	if s.secrets, err = readSecrets(o.SecretsFile); err != nil {
		return nil, m, err
	}
	for k, v := range o.Secrets {
		s.secrets[k] = v
	}
	for _, w := range m.Webhooks {
		if w.Preset == webhook.PresetToken {
			s.urlTokens[w.Name] = randomToken(24)
		}
	}
	if !o.NoInngest {
		if s.inngestURL, err = url.Parse(inngestURL(o)); err != nil {
			return nil, m, err
		}
	}
	return s, m, nil
}

func inngestURL(o Options) string {
	if o.InngestURL != "" {
		return o.InngestURL
	}
	return "http://127.0.0.1:" + o.InngestPort
}

// Env is the environment the app would receive for these options, without starting anything.
func Env(o Options) ([]string, error) {
	o = o.withDefaults()
	s, _, err := build(o)
	if err != nil {
		return nil, err
	}
	return s.appEnv(o.DatabaseURL), nil
}

// Run serves the three listeners, starts the dev server, migrates, starts the app and blocks
// until ctx is done, then stops what it started.
func Run(ctx context.Context, o Options, appCmd []string) error {
	o = o.withDefaults()
	s, m, err := build(o)
	if err != nil {
		return err
	}
	if m.Database != "none" && o.DatabaseURL == "" {
		return errors.New("a database URL is required because the manifest declares a database; start Postgres and pass its URL")
	}
	env := s.appEnv(o.DatabaseURL)

	servers := []*http.Server{
		{Addr: o.Listen, Handler: newEdge(s)},
		{Addr: o.API, Handler: newAPI(s)},
		{Addr: o.Internal, Handler: newInternal(s)},
	}
	for _, srv := range servers {
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			return fmt.Errorf("listen %s: %v", srv.Addr, err)
		}
		run.Go(ctx, "stub listener "+srv.Addr, serveLoop(srv, ln))
	}

	var children []*run.Cmd
	if !o.NoInngest && o.InngestURL == "" && !reachable(s.inngestURL.String()) {
		cmd := command(ctx, o.InngestCmd+" dev -u "+s.internalURL+m.Queue.Endpoint+" --no-discovery --port "+o.InngestPort, nil, "inngest", o.Log)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start inngest dev server (%s): %v; pass a running dev server URL or run without a workflow engine", o.InngestCmd, err)
		}
		children = append(children, cmd)
		s.logf("inngest dev server starting on %s (ui at %s)", s.inngestURL, s.inngestURL)
	}

	if o.Migrate && m.Migrate != "" && len(appCmd) > 0 {
		s.logf("migrate: %s", m.Migrate)
		cmd := command(ctx, m.Migrate, env, "migrate", o.Log)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("MIGRATE_FAILED: %s: %v", m.Migrate, err)
		}
	}

	if len(appCmd) > 0 {
		cmd := command(ctx, strings.Join(appCmd, " "), env, "", o.Log)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start app: %v", err)
		}
		children = append(children, cmd)
		s.logf("app started (pid %d): %s", cmd.Process.Pid, strings.Join(appCmd, " "))
	} else {
		s.logf("no app command given; expecting the app on %s with this environment:", s.upstream)
		for _, kv := range env {
			fmt.Fprintln(o.Log, "  "+kv)
		}
	}

	run.Spawn(ctx, "stub health check", func(ctx context.Context) error {
		s.waitHealthy(ctx)
		return nil
	})
	s.banner()

	<-ctx.Done()
	s.logf("stopping")
	for _, c := range children {
		terminate(c.Cmd, time.Duration(28)*time.Second)
	}
	for _, srv := range servers {
		_ = srv.Close()
	}
	return nil
}

// waitAtMost waits for a started child to exit, for no longer than grace, and answers whether it
// did.
func waitAtMost(c *exec.Cmd, grace time.Duration) bool {
	done := make(chan struct{})
	run.Spawn(context.Background(), "stub child wait", func(context.Context) error {
		_ = c.Wait()
		close(done)
		return nil
	})
	select {
	case <-done:
		return true
	case <-time.After(grace):
		return false
	}
}

// serveLoop is a listener's loop: it serves on ln until the server is closed, and listens on
// the address again when it is started after a failure.
func serveLoop(srv *http.Server, ln net.Listener) func(context.Context) error {
	return func(context.Context) error {
		if ln == nil {
			var err error
			if ln, err = net.Listen("tcp", srv.Addr); err != nil {
				return err
			}
		}
		err := srv.Serve(ln)
		ln = nil
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *stub) banner() {
	m := s.manifest
	s.logf("app %s: %s (org %s, app %s)", m.Name, s.edgeURL, s.orgID, s.appID)
	if s.identity != nil {
		s.logf("sign in: %s/.whisk/login?return=/  (as %s, %s, roles %s)", s.edgeURL, s.identity.Email, s.identity.Audience, strings.Join(s.identity.Roles, ","))
	} else {
		s.logf("no identity given: every request is anonymous and private routes answer 401/302")
	}
	s.logf("api: %s  (GET /v1/stub describes it; WHISK_SERVICE_TOKEN is in the app's environment)", s.apiURL)
	for _, w := range m.Webhooks {
		u := fmt.Sprintf("%s/hooks/%s/%s/%s", s.apiURL, s.orgID, s.appID, w.Name)
		if t, ok := s.urlTokens[w.Name]; ok {
			u += "/" + t
		}
		if w.Secret != "" && s.secrets[w.Secret] == "" {
			s.logf("webhook %s: %s  (secret %s is NOT set in the secrets file; deliveries will not verify)", w.Name, u, w.Secret)
		} else {
			s.logf("webhook %s: %s", w.Name, u)
		}
	}
	for _, name := range m.Secrets {
		if _, ok := s.secrets[name]; !ok {
			s.logf("NEEDS_HUMAN: secret %s has no value; add %s=... to the secrets file", name, name)
		}
	}
}

func (s *stub) describe() map[string]any {
	prefix := s.apiURL + "/v1/orgs/" + s.orgID + "/apps/" + s.appID
	tok := s.serviceToken
	return map[string]any{
		"org_id": s.orgID, "app_id": s.appID, "app": s.manifest.Name,
		"edge": s.edgeURL, "internal": s.internalURL,
		"events": prefix + "/events", "approvals": prefix + "/approvals", "links": prefix + "/uploads/links", "decide": s.apiURL + "/approvals/{id}",
		"email": prefix + "/email/send", "email_sent": prefix + "/email/sent",
		"webhooks": prefix + "/webhooks/{name}/events", "domains": prefix + "/domains", "email_domains": prefix + "/email/domains", "hooks": s.apiURL + "/hooks/" + s.orgID + "/" + s.appID + "/{source}",
		"inbox":    s.describeInbox(),
		"identity": s.identity, "public_routes": s.manifest.Routes.Public, "service_routes": s.manifest.ServiceRoutes(),
		// The service token is the app's own credential; a local run may read it here to drive
		// the queue endpoint from a test script. The platform never exposes it this way.
		"service_token": tok,
	}
}

// describeInbox is the inbox of a local run, or nil when the app declares none.
func (s *stub) describeInbox() map[string]any {
	if s.manifest.Inbox == nil {
		return nil
	}
	return map[string]any{"address": s.inboxAddress(), "handler": s.manifest.Inbox.Handler, "send": s.apiURL + "/v1/stub/inbox"}
}

// appEnv is the environment the platform would inject (environment.md), for a local run.
func (s *stub) appEnv(databaseURL string) []string {
	m := s.manifest
	env := []string{
		"PORT=" + portOf(s.upstream),
		"WHISK_APP_ID=" + s.appID,
		"WHISK_APP_NAME=" + m.Name,
		"WHISK_ORG_ID=" + s.orgID,
		"WHISK_ENV=production",
		"WHISK_REGION=eu",
		"WHISK_PUBLIC_URL=" + s.edgeURL,
		"WHISK_QUEUE_URL=" + s.apiURL + "/v1/orgs/" + s.orgID + "/apps/" + s.appID + "/events",
		kv("WHISK_SERVICE_TOKEN", s.serviceToken),
		"WHISK_INNGEST_URL=" + s.apiURL,
		kv("WHISK_INNGEST_SIGNING_KEY", s.signingKey),
		kv("WHISK_INNGEST_EVENT_KEY", s.eventKey),
		kv("WHISK_DELIVERY_KEY", base64.StdEncoding.EncodeToString(s.deliveryKey)),
		"WHISK_STOP_GRACE=28",
		"TZ=UTC",
		"WHISK_DEV=1",
	}
	if m.Database != "none" && databaseURL != "" {
		env = append(env, "DATABASE_URL="+databaseURL)
	}
	env = append(env, s.extraEnv...)
	for _, k := range sortedKeys(m.Env) {
		env = append(env, k+"="+m.Env[k])
	}
	for _, name := range m.Secrets {
		if v, ok := s.secrets[name]; ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

func (s *stub) waitHealthy(ctx context.Context) {
	deadline := time.Now().Add(time.Duration(s.manifest.Health.Timeout) * time.Second)
	target := s.upstream.String() + s.manifest.Health.Path
	for ctx.Err() == nil && time.Now().Before(deadline) {
		if reachableOK(target) {
			s.logf("app healthy: GET %s is 200; open %s", s.manifest.Health.Path, s.edgeURL)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	if ctx.Err() == nil {
		s.logf("HEALTH_CHECK_FAILED: GET %s did not return 200 within %d seconds", target, s.manifest.Health.Timeout)
	}
}

func reachable(u string) bool {
	resp, err := runhttp.Client(500 * time.Millisecond).Get(u)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func reachableOK(u string) bool {
	resp, err := runhttp.Client(2 * time.Second).Get(u)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// command runs a shell line with the given environment added, prefixing its output when a
// label is given so the app's own log stays clean. It runs for as long as ctx lasts; when ctx
// ends its process group is asked to stop, and killed 30 seconds later.
func command(ctx context.Context, line string, env []string, label string, log io.Writer) *run.Cmd {
	shell, flag := "sh", "-c"
	if os.PathSeparator == '\\' {
		shell, flag = "cmd", "/C"
	}
	cmd := run.Process(ctx, shell, flag, line)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = nil
	setProcessGroup(cmd.Cmd)
	cmd.WaitDelay = 30 * time.Second
	if label == "" {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd
	}
	cmd.Stdout = prefixed(ctx, label, log)
	cmd.Stderr = prefixed(ctx, label, log)
	return cmd
}

// prefixed is a pipe whose lines are copied to log behind the label. The copy outlives ctx, so
// what the child says while it stops is kept.
func prefixed(ctx context.Context, label string, log io.Writer) *os.File {
	r, w, err := os.Pipe()
	if err != nil {
		return os.Stderr
	}
	run.Spawn(context.WithoutCancel(ctx), "stub "+label+" output", func(context.Context) error {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			fmt.Fprintf(log, "%s: %s\n", label, sc.Text())
		}
		return nil
	})
	return w
}

// readSecrets parses NAME=value lines. A missing file is fine: no secrets are set.
func readSecrets(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s: line %q is not NAME=value", filepath.Base(path), line)
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out, sc.Err()
}

// SplitList splits a comma-separated flag value, dropping blanks.
func SplitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func orULID(s string) string {
	if s == "" {
		return newULID()
	}
	return s
}

func kv(name, value string) string { return name + "=" + value }

func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	return "8080"
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
