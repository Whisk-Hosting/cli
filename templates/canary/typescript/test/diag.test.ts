import assert from "node:assert/strict";
import { test } from "node:test";
import { diagPost } from "../src/diag.js";

test("diagPost posts to the internal name with the service token and reports the answer", async () => {
  process.env.WHISK_SERVICE_TOKEN = "whsk_service_test";
  const realFetch = globalThis.fetch;
  let seen: { url: string; init: RequestInit } | undefined;
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    seen = { url, init };
    return new Response('{"error":{"code":"DELIVERY_UNVERIFIED"}}', { status: 401, headers: { "x-whisk-request-id": "01REQ" } });
  }) as typeof fetch;
  try {
    const out = await diagPost({ app_id: "01TARGET", path: "/hooks/stripe", headers: { "X-Whisk-Webhook-Id": "01FAKE" }, body: "{}" });
    assert.equal(seen?.url, "http://01TARGET.internal.whisk:8443/hooks/stripe");
    assert.equal(seen?.init.method, "POST");
    assert.equal(seen?.init.body, "{}");
    const sent = seen?.init.headers as Record<string, string>;
    assert.equal(sent.authorization, "Bearer whsk_service_test");
    assert.equal(sent["X-Whisk-Webhook-Id"], "01FAKE");
    assert.deepEqual(out, { status: 401, body: '{"error":{"code":"DELIVERY_UNVERIFIED"}}', headers: { "content-type": "text/plain;charset=UTF-8", "x-whisk-request-id": "01REQ" } });

    globalThis.fetch = (async () => { throw new Error("getaddrinfo ENOTFOUND"); }) as typeof fetch;
    const failed = await diagPost({ app_id: "01TARGET", path: "/hooks/stripe" });
    assert.equal(failed?.status, 0);
    assert.match(String((failed as { error: string }).error), /ENOTFOUND/);
  } finally {
    globalThis.fetch = realFetch;
  }
  assert.equal(await diagPost({ path: "/x" }), null);
  assert.equal(await diagPost({ app_id: "a", path: "x" }), null);
});
