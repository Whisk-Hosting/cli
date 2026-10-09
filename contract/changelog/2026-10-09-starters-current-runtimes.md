# The starters build on current runtimes and libraries

New apps start on the newest stable versions, so the code an agent writes targets them:

- Python starter and canary: `python:3.14-slim` (3.13 now gets security fixes only), uv 0.12,
  SQLAlchemy 2.1 and the newest FastAPI, Alembic, Pydantic, Uvicorn, psycopg, Sentry and
  OpenTelemetry releases.
- Go starter and canary: built with `golang:1.27-alpine` and run on
  `gcr.io/distroless/static-debian13` (the debian12 images no longer get updates). Dependencies
  move to their newest releases, among them pgx 5.11, Inngest's Go SDK 0.16.5 and
  OpenTelemetry 1.47. The module still needs only Go 1.26.4.
- TypeScript starter and canary: `@sentry/node` 11, esbuild 0.28, ioredis 6 in the canary, and
  the newest Hono, Drizzle, Inngest, AWS SDK and OpenTelemetry releases. Sentry 11 no longer sets
  up OpenTelemetry unless asked, so `Sentry.init` drops `skipOpenTelemetrySetup` and the tracer
  in `whisk.ts` stays the only one.

The skill names `python:3.14-slim` as the Python starter's image.
