# templates

Specification: [docs/CONTRACT.md §11](../docs/CONTRACT.md). Read it before writing anything here.

Two sets of apps, one of each per language.

**Starters**, what `whisk init --template` writes (embedded in the CLI with `go generate` in
`cli/templates`). Each is the conventions embodied as working code and as little else: a home
page, `/health`, a `notes` table with its migration and routes, and one function on the queue
with its graph. No secrets, webhooks or schedules, so a deploy goes live with nothing for a person
to set up.

| Folder | Stack |
|---|---|
| `typescript/` | Hono on Node 24, Drizzle, Inngest SDK, pino |
| `python/` | FastAPI, SQLAlchemy and Alembic, Inngest SDK, structlog |
| `go/` | net/http with chi, pgx and goose, Inngest Go SDK, slog |

**Canaries**, in `canary/typescript`, `canary/python` and `canary/go`: the same stacks with every
platform feature wired in (a secret, a Stripe webhook, a nightly schedule, approvals, the key-value
store, storage) and the `/diag` routes the platform's harness and nightly checks drive. They
expose the same routes and functions, so one `canary.test` (identical in each folder) proves all
three. It runs against a deployed app or against `whisk-stub` from the contract; each README shows
the command. The canaries are not what a new app starts from.

MIT, published as its own repository once the platform is live. Nothing here may depend on
private code.
