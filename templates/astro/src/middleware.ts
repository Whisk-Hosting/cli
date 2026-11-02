// Every request passes through here: one server span continuing the edge's traceparent, so a
// trace's id is the request id (whisk.ts starts tracing when the platform sets
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT); one JSON log line carrying the request id; and any error
// sent to Sentry when the platform sets SENTRY_DSN, tagged with the request id so it links to
// its trace. whisk dev sets neither variable.
import { context, propagation, SpanKind, SpanStatusCode, trace } from "@opentelemetry/api";
import * as Sentry from "@sentry/node";
import { defineMiddleware } from "astro:middleware";
import { sql } from "./db.js";
import { identity, log, tracing } from "./whisk.js";

if (process.env.SENTRY_DSN) {
  Sentry.init({ dsn: process.env.SENTRY_DSN, environment: process.env.WHISK_ENV });
}

export const onRequest = defineMiddleware(async (ctx, next) => {
  const started = Date.now();
  const id = identity((n) => ctx.request.headers.get(n) ?? undefined);
  const line = { request_id: id.requestId, method: ctx.request.method, path: ctx.url.pathname, user: id.email };
  const parent = propagation.extract(context.active(), Object.fromEntries(ctx.request.headers));
  const name = `${ctx.request.method} ${ctx.routePattern || ctx.url.pathname}`;
  return trace.getTracer("whisk").startActiveSpan(name, { kind: SpanKind.SERVER, attributes: { "http.request.method": ctx.request.method, "url.path": ctx.url.pathname, "http.route": ctx.routePattern } }, parent, async (span) => {
    try {
      const res = await next();
      span.setAttribute("http.response.status_code", res.status);
      if (res.status >= 500) span.setStatus({ code: SpanStatusCode.ERROR });
      log.info({ ...line, status: res.status, ms: Date.now() - started }, "request");
      return res;
    } catch (err) {
      span.recordException(err as Error);
      span.setStatus({ code: SpanStatusCode.ERROR });
      Sentry.captureException(err, { tags: { request_id: id.requestId } });
      log.error({ ...line, status: 500, ms: Date.now() - started, err: err instanceof Error ? err.message : String(err) }, "unhandled error");
      throw err;
    } finally {
      span.end();
    }
  });
});

// SIGTERM: requests are given a moment to finish, then the pool closes, spans and errors are
// flushed, and the process exits within the grace period.
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
