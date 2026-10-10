# cli

Specification: [docs/CLI.md](../docs/CLI.md). Read it before writing anything here.

`whisk` is the command coding agents drive and humans occasionally type. It is a thin client of
the control plane API (`CONTROL-PLANE.md §5`) plus the local half that needs no platform: it
reads the contract, checks a repository, and runs an app the way the platform would. MIT,
published with the contract.

```
cmd/whisk/             the binary
whisk/                 the command tree (root, agent helpers, auth, init/apps, doctor, dev)
doctor/                every doctor rule from doctor-rules.md, applied to a repository
scaffold/              whisk init: the commented manifest and .gitignore for a detected stack
templates/             the three starter apps embedded in the binary (go generate from ../../templates)
dev/                   whisk dev: the compose stack and the contract's stub
internal/
  api/                 the control plane client: bearer token, idempotency keys, the error object,
                       deploys (SSE events, build logs), secrets, domains, members, access
  gitcmd/              git for deploy: the runner, token via the environment, sideband parsing
  config/              config.json, credentials (keychain or a 0600 file), .whisk/app.json
  output/              JSON for agents, tables for humans, exit codes (CLI.md §3)
  stack/               stack detection from package.json, pyproject.toml, go.mod, Dockerfile
```

## Building

```
make cli            # or: cd cli && go build -o ../bin/whisk ./cmd/whisk
```

One static binary, `CGO_ENABLED=0`. The three templates are baked in, so `whisk init --template`
works anywhere; `go generate ./templates` refreshes the embedded copy from `../../templates`
and a test fails when the two drift.

## What works without the platform

These need no control plane and are the whole of the hard local logic:

- `whisk init [--template ts|py|go]` — detect the stack, write a commented `whisk.yaml`, the
  `.gitignore` entries and example graphs, and create the app when a token is present.
- `whisk doctor [--fix]` — every rule in `doctor-rules.md`, by convention search over the tracked
  files. Levels and fix text come from the contract, so the document and the check cannot
  disagree; the three templates pass with zero findings. `--fix` applies the safe ones (the
  `.gitignore` entries, adding a webhook secret to the manifest, removing a service route from
  `routes.public`, scaffolding a graph from the step names in code) and repeats until nothing
  changes, so a fix that unlocks a skipped rule is still reported.
- `whisk dev` — Postgres, PgBouncer and the Inngest dev server (and Valkey and object storage when
  the manifest declares them) through Docker Compose in `.whisk/dev/`, then the contract's `stub`
  package: an edge that injects the identity headers for `--as`, honours the manifest's public
  routes, and serves the queue, approvals and webhook ingress locally. The app runs with the
  production environment and `WHISK_DEV=1`.
- `whisk skill`, `whisk errors <CODE>`, `whisk schema`, `whisk cron list`, `whisk webhooks presets`
  — the contract printed for an agent, with `--json` on everything.
- `whisk cron run <function>` — start one run of a cron function of the bound app now, exactly as
  its schedule would (`POST /orgs/:org/apps/:app/cron/:function/run`), and print the run id with
  `whisk runs show <id>` to follow it, or, when the engine has not listed the run yet, its event
  id with `whisk runs list --event <id>`; `--json` prints `{org, app, function, run}`.
- `whisk mcp` — the same contract as MCP resources and every command as an MCP tool over stdio
  (`whisk/mcp.go`: JSON-RPC 2.0 by hand, protocol 2025-06-18, tools generated from the cobra
  tree). `--list` prints what it offers.
- `whisk update` — self-update from the platform's `/dl/`: VERSION, SHA256SUMS, and
  SHA256SUMS.sig verified under the Ed25519 key in `whisk/release_pubkey.go` when one is embedded
  (`internal/release`; `cmd/whisk-sign` is what `make release-key` and `make cli-dist` run).
- `whisk login/logout/whoami/orgs/use` and `whisk apps` — the account and app commands, offline
  where they can be (`use` binds the directory even when the platform is unreachable).

## What works with the platform

The remote half is a thin client of the API (`CONTROL-PLANE.md §5`); every command has `--json`
and the exit codes of `CLI.md §3`:

- `whisk deploy` — doctor, `git init` and a `whisk deploy` commit when needed, the `whisk` remote,
  a push with the token handed to git through the environment (`internal/gitcmd`), the deploy id
  from the server's `whisk:` sideband lines (or a 30 s poll of the deploys list), then the SSE
  event stream until live (URL, plus a `NEEDS_HUMAN` block for unset secrets), blocked (exit 2)
  or failed (code, fix, the last 40 log lines, exit 6). `whisk deploys list/info/cancel`,
  `whisk rollback`.
- `whisk status`, `whisk open` — the app's state and current deploy; the dashboard page or app
  URL, derived from the API origin (`CLI.md §7`).
- `whisk scan [--now]` — the live app's known vulnerable packages with the version that fixes
  each, on the Business plan; `--now` checks again and waits (`CLI.md §5.5`).
- `whisk secrets list/declare/set/link/versions/rollback/delete/reads` — names and metadata only;
  `set` always prints the dashboard link and exits 2.
- `whisk connections list/show/grant/pause` — the app's connections through Whisk's broker:
  grants by environment, Whisk's summary of what a grant allows and how each key is used;
  `grant` always prints the connections page and exits 2; `pause NAME [--env]` and
  `pause --all` stop calls at once (`CLI.md §5.6`). A deploy blocked on a grant answers
  `GRANT_NEEDED` with that page, exit 2.
- `whisk domains add/verify/list/remove`, `whisk envs list/delete`.
- `whisk github` (and `status`), `whisk github repos/link/sync/unlink/install`: the app's
  optional two-way copy on GitHub; linking and installing are a person's (`CLI.md §5.7`).
- `whisk members list/invite/remove`, `whisk access show/grant/revoke`, `whisk deploy-keys create`,
  `whisk customers list/invite/block/unblock/remove`, `whisk agent-token create`,
  `whisk tokens list/revoke`.
- `whisk feedback` (and `send`), `whisk feedback list/show/resolve/reopen`: send feedback to
  Whisk and check back on your own with the team's note; list everyone's with `--everyone`, and
  resolve or reopen a piece with a note, as an operator or with a token carrying
  `feedback:read` or `feedback:resolve` (`CLI.md §5.12`).
- `whisk managed list/add/set/link`, `whisk pause`, `whisk resume`: the business's managed apps,
  whose code and releases are Whisk's (`CLI.md §5.2`, `MANAGED-APPS.md`); `whisk apps list`
  marks a copy `managed` and a paused app `paused`.
- `whisk runs list/show/replay/cancel`.
- `whisk db query/schema/shell/url/snapshot`, `whisk restore --at`, `whisk export`.

`whisk logs` and `whisk errors` need the logs and errors modules of the control plane and are not
in the binary yet; they are absent rather than stubbed.

## Testing

```
cd cli && go test ./...
```

Unit tests cover argument parsing, exit-code mapping and JSON shape against a fake API, every
doctor rule against a triggering and a passing fixture, the doctor rules against the three real
templates (zero findings), cron next-run times, the `whisk dev` compose plan, and the remote
commands against a fake API with a fake git runner (`remote_test.go`): the push path with the
token in the environment, a fake SSE stream ending live with unset secrets and ending failed with
the build log excerpt, pre-receive rejections, rollback's empty body, `secrets set` exiting 2,
domains, members, access grants and `--json` shapes; `cmd_data_test.go` does the same for
`db`, `restore`, `customers`, `tokens` and `runs replay/cancel`; `cmd_github_test.go`
for the `github` commands against the routes of `CONTROL-PLANE.md §6.26`; `cmd_managed_test.go`
for `managed`, `managed link`, `pause` and `resume` against the routes of `MANAGED-APPS.md §10`; `mcp_test.go`
drives the MCP server through initialize, resources and tool calls; `cmd_update_test.go` runs
`whisk update` against a fake release directory, signed and unsigned. `whisk dev`
itself is exercised against a local Postgres by running a canary app from `templates/canary/`
through `doctor` and `dev` and driving its `canary.test`.

## whisk dev and a second Inngest

`whisk dev` runs its own Inngest dev server. On a machine that already runs Inngest on its default
ports (`8288`, and the executor gRPC on `50052`/`50053`) — for example the `compose-up` platform
stack — the dev server shares the host network and its executor does not advance multi-step
function runs past the first step. Stop the other Inngest, or run `whisk dev` on a machine without
it, when exercising workflows. The queue, approval and webhook endpoints and every non-workflow
route are unaffected.
