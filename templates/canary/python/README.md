# whisk-python

A complete Whisk app: FastAPI on Python 3.12+, SQLAlchemy with Alembic on
Postgres, the Inngest SDK for functions, structlog for logs. It is the Python reference for the
conventions in `contract/SKILL.md` and the platform canary
the harness and nightly checks deploy.

What it shows: identity from the `X-Whisk-*` headers (`/`, `/whoami`, `/me`); public and
private routes; `/health`; a `notes` table with a migration; a cron function, an event function
and an approval function with their graphs in `workflows/`; a Stripe webhook handler at
`/hooks/stripe` that dedupes on the webhook id; JSON logs carrying the request id; Sentry;
OpenTelemetry traces whose id is the request id; graceful shutdown; a non-root Dockerfile.

```
app/main.py        routes, logging
app/whisk.py       identity, log, tracing, inngest client, approval helper (copy this into any app)
app/functions.py   nightly-summary (cron), canary-event, canary-approval
app/db.py          models and engine · alembic/ migrations
```

## Run it

With the CLI, from this folder: `whisk dev --as you@example.com -- uvicorn app.main:app --port 8080` (Docker with the compose
plugin). `whisk init --template py` writes the plainer starter in `templates/python`, not this
canary.

Without the CLI, with Postgres and uv available:

```
uv sync
mkdir -p .whisk/dev && printf 'STRIPE_WEBHOOK_SECRET=%s\nCANARY_SECRET=canary-value\n' "$STRIPE_SIGNING_SECRET" > .whisk/dev/secrets.env
PATH="$PWD/.venv/bin:$PATH" go run github.com/whisk-run/contract/cmd/whisk-stub@latest --as you@example.com \
  --database-url postgres://user:pass@localhost:5432/app -- uvicorn app.main:app --host 0.0.0.0 --port 8080
```

Open http://127.0.0.1:3000, sign in from the link, write a note at `/notes`. The stub prints
the webhook URL and the API it serves; `canary.test` exercises every convention against it:

```
sh canary.test http://127.0.0.1:3000 http://127.0.0.1:3001 <org id> <app id>
```

with `CANARY_SERVICE_TOKEN` and `CANARY_STRIPE_SECRET` set (see the script header; the stub
shows both at `GET http://127.0.0.1:3001/v1/stub`).

## Ship it

`whisk deploy`. Set `STRIPE_WEBHOOK_SECRET` when the deploy asks; paste the printed webhook URL
into Stripe. Migrations run from `alembic upgrade head` before traffic switches.
