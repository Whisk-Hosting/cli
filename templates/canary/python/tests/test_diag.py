"""POST /diag/call's helper: one POST to another app's internal name with this app's token."""

import io
import urllib.error
import urllib.request
from email.message import Message

import pytest

from app import diag


class _Resp(io.BytesIO):
    def __init__(self, status: int, body: bytes, headers: dict[str, str]):
        super().__init__(body)
        self.status = status
        self.headers = Message()
        for k, v in headers.items():
            self.headers[k] = v

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()


def test_diag_post(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("WHISK_SERVICE_TOKEN", "whsk_service_test")
    seen: list[urllib.request.Request] = []

    def refuse(req, timeout):
        seen.append(req)
        raise urllib.error.HTTPError(req.full_url, 401, "Unauthorized", _Resp(401, b"", {"X-Whisk-Request-Id": "01REQ"}).headers, io.BytesIO(b'{"error":{"code":"DELIVERY_UNVERIFIED"}}'))

    monkeypatch.setattr(urllib.request, "urlopen", refuse)
    out = diag.diag_post("01TARGET", "/hooks/stripe", {"X-Whisk-Webhook-Id": "01FAKE"}, "{}")
    req = seen[0]
    assert req.full_url == "http://01TARGET.internal.whisk:8443/hooks/stripe"
    assert req.get_method() == "POST" and req.data == b"{}"
    assert req.get_header("Authorization") == "Bearer whsk_service_test"
    assert req.get_header("X-whisk-webhook-id") == "01FAKE"
    assert out == {"status": 401, "body": '{"error":{"code":"DELIVERY_UNVERIFIED"}}', "headers": {"x-whisk-request-id": "01REQ"}}

    monkeypatch.setattr(urllib.request, "urlopen", lambda req, timeout: _Resp(200, b"ok", {"Content-Type": "text/plain"}))
    assert diag.diag_post("01TARGET", "/x") == {"status": 200, "body": "ok", "headers": {"content-type": "text/plain"}}

    def unreachable(req, timeout):
        raise urllib.error.URLError("Name or service not known")

    monkeypatch.setattr(urllib.request, "urlopen", unreachable)
    failed = diag.diag_post("01TARGET", "/x")
    assert failed is not None and failed["status"] == 0 and "not known" in failed["error"]

    assert diag.diag_post(None, "/x") is None
    assert diag.diag_post("a", "x") is None
