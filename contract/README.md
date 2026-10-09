# contract

Specification: [docs/CONTRACT.md](../docs/CONTRACT.md). Read it before writing anything here.

The public contract every Whisk app and coding agent builds against, and that the platform
implements. Documents are the truth; the Go module makes them executable so the CLI, the control
plane and the stub cannot drift from them.

```
SKILL.md               what an agent reads first
whisk.schema.json      manifest schema, conventions version 1 (JSON Schema 2020-12)
graph.schema.json      declared workflow graph schema
headers.md             identity, delivery and response headers, reserved paths
environment.md         every variable the platform injects
workflows.md           what the workflow engine supports, tested, with the platform's limits
webhook-presets.yaml   signature presets, one generic verifier
errors.md              every error code: status, when, fix, example
doctor-rules.md        every doctor rule: level, check, fix, detection idioms
CHANGELOG.md           additive changes by conventions version
fixtures/              manifests and graphs (valid and invalid), webhook deliveries per preset,
                       one platform-signed delivery with its key (deliveries/)

contract.go            embeds the documents (import github.com/whisk-run/contract)
manifest/              Parse and validate whisk.yaml: schema, rules, defaults, route classes
graph/                 Parse and validate graphs: schema, reachability, drift against runs
routes/                the route glob (* in a segment, ** across segments)
webhook/               presets and the HMAC verifier the ingress and the stub use
errors/                the catalogue as data, parsed from errors.md
apitypes/              the API's JSON, defined once; the dashboard's TypeScript is generated
                       from it (go test ./apitypes -update)
tokensig/              the Whisk-Signature a device-bound agent token's requests carry: the
                       canonical string, signing and verification
platformpage/          the page the edge and the platform answer a browser with when there is
                       no app page to show (waking, access walls, paused, unknown address)
wakewindow/            the edge's wake confirmation and grace spans and the node's subnet
                       quarantine, which must nest
doctor/                the rule table as data, parsed from doctor-rules.md
stub/                  a local stand-in for the platform, to run an app before any platform exists
cmd/whisk-stub/        the stub as a command; the CLI's whisk dev embeds the package instead
```

## Checking it

```
go test ./...
```

runs every fixture: each valid manifest and graph parses, each invalid one fails for the stated
reason, each webhook preset verifies its recorded delivery and rejects the forged, tampered and
stale variants, every error code and doctor rule in the specification has a section, and the
stub enforces the edge behaviour in `headers.md`.

## The stub

`whisk-stub` runs an app the way the platform would, on one machine, with nothing but Postgres
and Node (for the Inngest dev server) installed:

```
go run github.com/whisk-run/contract/cmd/whisk-stub@latest --as you@example.com \
  --database-url postgres://... -- <app command>
```

It reads `whisk.yaml`, injects the production environment (`environment.md`), runs the migrate
command, starts the app, and serves three listeners:

| Listener | Stands in for | Does |
|---|---|---|
| `:3000` | `<app>.<org>.whisk.page` | strips inbound `X-Whisk-*`, public and private routes, sign-in via `/.whisk/login`, identity headers for `--as`, CSRF, Altcha challenge, security headers, cookies bound to the app with `__Host-` and others kept from it, request ids |
| `:3001` | `api.whisk.run` and `hooks.whisk.run` | the queue endpoint, approvals (list and decide), webhook ingress with the presets, stored events and replay, and the workflow API (proxied to the Inngest dev server it starts) |
| `:3002` | the internal listener | platform deliveries and app-to-app calls with the service identity |

`GET :3001/v1/stub` describes the run: ids, URLs and the service token. Secrets come from
`.whisk/dev/secrets.env`. Not emulated: rate limits, previews, storage, email, key-value, custom
domains, sleep and wake. The CLI's `whisk dev` is this code with Postgres and the dev server
managed for you.

## Contributing

Presets come with a fixture directory under `fixtures/webhooks/<preset>/` holding a real
delivery and its expected verdict. Doctor rules come with a triggering and a passing fixture.
Template changes must keep `canary.test` passing. Everything here is MIT.
