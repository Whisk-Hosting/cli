// The template's trust boundary (CONTRACT.md §7): identity() reads any X-Whisk-* header values
// without throwing and trusts only exact role names; deliveries.verify() never throws on any
// headers or body, accepts exactly the signature the delivery key makes for that id, time and
// body, and refuses anything else: a changed byte, another key, a service-app call, a missing
// header.

import { createHmac } from "node:crypto";

import { FuzzedDataProvider } from "@jazzer.js/core";

import { deliveries, hasRole, identity } from "../../src/whisk.js";

const KEY = "t/XIoLwasp+aMSftgZasZlBv9QaR0yaNJQmq+JgCphc="; // gitleaks:allow: the public test vector, not a secret
const VERDICTS = new Set(["", "no_key", "service_app", "missing", "mismatch"]);
const ROLES = ["admin", "owner", "member", "", " admin", "Admin", "constructor", "__proto__"];

const check = (ok: boolean, property: string, input: unknown): void => {
  if (!ok) throw new Error(`${property}: ${JSON.stringify(input).slice(0, 400)}`);
};

const sign = (key: string, id: string, at: string, body: Uint8Array): string =>
  "v1=" + createHmac("sha256", Buffer.from(key, "base64")).update(`${id}\n${at}\n`).update(body).digest("hex");

// A header value as HTTP carries it: Latin-1, no line breaks or NUL.
const header = (s: string): string => s.replace(/[^ -ÿ\t]/g, "");

export const fuzz = (data: Buffer): void => {
  const p = new FuzzedDataProvider(data);

  const values = Object.fromEntries(["audience", "user-id", "email", "name", "org", "groups", "roles", "request-id"].map((h) => [`x-whisk-${h}`, p.consumeBoolean() ? p.consumeString(40) : undefined]));
  const id = identity((n) => values[n]);
  check(Array.isArray(id.groups) && Array.isArray(id.roles) && typeof id.requestId === "string", "an identity has lists and a request id", values);
  check(id.roles.every((r) => r !== "") && id.groups.every((g) => g !== ""), "no empty role or group", values);
  const role = p.pickValue(ROLES);
  check(hasRole(id, role) === (role !== "" && (values["x-whisk-roles"] ?? "").split(",").includes(role)), "a role is held only by its exact name", { values, role });

  process.env.WHISK_DELIVERY_KEY = p.consumeIntegralInRange(0, 15) === 0 ? p.pickValue(["", "not base64", "=="]) : KEY;
  const delivery = {
    id: p.consumeBoolean() ? header(p.consumeString(30)) : "01J8Z3WQ9XN4M6P8R0T2V4X6Y8",
    at: p.consumeBoolean() ? header(p.consumeString(30)) : "2026-09-23T10:15:30Z",
    service: p.consumeIntegralInRange(0, 7) === 0 ? header(p.consumeString(10)) : undefined,
  };
  const body = Uint8Array.from(p.consumeBytes(p.consumeIntegralInRange(0, 256)));
  const genuine = sign(KEY, delivery.id, delivery.at, body);
  const shape = p.consumeIntegralInRange(0, 4);
  // The signature: the genuine one, one with a byte changed, one under another key, or any text.
  const sig =
    shape === 0
      ? genuine
      : shape === 1
        ? genuine.slice(0, -1) + (genuine.endsWith("0") ? "1" : "0")
        : shape === 2
          ? sign(Buffer.from(p.consumeBytes(32)).toString("base64") || "AA==", delivery.id, delivery.at, body)
          : p.consumeRemainingAsString();
  const headers: Record<string, string | undefined> = {
    "x-whisk-webhook-id": delivery.id,
    "x-whisk-webhook-received-at": delivery.at,
    "x-whisk-delivery-signature": sig,
    "x-whisk-service-app": delivery.service,
  };
  let why: string;
  try {
    why = deliveries.verify((n) => headers[n], body);
  } catch (err) {
    throw new Error(`deliveries.verify threw ${String(err)}: ${JSON.stringify(headers).slice(0, 300)}`);
  }
  check(VERDICTS.has(why), "the verdict is one of five", { headers, why });
  const keyed = Buffer.from(process.env.WHISK_DELIVERY_KEY ?? "", "base64");
  const expected = keyed.length > 0 ? sign(process.env.WHISK_DELIVERY_KEY ?? "", delivery.id, delivery.at, body) : undefined;
  const accepted = why === "";
  check(accepted === (expected !== undefined && !delivery.service && delivery.id !== "" && delivery.at !== "" && sig === expected), "accepted exactly when signed under the key", { headers, why, body: Buffer.from(body).toString("base64") });
};
