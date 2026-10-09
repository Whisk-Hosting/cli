<p align="center">
  <a href="https://whisk.run">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset=".github/whisk-wordmark-light-ink.svg">
      <img alt="Whisk" src=".github/whisk-wordmark.svg" width="260">
    </picture>
  </a>
</p>

<p align="center">
  <strong>Hosting for business apps built by AI coding agents. This is its CLI, contract, starter apps and agent skill.</strong>
</p>

<p align="center">
  <a href="LICENSE"><img alt="MIT licence" src="https://img.shields.io/badge/licence-MIT-14213d"></a>
  <img alt="Go 1.26+" src="https://img.shields.io/badge/go-1.26%2B-14213d">
  <img alt="Linux, macOS, Windows" src="https://img.shields.io/badge/runs%20on-linux%20%7C%20macOS%20%7C%20windows-14213d">
  <img alt="Signed releases" src="https://img.shields.io/badge/releases-Ed25519%20signed-e3a92b">
</p>

<p align="center">
  <a href="https://whisk.run">whisk.run</a> ·
  <a href="#what-whisk-is">What Whisk is</a> ·
  <a href="#platform-primitives">Primitives</a> ·
  <a href="#the-app-contract">Contract</a> ·
  <a href="#install">Install</a> ·
  <a href="#the-cli">CLI</a> ·
  <a href="skills/whisk/SKILL.md">Agent skill</a>
</p>

---

## What Whisk is

Whisk is a hosting platform for business apps written by AI coding agents such as Claude Code,
Codex, Cursor and Gemini CLI. The agent installs the `whisk` CLI, describes the app in
`whisk.yaml` and runs `whisk deploy`. The app goes live at `https://<app>--<org>.whisk.page`
with sign-in, a Postgres database, backups, scheduled jobs, durable workflows, secrets, webhooks
and storage supplied by the platform. Apps use these through environment variables and
HTTP headers, not an SDK.

**Who it is for:** businesses that have their AI agent build internal tools, client portals,
approval flows, reports and integrations around their existing systems, and agencies that build
these apps for clients.

**What an app is:** one tool a team uses in a web browser, with its own web address, its own
data and its own list of who can use it. A whole CRM with many screens is one app.

**What runs it:** any container that speaks HTTP on port 8080. Languages and frameworks are not
restricted. Starter apps exist for TypeScript (Hono, Drizzle), Python (FastAPI, SQLAlchemy,
Alembic) and Go (chi, pgx, goose).

## Platform primitives

| Primitive | How an app uses it | Facts |
|---|---|---|
| **Sign-in** | Reads `X-Whisk-User-Id`, `X-Whisk-Email`, `X-Whisk-Roles`, `X-Whisk-Groups` on every request | No login code in the app. Routes are private unless listed in `routes.public`. Roles: owner, admin, developer, billing, member, guest. The edge strips any `X-Whisk-*` header sent from the internet |
| **Customer accounts** | `customer_identity: app` or `org` | An app's own outside users (portals) arrive in the same headers with audience `customer` |
| **Postgres** | `DATABASE_URL` | Own database per app (`database: app`), or own schema in an org-shared database (`shared:<name>`) with SQL `GRANT`s between apps. PgBouncer in transaction mode |
| **Migrations** | `migrate:` command in `whisk.yaml` | Run before traffic switches, after a snapshot. A failed migration restores the snapshot and keeps the previous deploy live |
| **Backups and restore** | `whisk restore --at <time>` | Continuous backups. Point-in-time restore to any minute: 7 days on Starter, Team and Agency, 30 days on Business, not on Free |
| **Queue** | `POST $WHISK_QUEUE_URL` | Events with the same `dedupe_key` within 24 hours are dropped |
| **Durable workflows** | Inngest SDK, functions declared in `whisk.yaml` | Inngest v1.44.0. Steps are retried and memoised. Event names are scoped to the app. Failed runs are parked for `whisk runs replay` |
| **Cron** | `cron:` and `tz:` on a function | Nothing runs on a timer in the container. A sleeping app wakes for the run. `whisk cron run` starts one now |
| **Approvals** | `approval(step, runId, name, {to, title, data})` | A workflow waits up to 7 days for a person. The platform renders the approval page and notifies a role, group or user |
| **Workflow graphs** | `workflows/*.graph.yaml` | Each function's steps and decisions as a diagram, with real runs overlaid. Doctor checks the graph matches the code |
| **Secrets** | Names in `secrets:`, values in the environment | Values are set by a person in the dashboard, never by the agent. Envelope encryption per tenant. A commit containing a secret is rejected at push. Values are scrubbed from logs |
| **Webhooks** | `webhooks:` with a preset and a handler path | The platform receives, verifies and stores every delivery at `hooks.whisk.run`, then calls the handler. Presets: Stripe, GitHub, Shopify, Xero, Slack, HubSpot, Twilio, Zoom, Linear, Standard Webhooks, plus custom HMAC and token. Retries at 1 min, 5 min, 30 min, 2 h and 12 h, then dead-lettered for replay |
| **Object storage** | S3 API: `WHISK_STORAGE_*` | Browser uploads go straight to storage up to 5 GB each. Every object is virus-scanned |
| **Video and audio** | `<whisk-video media="ID">` | Converted for every browser, with a poster image, served from the app's own address. Private or public with an embeddable player |
| **Key-value** | Redis protocol: `WHISK_KV_URL` | A per-app instance for counters, caches and locks. Not durable |
| **Previews** | `whisk deploy` from any branch other than `main` | Each branch gets its own URL and its own database. Production is untouched |
| **Domains** | `whisk domains add`, `whisk domains business add` | One app on a custom hostname, or every app on `*.yourdomain.com` with one wildcard CNAME. HTTPS certificates are issued on verification |
| **Logs and errors** | stdout and stderr | JSON lines are indexed by field. `whisk logs`, `whisk errors`. Business plans can forward logs to their own service |
| **Sleep and wake** | Automatic | Idle apps sleep and wake on the next request. The edge serves cached `Cache-Control` responses without waking the app. Paid plans can keep apps always on |
| **Export** | `whisk export` | The whole org as one archive: git repositories, databases and audit log |

Isolation: each app runs in a gVisor sandbox as non-root on a read-only filesystem, on its own
network.

## The app contract

An app on Whisk is a stateless HTTP server in a container. It must:

1. Listen on `PORT` (always 8080) on all interfaces.
2. Answer `GET /health` (or `health.path`) with 200 when ready.
3. Log to stdout and stderr.
4. Write files only under `/tmp`, which is memory and is emptied on restart.
5. Exit cleanly on SIGTERM within 28 seconds.
6. Read configuration and secrets from the environment.
7. Trust identity only from the `X-Whisk-*` headers.
8. Treat every queue delivery and webhook as possibly repeated, and dedupe on the provided IDs.
9. Start in under five seconds, because sleeping apps wake on demand.

A complete manifest:

```yaml
whisk: 1
name: purchase-orders
routes:
  public: ["/health"]
database: app
migrate: "node dist/migrate.js"
secrets: [XERO_CLIENT_SECRET, STRIPE_WEBHOOK_SECRET]
functions:
  - name: nightly-margin
    cron: "0 6 * * *"
    tz: America/New_York
    graph: workflows/nightly-margin.graph.yaml
  - name: po-approval
    event: po.created
    graph: workflows/po-approval.graph.yaml
webhooks:
  - name: stripe
    preset: stripe
    secret: STRIPE_WEBHOOK_SECRET
    handler: /hooks/stripe
```

The full schema is [`contract/whisk.schema.json`](contract/whisk.schema.json). The headers,
environment variables, error codes and doctor rules are in [`contract/`](contract).

## Install

```sh
curl -fsSL https://whisk.run/install.sh | sh        # Linux, macOS
```

```powershell
irm https://whisk.run/install.ps1 | iex             # Windows
```

No root needed. Or give your agent this one line:

> Read https://whisk.run/skill and put this app live on Whisk.

ChatGPT and Claude chats without a shell can connect `https://api.whisk.run/mcp` as a connector
instead.

## A session

From an empty folder to a live app (output trimmed):

```console
$ whisk login
Open https://whisk.run/device?code=KXQT-MWPD and enter the code KXQT-MWPD
Signed in as ana@acme.example (org acme), profile default, token stored in os keychain and bound to this computer.

$ whisk init --template ts
Wrote Dockerfile, package.json, src/index.ts, src/db.ts, whisk.yaml, workflows/… and 20 more.
Created app acme/orders: https://orders--acme.whisk.page
Next: whisk doctor, then whisk deploy.

$ whisk doctor
Doctor found nothing to fix.

$ whisk deploy
Live: https://orders--acme.whisk.page
```

`whisk deploy` pushes over git, builds, migrates, health-checks and switches traffic. When a
declared secret has no value yet, it ends with a `NEEDS_HUMAN` block naming the secret and the
dashboard link where a person sets it.

## The CLI

| Area | Commands |
|---|---|
| Start | `login`, `init`, `use`, `clone`, `apps` |
| Check | `doctor`, `doctor --fix`, `schema`, `skill`, `errors <CODE>` |
| Run locally | `dev` (Postgres, the workflow engine and a local edge that injects identity; `--as`, `--roles`, `--groups`), `dev tunnel` for webhooks |
| Ship | `deploy`, `deploys`, `status`, `rollback`, `envs`, `domains`, `open` |
| Data | `db query`, `db schema`, `restore --at`, `export` |
| Run | `runs`, `functions`, `cron`, `events`, `approvals`, `webhooks`, `logs` |
| People and access | `members`, `access`, `customers`, `tokens`, `agent-token`, `deploy-keys` |
| Settings | `secrets` (names only), `github`, `billing`, `update`, `feedback` |

Built for agents:

- `--json` on every command. Machine output on stdout, progress on stderr.
- Exit codes: 0 ok, 1 error, 2 needs a human, 3 validation failed, 4 not signed in, 5 platform
  unavailable, 6 deploy failed.
- Every error is `{"error": {"code", "message", "fix", "docs", "details"}}` with a stable code.
  More than 130 codes are documented in [`contract/errors.md`](contract/errors.md).
- Every change carries an idempotency key, so retries after a timeout are safe.
- No flag, prompt or stdin path accepts a secret value.
- Changes that alter or remove existing rows (`whisk db query` with update, delete, drop) need a
  confirm code and take a restore point first.
- `whisk mcp` serves the docs as MCP resources and every command as an MCP tool over stdio.
- `whisk skill`, `whisk schema`, `whisk errors` and `whisk doctor` work offline.
- Sign-in tokens are bound to a key pair on the computer that signed in. A copied token is
  refused elsewhere.
- One static binary (`CGO_ENABLED=0`) for Linux, macOS and Windows on amd64 and arm64. `git` is
  the only runtime dependency.

## Signed releases

Each release's `SHA256SUMS` is signed with an Ed25519 key held only on the build machine. The
public key is pinned in [`cli/whisk/release_pubkey.go`](cli/whisk/release_pubkey.go).
`whisk update` refuses a binary whose signature or checksum does not match, and refuses to
install an older version.

## The agent skill

[`skills/whisk/SKILL.md`](skills/whisk/SKILL.md) is the document an agent reads before it builds
or changes a Whisk app: the rules, sign-in, the database, secrets, workflows, webhooks, storage,
local development and the common errors with their fixes. It is in the layout agent skill
installers expect, so Claude Code, Codex and other agents can install it from this repository.
The same text is printed by `whisk skill` and served at https://whisk.run/skill.

## Build from source

Go 1.26 or newer:

```sh
git clone https://github.com/Whisk-Hosting/cli && cd cli/cli
go build -o ../dist/ ./cmd/whisk     # the binary lands in dist/
go test ./...
```

## Repository layout

```
cli/          the whisk command (Go)
contract/     the whisk.yaml and graph schemas, identity headers, environment variables,
              error catalogue, doctor rules, webhook presets and a local platform stub
templates/    starter apps in TypeScript, Python and Go (what whisk init --template writes), and
              in canary/ the fuller apps the platform's checks deploy; each passes doctor with
              zero findings
skills/       the agent skill
```

## Contributing

This repository is copied from the Whisk platform repository on every release, so a change made
here directly is replaced by the next copy. Issues are welcome for bugs and requests. A pull
request is a good way to show a fix, and accepted ones are carried across.

Report security problems as https://whisk.run/security describes, not in a public issue.

## Licence

MIT. See [LICENSE](LICENSE).
