import assert from "node:assert/strict";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { test } from "node:test";
import { domains, PlatformError } from "../src/whisk.js";

// The domains helper calls the app's own routes with the service token, decodes a domain and its
// records, and turns the platform's error into a PlatformError with its code and details.
test("domains helper", async () => {
  const seen: string[] = [];
  const server = createServer((req, res) => {
    seen.push(`${req.method} ${req.url} ${req.headers.authorization}`);
    res.setHeader("content-type", "application/json");
    if (req.method === "POST" && req.url === "/v1/orgs/o1/apps/a1/domains") {
      res.writeHead(201).end(JSON.stringify({ id: "d1", hostname: "results.lab.example", kind: "custom", verified: false, cert_status: "pending", status: "pending_dns", added_by: "app",
        records: [{ type: "TXT", name: "_whisk-verify.results.lab.example", value: "whisk-verify-1" }, { type: "CNAME", name: "results.lab.example", value: "results--acme.whisk.page" }], created_at: "2026-10-09T00:00:00Z" }));
    } else if (req.method === "GET") {
      res.writeHead(200).end(JSON.stringify({ items: [{ id: "d1", hostname: "results.lab.example", kind: "custom", status: "pending_dns" }] }));
    } else if (req.url?.endsWith("/verify")) {
      res.writeHead(409).end(JSON.stringify({ error: { code: "DOMAIN_UNVERIFIED", message: "not yet", fix: "add the records", details: { missing: ["TXT"] } } }));
    } else {
      res.writeHead(204).end();
    }
  });
  await new Promise<void>((ok) => server.listen(0, "127.0.0.1", ok));
  const { port } = server.address() as AddressInfo;
  process.env.WHISK_QUEUE_URL = `http://127.0.0.1:${port}/v1/orgs/o1/apps/a1/events`;
  process.env.WHISK_SERVICE_TOKEN = "svc";
  try {
    const d = await domains.add("results.lab.example");
    assert.equal(d.id, "d1");
    assert.equal(d.status, "pending_dns");
    assert.equal(d.added_by, "app");
    assert.deepEqual(d.records?.map((r) => r.type), ["TXT", "CNAME"]);
    assert.deepEqual((await domains.list()).map((x) => x.hostname), ["results.lab.example"]);
    await assert.rejects(domains.verify("d1"), (e: unknown) => e instanceof PlatformError && e.code === "DOMAIN_UNVERIFIED" && e.status === 409 && Array.isArray(e.details.missing));
    await domains.remove("d1");
    assert.deepEqual(seen, [
      "POST /v1/orgs/o1/apps/a1/domains Bearer svc",
      "GET /v1/orgs/o1/apps/a1/domains Bearer svc",
      "POST /v1/orgs/o1/apps/a1/domains/d1/verify Bearer svc",
      "DELETE /v1/orgs/o1/apps/a1/domains/d1 Bearer svc",
    ]);
  } finally {
    server.close();
  }
});
