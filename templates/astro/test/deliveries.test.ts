import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { Hono } from "hono";
import { deliveries, type Delivery, type RecordFn } from "../src/whisk.js";

// The signed delivery from contract/fixtures/deliveries when the contract sits beside this
// template, otherwise the same vector inline: a copy of the template must still test alone.
const inline = {
  key: "t/XIoLwasp+aMSftgZasZlBv9QaR0yaNJQmq+JgCphc=", // gitleaks:allow: the public test vector, not a secret
  signature: "v1=1d94b46d0053ffcb73494700dd0a91ea76a7a92bf735be3abbefbaaf802e9191",
  headers: {
    "X-Whisk-Webhook-Id": "01J8Z3WQ9XN4M6P8R0T2V4X6Y8", "X-Whisk-Webhook-Source": "stripe", "X-Whisk-Webhook-Received-At": "2026-09-23T10:15:30Z",
    "X-Whisk-Audience": "service", "Content-Type": "application/json",
  } as Record<string, string>,
  body: '{"type":"invoice.paid","id":"in_123"}',
};
const dir = join(import.meta.dirname, "..", "..", "..", "..", "contract", "fixtures", "deliveries");
const fixture = existsSync(dir)
  ? {
      key: readFileSync(join(dir, "key.b64"), "utf8").trim(),
      signature: readFileSync(join(dir, "signature"), "utf8").trim(),
      headers: JSON.parse(readFileSync(join(dir, "headers.json"), "utf8")) as Record<string, string>,
      body: readFileSync(join(dir, "body"), "utf8"),
    }
  : inline;

// The provider's own signature header, which the platform forwards with the
// x-whisk-webhook-orig- prefix.
const origSig = "t=1790000130,v1=0f7f0c2b6f7a3c1e4d5b6a7988776655443322110099aabbccddeeff00112233";

// An insert-once map standing in for recordEvent.
const memoryRecord = (seen: Map<string, { payload: unknown; verified: boolean }>): RecordFn => async (id, kind, payload, verified) => {
  if (seen.has(id)) return { id, kind, duplicate: true };
  seen.set(id, { payload, verified });
  return { id, kind, duplicate: false };
};

test("deliveries.handle verifies, refuses, records and dedupes", async () => {
  process.env.WHISK_DELIVERY_KEY = fixture.key;
  const seen = new Map<string, { payload: unknown; verified: boolean }>();
  const got: Delivery[] = [];
  const app = new Hono().post("/hooks/stripe", deliveries.handle(memoryRecord(seen), (d) => { got.push(d); }));
  const send = (mut: (h: Headers) => void, body = fixture.body) => {
    const headers = new Headers({ ...fixture.headers, "X-Whisk-Delivery-Signature": fixture.signature, "X-Whisk-Webhook-Orig-Stripe-Signature": origSig });
    mut(headers);
    return app.request("/hooks/stripe", { method: "POST", headers, body });
  };
  const refused = async (name: string, mut: (h: Headers) => void, body?: string) => {
    const res = await send(mut, body);
    const text = await res.text();
    assert.equal(res.status, 401, `${name}: ${res.status} ${text}`);
    const parsed = JSON.parse(text) as { error: { code: string; message: string; fix: string } };
    assert.equal(parsed.error.code, "DELIVERY_UNVERIFIED", name);
    assert.ok(parsed.error.message && parsed.error.fix, name);
  };
  await refused("wrong signature", (h) => h.set("X-Whisk-Delivery-Signature", "v1=" + "0".repeat(64)));
  await refused("altered body", () => {}, " " + fixture.body);
  await refused("missing signature", (h) => h.delete("X-Whisk-Delivery-Signature"));
  await refused("service app present", (h) => h.set("X-Whisk-Service-App", "01OTHERAPP"));
  assert.equal(got.length, 0, "a refused delivery reached the app");
  assert.equal(seen.size, 0, "a refused delivery reached the store");

  const ok = await send(() => {});
  assert.equal(ok.status, 200);
  assert.deepEqual(await ok.json(), { id: fixture.headers["X-Whisk-Webhook-Id"], kind: "webhook", duplicate: false });
  assert.equal(got.length, 1);
  const d = got[0]!;
  assert.equal(d.id, fixture.headers["X-Whisk-Webhook-Id"]);
  assert.equal(d.source, "stripe");
  assert.equal(d.receivedAt, fixture.headers["X-Whisk-Webhook-Received-At"]);
  assert.equal(Buffer.from(d.body).toString("utf8"), fixture.body);
  assert.equal(d.headers["stripe-signature"], origSig);
  assert.equal(seen.get(d.id)?.verified, true, "recorded row is not marked verified");

  const dup = await send(() => {});
  assert.equal(dup.status, 200);
  assert.deepEqual(await dup.json(), { id: d.id, kind: "webhook", duplicate: true });
  assert.equal(got.length, 1, "a duplicate id ran the app's function again");

  process.env.WHISK_DELIVERY_KEY = "";
  await refused("no key", () => {});
});
