# whisk-typescript

A complete Whisk app: Hono on Node 24, Drizzle on Postgres, the Inngest SDK
for functions, pino for logs. It is the TypeScript reference for the conventions in
`contract/SKILL.md` and the platform canary
the harness and nightly checks deploy.

What it shows: identity from the `X-Whisk-*` headers (`/`, `/whoami`, `/me`); public and
private routes; `/health`; a `notes` table with a migration; a cron function, an event function
and an approval function with their graphs in `workflows/`; a Stripe webhook handler at
`/hooks/stripe` that dedupes on the webhook id; JSON logs carrying the request id; Sentry;
OpenTelemetry traces whose id is the request id; graceful shutdown; a build that bundles the app
into one file per entry point; a non-root Dockerfile on distroless Node, with no shell and no
`node_modules` in the image.

```
src/index.ts       routes, logging, shutdown
src/whisk.ts       identity, log, tracing, inngest client, approval helper (copy this into any app)
src/functions.ts   nightly-summary (cron), canary-event, canary-approval
src/schema.ts      tables · src/db.ts client · src/migrate.ts runs drizzle/
```

## Run it

With the CLI, from this folder: `whisk dev --as you@example.com -- npm run dev` (Docker with the compose
plugin). `whisk init --template ts` writes the plainer starter in `templates/typescript`, not this
canary.

Without the CLI, with Postgres and Node 22 available:

```
npm install && npm run build
mkdir -p .whisk/dev && printf 'STRIPE_WEBHOOK_SECRET=%s\nCANARY_SECRET=canary-value\n' "$STRIPE_SIGNING_SECRET" > .whisk/dev/secrets.env
go run github.com/whisk-run/contract/cmd/whisk-stub@latest --as you@example.com \
  --database-url postgres://user:pass@localhost:5432/app -- npm start
```

Open http://127.0.0.1:3000, sign in from the link, write a note at `/notes`. The stub prints
the webhook URL and the API it serves; `canary.test` exercises every convention against it:

```
sh canary.test http://127.0.0.1:3000 http://127.0.0.1:3001 <org id> <app id>
```

with `CANARY_SERVICE_TOKEN` and `CANARY_STRIPE_SECRET` set (see the script header; the stub
shows both at `GET http://127.0.0.1:3001/v1/stub`).

## Test it

`npm test` runs the unit tests, then the fuzz target in `fuzz/targets/` once over its seeds.
`npm run fuzz` fuzzes it with Jazzer.js for `FUZZ_SECONDS` (default 30): identity headers and
the delivery signature check, which must never throw and never accept a forged delivery.

## Ship it

`whisk deploy`. Set `STRIPE_WEBHOOK_SECRET` when the deploy asks; paste the printed webhook URL
into Stripe. Migrations run from `node dist/migrate.js` before traffic switches.
