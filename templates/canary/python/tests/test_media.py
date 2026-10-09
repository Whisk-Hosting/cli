"""sign_media asks the app's own uploads/links endpoint with the service token for the paths
given, and answers the platform's links, or raises its refusal."""

import json
import threading
import urllib.error
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest

from app.whisk import sign_media


class _Platform(BaseHTTPRequestHandler):
    asked: dict = {}

    def do_POST(self):  # noqa: N802
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        if self.path != "/v1/orgs/o/apps/a/uploads/links" or self.headers["Authorization"] != "Bearer whsk_service_test":
            self.send_response(404)
            self.end_headers()
            return
        _Platform.asked = body
        if body["paths"][0] == "/notes":
            self.send_response(400)
            self.end_headers()
            self.wfile.write(b'{"error":{"code":"INVALID_REQUEST"}}')
            return
        out = {"expires_at": "2026-10-09T05:00:00Z", "links": [{"id": "01UP", "path": "/.whisk/img/01UP?w=800&exp=1&kid=1&sig=s"}]}
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(out).encode())

    def log_message(self, *args):
        pass


def test_sign_media(monkeypatch):
    srv = HTTPServer(("127.0.0.1", 0), _Platform)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    try:
        monkeypatch.setenv("WHISK_QUEUE_URL", f"http://127.0.0.1:{srv.server_port}/v1/orgs/o/apps/a/events")
        monkeypatch.setenv("WHISK_SERVICE_TOKEN", "whsk_service_test")
        out = sign_media(["/.whisk/img/01UP?w=800"], expires_in=1800)
        assert out["links"][0]["id"] == "01UP"
        assert out["links"][0]["path"].endswith("sig=s")
        assert _Platform.asked == {"paths": ["/.whisk/img/01UP?w=800"], "expires_in": 1800}
        with pytest.raises(urllib.error.HTTPError) as refused:
            sign_media(["/notes"])
        assert refused.value.code == 400
    finally:
        srv.shutdown()
