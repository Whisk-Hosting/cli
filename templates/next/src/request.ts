import type { NextRequest } from "next/server";
import { identity, type Identity, log } from "./whisk";

// who is the person a request comes from, as the platform's X-Whisk-* headers say.
export const who = (req: Request): Identity => identity((n) => req.headers.get(n) ?? undefined);

type Handler<P> = (req: NextRequest, ctx: { params: Promise<P> }) => Promise<Response>;

// logged wraps a route handler so every request writes one JSON line carrying the request id the
// platform gave it. An error is logged and thrown on, so Next.js answers 500 and reports it
// (instrumentation.ts onRequestError).
export const logged = <P = Record<string, never>>(fn: Handler<P>): Handler<P> => async (req, ctx) => {
  const started = Date.now();
  const id = who(req);
  const line = { request_id: id.requestId, method: req.method, path: req.nextUrl.pathname, user: id.email };
  try {
    const res = await fn(req, ctx);
    log.info({ ...line, status: res.status, ms: Date.now() - started }, "request");
    return res;
  } catch (err) {
    log.error({ ...line, status: 500, ms: Date.now() - started, err: err instanceof Error ? err.message : String(err) }, "unhandled error");
    throw err;
  }
};
