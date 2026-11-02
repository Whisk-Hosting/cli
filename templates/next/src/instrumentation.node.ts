// Started once in the Node.js server, before any request. whisk.ts starts tracing when the
// platform sets OTEL_EXPORTER_OTLP_TRACES_ENDPOINT: Next.js traces each request itself into it,
// continuing the edge's traceparent so a trace's id is the request id, and fetch and SQL are
// traced beneath. Sentry reports errors only and leaves tracing to whisk.ts.
import * as Sentry from "@sentry/node";
import { sql } from "./db";
import { log, tracing } from "./whisk";

if (process.env.SENTRY_DSN) {
  Sentry.init({ dsn: process.env.SENTRY_DSN, environment: process.env.WHISK_ENV });
}

// SIGTERM: Next.js leaves it to the app (NEXT_MANUAL_SIG_HANDLE in the Dockerfile). Requests are
// given a moment to finish, then the pool closes, spans and errors are flushed, and the process
// exits within the grace period.
const shutdown = () => {
  log.info("shutting down");
  setTimeout(async () => {
    await Promise.allSettled([sql.end({ timeout: 5 }), tracing?.shutdown(), Sentry.flush(2000)]);
    process.exit(0);
  }, 3000).unref();
  setTimeout(() => process.exit(1), (Number(process.env.WHISK_STOP_GRACE ?? 28) - 2) * 1000).unref();
};
process.once("SIGTERM", shutdown);
process.once("SIGINT", shutdown);
