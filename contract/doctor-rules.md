# Doctor rules

`whisk doctor` checks a repository against the conventions before anything is pushed. The
manifest rules (`W00x`, `W04x`, `W05x`) also run in the git pre-receive hook, so a push cannot
bypass them. Every finding has a stable rule ID, a level, a file and line where one applies, a
message, and a fix. Errors exit 3; warnings exit 0.

Output shape, one entry per finding:

```json
{"rule":"W030","level":"warning","file":"src/xero.ts","line":8,"message":"XERO_TENANT_ID is read from the environment but not declared.","fix":"Add XERO_TENANT_ID under secrets or env in whisk.yaml."}
```

`whisk doctor --fix` applies the fixes marked safe below and reports what it changed.

A warning reported at a line of code can be kept out of the report on purpose with a comment
`doctor: allow <rule>` on that line or the line above it, saying why (`// doctor: allow W091 the
edge test sets a parent-domain cookie on purpose`); doctor lists it under `skipped` instead.
Errors cannot be allowed.

Detection is by convention, not by executing the app: string and pattern search over tracked
files, per language. "Where detectable" means the check is skipped, not failed, when the
language or framework is not one doctor understands. The idioms doctor recognises are listed
under each rule so an agent can write code doctor will read correctly.

## W001

Level: error
Check: whisk.yaml is missing from the repository root or does not parse as YAML.
Fix: Run whisk init to write one, or fix the YAML syntax error at the line reported.
Safe fix: no

Details: the file must be exactly `whisk.yaml` at the root. `whisk.yml` and nested copies are
reported by name so the mistake is obvious. Doctor reads every file as text the way Windows
tools write it too: a UTF-8 byte-order mark is dropped and UTF-16 with a byte-order mark (what
Windows PowerShell's `>` writes) is read as the text it is. `whisk.yaml` itself must be UTF-8,
which is what the platform reads: one saved as UTF-16 is reported with how to save it as UTF-8,
and one that is not text at all is reported as such, not as missing.

## W002

Level: error
Check: whisk.yaml fails the schema for its conventions version or breaks a manifest rule.
Fix: Fix each listed problem; whisk schema prints the schema with a description for every field.
Safe fix: no

Details: one finding per problem, with the JSON path. Beyond the schema, the manifest rules are:
every `routes.challenge` entry must also match `routes.public`; function names are unique;
webhook names are unique; no name under `secrets`, `env` or `build.secrets` begins with
`WHISK_` or is one of `PORT`, `DATABASE_URL`, `SENTRY_DSN`, `TZ`; `functions[].tz` is a known
IANA zone; `functions[].cron` has five fields with valid ranges; `webhooks[].preset` is a known
preset, `hmac` or `token`. Unknown keys are reported as `MANIFEST_UNKNOWN_KEY` with the nearest
known key.

## W003

Level: error
Check: name in whisk.yaml differs from the app slug in .whisk/app.json.
Fix: Set name to the slug in .whisk/app.json, or run whisk use <org>/<app> to rebind the directory.
Safe fix: no

Details: skipped when `.whisk/app.json` does not exist yet, because `whisk init` writes both.

## W004

Level: warning
Check: a static folder in whisk.yaml holds no file the commit will carry.
Fix: Commit the folder (take it out of .gitignore), or serve build output from the app instead of static; static files are read from the commit, not from the built image.
Safe fix: no

Details: for each `static[].dir`, in a git repository at least one file under it must be
tracked; outside one, at least one file under it must not be git-ignored. The message says
whether the folder does not exist, is empty or ignored, or holds only files git does not track.
Build output such as `dist/` is normally git-ignored, so a `static` entry pointing into it fails
the deploy with `MANIFEST_INVALID` naming the folder.

## W005

Level: warning
Check: health.timeout is set above 60 seconds and the app is not always_on.
Fix: Make the app answer its health check within 60 seconds (doctor W100 to W104 name the usual causes), or set always_on: true so it never sleeps.
Safe fix: no

Details: a deploy waits `health.timeout` seconds for the health check, but a wake waits 60
(NODE-AGENT.md §A8): an app that needs longer deploys, then fails every wake after it sleeps
with `WAKE_TIMEOUT`. Only a timeout written in whisk.yaml is read; the default (90) is not
reported, since most apps answer in a few seconds.

## W006

Level: warning
Check: .whisk/app.json is tracked by git and the remote whisk points at another app's repository than the one it binds.
Fix: If the binding is right, run whisk use <org>/<app> to confirm it, which points the remote whisk at that app; otherwise run whisk use with the app this checkout pushes to.
Safe fix: no

Details: a binding can arrive with the repository, while the remote `whisk` is what this
machine cloned from or last deployed to, so a disagreement means the binding was not chosen
here. The bound app's `git_url` comes from the platform, so the check runs only when the
directory is bound, a token is available and a remote `whisk` exists; offline it is skipped.
`whisk deploy` refuses the same disagreement with `INVALID_REQUEST` unless `--org` and `--app`
name the target.

## W010

Level: error
Check: a tracked file contains a value matching a secret rule.
Fix: Remove the value, declare a name under secrets, read it from the environment, and rotate the leaked value with the provider.
Safe fix: no

Details: the same gitleaks rule set the pre-receive hook uses, so a repository that passes doctor
passes the push. The finding names the rule, file and line; the value itself is never printed.

## W011

Level: error
Check: a .env or .env.* file, or a file under .whisk/dev/, is tracked by git.
Fix: Remove it from the index with git rm --cached (git rm -r --cached .whisk/dev for the folder) and add .env*, .whisk/dev/ to .gitignore.
Safe fix: yes, the .gitignore entries; the git rm is reported, not performed

Details: `.env.example` with no values is allowed when every line is `NAME=` or a comment.
`.whisk/dev/` is `whisk dev`'s local folder; its `secrets.env` holds secret values. Outside a
git repository a file counts as tracked when `.gitignore` does not exclude it. The fix adds every
missing entry of `.whisk/dev/`, `.env`, `.env.*` and `!.env.example`.

## W020

Level: warning
Check: PORT is not read from the environment.
Fix: Listen on the port in the PORT environment variable, on all interfaces.
Safe fix: no

Details: idioms recognised. TypeScript and JavaScript: `process.env.PORT`, `Bun.env.PORT`,
`Deno.env.get("PORT")`. Python: `os.environ["PORT"]`, `os.environ.get("PORT")`,
`os.getenv("PORT")`, and a `uvicorn`/`gunicorn` command in the Dockerfile that uses `$PORT`.
Go: `os.Getenv("PORT")`, `os.LookupEnv("PORT")`. Any language: a helper whose name contains
`env` called with the name as its first literal argument (`env("PORT")`, `mustEnv("PORT")`), a
Dockerfile `CMD` or `ENTRYPOINT` that references `$PORT`, or a `package.json` script that does.
A literal `8080` with none of these is the warning; listening on `127.0.0.1` or `localhost` is
a second finding on the same rule.

## W021

Level: warning
Check: the code writes files outside /tmp.
Fix: Write temporary files under /tmp and durable files to storage.
Safe fix: no

Details: idioms flagged. Paths `./data`, `./uploads`, `./tmp`, `./cache`, `./logs` or `./db` in a
write call: `fs.writeFile`, `fs.mkdir`, `open(..., "w")`, `os.WriteFile`, `os.MkdirAll`,
`sqlite` file paths. Writes under `/tmp` and reads of any path are fine.

## W022

Level: warning
Check: the health path is not found as a route in code.
Fix: Add a route for the health path that returns 200 when the app can serve.
Safe fix: yes, when a `/health` route exists in code: `health.path` in whisk.yaml is set to `/health`

Details: the string of `health.path` (default `/health`) must appear as a route literal:
`app.get("/health"`, `@app.get("/health")`, `r.Get("/health"`, `HandleFunc("/health"`,
`HandleFunc("GET /health"`, a plain `node:http` comparison (`req.url === "/health"`,
`pathname == "/health"`, `case "/health":`), or a file-system route (`app/health/route.ts`,
`pages/api/health.ts`). When the code declares no route in any of these forms (a route table
held in an object, for instance) the check is skipped, and the skip names the route it could not
confirm and the forms doctor reads.

## W023

Level: warning
Check: the app runs a timer or scheduler inside itself.
Fix: Declare a function with cron in whisk.yaml and move the work into it; the platform runs it on schedule, once, whether the app is awake or not.
Safe fix: no

Details: an app sleeps when idle and may run in more than one container, so a timer inside it
stops when it sleeps and runs once per container while it is awake. Matched in server code (not
in a `static` folder or a `public/` or `client/` folder, which run in the browser): JavaScript
`setInterval(` and imports of `node-cron`, `cron`, `node-schedule`, `toad-scheduler`, `agenda`,
`bree` and `croner`; Python imports of `apscheduler`, `schedule`, `crontab` and `rocketry`, and
`threading.Timer(`; Go imports of `github.com/robfig/cron` and `github.com/go-co-op/gocron`, and
`time.NewTicker(` and `time.Tick(`. One finding per file, at its first match.

## W024

Level: warning
Check: the app keeps session state on the database connection: LISTEN, an advisory lock, or SET outside a transaction.
Fix: Use the transaction forms (pg_advisory_xact_lock, SET LOCAL inside the transaction) or an event to the app instead of LISTEN; DATABASE_URL is a transaction pool.
Safe fix: no

Details: the pooled connection hands each transaction whatever server connection is free, so
state set on one is gone, or on someone else's, by the next statement. Matched in server code
other than migrations (a path containing `migrat`, or `alembic/`, which run on the direct
connection): a string starting with `LISTEN `; `pg_advisory_lock(`, `pg_try_advisory_lock(` and
their `_shared` forms; and a string starting with `SET` (or `SET SESSION`) for `search_path`,
`role`, `statement_timeout`, `timezone`, `time zone`, `application_name`, `lock_timeout` or a
dotted custom setting. `SET LOCAL` and `pg_advisory_xact_lock` are fine and not matched. One
finding per file, at its first match.

## W030

Level: warning
Check: an environment name is read in code but not declared in secrets, env or the platform set.
Fix: Add the name under secrets (values a human sets) or env (plain configuration) in whisk.yaml.
Safe fix: no

Details: reads recognised: `process.env.NAME`, `process.env["NAME"]`, `Bun.env.NAME`,
`Deno.env.get("NAME")`, `os.environ["NAME"]`, `os.environ.get("NAME")`, `os.getenv("NAME")`,
`os.Getenv("NAME")`, `os.LookupEnv("NAME")`, and a helper whose name contains `env` called with
the name as its first literal argument (`env("NAME")`, `mustEnv("NAME")`). Test files
(including fuzz targets under a `fuzz/` folder) and vendored directories are not read. The platform set is every name in `environment.md` plus
`WHISK_*` (reserved for the platform, so a key newer than the CLI is never reported),
`INNGEST_*`, `NODE_ENV`, `PYTHONPATH`, `HOME`, `PATH`. Names read with a default
value (`?? "x"`, `.get("NAME", "x")`) are still reported, at the same level.

## W031

Level: warning
Check: a declared secret is never referenced in code.
Fix: Remove the name from secrets, or read it where it is needed.
Safe fix: no

Details: uses the same read idioms as W030, and also counts a secret as read when its name is
a whole string literal anywhere in code (`"HUBSPOT_API_KEY"`, `'HUBSPOT_API_KEY'`), which covers
reading secrets through a table of names (`env[KEYS.hubspot]`). A secret referenced only in
`webhooks[].secret` or `build.secrets` is not unused.

## W040

Level: error
Check: a functions[].graph file is missing or does not validate against graph.schema.json.
Fix: Create the graph file with one step per step name used in the function, or fix the listed problem.
Safe fix: yes, a scaffold graph is written from the step names found in code when the file is missing

Details: the graph rules beyond the schema: step ids are unique; every `next` and every branch
target names a step in the file or `end`; every step is reachable from the first step; a decision
node (`?`) has at least two branches.

## W041

Level: warning
Check: a graph step id is not found as a string literal in code.
Fix: Name the step in code exactly as the graph id, or remove the id from the graph.
Safe fix: no

Details: decision nodes (ids ending `?`) are exempt. A step of kind `approval` is satisfied by
the id alone; the `/request` step the approval helper creates is not expected in the graph. The
literal must appear in the file that defines the function or a file it imports; an id that only
appears in a comment does not count.

## W042

Level: warning
Check: a step name used in code is not present in the graph.
Fix: Add the step to the graph where it runs, or rename it to a declared id.
Safe fix: no

Details: step calls recognised: `step.run("name"`, `step.sleep("name"`, `step.sleepUntil("name"`,
`step.waitForEvent("name"`, `step.sendEvent("name"`, `step.approval("name"`, and their Python
(`step.run("name"`, `step.wait_for_event("name"`, `step.send_event("name"`, `step.sleep("name"`)
and Go (`step.Run(ctx, "name"`, `step.WaitForEvent[...](ctx, "name"`, `step.Sleep(ctx, "name"`,
`step.Send(ctx, "name"`) forms. Names ending `/request` are folded into the approval they belong
to. A name built at runtime (a template literal with `${}`) is not a literal and is not
checked. A name found in a file that no declared function uses is reported once, naming the
file.

## W043

Level: warning
Check: a function declared in whisk.yaml has no function with that id in code.
Fix: Make the name under functions in whisk.yaml the id the code gives the function, or the reverse.
Safe fix: no

Details: the platform registers each declared function by matching its name with the id the
app's code serves; one that matches nothing is never registered, so its cron or event starts
nothing (a deploy reports `FUNCTIONS_NOT_REGISTERED`). Ids read: JavaScript
`createFunction({ id: "<id>" ...`, Python `create_function(fn_id="<id>" ...`, Go
`FunctionOpts{ID: "<id>" ...`. Skipped, with the forms it reads, when no id is found in code.

## W050

Level: warning
Check: a webhook handler path is not found as a route in code.
Fix: Add a POST route at the handler path, or change webhooks[].handler to the route that exists.
Safe fix: no

Details: same route idioms as W022, for POST.

## W051

Level: error
Check: a webhook secret is not listed under secrets.
Fix: Add the name to secrets in whisk.yaml.
Safe fix: yes

Details: `token` presets have no secret and are exempt.

## W052

Level: warning
Check: a webhook handler or the queue endpoint is listed under routes.public.
Fix: Remove it from routes.public; the platform delivers to these routes itself and the edge refuses other callers.
Safe fix: yes

Details: listing them does not open them, because the edge refuses internet requests to service
routes regardless, but it signals a misunderstanding and is removed.

## W053

Level: warning
Check: a webhook handler does not verify the delivery through the template helper or X-Whisk-Delivery-Signature.
Fix: Wrap the handler in the template's deliveries helper (deliveries.Handle in Go, deliveries.handle in TypeScript and Python), or verify X-Whisk-Delivery-Signature under WHISK_DELIVERY_KEY yourself before doing any work.
Safe fix: no

Details: for each `webhooks[].handler`, the file that registers the route (same route idioms
as W050) must reference `deliveries.Handle`, `deliveries.handle` or the literal
`X-Whisk-Delivery-Signature`. Without the check a handler trusts the network: any same-org app
that can reach it could post a forged delivery. Skipped when no route for the handler is found
(W050 reports that).

## W060

Level: error
Check: the Dockerfile runs as root, or neither exposes 8080 nor reads PORT.
Fix: Add a USER instruction with a non-root user and either EXPOSE 8080 or a CMD that reads PORT.
Safe fix: no

Details: the last `USER` instruction must be present and not `root` or `0`. The platform starts
containers as uid 65534 when no user is set, which breaks images whose files are owned by
another user, so an explicit non-root `USER` is required. `EXPOSE 8080` or `$PORT` in `CMD` or
`ENTRYPOINT` satisfies the port half.

## W061

Level: warning
Check: no lockfile exists for the detected package manager.
Fix: Generate one and commit it: package-lock.json, pnpm-lock.yaml, bun.lock, uv.lock, poetry.lock, requirements.txt with pins, or go.sum.
Safe fix: no

Details: detection by manifest file: `package.json` expects one of the Node lockfiles,
`pyproject.toml` one of the Python lockfiles or a pinned `requirements.txt`, `go.mod` expects
`go.sum`. Without a lockfile builds are slow and not reproducible.

## W062

Level: warning
Check: the migrate command needs a shell or package manager the final image does not have.
Fix: Write migrate as one plain command such as node dist/migrate.js, or base the Dockerfile's last stage on an image with a shell (for Node, FROM node:24-trixie-slim with USER node and CMD ["node", "dist/index.js"]).
Safe fix: no

Details: an image without `/bin/sh` runs `migrate` split into words, so `npm`, `npx`, `yarn`,
`pnpm`, `bun`, `sh` or `bash` as the first word, or shell syntax (`&&`, `|`, `;`, `<`, `>`, `$`,
backticks, `*`), fails at deploy with "not found". Images known to have no shell: `scratch` and
`gcr.io/distroless/*` other than their `:debug` tags, as the last stage's `FROM`. The templates'
Node image is distroless; an app that needs system programs (ffmpeg, ImageMagick, a browser)
moves its last stage to `node:24-trixie-slim` and installs them with `apt-get`.

## W063

Level: warning
Check: a local lockfile, or the live app's image, has a critical or high package finding with a fix (Business plan).
Fix: Move the package to the fixed version or newer in the lockfile (an image package by moving to a newer base image), deploy, then whisk scan --now.
Safe fix: no

Details: known only when the directory is bound to an app on a plan with package scanning
(`GET /apps/:app/packages`, CONTROL-PLANE.md §6.28). Doctor sends the local lockfiles to `POST
/apps/:app/packages/check`, so lockfile findings are about the files as they are now, before any
deploy, and the list shrinks as the agent updates them. When that check cannot be made, the
lockfile findings come from the app's newest scan instead, each left out once the local lockfile
no longer holds that version (for `package-lock.json`, no `node_modules/<package>` entry at it;
for other lockfiles, no line naming both), and the check's failure is listed as skipped. Image
findings come from the newest scan, name the `Dockerfile` and stay until the next deploy is
scanned. One finding per package version and place, with how many known vulnerabilities it has,
the worst severity, one ID and the highest fixed version. Nothing is reported on other plans.

## W064

Level: warning
Check: a build secret is not mounted by the Dockerfile.
Fix: Mount it where the build needs it, for example RUN --mount=type=secret,id=NPM_TOKEN,env=NPM_TOKEN npm ci, or remove it from build.secrets.
Safe fix: no

Details: build secrets reach the build only as BuildKit secret mounts, so a name in
`build.secrets` that no `--mount=type=secret` in the Dockerfile names (`id=<name>` or
`env=<name>`) is never seen by the build. Skipped without a Dockerfile: Railpack reads the build
secrets itself.

## W070

Level: error
Check: the repository exceeds the plan size limit.
Fix: Remove build output, media and archives from the repository and its history; use storage for files.
Safe fix: no

Details: measured as the packed size of all tracked history (`git count-objects`). The limit
comes from `/apps/:app/validate` when the directory is bound to an app and a token is available;
otherwise the Free plan's 100 MB is assumed. The five largest blobs are listed. Skipped when the
directory is not a git repository yet.

## W080

Level: warning
Check: always_on, customer_identity, storage, kv or email needs a plan the org does not have.
Fix: Ask an owner to upgrade, or remove the setting; the deploy will be refused otherwise.
Safe fix: no

Details: known only when the directory is bound to an app, because the answer comes from
`/apps/:app/validate`. Reported as a warning here and as an error at deploy, except `kv`: an
app on a plan without it (the free app) deploys without a cache and without `WHISK_KV_URL`,
unless it already had a cache, which it keeps. The push and the deploy's logs then carry the
line `whisk: W080 kv: the <plan> plan includes no key-value store, …`; the fix is to remove
`kv` from whisk.yaml or move to Starter or above.

## W090

Level: warning
Check: the app renders its own password field.
Fix: Remove the login form and read who is signed in from the X-Whisk-* headers; set customer_identity for the app's own users, and declare another system's password as a secret.
Safe fix: no

Details: the platform signs people in before a request reaches the app, so an app that asks for
a password is either rebuilding sign-in or collecting someone else's, and a page that asks for a
password is what the abuse scan holds for review. Matched in code and in templates and markup
(`.html`, `.jinja`, `.ejs`, `.hbs`, `.njk`, `.svelte`, `.vue`, `.astro`, `.tmpl`, `.gohtml`,
`.templ`, `.erb`, `.mustache`): `type="password"` in any quoting or none, `type: "password"`,
`.type = "password"`, and Python's `PasswordField` and `PasswordInput`. Tests and vendored
folders are left out. One finding per file, at its first match.

## W091

Level: warning
Check: the app sets a cookie with a Domain attribute.
Fix: Leave Domain out; the platform binds every app cookie to the app's own address as __Host-<name>.
Safe fix: no

Details: the edge drops a cookie that names a Domain, because a cookie shared across addresses
would reach every app under the domain. Matched in server code: `; Domain=` in a cookie string,
a `domain:` option to `setCookie`, `cookie`, `cookies.set` or `serialize` (JavaScript), a
`domain=` argument to `set_cookie` (Python), and `Domain:` in an `http.Cookie{}` (Go). One
finding per file, at its first match.

## W092

Level: warning
Check: the app reads who the caller is from the Authorization header or a token it verifies itself.
Fix: Read identity from the X-Whisk-* headers the platform sets (X-Whisk-User-Id, X-Whisk-Email, X-Whisk-Roles); call other apps with a service token and let the platform vouch for the caller.
Safe fix: no

Details: the platform signs people in before a request reaches the app and strips any
`X-Whisk-*` header from the internet, so its headers are the only identity it vouches for; an
`Authorization` header or a token the app decodes is whatever the caller sent. Matched in server
code: reading the incoming `Authorization` header (`c.req.header("Authorization")`,
`req.headers.authorization`, `request.headers.get("Authorization")`, `HTTP_AUTHORIZATION`,
`r.Header.Get("Authorization")`), and verifying a token (`jwt.verify(`, `jwtVerify(`,
`jwt.decode(`, `jwt.Parse(`). Setting the header on an outgoing request is not matched. One
finding per file, at its first match.

## W093

Level: warning
Check: the app uses an outside service for sign-in, email or a cache that Whisk already provides.
Fix: Use the built-in instead (the platform's login, email: true, kv: true), unless the person asked for that service by name.
Safe fix: no

Details: matched by import in server code. Sign-in: JavaScript `@auth0/*`, `auth0`, `@clerk/*`,
`next-auth`, `@auth/core`, `@supabase/auth-js`, `@supabase/auth-helpers-*`, `firebase/auth`,
`lucia`; Python `auth0`, `clerk_backend_api`, `flask_login`, `allauth`; Go `github.com/clerk/*`,
`github.com/auth0/*`, `github.com/markbates/goth`. Email: JavaScript `nodemailer`,
`@sendgrid/mail`, `resend`, `mailgun.js`, `mailgun-js`, `postmark`, `@aws-sdk/client-ses`;
Python `smtplib`, `sendgrid`, `resend`, `postmarker`, `mailgun`; Go `net/smtp`,
`github.com/sendgrid/*`, `github.com/resend/*`, `github.com/wneessen/go-mail`,
`gopkg.in/gomail.v2`. Cache: `@upstash/redis`, `@upstash/ratelimit`, `upstash_redis`,
`github.com/upstash/*`. One finding per library.

## W100

Level: warning
Check: the app's typical start, measured by the platform, is over 3 seconds.
Fix: Bundle Node apps into one file at build time, build TypeScript at deploy time, move database changes to whisk.yaml migrate, and load heavy libraries only when first used.
Safe fix: no

Details: the platform records how long the app's production container takes to answer its
health check each time it starts, on a wake and on a deploy; the typical start is the median of
the last 10. Doctor reads it from `/apps/:app/validate` (its `start`), so the rule is known only
when the directory is bound to an app and the platform answered, and it is skipped, with the
reason, when the directory is unbound, the platform was not asked, or no start is recorded yet.
The message names the app, the time and how many starts it is the typical of: `customer-health
takes 7.3 s to start (typical of the last 10 starts). Visitors wait that long after it sleeps.`
Most apps start in about a second. W101, W102, W103 and W104 name the usual causes.

## W101

Level: warning
Check: the start command compiles TypeScript as the app starts (tsx, ts-node).
Fix: Compile with tsc or a bundler in the build (a build script, or a RUN step in the Dockerfile) and start the built JavaScript with node.
Safe fix: no

Details: the start command is the final stage's `CMD` and `ENTRYPOINT` when the build uses a
Dockerfile, else package.json's `start` and `prestart` scripts (what the build runs without
one). A package script the command runs (`npm start`, `npm run <name>`, `pnpm <name>`,
`yarn <name>`, `bun run <name>`, with its `pre` script) and a shell script it runs
(`./start.sh`, `/app/scripts/start.sh`) are read too, up to three levels. Matched as a command
word: `tsx`, `ts-node`, `ts-node-dev`, `ts-node-esm`, `babel-node`, `node --import tsx`,
`node -r ts-node/register`, `node --loader ts-node/esm`; a file name ending `.tsx` is not one.
Scripts other than the start command (`dev`, `test`) are not read. Bun and Deno run TypeScript
without compiling it and are not reported. One finding per file, at the line of the command.

## W102

Level: warning
Check: migrations run every time the app starts, in the start command or the entry code.
Fix: Move the migration to migrate in whisk.yaml, which runs it once per deploy with a restore point first, and start only the app.
Safe fix: no

Details: the start command is read as for W101. Commands matched, each a tool with its migrate
verb: `prisma migrate deploy`, `prisma db push`, `drizzle-kit migrate`, `drizzle-kit push`,
`knex migrate:latest`, `knex migrate:up`, `sequelize db:migrate`, `typeorm migration:run`,
`alembic upgrade`, `manage.py migrate`, `django-admin migrate`, `goose ... up`,
`migrate -path ... up`, a package script whose name contains `migrat` (`npm run migrate`,
`yarn db:migrate`), and a migrate file run directly (`node dist/migrate.js`,
`python migrate.py`). In the entry files (W103): Drizzle's `migrate(db, { migrationsFolder })`,
Knex's `.migrate.latest()`, Kysely's `.migrateToLatest()`, `umzug.up()`, Alembic's
`command.upgrade(` and Django's `call_command("migrate")`, and the commands above in a string.
Comments are not read. A separate migrate file that only the manifest's `migrate` runs is not
an entry and is never reported. The message says whether whisk.yaml already has `migrate`. One
finding per file, at its first match.

## W103

Level: warning
Check: the app's entry file loads a heavy library when the app starts.
Fix: Import the library inside the function that uses it, so it loads on first use rather than at every start.
Safe fix: no

Details: the entry files are the code files the start command names (built JavaScript such as
`dist/index.js` is read as its source, `src/index.ts`; `uvicorn app.main:app` as `app/main.py`;
`python -m app` as `app.py` or `app/__main__.py`), package.json's `main`, and the conventional
names `src/index`, `src/server`, `src/main`, `index`, `server`, `main` (`.ts`, `.js`, `.mjs`),
`main.py`, `app.py`, `server.py`, `app/main.py`, `src/main.py`, `wsgi.py` and `asgi.py`.
Libraries reported, each a second or more to load: TypeScript and JavaScript `aws-sdk` (version
2; the version 3 clients are small), `googleapis`, `firebase-admin`, `puppeteer`, `playwright`,
`@tensorflow/tfjs-node`, `@tensorflow/tfjs`, `@huggingface/transformers`,
`@xenova/transformers`, by exact package name; Python `pandas`, `torch`, `tensorflow`,
`transformers`, `sklearn`, `scipy`, `matplotlib`, `spacy`, and their submodules. Loads matched:
a top-level static `import ... from "pkg"` or `import "pkg"` (not `import type`) and a
top-level `const x = require("pkg")` at the start of a line; in Python an `import pkg` or
`from pkg import` at the start of a line. An import indented inside a function, a dynamic
`import()`, and comments are not reported. One finding per file, at its first match.

## W104

Level: warning
Check: a Node app starts unbundled, loading its libraries from node_modules file by file.
Fix: Bundle the app into one file at build time (for example esbuild src/index.ts --bundle --platform=node --format=esm --outfile=dist/index.mjs), and run that.
Safe fix: no

Details: Node resolves and reads each file of each library as the app starts, thousands of file
lookups for an ordinary app, and every lookup is slow inside the sandbox an app runs in. One
measured app made about 4,450 lookups and opened about 430 files before it listened; bundled
into one file it made 60 and used a third of the CPU. Reported when all three hold: the start
command (read as for W101) runs a JavaScript file with node (`node dist/index.js`, `node
--enable-source-maps server.mjs`); the running image holds node_modules (a Dockerfile whose final
stage copies `node_modules` in or runs `npm ci`, `npm install`, `yarn`, `pnpm install` or `bun
install`, or, without a Dockerfile, a package.json with `dependencies`); and the build does not
bundle. A build bundles when a package.json script or a Dockerfile `RUN` runs `esbuild`, `tsup`,
`rollup`, `webpack`, `ncc`, `bun build`, or `vite build --ssr`, when a package.json script runs
a file with node (`node build.mjs`, as the TypeScript template does) that loads `esbuild`,
`rollup`, `tsup`, `webpack` or `@vercel/ncc`, or when an
`esbuild.config.*`, `tsup.config.*`, `rollup.config.*` or `webpack.config.*` is at the root. One
finding, at the line of the start command. The message names the app: `<name> loads its
libraries file by file at start, which is slow inside Whisk's sandbox.`

An ESM bundle that includes CommonJS dependencies (pg, pino and many others call `require`)
needs `require` defined, which esbuild adds with a banner:

```
esbuild src/index.ts --bundle --platform=node --format=esm --outfile=dist/index.mjs \
  --banner:js="import { createRequire } from 'node:module'; const require = createRequire(import.meta.url);"
```

Then copy only `dist/` into the final image, not node_modules, and start with
`node dist/index.mjs`. A library that loads files of its own at run time (native addons, some
template engines) is marked external (`--external:<name>`) and installed in the image alone.
