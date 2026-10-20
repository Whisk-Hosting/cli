package hostfiles

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func readFrom(files map[string]string) ([]string, func(string) []byte) {
	var list []string
	for name := range files {
		list = append(list, name)
	}
	return list, func(p string) []byte { return []byte(files[p]) }
}

func TestRead(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		check func(t *testing.T, f Found)
	}{
		{
			name:  "no host files",
			files: map[string]string{"package.json": `{}`},
			check: func(t *testing.T, f Found) {
				if !f.Empty() || len(f.Workers) != 0 || len(f.Notes) != 0 {
					t.Errorf("found something: %+v", f)
				}
			},
		},
		{
			name: "Procfile with a Postgres queue",
			files: map[string]string{
				"Procfile":     "web: node dist/server.js\nrelease: node dist/migrate.js\nworker: node dist/worker.js\n# a comment\n",
				"package.json": `{"dependencies":{"graphile-worker":"^0.16.0","hono":"^4"}}`,
			},
			check: func(t *testing.T, f Found) {
				want := []Worker{{Name: "worker", Command: "node dist/worker.js", Database: "direct"}}
				if f.Migrate != "node dist/migrate.js" || f.Web != "node dist/server.js" || !reflect.DeepEqual(f.Workers, want) || f.KV {
					t.Errorf("got %+v", f)
				}
			},
		},
		{
			name: "Sidekiq needs kv and stays pooled",
			files: map[string]string{
				"Procfile": "web: bundle exec puma -C config/puma.rb\nworker: bundle exec sidekiq -C config/sidekiq.yml\n",
				"Gemfile":  "gem \"rails\"\ngem \"sidekiq\"\n",
			},
			check: func(t *testing.T, f Found) {
				if len(f.Workers) != 1 || f.Workers[0].Database != "pooled" || !f.KV {
					t.Errorf("got %+v", f)
				}
			},
		},
		{
			name: "que never matches unique",
			files: map[string]string{
				"Procfile":     "jobs: node jobs.js\n",
				"package.json": `{"dependencies":{"unique-names-generator":"1"}}`,
			},
			check: func(t *testing.T, f Found) {
				if f.Workers[0].Database != "pooled" || f.KV {
					t.Errorf("got %+v", f.Workers)
				}
			},
		},
		{
			name:  "more workers than an app may run",
			files: map[string]string{"Procfile": manyWorkers(MaxWorkers + 1)},
			check: func(t *testing.T, f Found) {
				if len(f.Workers) != MaxWorkers || len(f.Notes) != 1 || !strings.Contains(f.Notes[0], fmt.Sprintf("`run %d`", MaxWorkers)) {
					t.Errorf("got %+v", f)
				}
			},
		},
		{
			name: "worker names are made valid and unique",
			files: map[string]string{
				"Procfile": "Worker_Imports: x\n1st: y\nworker-imports: z\n",
			},
			check: func(t *testing.T, f Found) {
				got := []string{f.Workers[0].Name, f.Workers[1].Name, f.Workers[2].Name}
				want := []string{"worker-imports", "worker-1st", "worker-imports-2"}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("names %v, want %v", got, want)
				}
			},
		},
		{
			name: "Heroku app.json",
			files: map[string]string{
				"app.json": `{"env":{"STRIPE_SECRET_KEY":{"required":true},"SESSION_SECRET":{"generator":"secret"},"LOG_LEVEL":{"value":"info"},"NODE_ENV":"production","DATABASE_URL":{"required":true}},
				"addons":["heroku-postgresql:essential-0",{"plan":"heroku-redis:mini"},"papertrail"]}`,
			},
			check: func(t *testing.T, f Found) {
				if !reflect.DeepEqual(f.Secrets, []string{"SESSION_SECRET", "STRIPE_SECRET_KEY"}) {
					t.Errorf("secrets %v", f.Secrets)
				}
				if !reflect.DeepEqual(f.Env, map[string]string{"LOG_LEVEL": "info", "NODE_ENV": "production"}) {
					t.Errorf("env %v", f.Env)
				}
				if !f.Database || !f.KV || len(f.Notes) != 1 || !strings.Contains(f.Notes[0], "papertrail") {
					t.Errorf("got %+v", f)
				}
			},
		},
		{
			name: "render.yaml",
			files: map[string]string{
				"render.yaml": `
services:
  - type: web
    name: api
    runtime: docker
    dockerfilePath: ./docker/api.Dockerfile
    preDeployCommand: npm run migrate
    startCommand: node dist/server.js
    healthCheckPath: /healthz
    disk: {name: data, mountPath: /data, sizeGB: 1}
    envVars:
      - key: DATABASE_URL
        fromDatabase: {name: db, property: connectionString}
      - key: REDIS_URL
        fromService: {type: redis, name: cache, property: connectionString}
      - key: STRIPE_SECRET_KEY
        sync: false
      - key: JWT_SECRET
        generateValue: true
      - key: APP_ENV
        value: production
      - fromGroup: shared
  - type: worker
    name: jobs
    startCommand: node dist/worker.js
  - type: cron
    name: nightly
    schedule: "0 3 * * *"
    startCommand: node dist/nightly.js
  - type: web
    name: marketing
    startCommand: npm start
databases:
  - name: db
`,
			},
			check: func(t *testing.T, f Found) {
				if f.Dockerfile != "./docker/api.Dockerfile" || f.Migrate != "npm run migrate" || f.Health != "/healthz" || !f.Database || !f.KV {
					t.Errorf("got %+v", f)
				}
				if !reflect.DeepEqual(f.Secrets, []string{"JWT_SECRET", "STRIPE_SECRET_KEY"}) || f.Env["APP_ENV"] != "production" {
					t.Errorf("secrets %v env %v", f.Secrets, f.Env)
				}
				if len(f.Workers) != 1 || f.Workers[0].Name != "jobs" {
					t.Errorf("workers %+v", f.Workers)
				}
				notes := strings.Join(f.Notes, "\n")
				for _, want := range []string{"persistent disk", "environment group shared", "marketing", "0 3 * * *", "node dist/server.js"} {
					if !strings.Contains(notes, want) {
						t.Errorf("notes miss %q:\n%s", want, notes)
					}
				}
			},
		},
		{
			name: "railway.json with a list of pre-deploy commands",
			files: map[string]string{
				"railway.json": `{"build":{"dockerfilePath":"Dockerfile.prod"},"deploy":{"startCommand":"python -m app","preDeployCommand":["alembic upgrade head","python seed.py"],"healthcheckPath":"/health"}}`,
			},
			check: func(t *testing.T, f Found) {
				if f.Migrate != "alembic upgrade head && python seed.py" || f.Dockerfile != "Dockerfile.prod" || f.Health != "/health" || f.Web != "python -m app" {
					t.Errorf("got %+v", f)
				}
			},
		},
		{
			name: "railway.toml cron service",
			files: map[string]string{
				"railway.toml": "[deploy]\nstartCommand = \"python report.py\"\ncronSchedule = \"*/15 * * * *\"\n",
			},
			check: func(t *testing.T, f Found) {
				if f.Web != "" || len(f.Crons) != 1 || !strings.Contains(strings.Join(f.Notes, " "), "*/15 * * * *") {
					t.Errorf("got %+v", f)
				}
			},
		},
		{
			name: "fly.toml processes",
			files: map[string]string{
				"fly.toml": `
app = "track"
[build]
  dockerfile = "Dockerfile.fly"
[deploy]
  release_command = "bin/rails db:migrate"
[env]
  RAILS_ENV = "production"
  PORT = "8080"
  WORKERS = 2
[processes]
  web = "bin/rails server"
  jobs = "bundle exec good_job start"
[http_service]
  internal_port = 8080
  processes = ["web"]
  [[http_service.checks]]
    path = "/up"
[mounts]
  source = "data"
  destination = "/data"
`,
			},
			check: func(t *testing.T, f Found) {
				if f.Dockerfile != "Dockerfile.fly" || f.Migrate != "bin/rails db:migrate" || f.Health != "/up" || f.Web != "bin/rails server" {
					t.Errorf("got %+v", f)
				}
				want := []Worker{{Name: "jobs", Command: "bundle exec good_job start", Database: "direct"}}
				if !reflect.DeepEqual(f.Workers, want) {
					t.Errorf("workers %+v", f.Workers)
				}
				if !reflect.DeepEqual(f.Env, map[string]string{"RAILS_ENV": "production", "WORKERS": "2"}) {
					t.Errorf("env %v", f.Env)
				}
				if !strings.Contains(strings.Join(f.Notes, " "), "volume") {
					t.Errorf("notes %v", f.Notes)
				}
			},
		},
		{
			name: "Procfile wins over fly.toml, workers merge",
			files: map[string]string{
				"Procfile": "web: a\nrelease: m1\nworker: w\n",
				"fly.toml": "[deploy]\nrelease_command = \"m2\"\n[processes]\napp = \"b\"\nworker = \"w\"\nclock = \"c\"\n",
			},
			check: func(t *testing.T, f Found) {
				if f.Web != "a" || f.Migrate != "m1" || len(f.Workers) != 2 || !reflect.DeepEqual(f.Files, []string{"Procfile", "fly.toml"}) {
					t.Errorf("got %+v", f)
				}
			},
		},
		{
			name:  "a file that cannot be read is named",
			files: map[string]string{"fly.toml": "[[[ not toml"},
			check: func(t *testing.T, f Found) {
				if len(f.Files) != 1 || len(f.Notes) != 1 || !strings.Contains(f.Notes[0], "fly.toml could not be read") {
					t.Errorf("got %+v", f)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			list, read := readFrom(c.files)
			c.check(t, Read(list, read))
		})
	}
}

func manyWorkers(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "w%d: run %d\n", i, i)
	}
	return b.String()
}
