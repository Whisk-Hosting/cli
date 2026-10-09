import assert from "node:assert/strict";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { test } from "node:test";
import { signMedia } from "../src/whisk.js";

// signMedia asks the app's own uploads/links endpoint with the service token for the paths
// given, and answers the platform's links, or throws its refusal.
test("signMedia asks the platform for signed links with the service token", async () => {
  let asked: { paths: string[]; expires_in?: number } | undefined;
  const srv = createServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      if (req.url !== "/v1/orgs/o/apps/a/uploads/links" || req.headers.authorization !== "Bearer whsk_service_test") {
        res.writeHead(404).end();
        return;
      }
      asked = JSON.parse(body);
      if (asked?.paths[0] === "/notes") {
        res.writeHead(400, { "content-type": "application/json" }).end('{"error":{"code":"INVALID_REQUEST"}}');
        return;
      }
      res.writeHead(200, { "content-type": "application/json" }).end(
        '{"expires_at":"2026-10-09T05:00:00Z","links":[{"id":"01UP","path":"/.whisk/img/01UP?w=800&exp=1&kid=1&sig=s"}]}',
      );
    });
  });
  await new Promise<void>((resolve) => srv.listen(0, "127.0.0.1", resolve));
  const { port } = srv.address() as AddressInfo;
  process.env.WHISK_QUEUE_URL = `http://127.0.0.1:${port}/v1/orgs/o/apps/a/events`;
  process.env.WHISK_SERVICE_TOKEN = "whsk_service_test";
  try {
    const out = await signMedia(["/.whisk/img/01UP?w=800"], 1800);
    assert.equal(out.expiresAt, "2026-10-09T05:00:00Z");
    assert.equal(out.links[0].id, "01UP");
    assert.match(out.links[0].path, /sig=s$/);
    assert.deepEqual(asked, { paths: ["/.whisk/img/01UP?w=800"], expires_in: 1800 });
    await assert.rejects(signMedia(["/notes"]), /INVALID_REQUEST/);
  } finally {
    srv.close();
  }
});
