import { serve as nodeServe } from "@hono/node-server";
import { httpInstrumentationMiddleware } from "@hono/otel";
import * as Sentry from "@sentry/node";
import { desc, eq, sql as rawSql } from "drizzle-orm";
import { Hono } from "hono";
import { serve as inngestServe } from "inngest/hono";
import { connect } from "node:net";
import { db, events, notes, recordEvent, sql } from "./db.js";
import { functions } from "./functions.js";
import { diagCall, diagPG, diagPost, diagRead, diagReport, diagShare } from "./diag.js";
import { diagKV } from "./kv.js";
import { diagSync } from "./sync.js";
import { deliveries, enqueue, env, hasRole, identity, inngest, log, tracing } from "./whisk.js";

// Errors only: when tracing is on, OpenTelemetry belongs to the tracer in whisk.ts.
if (process.env.SENTRY_DSN) Sentry.init({ dsn: process.env.SENTRY_DSN, environment: process.env.WHISK_ENV, skipOpenTelemetrySetup: !!tracing });

const app = new Hono();

// Every request: a server span continuing the edge's traceparent (a no-op without tracing).
app.use("*", httpInstrumentationMiddleware());

// Every request: identity from the platform headers, one JSON log line with the request id.
app.use("*", async (c, next) => {
  const started = Date.now();
  await next();
  const id = identity((n) => c.req.header(n));
  log.info({ request_id: id.requestId, method: c.req.method, path: c.req.path, status: c.res.status, audience: id.audience, user: id.email, ms: Date.now() - started }, "request");
});

const who = (c: { req: { header: (n: string) => string | undefined } }) => identity((n) => c.req.header(n));
const whiskHeaders = (c: { req: { raw: Request } }) =>
  Object.fromEntries([...c.req.raw.headers.entries()].filter(([k]) => k.startsWith("x-whisk-")));

// Public routes (routes.public in whisk.yaml): "/", "/health", "/whoami".
app.get("/", (c) => {
  const id = who(c);
  const greeting = id.audience === "anonymous" ? "You are not signed in." : `Signed in as ${id.name || id.email} (${id.audience}, roles: ${id.roles.join(", ") || "none"}).`;
  return c.html(`<!doctype html><title>${env("WHISK_APP_NAME", "whisk-typescript")}</title><main style="font-family:system-ui;max-width:40rem;margin:4rem auto">
<h1>${env("WHISK_APP_NAME", "whisk-typescript")}</h1><p>${greeting}</p>
<p><a href="/me">/me</a> · <a href="/notes">/notes</a> · <a href="/.whisk/login?return=/">sign in</a> · <a href="/.whisk/logout?return=/">sign out</a></p></main>`);
});
app.get("/health", async (c) => {
  try {
    await sql`select 1`;
    return c.json({ ok: true });
  } catch (err) {
    return c.json({ ok: false, error: String(err) }, 503);
  }
});
app.get("/whoami", (c) => c.json(whiskHeaders(c)));

// Private routes: the platform has already signed the caller in.
app.get("/me", (c) => c.json(whiskHeaders(c)));
app.get("/notes", async (c) => c.json(await db.select().from(notes).orderBy(desc(notes.id)).limit(100)));
app.post("/notes", async (c) => {
  const id = who(c);
  if (!id.userId) return c.json({ error: "a signed-in person is required" }, 403);
  const body = (await c.req.json().catch(() => ({}))) as { body?: string };
  if (!body.body?.trim()) return c.json({ error: "body is required" }, 400);
  const [note] = await db.insert(notes).values({ authorId: id.userId, authorEmail: id.email ?? "", body: body.body.trim() }).returning();
  return c.json(note, 201);
});
app.delete("/notes/:id", async (c) => {
  if (!hasRole(who(c), "owner", "admin")) return c.json({ error: "owner or admin role required" }, 403);
  await db.delete(notes).where(eq(notes.id, Number(c.req.param("id"))));
  return c.body(null, 204);
});
app.get("/events", async (c) => {
  const kind = c.req.query("kind");
  const rows = await db.select().from(events).where(kind ? eq(events.kind, kind) : rawSql`true`).orderBy(desc(events.createdAt)).limit(100);
  return c.json(rows);
});

// Webhook handler: deliveries.handle proves the delivery is the platform's, records it as
// verified and dedupes on the webhook id before this function runs (CONTRACT.md §7).
app.post("/hooks/stripe", deliveries.handle(recordEvent, (d, c) => {
  log.info({ request_id: who(c).requestId, webhook_id: d.id, bytes: d.body.length }, "stripe delivery");
}));

// Diagnostics used by the platform canary; private, and safe to delete in your own app.
app.get("/diag", (c) => c.json(diagReport()));
app.get("/diag/call", async (c) => c.json(await diagCall(c.req.query("to") ?? "", c.req.query("token") !== "none", c.req.query("via"))));
app.post("/diag/call", async (c) => {
  const out = await diagPost(await c.req.json().catch(() => ({})));
  return out ? c.json(out) : c.json({ error: "a JSON body with app_id and a path starting with / is required" }, 400);
});
// Sends one event through WHISK_QUEUE_URL with the app's own service token, so the platform can
// prove an app enqueues its own events.
app.post("/diag/enqueue", async (c) => {
  const body = await c.req.json().catch(() => ({}));
  if (typeof body?.name !== "string" || body.name === "") return c.json({ error: "a JSON body with a name is required" }, 400);
  try {
    return c.json(await enqueue(body.name, body.data ?? {}, body.dedupe_key));
  } catch (err) {
    return c.json({ error: String(err instanceof Error ? err.message : err) }, 502);
  }
});
app.get("/diag/pg", async (c) => c.json(await diagPG(c.req.query("role"), c.req.query("database"))));
app.post("/diag/pg/share", async (c) => {
  const out = await diagShare(await c.req.json().catch(() => ({})));
  return out ? c.json(out) : c.json({ error: "a JSON body with to (an app role) and marker is required" }, 400);
});
app.get("/diag/pg/read", async (c) => {
  const out = await diagRead(c.req.query("schema"), c.req.query("marker"));
  return out ? c.json(out) : c.json({ error: "schema (an app role) and marker are required" }, 400);
});
app.get("/diag/connect", async (c) => {
  const [host, port] = (c.req.query("to") ?? "").split(":");
  const connected = await new Promise<boolean>((resolve) => {
    const s = connect({ host, port: Number(port), timeout: 2000 });
    s.once("connect", () => { s.destroy(); resolve(true); });
    s.once("error", () => resolve(false));
    s.once("timeout", () => { s.destroy(); resolve(false); });
  });
  return c.json({ to: `${host}:${port}`, connected });
});
app.get("/diag/secret", (c) => c.json({ name: "CANARY_SECRET", set: !!process.env.CANARY_SECRET, length: process.env.CANARY_SECRET?.length ?? 0 }));
// Deliberately prints the secret so the platform's log scrubbing can be proven: the line
// reaches the log store as [REDACTED:CANARY_SECRET].
app.get("/diag/log-secret", (c) => {
  const value = process.env.CANARY_SECRET ?? "";
  log.info({ value }, "canary secret log line");
  return c.json({ logged: true, length: value.length });
});
app.get("/diag/kv", async (c) => {
  const ttl = Number(c.req.query("ttl") ?? "");
  return c.json(await diagKV(c.req.query("key") ?? "diag", c.req.query("value"), Number.isFinite(ttl) ? ttl : undefined));
});
app.get("/diag/sync", async (c) => {
  const [status, out] = await diagSync(c.req.query("tree") ?? "diag", c.req.query("file") ?? "diag.txt", c.req.query("value"));
  return c.json(out, status);
});
app.get("/diag/boom", () => { throw new Error("deliberate exception for error tracking"); });
// A cacheable answer: the edge answers a repeat for 60 seconds without waking the app.
// With ?cookie=1 it also sets a cookie, which keeps it out of the edge cache.
app.get("/diag/cache", (c) => {
  c.header("Cache-Control", "private, max-age=60");
  if (c.req.query("cookie") === "1") c.header("Set-Cookie", "diag_cache=1; Path=/; HttpOnly; Secure; SameSite=Lax");
  return c.json({ at: process.hrtime.bigint().toString() });
});

// Function runs arrive here from the platform with the service identity (queue.endpoint).
const inngestHandler = inngestServe({ client: inngest, functions });
app.on(["GET", "POST", "PUT"], "/.whisk/inngest", (c) => inngestHandler(c));

app.onError((err, c) => {
  Sentry.captureException(err, { tags: { request_id: who(c).requestId } });
  log.error({ request_id: who(c).requestId, err: err.message }, "unhandled error");
  return c.json({ error: "internal error", request_id: who(c).requestId }, 500);
});

const server = nodeServe({ fetch: app.fetch, port: Number(env("PORT", "8080")), hostname: "0.0.0.0" }, (info) =>
  log.info({ port: info.port }, "listening"),
);

// SIGTERM: stop accepting, finish in-flight requests, close the pool, flush spans, exit within
// the grace period.
const shutdown = () => {
  log.info("shutting down");
  server.close(async () => { await sql.end({ timeout: 5 }); await tracing?.shutdown(); process.exit(0); });
  setTimeout(() => process.exit(1), (Number(process.env.WHISK_STOP_GRACE ?? 28) - 2) * 1000).unref();
};
process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
