"""The domains helper calls the app's own routes with the service token, decodes a domain and its
records, and turns the platform's error into a PlatformError with its code and details."""

import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest

from app.whisk import PlatformError, domains

DOMAIN = {
    "id": "d1",
    "hostname": "results.lab.example",
    "kind": "custom",
    "verified": False,
    "cert_status": "pending",
    "status": "pending_dns",
    "added_by": "app",
    "records": [
        {"type": "TXT", "name": "_whisk-verify.results.lab.example", "value": "whisk-verify-1"},
        {"type": "CNAME", "name": "results.lab.example", "value": "results--acme.whisk.page"},
    ],
    "created_at": "2026-10-09T00:00:00Z",
}


def test_domains_helper(monkeypatch: pytest.MonkeyPatch) -> None:
    seen: list[str] = []

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args: object) -> None:
            pass

        def answer(self, status: int, body: object | None) -> None:
            seen.append(f"{self.command} {self.path} {self.headers.get('Authorization')}")
            raw = b"" if body is None else json.dumps(body).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def do_GET(self) -> None:
            self.answer(200, {"items": [DOMAIN]})

        def do_POST(self) -> None:
            self.rfile.read(int(self.headers.get("Content-Length") or 0))
            if self.path.endswith("/verify"):
                self.answer(409, {"error": {"code": "DOMAIN_UNVERIFIED", "message": "not yet", "fix": "add the records", "details": {"missing": ["TXT"]}}})
            else:
                self.answer(201, DOMAIN)

        def do_DELETE(self) -> None:
            self.answer(204, None)

    server = HTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        monkeypatch.setenv("WHISK_QUEUE_URL", f"http://127.0.0.1:{server.server_port}/v1/orgs/o1/apps/a1/events")
        monkeypatch.setenv("WHISK_SERVICE_TOKEN", "svc")
        d = domains.add("results.lab.example")
        assert d["id"] == "d1" and d["status"] == "pending_dns" and d["added_by"] == "app"
        assert [r["type"] for r in d["records"]] == ["TXT", "CNAME"]
        assert [x["hostname"] for x in domains.list()] == ["results.lab.example"]
        with pytest.raises(PlatformError) as e:
            domains.verify("d1")
        assert e.value.code == "DOMAIN_UNVERIFIED" and e.value.status == 409 and e.value.details["missing"] == ["TXT"]
        domains.remove("d1")
        assert seen == [
            "POST /v1/orgs/o1/apps/a1/domains Bearer svc",
            "GET /v1/orgs/o1/apps/a1/domains Bearer svc",
            "POST /v1/orgs/o1/apps/a1/domains/d1/verify Bearer svc",
            "DELETE /v1/orgs/o1/apps/a1/domains/d1 Bearer svc",
        ]
    finally:
        server.shutdown()
