import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { Hono } from "hono";
import { deliveries, hasRole, identity, type RecordFn } from "../src/whisk.js";

// The Big List of Naughty Strings from contract/naughty when the contract sits beside this
// template, otherwise a few of its worst inline: a copy of the template must still test alone.
const blns = join(import.meta.dirname, "..", "..", "..", "..", "contract", "naughty", "blns.json");
const base: string[] = existsSync(blns)
  ? (JSON.parse(readFileSync(blns, "utf8")) as string[])
  : ["", "undefined", "__proto__", "<script>alert(1)</script>", "' OR 1=1 --", "%s%n", "‮", "Ω≈ç√∫", "😍", "../../etc/passwd"];
const naughty = [...base, "", " ", "\u0000", "a\u0000b", "\r\n", "\ud800", "constructor", "admin", "a".repeat(70000)];

const VERDICTS = ["", "no_key", "service_app", "missing", "mismatch"];
const KEY = "t/XIoLwasp+aMSftgZasZlBv9QaR0yaNJQmq+JgCphc="; // gitleaks:allow: the public test vector, not a secret

test("identity reads any header value without throwing and trusts only exact role names", () => {
  for (const s of naughty) {
    const id = identity((name) => (name.startsWith("x-whisk-") ? s : undefined));
    assert.equal(id.email, s);
    assert.equal(id.name, s);
    assert.ok(id.groups.every((g) => g !== ""), "an empty group");
    assert.ok(id.roles.every((r) => r !== ""), "an empty role");
    assert.equal(hasRole(id, "admin"), s.split(",").includes("admin"));
    assert.equal(hasRole(id, ""), false);
  }
  assert.deepEqual(identity(() => undefined).roles, []);
});

test("deliveries.verify answers one of its verdicts for any header or body, and never accepts one", () => {
  process.env.WHISK_DELIVERY_KEY = KEY;
  for (const s of naughty) {
    for (const headers of [
      { "x-whisk-webhook-id": s, "x-whisk-webhook-received-at": s, "x-whisk-delivery-signature": s },
      { "x-whisk-webhook-id": "01J8", "x-whisk-webhook-received-at": "2026-09-23T10:15:30Z", "x-whisk-delivery-signature": `v1=${s}` },
      // The same length in characters as a real signature, but not in bytes.
      { "x-whisk-webhook-id": "01J8", "x-whisk-webhook-received-at": "2026-09-23T10:15:30Z", "x-whisk-delivery-signature": `v1=${s}`.padEnd(67, "é").slice(0, 67) },
      { "x-whisk-webhook-id": "01J8", "x-whisk-webhook-received-at": "2026-09-23T10:15:30Z", "x-whisk-delivery-signature": "v1=" + "0".repeat(64), "x-whisk-service-app": s },
    ] as Record<string, string>[]) {
      const why = deliveries.verify((n) => headers[n], new TextEncoder().encode(s));
      assert.ok(VERDICTS.includes(why), `verdict ${why}`);
      assert.notEqual(why, "", `accepted a forged delivery: ${JSON.stringify(headers).slice(0, 80)}`);
    }
  }
});

test("deliveries.handle refuses a forged delivery with 401, never 500, whatever its headers carry", async () => {
  process.env.WHISK_DELIVERY_KEY = KEY;
  const record: RecordFn = async (id, kind) => ({ id, kind, duplicate: false });
  const app = new Hono().post("/hooks/x", deliveries.handle(record, () => assert.fail("a forged delivery ran the app's function")));
  // Header values arrive as Latin-1, so a raw byte 0xE9 in a signature reads as "é".
  for (const sig of ["v1=" + "é".repeat(64), "v1=" + "0".repeat(63) + "é", "v1=ÿ" + "0".repeat(63)]) {
    const res = await app.request("/hooks/x", { method: "POST", headers: { "x-whisk-webhook-id": "01J8", "x-whisk-webhook-received-at": "2026-09-23T10:15:30Z", "x-whisk-delivery-signature": sig }, body: "{}" });
    assert.equal(res.status, 401, `${sig.slice(0, 8)}: ${res.status}`);
  }
});
