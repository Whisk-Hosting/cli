// The Whisk conventions in one file: identity from headers, JSON logging with the request id,
// tracing, the Inngest client wired to the platform, the approval helper and the webhook
// deliveries helper. Copy this file as-is.
import { SpanKind, SpanStatusCode, trace } from "@opentelemetry/api";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-proto";
import { UndiciInstrumentation } from "@opentelemetry/instrumentation-undici";
import { defaultResource, detectResources, envDetector } from "@opentelemetry/resources";
import { BatchSpanProcessor, NodeTracerProvider } from "@opentelemetry/sdk-trace-node";
import type { Context } from "hono";
import { Inngest, type GetStepTools } from "inngest";
import { createHmac, timingSafeEqual } from "node:crypto";
import pino from "pino";

export const env = (name: string, fallback?: string): string => {
  const v = process.env[name] ?? fallback;
  if (v === undefined) throw new Error(`${name} is not set`);
  return v;
};

export const log = pino({ level: process.env.LOG_LEVEL ?? "info", base: { app: process.env.WHISK_APP_NAME } });

// Tracing starts when the platform sets OTEL_EXPORTER_OTLP_TRACES_ENDPOINT (whisk dev does not);
// the exporter reads the other OTEL_* variables itself. Requests are traced by the middleware
// in index.ts, which continues the edge's traceparent, so a trace's id is the request id.
// fetch calls are traced too, and SQL through traceQueries. Shut it down to flush spans.
export const tracing = process.env.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
  ? new NodeTracerProvider({ resource: defaultResource().merge(detectResources({ detectors: [envDetector] })), spanProcessors: [new BatchSpanProcessor(new OTLPTraceExporter())] })
  : undefined;
if (tracing) {
  tracing.register();
  new UndiciInstrumentation();
}

type Query = { then: (ok?: (v: unknown) => unknown, fail?: (e: unknown) => unknown) => Promise<unknown> };

// traceQueries makes each statement sent through sql.unsafe, which is how Drizzle sends SQL, a
// client span; postgres.js has no OpenTelemetry instrumentation of its own.
export const traceQueries = <T extends { unsafe: (...args: never[]) => unknown }>(sql: T): T => {
  if (!tracing) return sql;
  const unsafe = sql.unsafe.bind(sql) as (query: string, ...rest: unknown[]) => Query;
  const traced = (query: string, ...rest: unknown[]) => {
    const q = unsafe(query, ...rest);
    const run = q.then.bind(q);
    const name = query.trimStart().split(/\s/, 1)[0].toUpperCase();
    q.then = (ok, fail) => trace.getTracer("whisk").startActiveSpan(name, { kind: SpanKind.CLIENT, attributes: { "db.system.name": "postgresql", "db.query.text": query } }, (span) =>
      run((v) => { span.end(); return v; }, (e) => { span.recordException(e as Error); span.setStatus({ code: SpanStatusCode.ERROR }); span.end(); throw e; }).then(ok, fail));
    return q;
  };
  return Object.assign(sql, { unsafe: traced });
};

// Identity as delivered by the platform. Only the X-Whisk-* headers are trusted; nothing else
// on the request says who the caller is.
export type Identity = {
  audience: "anonymous" | "team" | "customer" | "service";
  userId?: string;
  email?: string;
  name?: string;
  org?: string;
  groups: string[];
  roles: string[];
  requestId: string;
};

const list = (v: string | undefined) => (v ?? "").split(",").filter(Boolean);

export const identity = (h: (name: string) => string | undefined): Identity => ({
  audience: (h("x-whisk-audience") as Identity["audience"]) ?? "anonymous",
  userId: h("x-whisk-user-id"),
  email: h("x-whisk-email"),
  name: h("x-whisk-name"),
  org: h("x-whisk-org"),
  groups: list(h("x-whisk-groups")),
  roles: list(h("x-whisk-roles")),
  requestId: h("x-whisk-request-id") ?? "",
});

export const hasRole = (id: Identity, ...roles: string[]) => roles.some((r) => id.roles.includes(r));

// Who may see and change a record (skill §4, "Who may see and change what"). A record belongs
// to the person who made it: store their X-Whisk-User-Id beside it as its owner. The business's
// own team sees every record; a customer sees only their own; anyone else sees nothing. Query
// with scopeFor so the database does the filtering, and answer 404, not 403, for a record the
// person may not see, so its id tells them nothing. test/access.test.ts checks these rules over
// thousands of generated people and records; keep it passing when you change them.
export type Scope = { kind: "all" } | { kind: "owner"; ownerId: string } | { kind: "none" };

export const scopeFor = (id: Identity): Scope => {
  if (!id.userId) return { kind: "none" };
  if (id.audience === "team") return { kind: "all" };
  if (id.audience === "customer") return { kind: "owner", ownerId: id.userId };
  return { kind: "none" };
};

export const inScope = (scope: Scope, ownerId: string) =>
  scope.kind === "all" || (scope.kind === "owner" && ownerId !== "" && scope.ownerId === ownerId);

export const canSee = (id: Identity, ownerId: string) => inScope(scopeFor(id), ownerId);

// Changing or deleting: a customer their own records; on the team, the record's owner or an
// owner or admin of the business.
export const canChange = (id: Identity, ownerId: string) =>
  canSee(id, ownerId) && (id.audience !== "team" || ownerId === id.userId || hasRole(id, "owner", "admin"));

// The Inngest client. The platform delivers runs to queue.endpoint and signs them with
// WHISK_INNGEST_SIGNING_KEY; events sent from inside a function go to WHISK_INNGEST_URL. Event
// names are plain everywhere ("po.created"): the platform keeps each app's events to itself.

export const inngest = new Inngest({
  id: env("WHISK_APP_NAME", "whisk-typescript"),
  eventKey: process.env.WHISK_INNGEST_EVENT_KEY,
  signingKey: process.env.WHISK_INNGEST_SIGNING_KEY,
  baseUrl: process.env.WHISK_INNGEST_URL,
  isDev: process.env.WHISK_DEV === "1",
  logger: log,
});

export type Step = GetStepTools<typeof inngest>;

export type Decision = {
  approval_id: string;
  decision: "approved" | "rejected" | "expired";
  actor?: { user_id: string; email: string; name: string };
  at?: string;
  note?: string;
};

// approval pauses the run until a human decides. It sends whisk/approval.requested in a step
// named `${name}/request` and waits in a step named `name` for whisk/approval.decided with the
// same approval id. The platform notifies `to` (an org role, group:<name> or user:<email>).
export const approval = async (
  step: Step,
  runId: string,
  name: string,
  req: { to: string; title: string; data?: unknown; timeout?: string },
): Promise<Decision> => {
  const approvalId = `${runId}:${name}`;
  await step.sendEvent(`${name}/request`, {
    name: "whisk/approval.requested",
    data: { approval_id: approvalId, to: req.to, title: req.title, data: req.data ?? null, run_id: runId },
  });
  const decided = await step.waitForEvent(name, {
    event: "whisk/approval.decided",
    if: `async.data.approval_id == "${approvalId}"`,
    timeout: req.timeout ?? "7d",
  });
  return decided ? (decided.data as Decision) : { approval_id: approvalId, decision: "expired" };
};

// enqueue sends an event through the platform queue with the service token.
export const enqueue = async (name: string, data: unknown, dedupeKey?: string): Promise<{ id: string }> => {
  const res = await fetch(env("WHISK_QUEUE_URL"), {
    method: "POST",
    headers: { "content-type": "application/json", authorization: `Bearer ${env("WHISK_SERVICE_TOKEN")}` },
    body: JSON.stringify({ name, data, dedupe_key: dedupeKey }),
  });
  if (!res.ok) throw new Error(`enqueue ${name}: ${res.status} ${await res.text()}`);
  return (await res.json()) as { id: string };
};

// Delivery is one webhook delivery, proven to be the platform's: the id to dedupe on, the
// source name from the manifest, when the platform received it, the provider's raw body and
// the provider's own headers (the x-whisk-webhook-orig- prefix removed).
export type Delivery = {
  id: string;
  source: string;
  receivedAt: string;
  body: Uint8Array;
  headers: Record<string, string>;
};

export type RecordFn = (id: string, kind: string, payload: unknown, verified: boolean) => Promise<{ id: string; kind: string; duplicate: boolean }>;

// "v1=" + hex(HMAC-SHA256(key, id "\n" receivedAt "\n" body)).
const deliverySignature = (key: Buffer, id: string, receivedAt: string, body: Uint8Array) =>
  "v1=" + createHmac("sha256", key).update(`${id}\n${receivedAt}\n`).update(body).digest("hex");

// Compared as bytes: a header can carry Latin-1 characters, so the same length in characters
// is not the same length in bytes, and timingSafeEqual throws on unequal lengths.
const constantTimeEqual = (a: string, b: string) => {
  const x = Buffer.from(a);
  const y = Buffer.from(b);
  return x.length === y.length && timingSafeEqual(x, y);
};

// deliveries wraps a webhook handler (CONTRACT.md §7): it reads the raw body, checks
// x-whisk-delivery-signature under WHISK_DELIVERY_KEY in constant time, refuses an app-to-app
// call (x-whisk-service-app) or an unsigned request with 401 DELIVERY_UNVERIFIED, records the
// delivery with verified: true and dedupes on x-whisk-webhook-id, and only then runs the app's
// function. A repeat (retry, replay) answers the recorded row without running it.
export const deliveries = {
  // verify answers "" when the request is a platform delivery, or why it is not: no_key,
  // service_app, missing, mismatch.
  verify: (header: (name: string) => string | undefined, body: Uint8Array): string => {
    const key = Buffer.from(process.env.WHISK_DELIVERY_KEY ?? "", "base64");
    const id = header("x-whisk-webhook-id") ?? "";
    const receivedAt = header("x-whisk-webhook-received-at") ?? "";
    const sig = header("x-whisk-delivery-signature") ?? "";
    if (key.length === 0) return "no_key";
    if (header("x-whisk-service-app")) return "service_app";
    if (!id || !receivedAt || !sig.startsWith("v1=")) return "missing";
    return constantTimeEqual(sig, deliverySignature(key, id, receivedAt, body)) ? "" : "mismatch";
  },
  // handle is the Hono handler for a webhook handler route. fn runs once per delivery id
  // with the typed event; an exception answers 500 so the platform retries.
  handle: (record: RecordFn, fn: (delivery: Delivery, c: Context) => Promise<void> | void) => async (c: Context) => {
    const body = new Uint8Array(await c.req.arrayBuffer());
    const why = deliveries.verify((n) => c.req.header(n), body);
    if (why) {
      return c.json({ error: { code: "DELIVERY_UNVERIFIED", message: `This request is not a signed platform delivery (${why}).`, fix: "Only the platform delivers to a webhook handler, signing each delivery under WHISK_DELIVERY_KEY. Send the webhook through the source URL, not to the handler directly." } }, 401);
    }
    const headers = Object.fromEntries([...c.req.raw.headers.entries()].filter(([k]) => k.startsWith("x-whisk-webhook-orig-")).map(([k, v]) => [k.slice("x-whisk-webhook-orig-".length), v]));
    const delivery: Delivery = { id: c.req.header("x-whisk-webhook-id")!, source: c.req.header("x-whisk-webhook-source") ?? "", receivedAt: c.req.header("x-whisk-webhook-received-at")!, body, headers };
    const text = Buffer.from(body).toString("utf8");
    const payload = ((): unknown => { try { return JSON.parse(text); } catch { return { raw: "not json" }; } })();
    const rec = await record(delivery.id, "webhook", { source: delivery.source, payload }, true);
    if (!rec.duplicate) await fn(delivery, c);
    return c.json(rec);
  },
};
