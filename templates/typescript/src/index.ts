import { serve as nodeServe } from "@hono/node-server";
import { httpInstrumentationMiddleware } from "@hono/otel";
import * as Sentry from "@sentry/node";
import { desc, eq } from "drizzle-orm";
import { Hono } from "hono";
import { html } from "hono/html";
import { serve as inngestServe } from "inngest/hono";
import { dbFor, notes, sql } from "./db.js";
import { functions } from "./functions.js";
import { canChange, canSee, enqueue, env, identity, inngest, log, scopeFor, tracing } from "./whisk.js";

// Errors only: Sentry leaves OpenTelemetry to the tracer in whisk.ts.
if (process.env.SENTRY_DSN) Sentry.init({ dsn: process.env.SENTRY_DSN, environment: process.env.WHISK_ENV });

const app = new Hono();
const appName = env("WHISK_APP_NAME", "whisk-typescript");

// Every request: a server span continuing the edge's traceparent (a no-op without tracing).
app.use("*", httpInstrumentationMiddleware());

// Every request: one JSON log line carrying the request id the platform gave it.
app.use("*", async (c, next) => {
  const started = Date.now();
  await next();
  const id = identity((n) => c.req.header(n));
  log.info({ request_id: id.requestId, method: c.req.method, path: c.req.path, status: c.res.status, user: id.email, ms: Date.now() - started }, "request");
});

const who = (c: { req: { header: (n: string) => string | undefined } }) => identity((n) => c.req.header(n));

// Public routes (routes.public in whisk.yaml): "/" and "/health". Everything else is private:
// the platform signs people in before a request reaches it.
app.get("/", (c) => {
  const id = who(c);
  const greeting = id.audience === "anonymous" ? "You are not signed in." : `Signed in as ${id.name || id.email}.`;
  return c.html(html`<!doctype html><title>${appName}</title>
<main style="font-family:system-ui;max-width:40rem;margin:4rem auto">
<h1>${appName}</h1><p>${greeting}</p>
<p><a href="/notes">Notes</a> · <a href="/.whisk/login?return=/">Sign in</a> · <a href="/.whisk/logout?return=/">Sign out</a></p></main>`);
});
app.get("/health", async (c) => {
  try {
    await sql`select 1`;
    return c.json({ ok: true });
  } catch (err) {
    return c.json({ ok: false, error: String(err) }, 503);
  }
});

// Each note belongs to its author (authorId). The team sees every note; a customer, when the app
// has customer_identity, sees only their own (whisk.ts scopeFor), and the table's row-level
// security keeps to the same rule should a query forget it (dbFor).
app.get("/notes", async (c) => {
  const id = who(c);
  const scope = scopeFor(id);
  if (scope.kind === "none") return c.json([]);
  const mine = scope.kind === "owner" ? eq(notes.authorId, scope.ownerId) : undefined;
  return c.json(await dbFor(id, (tx) => tx.select().from(notes).where(mine).orderBy(desc(notes.id)).limit(100)));
});
app.post("/notes", async (c) => {
  const id = who(c);
  const authorId = id.userId;
  if (!authorId) return c.json({ error: "a signed-in person is required" }, 403);
  const text = ((await c.req.json().catch(() => ({}))) as { body?: string }).body?.trim();
  if (!text) return c.json({ error: "body is required" }, 400);
  const [note] = await dbFor(id, (tx) => tx.insert(notes).values({ authorId, authorEmail: id.email ?? "", body: text }).returning());
  // Hand the slow part to a function. A preview runs no functions, so it sends no events.
  if (process.env.WHISK_QUEUE_URL && !(process.env.WHISK_ENV ?? "").startsWith("preview:")) {
    await enqueue("note.added", { note_id: note.id }, `note-${note.id}`).catch((err: unknown) =>
      log.warn({ request_id: id.requestId, note_id: note.id, err: String(err) }, "note.added not sent"),
    );
  }
  return c.json(note, 201);
});
app.delete("/notes/:id", async (c) => {
  const id = who(c);
  const noteId = Number(c.req.param("id"));
  if (!Number.isSafeInteger(noteId) || noteId < 1 || noteId > 2147483647) return c.json({ error: "no such note" }, 404);
  return dbFor(id, async (tx) => {
    const [note] = await tx.select().from(notes).where(eq(notes.id, noteId));
    // A note the person may not see is answered as if it did not exist.
    if (!note || !canSee(id, note.authorId)) return c.json({ error: "no such note" }, 404);
    if (!canChange(id, note.authorId)) return c.json({ error: "only its author or an owner or admin can delete it" }, 403);
    await tx.delete(notes).where(eq(notes.id, note.id));
    return c.body(null, 204);
  });
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
