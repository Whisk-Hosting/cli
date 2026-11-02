// Next.js runs register() once when the server starts, and onRequestError for every error a
// route, page or server action throws. Errors go to Sentry when the platform sets SENTRY_DSN,
// each tagged with the request id so it links to its trace; whisk dev sets neither variable.
export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") await import("./instrumentation.node");
}

export async function onRequestError(err: unknown, request: { path: string; method: string; headers: Record<string, string | string[] | undefined> }) {
  if (process.env.NEXT_RUNTIME !== "nodejs" || !process.env.SENTRY_DSN) return;
  const Sentry = await import("@sentry/node");
  Sentry.captureException(err, { tags: { request_id: String(request.headers["x-whisk-request-id"] ?? "") } });
}
