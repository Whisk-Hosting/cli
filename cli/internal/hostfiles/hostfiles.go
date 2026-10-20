// Package hostfiles reads what an app's previous host was told about it (a Procfile, Heroku's
// app.json, render.yaml, railway.json or railway.toml, fly.toml) so whisk init can write a
// whisk.yaml that runs the same app the same way: its migrate step, its workers, its health path,
// its Dockerfile, the names of its secrets and its plain settings. Everything here is pure; the
// caller passes the file list and a reader.
package hostfiles

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
	"github.com/whisk-run/contract/manifest"
	"gopkg.in/yaml.v3"
)

// MaxWorkers is how many workers one app may declare (CONTRACT.md §3.2).
const MaxWorkers = manifest.MaxWorkers

// Worker is one long-running process beside the web process.
type Worker struct {
	Name     string
	Command  string
	Database string // pooled | direct
}

// Cron is a scheduled command the old host ran, which on Whisk becomes a function.
type Cron struct {
	Name     string
	Schedule string
	Command  string
	From     string
}

// Found is what the files said, merged in the order Files lists them.
type Found struct {
	Files      []string          // the host files read, in the order they were read
	Web        string            // the web process's start command, when a file names one
	Migrate    string            // run before traffic switches
	Workers    []Worker          // at most MaxWorkers
	Health     string            // the health check path
	Dockerfile string            // a Dockerfile path other than the default
	Secrets    []string          // names whose values a person sets
	Env        map[string]string // plain values
	Database   bool              // the old host gave it a Postgres database
	KV         bool              // the old host gave it Redis, or a worker needs it
	Crons      []Cron
	Notes      []string // what the agent still has to do by hand, one sentence each
}

// Empty reports whether no host file was found.
func (f Found) Empty() bool { return len(f.Files) == 0 }

var (
	reSecret     = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)
	reWorkerName = regexp.MustCompile(`[^a-z0-9]+`)
	// The platform sets these itself; an app never declares them.
	platformVars = map[string]bool{"PORT": true, "DATABASE_URL": true, "REDIS_URL": true, "REDIS_TLS_URL": true, "HOST": true}
)

// Read reads every host file present among files (paths relative to the app's root) and merges
// what they say. A later file fills only what an earlier one left empty, except workers, secrets
// and settings, which are unions.
func Read(files []string, read func(string) []byte) Found {
	set := map[string]bool{}
	for _, f := range files {
		set[f] = true
	}
	f := Found{Env: map[string]string{}}
	for _, src := range []struct {
		name  string
		parse func([]byte) (Found, error)
	}{
		{"Procfile", parseProcfile},
		{"app.json", parseAppJSON},
		{"render.yaml", parseRender},
		{"railway.json", parseRailwayJSON},
		{"railway.toml", parseRailwayTOML},
		{"fly.toml", parseFly},
	} {
		if !set[src.name] {
			continue
		}
		got, err := src.parse(read(src.name))
		f.Files = append(f.Files, src.name)
		if err != nil {
			f.Notes = append(f.Notes, fmt.Sprintf("%s could not be read (%v); carry what it sets across by hand.", src.name, err))
			continue
		}
		f = merge(f, got)
	}
	return finish(f, deps(set, read))
}

func merge(a, b Found) Found {
	first := func(x, y string) string {
		if x != "" {
			return x
		}
		return y
	}
	a.Web = first(a.Web, b.Web)
	a.Migrate = first(a.Migrate, b.Migrate)
	a.Health = first(a.Health, b.Health)
	a.Dockerfile = first(a.Dockerfile, b.Dockerfile)
	a.Database = a.Database || b.Database
	a.KV = a.KV || b.KV
	for _, w := range b.Workers {
		if !hasWorker(a.Workers, w) {
			a.Workers = append(a.Workers, w)
		}
	}
	a.Secrets = append(a.Secrets, b.Secrets...)
	for k, v := range b.Env {
		if _, ok := a.Env[k]; !ok {
			a.Env[k] = v
		}
	}
	a.Crons = append(a.Crons, b.Crons...)
	a.Notes = append(a.Notes, b.Notes...)
	return a
}

func hasWorker(ws []Worker, w Worker) bool {
	for _, x := range ws {
		if x.Name == w.Name || x.Command == w.Command {
			return true
		}
	}
	return false
}

// finish names and sizes the workers, picks each one's database connection from the queue
// library it uses, and tidies the names.
func finish(f Found, d string) Found {
	seen := map[string]bool{}
	var ws []Worker
	for _, w := range f.Workers {
		w.Name = workerName(w.Name, seen)
		w.Database = manifest.WorkerPooled
		switch queueKind(w.Command, d) {
		case "postgres":
			w.Database = manifest.WorkerDirect
		case "redis":
			f.KV = true
		}
		ws = append(ws, w)
	}
	if len(ws) > MaxWorkers {
		for _, w := range ws[MaxWorkers:] {
			f.Notes = append(f.Notes, fmt.Sprintf("Worker %s (`%s`) is past the %d an app may run; fold it into another worker or rewrite it as functions.", w.Name, w.Command, MaxWorkers))
		}
		ws = ws[:MaxWorkers]
	}
	f.Workers = ws
	for _, c := range f.Crons {
		f.Notes = append(f.Notes, fmt.Sprintf("%s runs `%s` on the schedule `%s`; make it a function with that cron, calling the same code.", c.From, c.Command, c.Schedule))
	}
	f.Secrets = names(f.Secrets, f.Env)
	for k, v := range f.Env {
		switch {
		case platformVars[k] || strings.HasPrefix(k, "WHISK_") || !reSecret.MatchString(k):
			delete(f.Env, k)
		case !fits(v, maxEnv):
			delete(f.Env, k)
			f.Notes = append(f.Notes, fmt.Sprintf("The setting %s is too long for env: or holds characters it cannot; set it by hand.", k))
		}
	}
	if !fits(f.Migrate, maxCommand) {
		f.Migrate = ""
		f.Notes = append(f.Notes, "The release command is too long for migrate: or holds characters it cannot; write migrate: by hand.")
	}
	var kept []Worker
	for _, w := range f.Workers {
		if fits(w.Command, maxCommand) {
			kept = append(kept, w)
		} else {
			f.Notes = append(f.Notes, fmt.Sprintf("Worker %s's command is too long or holds characters whisk.yaml cannot; write it by hand.", w.Name))
		}
	}
	f.Workers = kept
	if !fits(f.Dockerfile, maxPath) {
		f.Dockerfile = ""
	}
	if !strings.HasPrefix(f.Health, "/") || !fits(f.Health, maxPath) {
		f.Health = ""
	}
	if f.Web != "" {
		f.Notes = append(f.Notes, fmt.Sprintf("The web process started with `%s`; make that the image's start command (the Dockerfile's CMD, or the start script Railpack finds), listening on PORT.", f.Web))
	}
	return f
}

// The longest values whisk.yaml takes (contract/whisk.schema.json).
const (
	maxCommand = 500
	maxEnv     = 4096
	maxPath    = 200
)

// fits reports whether a value can go into whisk.yaml as it is: valid UTF-8, no control
// characters but tab, and no longer than max bytes.
func fits(v string, max int) bool {
	if len(v) > max || !utf8.ValidString(v) {
		return false
	}
	return strings.IndexFunc(v, func(r rune) bool { return r != '\t' && unicode.IsControl(r) }) < 0
}

// Clean makes text safe for a YAML comment or a terminal line: valid UTF-8, with every control
// character, line or paragraph separator (each ends a YAML comment) and character YAML refuses
// in a stream (U+FFFE, U+FFFF) a space.
func Clean(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp) || r == 0xFFFE || r == 0xFFFF {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(text, "?"))
}

// workerName makes a worker name CONTRACT.md §3.2 allows (^[a-z]([a-z0-9-]{0,28}[a-z0-9])?$),
// unique among those seen.
func workerName(raw string, seen map[string]bool) string {
	n := strings.Trim(reWorkerName.ReplaceAllString(strings.ToLower(raw), "-"), "-")
	if n == "" || n[0] < 'a' || n[0] > 'z' {
		n = "worker-" + n
		n = strings.TrimSuffix(n, "-")
	}
	if len(n) > 30 {
		n = strings.TrimRight(n[:30], "-")
	}
	base := n
	for i := 2; seen[n]; i++ {
		suffix := fmt.Sprintf("-%d", i)
		if len(base)+len(suffix) > 30 {
			base = strings.TrimRight(base[:30-len(suffix)], "-")
		}
		n = base + suffix
	}
	seen[n] = true
	return n
}

// queueKind says what a worker's job queue keeps its jobs in: "postgres" for the queues that need
// LISTEN, advisory locks or a session (so a direct connection), "redis" for the ones that need a
// Redis (so kv), "" for the rest, which run on the pooled connection. The command decides first,
// then the app's dependencies.
func queueKind(command, deps string) string {
	for _, text := range []string{command, deps} {
		t := strings.ToLower(text)
		switch {
		case rePostgresQueue.MatchString(t):
			return "postgres"
		case reRedisQueue.MatchString(t):
			return "redis"
		}
	}
	return ""
}

// A library's name as a whole word, so "que" never matches "unique".
var (
	rePostgresQueue = word("graphile-worker", "graphile_worker", "pg-boss", "good_job", "goodjob", "oban", "procrastinate", "riverqueue", "que", "pgqueuer")
	reRedisQueue    = word("sidekiq", "celery", "resque", "bullmq", "bull", "rq", "dramatiq", "arq", "huey", "asynq", "machinery")
)

func word(names ...string) *regexp.Regexp {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`(^|[^a-z0-9_-])(` + strings.Join(quoted, "|") + `)($|[^a-z0-9_-])`)
}

// deps is the app's dependency files' text, for queueKind to read beside each command.
func deps(set map[string]bool, read func(string) []byte) string {
	var b strings.Builder
	for _, name := range []string{"package.json", "requirements.txt", "pyproject.toml", "Gemfile", "Gemfile.lock", "mix.exs", "go.mod"} {
		if set[name] {
			b.Write(read(name))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// names is the sorted, unique secret names, without the platform's own and without any name
// that also has a plain value.
func names(list []string, env map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range list {
		if _, plain := env[n]; plain || seen[n] || platformVars[n] || strings.HasPrefix(n, "WHISK_") || !reSecret.MatchString(n) {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ---- Procfile -------------------------------------------------------------------------------

func parseProcfile(src []byte) (Found, error) {
	f := Found{Env: map[string]string{}}
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, cmd, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name, cmd = strings.TrimSpace(name), strings.TrimSpace(cmd)
		switch name {
		case "web":
			f.Web = cmd
		case "release":
			f.Migrate = cmd
		default:
			f.Workers = append(f.Workers, Worker{Name: name, Command: cmd})
		}
	}
	return f, nil
}

// ---- Heroku app.json ------------------------------------------------------------------------

func parseAppJSON(src []byte) (Found, error) {
	var a struct {
		Env     map[string]json.RawMessage `json:"env"`
		Addons  []json.RawMessage          `json:"addons"`
		Scripts struct {
			Postdeploy string `json:"postdeploy"`
		} `json:"scripts"`
	}
	if err := json.Unmarshal(src, &a); err != nil {
		return Found{}, err
	}
	f := Found{Env: map[string]string{}}
	for k, raw := range a.Env {
		var plain string
		if json.Unmarshal(raw, &plain) == nil {
			f.Env[k] = plain
			continue
		}
		var v struct {
			Value     *string `json:"value"`
			Generator string  `json:"generator"`
		}
		_ = json.Unmarshal(raw, &v)
		if v.Value != nil && v.Generator == "" {
			f.Env[k] = *v.Value
		} else {
			f.Secrets = append(f.Secrets, k)
		}
	}
	for _, raw := range a.Addons {
		var plan string
		if json.Unmarshal(raw, &plan) != nil {
			var o struct {
				Plan string `json:"plan"`
			}
			_ = json.Unmarshal(raw, &o)
			plan = o.Plan
		}
		switch {
		case strings.HasPrefix(plan, "heroku-postgresql"):
			f.Database = true
		case strings.HasPrefix(plan, "heroku-redis"), strings.HasPrefix(plan, "heroku-key-value"):
			f.KV = true
		case plan != "":
			f.Notes = append(f.Notes, fmt.Sprintf("app.json adds the add-on %s; find what Whisk has built in for it, or keep it as an outside service and add its key as a secret.", plan))
		}
	}
	return f, nil
}

// ---- render.yaml ----------------------------------------------------------------------------

type renderEnvVar struct {
	Key           string  `yaml:"key"`
	Value         *string `yaml:"value"`
	Sync          *bool   `yaml:"sync"`
	GenerateValue any     `yaml:"generateValue"`
	FromDatabase  any     `yaml:"fromDatabase"`
	FromService   *struct {
		Type string `yaml:"type"`
	} `yaml:"fromService"`
	FromGroup string `yaml:"fromGroup"`
}

type renderService struct {
	Type             string         `yaml:"type"`
	Name             string         `yaml:"name"`
	StartCommand     string         `yaml:"startCommand"`
	PreDeployCommand string         `yaml:"preDeployCommand"`
	HealthCheckPath  string         `yaml:"healthCheckPath"`
	DockerfilePath   string         `yaml:"dockerfilePath"`
	Schedule         string         `yaml:"schedule"`
	Disk             any            `yaml:"disk"`
	EnvVars          []renderEnvVar `yaml:"envVars"`
}

func parseRender(src []byte) (Found, error) {
	var r struct {
		Services  []renderService  `yaml:"services"`
		Databases []map[string]any `yaml:"databases"`
	}
	if err := yaml.Unmarshal(src, &r); err != nil {
		return Found{}, err
	}
	f := Found{Env: map[string]string{}, Database: len(r.Databases) > 0}
	webs := 0
	for _, s := range r.Services {
		switch s.Type {
		case "web", "pserv":
			webs++
			if webs > 1 {
				f.Notes = append(f.Notes, fmt.Sprintf("render.yaml has another web service, %s; on Whisk it is an app of its own, with its own whisk.yaml.", s.Name))
				continue
			}
			f.Web, f.Migrate, f.Health, f.Dockerfile = s.StartCommand, s.PreDeployCommand, s.HealthCheckPath, s.DockerfilePath
			if s.Disk != nil {
				f.Notes = append(f.Notes, fmt.Sprintf("%s has a persistent disk; Whisk apps have none, so files it keeps there move to storage (storage: true).", s.Name))
			}
		case "worker", "background_worker":
			f.Workers = append(f.Workers, Worker{Name: s.Name, Command: s.StartCommand})
		case "cron":
			f.Crons = append(f.Crons, Cron{Name: s.Name, Schedule: s.Schedule, Command: s.StartCommand, From: "Render's cron job " + s.Name})
		case "redis", "keyvalue":
			f.KV = true
			continue
		}
		for _, v := range s.EnvVars {
			switch {
			case v.FromDatabase != nil:
				f.Database = true
			case v.FromService != nil:
				if v.FromService.Type == "redis" || v.FromService.Type == "keyvalue" {
					f.KV = true
				}
			case v.FromGroup != "":
				f.Notes = append(f.Notes, fmt.Sprintf("render.yaml reads settings from the environment group %s; list its secret names under secrets: and its plain values under env:.", v.FromGroup))
			case v.Value != nil:
				f.Env[v.Key] = *v.Value
			case v.Key != "":
				f.Secrets = append(f.Secrets, v.Key)
			}
		}
	}
	return f, nil
}

// ---- railway.json and railway.toml ----------------------------------------------------------

type railway struct {
	Build struct {
		DockerfilePath string `json:"dockerfilePath" toml:"dockerfilePath"`
	} `json:"build" toml:"build"`
	Deploy struct {
		StartCommand     string `json:"startCommand" toml:"startCommand"`
		PreDeployCommand any    `json:"preDeployCommand" toml:"preDeployCommand"`
		HealthcheckPath  string `json:"healthcheckPath" toml:"healthcheckPath"`
		CronSchedule     string `json:"cronSchedule" toml:"cronSchedule"`
	} `json:"deploy" toml:"deploy"`
}

func parseRailwayJSON(src []byte) (Found, error) {
	var r railway
	if err := json.Unmarshal(src, &r); err != nil {
		return Found{}, err
	}
	return fromRailway(r), nil
}

func parseRailwayTOML(src []byte) (Found, error) {
	var r railway
	if err := toml.Unmarshal(src, &r); err != nil {
		return Found{}, err
	}
	return fromRailway(r), nil
}

func fromRailway(r railway) Found {
	f := Found{Env: map[string]string{}, Dockerfile: r.Build.DockerfilePath, Health: r.Deploy.HealthcheckPath}
	switch c := r.Deploy.PreDeployCommand.(type) {
	case string:
		f.Migrate = c
	case []any:
		var parts []string
		for _, p := range c {
			if s, ok := p.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		f.Migrate = strings.Join(parts, " && ")
	}
	if r.Deploy.CronSchedule != "" {
		f.Crons = append(f.Crons, Cron{Schedule: r.Deploy.CronSchedule, Command: r.Deploy.StartCommand, From: "Railway's cron service"})
	} else {
		f.Web = r.Deploy.StartCommand
	}
	return f
}

// ---- fly.toml -------------------------------------------------------------------------------

func parseFly(src []byte) (Found, error) {
	var c struct {
		Build struct {
			Dockerfile string `toml:"dockerfile"`
		} `toml:"build"`
		Deploy struct {
			ReleaseCommand string `toml:"release_command"`
		} `toml:"deploy"`
		Env         map[string]any    `toml:"env"`
		Processes   map[string]string `toml:"processes"`
		Mounts      any               `toml:"mounts"`
		HTTPService *struct {
			Processes []string `toml:"processes"`
			Checks    []struct {
				Path string `toml:"path"`
			} `toml:"checks"`
		} `toml:"http_service"`
		Checks map[string]struct {
			Type string `toml:"type"`
			Path string `toml:"path"`
		} `toml:"checks"`
	}
	if err := toml.Unmarshal(src, &c); err != nil {
		return Found{}, err
	}
	f := Found{Env: map[string]string{}, Dockerfile: c.Build.Dockerfile, Migrate: c.Deploy.ReleaseCommand}
	for k, v := range c.Env {
		f.Env[k] = fmt.Sprint(v)
	}
	web := map[string]bool{"app": true}
	if c.HTTPService != nil {
		if len(c.HTTPService.Processes) > 0 {
			web = map[string]bool{}
			for _, p := range c.HTTPService.Processes {
				web[p] = true
			}
		}
		for _, ch := range c.HTTPService.Checks {
			if ch.Path != "" && f.Health == "" {
				f.Health = ch.Path
			}
		}
	}
	for _, name := range sortedKeys(c.Checks) {
		if ch := c.Checks[name]; ch.Type == "http" && ch.Path != "" && f.Health == "" {
			f.Health = ch.Path
		}
	}
	for _, name := range sortedKeys(c.Processes) {
		if web[name] {
			if f.Web == "" {
				f.Web = c.Processes[name]
			}
			continue
		}
		f.Workers = append(f.Workers, Worker{Name: name, Command: c.Processes[name]})
	}
	if c.Mounts != nil {
		f.Notes = append(f.Notes, "fly.toml mounts a volume; Whisk apps have no disk, so files kept there move to storage (storage: true).")
	}
	return f, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
