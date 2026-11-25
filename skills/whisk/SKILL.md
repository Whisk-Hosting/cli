---
name: whisk
description: "Build, check and ship business apps on Whisk (whisk.run): whisk.yaml, login, database, secrets, workflows, webhooks and the whisk CLI. Use when creating or changing an app that runs on Whisk."
---

# Building and shipping on Whisk

Conventions version 1. This is the document a coding agent reads before touching a Whisk app.
Every file it refers to is published beside it at `https://skill.whisk.run/<file>`:
`whisk.schema.json`, `graph.schema.json`, `headers.md`, `environment.md`,
`webhook-presets.yaml`, `errors.md`, `doctor-rules.md`. Working starter apps come with the CLI
(`whisk init --template`).

## Quick start

The person wants something live they can open, so get them an address first and build after.
Do these in order, before writing any of their app:

1. Install the CLI if `whisk` is not on the PATH:
   `curl -fsSL https://github.com/Whisk-Hosting/cli/releases/latest/download/install.sh | sh`
   (Windows in §3). If it says its folder is not on your PATH, run `whisk` by the full path it
   prints.
2. `whisk login --agent "<your name>"`. Show the person the link and code at once, while they
   are still watching, then run the `whisk login --resume` command it prints.
3. In an empty folder named for the app (the name becomes its address):
   `whisk init --template ts` (or `py`, `go`, `next`, `astro`). If it prints `FREE_APP_TAKEN`
   or `PLAN_LIMIT_APPS`, tell the person before going further.
4. `whisk deploy -m "Starter"`, about a minute, and give the person the address it prints.
5. Build what they asked for on top of the starter, deploying each piece as it works. Read §2
   before writing code, and the sections below as the app needs them: sign-in §4, data §5,
   secrets §6, jobs §7, webhooks §8, files and email §9, failures §11.

With existing code, skip the template and run `whisk init` in its folder (§3).

This page holds what every app needs. What only some apps need is in the files below, each
the rest of one section here; the section's last line names its file. Fetch a file when the app
needs what it covers:

| File | Read it when the app |
|---|---|
| `live.md` | needs its own domain, redirects, the CDN, analytics or package scanning, or is live and needs looking after |
| `identity.md` | signs in its own customers, is a Promoted app, has public pages, serves MCP, or uses feature flags or roles |
| `database.md` | keeps one customer's rows from another's, guards its schema, undoes a mistake or brings an existing database |
| `jobs.md` | needs approvals, workflow graphs, cron schedules, engine features or idempotency |
| `connections.md` | receives email, calls another app of the business, reaches the internet, or uses an outside service or an AI model |
| `files-email-kv.md` | takes uploads, video, images or documents, sends email or push notifications, makes PDFs, or uses key-value |

Each is at `https://skill.whisk.run/<file>`, printed by `whisk skill <file>`, and served by
`whisk mcp`.

## 1. What Whisk is

Whisk hosts websites, online shops, customer portals and internal tools written by coding agents. An app is a container that speaks HTTP; the
platform adds login, a Postgres database, a queue with durable workflows, secrets, webhooks,
storage, email and backups, all by convention rather than by SDK. Tenant apps live at
`<app>.<org>.whisk.page` on a paid plan, `<app>--<org>.whisk.page` on the free plan; the
platform is at `whisk.run`. Use the hostname the API returns rather than building one.

What you will do: `whisk login`, `whisk init`, `whisk doctor`, `whisk deploy`, then tell the
human which secrets to set and where. The human's part is a few screens in the dashboard. Yours
is everything else, and one more thing: whenever Whisk gets in your way, tell us with
`whisk feedback` (section 12). You are the user Whisk is built for, so your feedback is how it
gets better.

### Whisk already does this

Use the built-in. Do not add an outside service, SDK account or library for any of these unless
the human asks for that service by name; if a built-in falls short, say so with `whisk feedback`
(§12) rather than working around it.

| Need | Whisk's built-in | Instead of |
|---|---|---|
| Sign-in, users, roles | the platform's login and `X-Whisk-*` headers (§4) | Auth0, Clerk, NextAuth, Supabase Auth, a password page |
| Database and backups | Postgres per app, `DATABASE_URL`, backups and restore points (§5) | Supabase, Neon, Firebase, SQLite files |
| Scheduled and background work | functions, steps, retries, approvals (§7) | node-cron, Celery, BullMQ, an outside cron, Zapier |
| Secrets | declared names, set in the dashboard (§6) | `.env` files, Doppler, Vault |
| Incoming webhooks | `webhooks:` with verification, retries, replay (§8) | a relay service, your own signature code |
| Receiving email | `inbox:`, an address per app, delivered like a webhook (§8) | SES receiving, Mailgun routes, Postmark inbound, an IMAP poller |
| Files and uploads | `storage: true` (§9) | an S3 or R2 account, Cloudinary, UploadThing |
| Video and audio | `<whisk-video>` (§9) | Mux, Vimeo, YouTube embeds |
| Email | `email: true` (§9) | SendGrid, Resend, Mailgun, SMTP |
| Push notifications | `/.whisk/push.js` and `…/push/send` (§9) | OneSignal, Firebase Cloud Messaging, Pusher Beams, the `web-push` package |
| Cache, counters, locks | `kv: true` (§9) | Upstash, Redis Cloud |
| Errors | `SENTRY_DSN`, Whisk's own error store, `whisk errors` (§3) | a Sentry account, Bugsnag, Rollbar |
| Logs and request tracing | stdout, `X-Whisk-Request-Id`, `whisk logs`, OpenTelemetry traces to `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, `whisk traces` (§2, §11) | Datadog, Logtail, LogRocket, Honeycomb, an APM agent |
| Analytics | `<script defer src="/.whisk/analytics.js"></script>`, `whisk analytics` (§3) | Google Analytics, Plausible, PostHog, Mixpanel, a tracking pixel |
| Bot protection | `routes.challenge` (§4) | reCAPTCHA, hCaptcha, Turnstile |
| Hosting, HTTPS, domains, caching | the platform (§3) | Vercel, Cloudflare, a CDN |

### One app or two

To the people you work for, an app is one tool their team uses in a web browser, with its own
web address, its own data and its own list of who can use it. Size is not a reason to split: a
whole CRM with many screens is one app. When asked for something new, decide whether it belongs
in an existing app or is a new one:

- Same people, same information: add it to the existing app. A report on the CRM's customers
  belongs in the CRM.
- Different people should see it (a client portal beside the staff tools): a new app, because
  access is set per app.
- Something they would switch off, roll back or hand to someone else on its own: a new app.
- Someone else maintains it: a new app, so their changes do not collide with yours.
- Needing the other app's data is not a reason to merge: use a shared database (§5). Reuse the
  key the business already holds for a service rather than asking for a new one (§6).

Say which you chose and why in one sentence. When the rules do not settle it, ask the human.

## 2. The rules

An app on Whisk is a stateless HTTP server in a container.

1. Listen on `PORT` (always 8080) on all interfaces.
2. Answer `GET <health.path>` with 200 when ready to serve. Default `/health`.
3. Log to stdout and stderr. JSON lines are indexed by field; plain lines are indexed as text.
4. Write files only under `/tmp`. Everything else is read-only. `/tmp` is emptied on every
   restart; files that must last go to storage (§9, files a job keeps). `/tmp` is memory: what you
   keep there and what the process uses share `WHISK_MEMORY_BYTES` (256 MiB on Free), and a run
   that goes over is killed without SIGTERM.
5. Exit cleanly on SIGTERM within the grace period (default 28 seconds). A deploy sends new
   requests to the new version at once and sends SIGTERM to the old one only once the requests it
   already holds have finished, or after 15 minutes.
6. Read configuration and secrets from the environment. Never from files in the repository.
7. Trust identity only from the `X-Whisk-*` headers, which only the platform can set.
8. Treat every queue delivery and webhook as possibly repeated; use the provided IDs to dedupe.
9. Start fast. Under five seconds from process start to health 200 is the target, because
   sleeping apps wake on demand.

**Limits.** Every plan's limits are in one table at https://whisk.run/usage: memory, processor,
storage, database connections, workflow runs, email, requests, domains and outbound traffic.
Read it before building anything that has to grow, and design inside it.

**Keep it light.** Memory is what an app costs to run, and a small app wakes fast and is never
killed for running out. Stream large files and query results in chunks (storage reads, CSV and
spreadsheet parsing, database cursors) instead of loading them whole into memory or `/tmp`.
Process a big job as many small steps, each handling one batch. Let the database filter, sort and
total rather than fetching every row to do it in code. React to an event or webhook when
something changes instead of a schedule that checks every few minutes, and choose the longest
schedule the business can live with. Load heavy libraries only in the code path that needs them.
Pages and API reads that can be a little old should send `Cache-Control: private,
max-age=<seconds>` (`public` for a page anyone may see): the edge answers repeat requests from its
cache, per person and for up to an hour, without waking the app. A response that sets a cookie is
never cached.

**Live updates.** Server-sent events and WebSockets work on every plan and address, sign-in
included; prefer server-sent events when only the server talks. Writes reach the visitor at once
and nothing times a stream out, so send a heartbeat every 30 seconds (`:` on an event stream, a
ping on a WebSocket) to keep phone networks and proxies from dropping it. An open stream keeps the
app awake by itself. Streams close on every deploy (close them on SIGTERM) and now and then
when Whisk updates its servers, so every client reconnects with backoff: give events an
`id:` and resume from `Last-Event-ID`, and keep what a stream reports in the database or
key-value store so a reconnect misses nothing. Close a stream after an hour or so: the sign-in
is checked when it opens, and the reconnect checks it again.

## 3. Getting an app live

If `whisk` is not on the PATH, install it first; neither script needs root:

```
curl -fsSL https://github.com/Whisk-Hosting/cli/releases/latest/download/install.sh | sh        # linux, macOS
irm https://github.com/Whisk-Hosting/cli/releases/latest/download/install.ps1 | iex             # windows PowerShell
powershell -NoProfile -Command "irm https://github.com/Whisk-Hosting/cli/releases/latest/download/install.ps1 | iex"   # windows Git Bash or cmd
```

Each command runs in a fresh shell for most agents, so an `export PATH` does not last: when the
script says its folder is not on your PATH, run `whisk` by the full path it prints.

If `whisk` is already installed, run `whisk update` once at the start of a session: this
document describes the CLI the platform serves, and an older one lacks commands it names. Later,
any `--json` output that carries `cli_update`, or an `UNKNOWN_COMMAND` error, means the same:
run `whisk update` and retry.

```
whisk login --agent "<your name>"
                         name yourself as people know you ("Claude Code", "Codex", "Cursor"):
                         the approval page, deploys and token lists show it. Exits 2 at once
                         with a URL and a code in the NEEDS_HUMAN block (also under --json):
                         show both to the human, then run the resume command it names
                         (whisk login --resume <device_code>), which waits for approval.
                         The token works only from this computer: use it through whisk,
                         never copy it elsewhere
whisk init --template ts|py|go|next|astro|shop writes whisk.yaml, .whisk/app.json, and
                         the template; next and astro are Next.js and Astro; shop is a whole online shop (on Whisk's commerce engine) for a business selling
                         to the public, and runs only as a Promoted app
whisk doctor             checks the repository; exit 3 means fix the listed problems
whisk deploy -m "…"      commits, pushes, builds, migrates, health-checks, switches traffic
whisk logs -f            tails the app
whisk clone <org>/<app>  copies an existing app's code here; git pull and push then work
```

If `whisk login --resume` is cut off by your shell's time limit, run it again: the code stays
good until it expires and the waiting key is kept.

**Following and recovering.** These are the commands to reach for when something takes a while
or goes wrong:

```
whisk deploy --no-wait -m "…"   prints the deploy id at once; a deploy can take up to 25 minutes
whisk deploys info <id>          its status and phase; deploys cancel <id> stops a stuck one
whisk deploys log <id>           its whole build log and, for a failed start, the app's last lines
whisk status                     running, sleeping or broken, and why it stopped answering
whisk whoami · whisk orgs        who you are and which businesses this token reaches
whisk use <org>/<app>            binds this folder to an app (the NOT_FOUND fix names it)
whisk secrets list               declared names, set or unset; never values
whisk functions list             declared functions, their triggers and drift from the code
whisk events send <name> --data @event.json    sends an event to test an event function
whisk runs show <id> · whisk runs replay <id> · whisk runs cancel <id>
whisk restore show <id> --wait   follows a restore (whisk restore list lists them)
```

Every command takes `--org <slug>` and `--app <slug>` instead of `.whisk/app.json`, and
`--quiet` to drop progress lines. In CI, where nobody can approve a login, a person runs
`whisk agent-token create --label "ci"` and puts the token in `WHISK_TOKEN`.

Run `whisk login` in the app's folder, or with `--org` (and `--app`): the approval page then
starts on that app or business, the narrowest login that fits. The person can choose all their
businesses instead; that login acts as them in every business they belong to, including ones
they join later: `whisk orgs` lists them and `--org <slug>` picks one when the folder names none.
A computer has one sign-in: do not pass `--profile` to make another, and do not try other
profiles to find a business. When the business you need is missing, run `whisk whoami`: under
"Not in this login" (`not_in_login` with `--json`) it names each of the person's businesses the
login does not cover, with the link where the person adds it. When a command meets one, it exits
2 with a NEEDS_HUMAN block whose link (`details.url`, `details.reason` `BUSINESS_NOT_IN_LOGIN`)
is the same page. Show the person the link, then run the command again. Do not run `whisk login`
again for it. A business `whoami` does not name is one the person is not in: ask them to join it
or to have an owner invite them. A person adds a new business themselves at
`https://whisk.run/new`.

If you speak MCP rather than a shell, run `whisk mcp` as a stdio server: this document and
every error code are its resources, every command is a tool.

In a chat with no shell, such as ChatGPT or Claude, the person connects Whisk's assistant
connector (`https://api.whisk.run/mcp`) and signs in once. Its tools cover deploying (`deploy_app`
takes the files themselves), rolling back, logs, runs (and replaying one), the database's tables
and queries, access and feedback; `whisk_build_guide` names which command each one stands for and
what a chat cannot do (cron runs on demand, webhook deliveries, errors and traces, restores,
previews, domains, export), which needs the dashboard or a developer with the CLI.

Every command takes `--json`. Exit codes: 0 success; 1 an error, including a refused request
such as a bad slug or uncommitted files (`INVALID_REQUEST`), read `code` and `fix`; 2 needs a
human, show them the printed URL and wait; 3 doctor or the manifest found problems, fix the
listed ones and retry; 4 not signed in or not allowed: read `code` (for `AUTH_REQUIRED` or
`TOKEN_EXPIRED` run `whisk login`; for `AGENT_CATEGORY`, `AGENT_PAUSED`, `FORBIDDEN_ROLE` or
`TOKEN_SCOPE` logging in again does nothing, so tell the human what to grant or resume); 5
network or Whisk's side, retry with backoff and do not change the code; 6 deploy failed in the
app, read the log excerpt.

Start from a template unless the human already has code: `whisk init --template ts` (Hono,
Drizzle), `--template py` (FastAPI, SQLAlchemy, Alembic), `--template go` (chi, pgx, goose). Each is
a plain starter (a notes page, a health route, one event function; a few hundred lines, the Go
one about 900) that passes doctor with zero warnings, deploys with nothing for a person to set,
and shows the conventions below working. `whisk init --template` works in a folder holding only
dot-files and `CLAUDE.md` or `AGENTS.md`. Existing code deploys too: `whisk init` in its folder
writes `whisk.yaml` for it, carrying across what its previous host's Procfile, `app.json`,
`render.yaml`, `railway.json`, `railway.toml` or `fly.toml` says (migrate step, workers, health
path, Dockerfile, secret names, settings) and listing at the top what is left to do by hand. Keep
what the app already does for itself (its own sign-in and job queue on a Promoted app, its payment
provider) and move what Whisk does for it: files to storage, mail to `email: true`, scheduled
commands to functions, backups to Whisk's. Then make it follow the rules and run doctor.

**Errors and traces.** Wire both in before the first deploy, in every app, whatever it is built
with. They are how you and the human find out what broke and what is slow once real people use
the app: `whisk errors` and the Errors page list its exceptions, grouped, with the stack;
`whisk traces` and the Traces page list every request with the edge's timing, and the app's own
spans show where inside the app each request's time went: its queries, its calls out. The platform sets
`SENTRY_DSN` and the `OTEL_*` variables (`environment.md`) in every deployed container, so all
the app does is start a Sentry SDK and an OpenTelemetry SDK when they are set. `whisk dev` sets
neither, so nothing is sent while you work. The templates already do this. Until an app does,
`whisk doctor` warns (`W140`, `W141`) and `whisk deploy` says "Not set up: errors and traces."
Without an SDK, the stacks the app prints (a Node error and its frames, a Python traceback, a Go
panic) still reach the Errors page from its logs, but without the request, the user, or the
exceptions a framework catches and answers 500 for without printing.

Any Node server (Express, Fastify, Hono, Koa, Astro, SvelteKit, React Router, Remix, Nuxt) loads
one file before the app. Install `@sentry/node @opentelemetry/sdk-node
@opentelemetry/instrumentation-http @opentelemetry/instrumentation-undici
@opentelemetry/instrumentation-pg`, write `instrument.mjs`, and start the server with
`node --import ./instrument.mjs <entry>` in the Dockerfile's `CMD` or the `start` script:

```js
import * as Sentry from "@sentry/node";
import { NodeSDK } from "@opentelemetry/sdk-node";
import { HttpInstrumentation } from "@opentelemetry/instrumentation-http";
import { UndiciInstrumentation } from "@opentelemetry/instrumentation-undici";
import { PgInstrumentation } from "@opentelemetry/instrumentation-pg";

if (process.env.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT) {
  const sdk = new NodeSDK({
    instrumentations: [
      new HttpInstrumentation({ ignoreIncomingRequestHook: (req) => req.url === "/health" }),
      new UndiciInstrumentation(), // fetch
      new PgInstrumentation(),     // pg, and Drizzle or Kysely over pg
    ],
  });
  sdk.start();
  process.once("SIGTERM", () => sdk.shutdown().finally(() => process.exit(0)));
}
if (process.env.SENTRY_DSN) {
  Sentry.init({ dsn: process.env.SENTRY_DSN, environment: process.env.WHISK_ENV });
}
```

The SDK reads the endpoint, the token and the app's name from the `OTEL_*` variables itself, and
the HTTP instrumentation continues the edge's `traceparent`, so each request's trace id is its
request id. Leave out `@opentelemetry/auto-instrumentations-node`: it loads dozens of
instrumentations and slows every start. Sentry 11 reports errors and leaves tracing to the SDK
above; leave its `enableOpenTelemetrySetup` off.

Next.js: put the same code in `instrumentation.node.ts` (without the `SIGTERM` line, and without
the HTTP instrumentation, since Next.js traces each request itself) and add `instrumentation.ts`
at the project root, or in `src/`. A new app can start from `whisk init --template next` or
`--template astro`, which have both wired.

```ts
export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") await import("./instrumentation.node");
}
export async function onRequestError(err: unknown, request: { headers: Record<string, string | string[] | undefined> }) {
  const Sentry = await import("@sentry/node");
  Sentry.captureException(err, { tags: { request_id: String(request.headers["x-whisk-request-id"] ?? "") } });
}
```

Frameworks catch errors themselves and answer 500, so tell Sentry about them where the framework
hands them over, tagged with the request's `X-Whisk-Request-Id` as `request_id` so the error
links to its trace:

| Framework | Where |
|---|---|
| Express | `Sentry.setupExpressErrorHandler(app)` after the routes |
| Fastify | `Sentry.setupFastifyErrorHandler(app)` |
| Koa | `Sentry.setupKoaErrorHandler(app)` |
| Hono | `app.onError((err, c) => { Sentry.captureException(err); … })` |
| SvelteKit | `handleError` in `src/hooks.server.ts` |
| Astro | a middleware: `try { return await next() } catch (e) { Sentry.captureException(e); throw e }` |
| React Router, Remix | `export function handleError(error)` in `entry.server` |
| Nuxt | a Nitro plugin: `nitroApp.hooks.hook("error", (e) => Sentry.captureException(e))` |

Python: add `sentry-sdk`, `opentelemetry-sdk`, `opentelemetry-exporter-otlp-proto-http` and the
framework's instrumentation (`opentelemetry-instrumentation-fastapi`, `-django` or `-flask`),
then where the app is made:

```python
import os, sentry_sdk
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

if os.environ.get("SENTRY_DSN"):
    sentry_sdk.init(dsn=os.environ["SENTRY_DSN"], environment=os.environ.get("WHISK_ENV"))
if os.environ.get("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"):
    provider = TracerProvider()
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
    trace.set_tracer_provider(provider)
    FastAPIInstrumentor.instrument_app(app, exclude_spans=["receive", "send"])
```

Go: `sentry.Init(sentry.ClientOptions{Dsn: os.Getenv("SENTRY_DSN")})` when it is set, and when
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` is set, `otlptracehttp.New(ctx)` as the exporter of
`sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))`, `otel.SetTracerProvider`,
`otel.SetTextMapPropagator(propagation.TraceContext{})`, and the router wrapped in
`otelhttp.NewHandler`. The Go template's `whisk.go` (`startTracing`, `traceRequests`,
`traceQueries`) is the whole of it, database queries included, to copy.

**Everything `whisk.yaml` takes.** Unknown keys are errors; `whisk schema` prints the JSON schema.
Settings marked paid are refused with `PLAN_FEATURE` on the free app, except `kv`, which
deploys without a cache and a `W080` line instead (§9). Storage, email and customer identity
are on every plan, the free app included, within its allowances.

```yaml
whisk: 1                          # required
name: job-tracker                 # required; the app's address derives from it
title: Job board                  # what people call it, two to four plain words
description: Every open job, who is on it and what is due this week.   # one sentence, 120 at most
category: Jobs                    # the group the business's home lists it under
routes:
  public: ["/", "/health"]        # everything else needs a signed-in person (§4)
  challenge: ["/contact"]         # public POSTs here need a solved bot challenge (§4)
  csrf_off: []                    # rare: routes exempt from the CSRF check, for embeds
  device_header: X-Device-Id      # optional: your own clients' install header, rate limited per install (§4)
  headers: { X-Frame-Options: DENY }   # add or override response headers
health: { path: /health, timeout: 30 } # seconds from start to the first 200; default 90
database: app                     # app | shared:<name> | none (§5)
migrate: "node dist/migrate.js"   # runs before traffic switches, at most 600 seconds (§5)
database_role: restricted         # the app cannot change its schema; set it on every new app (§5)
build:
  dockerfile: Dockerfile          # optional; the stack is detected without one
  secrets: [NPM_TOKEN]            # available only while building, never at run time
secrets: [XERO_CLIENT_SECRET]     # names only (§6)
env: { LOG_LEVEL: info }          # plain configuration, visible in logs
queue: { endpoint: /.whisk/inngest }
functions: []                     # cron and event functions (§7)
webhooks: []                      # incoming webhooks (§8)
inbox: { handler: /inbound/email } # receive email at the app's own address (§8)
static:                           # committed folders served by the edge, cached for a year
  - { dir: public/assets, path: /assets }
redirects:                        # old addresses the edge redirects, before the app wakes
  - { from: /about-us, to: /about }               # 301 unless status says otherwise
redirects_file: redirects.txt     # more rules, one a line: from to [status] [query=drop]
storage: false                    # files, uploads, images, video (§9) · every plan
kv: false                         # a Redis of its own (§9) · paid
kv_evict: true                    # false for a job queue: a full cache refuses writes (§9)
email: false                      # sending email (§9) · every plan
always_on: false                  # never sleeps · paid
b2b: false                        # the shop template's trade ordering, Promoted apps only (§4) · paid
commerce: medusa                  # the shop template's own setting: keep it in a shop made from it (§4)
content: payload                  # the site and shop templates' own setting: keep it (§4)
workers: []                       # an existing app's own job processes, Promoted apps only (§4) · paid
calls: []                         # other apps of the business this one calls (§8)
accepted_callers: []              # other apps of the business that may call this one (§8)
customer_identity: none           # none | app | org: the app's own users (§4) · every plan
sign_in: { logo: brand/logo.png, colour: "#0b6e4f" }   # customers' sign-in look (§4) · app pool
network: public                   # the only value, the same as leaving it out: apps are public
previews: { database: empty, ttl_days: 3 }   # previews start empty; days a preview lasts
```

`static` folders are read from the commit, not from the built image: a folder the commit does
not hold, such as git-ignored build output like `dist/`, fails the deploy with `MANIFEST_INVALID`
naming it (`whisk doctor` warns first, `W004`). Serve build output from the app itself, or
commit the folder. Static files never wake the app and are cached for a year, so give them
content-hashed names. `always_on` is for apps that must answer at once. People can also
switch it on the app's page in the dashboard, which commits the change to whisk.yaml and
deploys it, so pull before you push again. An open
WebSocket or event stream keeps an app awake by itself, and everything else should sleep. A sleeping app has 60 seconds to answer its health check when a
visitor wakes it, whatever `health.timeout` allows a deploy, so an app that needs longer sets
`always_on` (`W005`). A sleeping app starts again on its next request, so
start fast: the TypeScript template bundles each entry point into one file with esbuild
(`build.mjs`), so its image holds no `node_modules` and Node reads a few files instead of thousands.
A package that must stay a file on disk (a native addon, or one that reads its own files at run
time) goes in esbuild's `external` list, and the Dockerfile copies it into the image.
A preview starts with an empty database of its own; seed what testing needs from a migration or
a script, never from production.

**Settings apply in seconds.** When a push changes only `whisk.yaml` since the live deploy and
leaves `build`, `env`, the `static` folders and `b2b` as they were, Whisk skips the build and runs
the live image with the new settings: routes and who can visit, `always_on`, `health`, secrets and
the rest. It goes live in the time a deploy takes. Commit a settings change on its own to get
this, and tell the human it applies straight away. A route made public still waits for a person
to allow it.

**Describe the app.** The business's home lists every app by its `title`, `description` and
`category`, so set all three in whisk.yaml before the first deploy. People open apps from the home,
and most of them never see the code. The title is what the people who use the app would call it,
in two to four plain words: "Job board", never "jobtrackr-v2". The description is one sentence of
at most 120 characters for someone who uses the app, starting with what they do in it: "Log your
hours each day and send them for approval on Friday." Leave out the app's name, technical words,
the stack, and words like "powerful" or "AI-powered". The category is the group it sits in, such
as Sales or Jobs. Reuse one the business already has: `whisk doctor` lists them (`W160`). When the
app changes what it is for, change the description in the same commit. An owner can change all
three on the home, and what they set stays, so doctor tells you when your values are no longer the
ones shown.

**Say what changed.** Every deploy shows the subject line of its commit to the business, on the
app's Deploys page and in `whisk deploys list`. Write it for the people who use the app, in one
plain sentence about what they will notice: `whisk deploy -m "Add a CSV export to the jobs
page"`, not "fix bug", "wip" or "whisk deploy". `-m` commits uncommitted changes with that
message; when you commit yourself, the first line of your commit message is what shows. Keep it
under 72 characters and leave file names and internals out.

`whisk deploy` from `main` or `master` deploys production, and nothing else does. From any
other branch it starts a preview at its own URL, says so, and leaves production alone; to put a
branch live, merge it into `main` and run `whisk deploy` from `main` (`PRODUCTION_NEEDS_MAIN`
otherwise). Work on `main` unless the human asks to try something out first. Check `environment` in the `--json` summary before telling the
human something is live. A plain `git push` of a branch only stores it: start its preview with
`whisk envs start <branch>` when someone needs to see it, and from then on every push to that
branch updates it. Each app may have one preview on the free app and three on a paid plan
(`PLAN_LIMIT_PREVIEWS`); remove one you are done with using `whisk envs delete
preview:<branch>`. Previews sleep when idle and go a few days after their last push or visit.
A preview runs no scheduled functions, workflows or events (`PREVIEW_NO_WORKFLOWS`); skip
sending events when `WHISK_ENV` starts with `preview:`, and test functions with `whisk dev`.
`whisk apps delete <app> [<app>...]` (or `<org>/<app>`) prints one link where a person deletes
them all in one step; show it to them. A deleted app keeps its data for 7 days, and `whisk apps
deleted` then `whisk apps restore <id>` bring it back stopped.

**Several people on one app.** Other people and their agents may be changing the same app while
you work. Start from the latest code: in a copy made by `whisk clone`, run `git pull --rebase whisk
main` before you change anything (plain git is signed in there). If someone pushes while you are
working, `whisk deploy` fetches their commits and rebases yours on top when that goes cleanly,
then pushes; the `--json` summary has `caught_up` when it did, so run the tests again before you
tell the human it is done. When it cannot, you get `PUSH_BEHIND` and your copy is left as it was:
run the `git rebase` or `git merge` its fix names, resolve each file in `details.conflicts` so
both changes still work, run the tests, and deploy again. For a bigger change, work on a branch
and show it on a preview, so nobody else's work is in the way until it is merged into `main`.
Never force-push to Whisk: it deletes the other person's work.

`whisk deploy` ends with the URL, the address of each declared webhook, and any `warnings`: a
deploy that went live with something wrong, such as `FUNCTIONS_NOT_REGISTERED` (a function name
in `whisk.yaml` the code does not use), whose fix says what to change. If declared secrets
(`build.secrets` included) have no value yet it also prints a `NEEDS_HUMAN` block naming them and
the dashboard URL. Relay that block to the human verbatim. Do not ask them for the values; the
app restarts on its own when they are set.

Some changes are the business's to allow, not yours. A production deploy waits with
`GRANT_NEEDED` (exit 2, a `NEEDS_HUMAN` block) when, compared with the live app, it calls
another app, adds a webhook, makes a route public, drops a route's challenge
or CSRF check, or changes `customer_identity`; when it names a business secret this app is not on
the list of, or a key another app of the business holds (§6); or when a connection needs a grant. The block names each change in a plain sentence
and gives one link where an owner or admin approves them all with one tap. Relay it verbatim and
say plainly what each change lets the app do; never try to approve it yourself. The first
deploy, a deploy that widens nothing and a deploy the same as one approved before do not wait,
and previews never do.

**More in [`live.md`](https://skill.whisk.run/live.md):** their own domain; one main address; a new address; redirects; their customers' domains; serving from near the visitors; looking after a live app; analytics; known vulnerabilities in packages (Business plan).

## 4. Identity

The platform signs people in. Your app never sees a password, a token or a login form. On
every request, read who it is from headers (full table in `headers.md`):

| Header | Meaning |
|---|---|
| `X-Whisk-Audience` | `anonymous`, `team`, `customer` or `service` |
| `X-Whisk-User-Id` | stable ULID; store this, never the email, as the key |
| `X-Whisk-Email`, `X-Whisk-Name` | verified email; display name (may be empty) |
| `X-Whisk-Org` | org ULID |
| `X-Whisk-Groups` | comma-separated group names (team only) |
| `X-Whisk-Roles` | comma-separated org roles: `owner`, `admin`, `developer`, `billing`, `member`, plus `guest` (team only) |
| `X-Whisk-Flags` | comma-separated feature flags on for this person, when any is (below) |
| `X-Whisk-Request-Id` | trace ID; put it in every log line |
| `X-Whisk-Country` | the visitor's country, `NZ`, when the edge knows it; a hint, not proof |

Routes are private unless listed under `routes.public` in the manifest. A private route only
ever receives requests with a `team` or `customer` identity (or `service`, for platform
deliveries and app-to-app calls); the platform has already redirected browsers to login and
answered API clients 401. A public route receives `anonymous` when nobody is signed in and the
full identity when someone is.

Someone not signed in is rate limited by their network address, which an office shares. When the
app's own clients (a desktop or phone install that signs its requests itself) call public routes,
have each send a header naming its install and list it as `routes.device_header` (for example
`X-Device-Id`): each install then has its own allowance, and the installs at one address together
have ten times it. The edge does not check the header, so the app still authenticates every
request.

Authorise inside the app by role or group:

```ts
const roles = (c.req.header("X-Whisk-Roles") ?? "").split(",").filter(Boolean);
if (!roles.includes("admin") && !roles.includes("owner")) return c.text("Forbidden", 403);
```

Never read identity from a cookie, a query parameter, a body, or an `Authorization` header a
browser sent; the headers are the only source and the platform strips any copy arriving from the
internet. To sign someone out, redirect to `/.whisk/logout?return=/`. To force login from a
public page, redirect to `/.whisk/login?return=<path>`. Both are handled in front of the app.

**More in [`identity.md`](https://skill.whisk.run/identity.md):** using an app as your person; promoted apps; public pages; the Whisk bar; an MCP server in the app; feature flags; who may see and change what.

## 5. Database

`DATABASE_URL` is a pooled Postgres connection in transaction mode. Use it as-is with any
driver: its certificate verifies, Node's `pg` included. No session-level state: no `SET` outside a transaction, no advisory locks held across statements,
no `LISTEN`. Prepared statements are fine inside a transaction. Before a migration relies on a
function from a recent release (such as `uuidv7()`), check the server's version:
`whisk db schema --json` prints it as `postgres`.

Migrations run from the manifest's `migrate` command before traffic switches, with a direct
connection the platform provides for that step and a snapshot taken first. The previous deploy
stays live and keeps writing while the migration runs. A migration that runs longer than 600
seconds is stopped; split a long backfill into a function instead. A failed migration that
changed nothing, or only added tables, indexes, views or functions before it stopped, leaves the
database as it is, so nothing users wrote is lost; the error names what it added, so make it
safe to run again (`IF NOT EXISTS`). One that dropped or changed something the live version
uses, or added a unique index, is rolled back to the snapshot, and the error names the replaced
database, kept for 7 days, where anything users wrote meanwhile can be read back with
`whisk db query --database`. So run each migration in one transaction (most tools do on
Postgres), and keep migrations forward-only and idempotent where the tool allows. `whisk dev`
also runs the `migrate` command on your own machine before it starts the app.

Give every new app `database_role: restricted`, as the starters do: the running app then reads
and writes rows under every row-level security policy and cannot turn a policy off or change a
table, and only `migrate` runs as the owner. Make each table that holds a customer's rows enable
and force row-level security, with its policy, in the migration that creates it (W095). An app
that already runs without the setting keeps working as it is; add it once nothing outside
`migrate` changes the schema (W105).

`database: app` (default) gives the app its own database. `database: shared:<name>` gives the
app its own schema inside a database shared across the org, for apps that must join across each
other. `database: none` for apps without one. When an app needs data another app of the
business keeps, read it from there: a grant in the owning app's migration when both are on one
shared database (below), or a call to the other app (§8). Prefer that to a new database or a copy
of the data.

In a shared database each app connects as its own role and owns one schema of the same name,
`app_<app id in lower case>`, which is first on its `search_path`, so unqualified tables land
there. Every schema is private to its app until that app grants access; nothing in `whisk.yaml`
opens it. The owning app grants read access in one of its migrations, naming the reader's role
(`whisk apps info <reader> --json` prints the reader's id as `app.id`; inside an app,
`select current_user` names its own role):

```sql
GRANT USAGE ON SCHEMA app_<owner id> TO app_<reader id>;
GRANT SELECT ON ALL TABLES IN SCHEMA app_<owner id> TO app_<reader id>;
ALTER DEFAULT PRIVILEGES IN SCHEMA app_<owner id> GRANT SELECT ON TABLES TO app_<reader id>;
```

The first two cover the tables that exist; the third covers tables the owner creates later. The
reader then queries `app_<owner id>.<table>` and joins it with its own tables. Grant `INSERT`,
`UPDATE` or `DELETE` the same way only when the reader must write; `REVOKE` takes access back.

**More in [`database.md`](https://skill.whisk.run/database.md):** row-level security: one customer never sees another's rows; keep the app's own queries from changing the schema; looking at the data from your machine; undoing a mistake; bringing an existing database.

## 6. Secrets

**One key per service in a business.** Before you declare a secret, run
`whisk secrets list --scope org`: it lists every key the business holds and which app holds it.
When the service already has a key there (Stripe, Xero, a maps API, an SMTP login), use that key:
name it exactly as listed, keep the code that reads it, and let the deploy offer it (below). One
key for the whole business is simpler for everyone: one value to rotate, one bill, one place to
look. Asking a person for a new key, or renaming the secret and rebuilding around a new one,
is only for when the business has no key for that service yet, or the person asks for a separate
one. If you are unsure, ask the person whether to use the key the business already has; never
ask them for a new one first.

Declare names in the manifest; humans set values in the dashboard.

```yaml
secrets: [XERO_CLIENT_SECRET, SLACK_WEBHOOK_URL]
```

The value arrives in the environment under that name. Never write a value into code, a file, a
commit message or a log line; the push is rejected if a commit contains one, and every value is
scrubbed from logs. Never ask the human for a value in chat. When a secret is needed, say:

> Set `XERO_CLIENT_SECRET` at https://whisk.run/o/<org>/secrets. I have declared it; the app
> restarts when you save it.

`whisk secrets declare NAME` creates the slot before the first deploy if you want the human to
fill it early. `whisk secrets set NAME1 NAME2 …` never takes a value: it prints one link that
opens the secrets page with a paste box ready for those names, so the human pastes them all at
once (a `.env`-style `NAME=value` block works). Relay that link. Org-scoped secrets are
shared by every app in the org; app-scoped ones shadow them. A slot nobody filled, such as one
whisk.yaml no longer names, you remove with `whisk secrets delete NAME`; one with a value only a
person deletes.

Previews get a secret only when a person has shared it with previews, so by default a preview
runs without production's keys and must cope with them missing. If a preview needs one, run
`whisk secrets set NAME --previews` and relay the link; turning it on is the human's decision.

**How the business's key reaches this app.** A production deploy that names a key another app
of the business holds waits with `GRANT_NEEDED` (kind `secret`, whose `change` names that app)
instead of asking for a value: the one tap that allows the deploy shares the value and lets this
app read it. Relay that link with everything else the deploy waits on; never send the person to
paste the key or to the Keys page as a separate step. Outside production, `SECRET_UNSET` says which
app holds it (`details.elsewhere`): run `whisk secrets share NAME --from <app>`. A business secret
reaches only the apps on its list (`whisk secrets apps NAME`), and the deploy's one approval adds
the app to it. A preview starts without a business secret its app is not on the list of and says
so (`SECRET_NOT_GRANTED`). Keys never move between businesses. Values shorter than 8 characters
are refused (`SECRET_TOO_SHORT`) because they cannot be scrubbed; put those under `env`.

## 7. Jobs and workflows

**Enqueue.** `POST $WHISK_QUEUE_URL` with `Authorization: Bearer $WHISK_SERVICE_TOKEN` and

```json
{"name": "po.created", "data": {"po_id": 1042}, "dedupe_key": "po-1042"}
```

Returns `{"id": "..."}`. Events with the same `dedupe_key` within 24 hours are dropped.

**Functions** are declared in the manifest with a trigger and a graph, and implemented with the
Inngest SDK for the language (the templates wire it). The platform delivers runs to
`queue.endpoint` (default `/.whisk/inngest`) with the service identity; the SDK verifies each
request with `WHISK_INNGEST_SIGNING_KEY`. Use the SDK the templates use (`inngest` 4 for
TypeScript and JavaScript, `inngest` 0.5 or later for Python, `inngestgo` for Go) and give its
client all three settings, as the template does:

```js
new Inngest({ id: "my-app", eventKey: process.env.WHISK_INNGEST_EVENT_KEY,
  signingKey: process.env.WHISK_INNGEST_SIGNING_KEY, baseUrl: process.env.WHISK_INNGEST_URL })
```

When they are wrong the deploy still goes live, with the warning `FUNCTIONS_NOT_REGISTERED`, and
no function runs: read the deploy's warnings (`whisk status`, or `get_deploy`) after every deploy
that changes functions, and fix that one before saying a schedule works.

```yaml
functions:
  - name: nightly-margin
    cron: "0 6 * * *"
    tz: Pacific/Auckland
    graph: workflows/nightly-margin.graph.yaml
  - name: po-approval
    event: po.created
    graph: workflows/po-approval.graph.yaml
```

```ts
export const poApproval = inngest.createFunction(
  { id: "po-approval", triggers: [{ event: "po.created" }], retries: 3 },
  async ({ event, step, runId }) => {
    const po = await step.run("fetch-po", () => fetchPo(event.data.po_id));
    if (po.total > 5000) {
      const ok = await approval(step, runId, "request-approval", { to: "owner", title: `PO ${po.id}`, data: po });
      if (ok.decision !== "approved") return;
    }
    await step.run("post-po", () => post(po));
  },
);
```

Steps are the unit of retry and memoisation: each `step.run` runs once and its result is kept,
so a run resumes after the last completed step. Do work only inside steps; code between steps
re-executes on every resume.

**More in [`jobs.md`](https://skill.whisk.run/jobs.md):** approvals; graphs; cron; engine features; idempotency.

## 8. Webhooks, other apps and outside systems

Declare the source; the platform receives, verifies and stores every delivery, then calls your
handler with a clean request.

```yaml
webhooks:
  - name: stripe
    preset: stripe                 # see webhook-presets.yaml; or hmac with settings; or token
    secret: STRIPE_WEBHOOK_SECRET  # must also be under secrets
    handler: /hooks/stripe
```

`whisk deploy` (and `whisk webhooks list`) prints `https://hooks.whisk.run/<org>/<app>/stripe`.
Tell the human to paste it into the provider and to set the signing secret. The handler
receives `POST` with the original raw body, the original headers as `X-Whisk-Webhook-Orig-*`,
plus `X-Whisk-Webhook-Id`, `X-Whisk-Webhook-Source` and `X-Whisk-Webhook-Received-At`, with
`X-Whisk-Audience: service`. Respond 2xx within 30 seconds; anything else is retried at 1m, 5m,
30m, 2h and 12h, then dead-lettered for replay. Dedupe on `X-Whisk-Webhook-Id`: a retry or replay
carries the same id, and a delivery that Stripe, GitHub, Shopify, Slack, Linear or a
`standard-webhooks` sender sends again under its own id is not sent to the app again. Any other
provider's repeat arrives as a new delivery, so dedupe those on the provider's id in the payload. The handler does not need to be in `routes.public` and cannot be reached
from the internet. `ip_allowlist` (CIDRs) also refuses deliveries from anywhere else; a provider
with no preset uses `preset: hmac` with its signature settings under `hmac` (the fields of a
preset in `webhook-presets.yaml`). A provider that checks the address first by sending a value
to echo (Business Central, Microsoft Graph: `?validationToken=`) needs `handshake: {query:
validationToken}` on the source; the platform answers it and the handler never sees it.
`whisk webhooks events <name>` lists recent deliveries with why
the last attempt failed (the handler's status and answer), and `whisk webhooks replay <name>
<id>` sends one to the handler again.

**More in [`connections.md`](https://skill.whisk.run/connections.md):** receiving email; calling another of the business's apps; routes only apps may call; apps Whisk runs for the business; reaching the internet; services outside Whisk: connections; AI models: a connection with a spend cap.

## 9. Storage, email, key-value

**Storage** (`storage: true`). Use any S3 client with `WHISK_STORAGE_ENDPOINT`,
`WHISK_STORAGE_BUCKET`, `WHISK_STORAGE_ACCESS_KEY`, `WHISK_STORAGE_SECRET_KEY`,
`WHISK_STORAGE_REGION`, writing under `WHISK_STORAGE_PREFIX`, which is this environment's own.
The key reaches only that prefix: always list with the prefix, never the whole bucket, and keep
no other app's file paths. Read the bucket's name from `WHISK_STORAGE_BUCKET` each time
the app starts, never a copy of it: an app is given a bucket of its own once, with every file at
the same key, and starts again with the new name. A preview has its own prefix and key, apart from production's. Use
path-style addressing (`forcePathStyle: true` in the AWS SDK for JavaScript,
`addressing_style: "path"` in boto3, `BucketLookupPath` in minio-go): the bucket goes in the
path, not the hostname. The endpoint is reachable from your app and from browsers alike.

**More in [`files-email-kv.md`](https://skill.whisk.run/files-email-kv.md):** files people send; video and audio; images; documents and other files; a website's own files at their own paths; signed links (an app with its own sign-in); files a job keeps between runs; email; push notifications; PDFs; key-value.

## 10. Local development

`whisk dev` needs Docker with the compose plugin. It starts Postgres, the workflow engine and a
local edge that injects identity, runs the migration, then runs the command after `--` with the
same environment the app has in production:

```
whisk dev --as ana@acme.example --groups finance --roles owner -- npm run dev       # ts
uv run whisk dev --as ana@acme.example --roles owner -- uvicorn app.main:app --port 8080   # py
whisk dev --as ana@acme.example --roles owner -- go run .                          # go
```

Without `-- <command>` it prints the environment and expects the app on port 8080, started by
you. Requests to `localhost:3000` carry the headers for that person; public routes behave as in
production; `/.whisk/login` and `/.whisk/logout` work. Secrets for local runs come from
`.whisk/dev/secrets.env`, which is git-ignored and filled by the human. Webhooks locally:
`whisk dev tunnel stripe` prints the URL to paste into the provider and forwards every verified
delivery to the app running here for an hour. Email locally: `whisk inbox send message.eml` hands a
message to the inbox handler, stored and delivered as on the platform (§8).

Before the CLI is installed, `whisk-stub` (`contract/cmd/whisk-stub` in the public whisk CLI
repository, `go run` it from there) does the same job with an existing Postgres and the Inngest
dev server.

## 11. When something fails

Every error is `{"error": {"code", "message", "fix", "docs", "details"}}`. Read `code`, do what
`fix` says. A code starting `PLATFORM_`, or `details.fault: "platform"` on a deploy or a run, is
Whisk's side: do not change your code or write workarounds for it. Deploy again, or wait. A run
Whisk interrupted (`PLATFORM_RUN_INTERRUPTED`) before any step finished is run again by Whisk
once, and `rerun_run_id` names the new run; one with a finished step is yours to replay. Set
`retries` in the function's options in code and the engine retries a lost step inside the same
run itself; `retries` in `whisk.yaml` only describes it for people. `details.fault: "app"` is yours to fix. The ones you will meet most:

| Code | Do this |
|---|---|
| `MANIFEST_INVALID` | fix each problem in `details.problems`; `whisk doctor` lists them |
| `SECRET_IN_COMMIT` | remove the value, declare a name, rewrite the commit, rotate the value |
| `SECRET_UNSET` | relay the names and URL to the human; nothing else |
| `HEALTH_CHECK_FAILED` | listen on `PORT` on `0.0.0.0`; make the health route return 200; read the log excerpt |
| `CONTAINER_CRASHED` | the app exited; read the log lines (`whisk deploys log <id>`), usually a missing variable or file |
| `FUNCTIONS_NOT_REGISTERED` | a warning on a live deploy: make each function name in `whisk.yaml` the id the code uses |
| `PACKAGE_MALICIOUS` | a lockfile names a known malicious package: remove it and whatever brought it in, never install it to look, and if it was ever installed ask the human to rotate the secrets it could read |
| `PACKAGE_LOOKALIKE` | a warning on a live deploy: a dependency's name looks like a popular package's; fix the typo and redeploy, or leave it if the name is right |
| `BUILD_FAILED` | read the log excerpt; usually a dependency or compile error |
| `MIGRATE_FAILED` | fix the migration and redeploy; the message says whether the database was rolled back and where users' writes were kept |
| `AUTH_REQUIRED` | the route is private; list it under `routes.public` or sign in; from the CLI, `whisk login` |
| `FORBIDDEN_ROLE` | your token's role cannot do this; tell the human which permission it needs, do not log in again |
| `AGENT_CATEGORY` | the human has not allowed agents this kind of action (`details` names it); ask them to allow it in the dashboard or do it themselves |
| `AGENT_PAUSED` | the human paused this agent; stop and tell them, nothing else works until they resume it |
| `CSRF_REJECTED` | POST from the app's own pages, or use a bearer token |
| `RUN_PARKED` | `whisk runs show <id>`, fix the step, deploy, `whisk runs replay <id>` |
| `PLATFORM_DEPLOY_FAILED` | not the code; run `whisk deploy` again, and wait a few minutes if it repeats |
| `RATE_LIMITED` | wait `Retry-After` |
| `PUSH_BEHIND` | someone else pushed first: run the `git rebase` or `git merge` the fix names, resolve `details.conflicts`, test, deploy again; never force-push |

`whisk logs --since 1h --grep <request id>` finds a request. `whisk runs show <id>` shows a
workflow run step by step. `whisk errors <CODE>` prints the full entry from `errors.md`.
`whisk traces --slowest` lists slow requests; `whisk traces show <request id>` shows where one request's time went, span by span.
`whisk logs forwarding` shows whether the business also sends its logs to its own log service
(Business plan) and how that is going; an owner adds one in the dashboard, never you.
If an error's `fix` did not work, was unclear, or you had to guess, send feedback (section 12)
once you are past it.

## 12. Tell Whisk what got in your way

Whisk is built for coding agents, and you meet its rough edges first. The Whisk team reads every
piece of feedback and turns it into fixes. Send it at any point, without asking the human first:
when something looks like a bug (a `fix` that did not help, a 500, docs that disagree with
behaviour), when something was hard (you retried, guessed or needed the human), when something
is missing, and when something worked well.

```
whisk feedback "deploy said HEALTH_CHECK_FAILED with an empty excerpt; the app listened on 3000. Naming the port would have saved three deploys." --kind bug --code HEALTH_CHECK_FAILED
```

`--kind` is `bug`, `difficulty` (default), `idea` or `praise`; `--code` and `--command` name what
you hit. Say what you did, what happened, what you expected and what would have helped. If the
CLI will not install or login fails, post it without a token:
`curl -fsS https://api.whisk.run/v1/feedback -H 'Content-Type: application/json' -d '{"message":"..."}'`,
or in PowerShell, where `curl` is another command:
`Invoke-RestMethod -Method Post -Uri https://api.whisk.run/v1/feedback -ContentType application/json -Body (@{message='...'} | ConvertTo-Json)`.

Every piece gets an id; keep it. Come back to what you sent:

```
whisk feedback list          # what you and your org sent, open and resolved, with the team's notes
whisk feedback show <id>     # one piece in full: its status and what the Whisk team said
```

Run `whisk feedback list` at the start of a session in a project where you sent feedback before.
A resolved piece usually means what got in your way is fixed, and its note says how or what to
do now: retry what failed, or drop the workaround you used. Do not send the same feedback again
while it is open; if a resolved problem is still there, send a new piece naming the old id.
Feedback sent without a login is read by its id alone, with `whisk feedback show <id>` or
`curl -fsS https://api.whisk.run/v1/feedback/<id>` (`Invoke-RestMethod https://api.whisk.run/v1/feedback/<id>`).

Never include a secret value, a token or personal data; name the secret instead. At most 20 an
hour signed in, 5 without a login; past that, combine them.

## 13. Do not

- Add an outside service for something Whisk already does (§1, Whisk already does this).
- Run cron, timers or long-lived worker loops inside the app. Declare a function (doctor
  warns, W023).
- Load a whole file, export or table into memory or `/tmp` when it can be streamed in chunks.
- Poll on a schedule for something a webhook or event can tell you.
- Write to disk outside `/tmp`. Use the database or storage.
- Hard-code hostnames. Use `WHISK_PUBLIC_URL` and relative links.
- Print secrets, tokens or the environment.
- Force-push to Whisk. Bring the other person's commits into yours and push again (§3, Several
  people on one app).
- Read cookies in page script by their bare name. In the browser every app cookie is
  `__Host-<name>`, bound to the app's own address; the app's server sees `<name>`. Script that sets a cookie the server should read sets `__Host-<name>` with
  `Secure; Path=/`, or the platform drops it. A client that reads a CSRF token cookie (axios's
  `XSRF-TOKEN`, Django's `csrftoken`) is pointed at the `__Host-` name, for example axios
  `xsrfCookieName: "__Host-XSRF-TOKEN"`; the platform already refuses cross-app form posts.
- Fetch another app's routes from page script. Those requests arrive without cookies; call the
  other app from your server with a service token.
- Accept `X-Whisk-*` values from anywhere but the request the platform delivered.
- Build a login page, a password field, a session store or a password reset, unless the app is
  a Promoted app that keeps its own sign-in (§4). The platform owns sign-in; doctor warns on a password field (W090), and a public page asking for a password in
  another company's name is held for review.
- Hold a database connection open across requests with session state; the pool is
  transactional (doctor warns on `LISTEN`, advisory locks and `SET`, W024). A Promoted app's
  worker with `database: direct` is the one place for them (§4).
- Make the app slow to start: a sleeping app's visitor waits for it. Bundle a Node app into one
  file in the build (`esbuild src/index.ts --bundle --platform=node --format=esm
  --outfile=dist/index.mjs`) and start that, not `tsx` or a `node_modules` tree; run migrations
  through `migrate` in whisk.yaml, not at start; import heavy libraries inside the function that
  uses them. Doctor warns on each (W101 to W104) and on a measured start over 3 seconds (W100).
- Ask the human for a secret value. Declare the name and point them at the dashboard.
