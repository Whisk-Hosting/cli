"""The deliveries helper against the signed delivery in contract/fixtures/deliveries when the
contract sits beside this template, otherwise the same vector inline: a copy of the template
must still test alone."""

import json
from pathlib import Path
from typing import Any

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.whisk import Delivery, deliveries

INLINE = {
    "key": "t/XIoLwasp+aMSftgZasZlBv9QaR0yaNJQmq+JgCphc=",  # gitleaks:allow: the public test vector, not a secret
    "signature": "v1=1d94b46d0053ffcb73494700dd0a91ea76a7a92bf735be3abbefbaaf802e9191",
    "headers": {
        "X-Whisk-Webhook-Id": "01J8Z3WQ9XN4M6P8R0T2V4X6Y8",
        "X-Whisk-Webhook-Source": "stripe",
        "X-Whisk-Webhook-Received-At": "2026-09-23T10:15:30Z",
        "X-Whisk-Audience": "service",
        "Content-Type": "application/json",
    },
    "body": b'{"type":"invoice.paid","id":"in_123"}',
}
FIXTURE_DIR = Path(__file__).resolve().parents[3] / "contract" / "fixtures" / "deliveries"
# The provider's own signature header, which the platform forwards with the x-whisk-webhook-orig- prefix.
ORIG_SIG = "t=1790000130,v1=0f7f0c2b6f7a3c1e4d5b6a7988776655443322110099aabbccddeeff00112233"


def fixture() -> dict[str, Any]:
    if not FIXTURE_DIR.is_dir():
        return INLINE
    return {
        "key": (FIXTURE_DIR / "key.b64").read_text().strip(),
        "signature": (FIXTURE_DIR / "signature").read_text().strip(),
        "headers": json.loads((FIXTURE_DIR / "headers.json").read_text()),
        "body": (FIXTURE_DIR / "body").read_bytes(),
    }


def memory_record(seen: dict[str, dict[str, Any]]):
    """An insert-once dict standing in for record_event."""

    def record(id: str, kind: str, payload: Any, verified: bool) -> dict[str, Any]:
        if id in seen:
            return {"id": id, "kind": kind, "duplicate": True}
        seen[id] = {"payload": payload, "verified": verified}
        return {"id": id, "kind": kind, "duplicate": False}

    return record


def test_handle_verifies_refuses_records_and_dedupes(monkeypatch: pytest.MonkeyPatch) -> None:
    f = fixture()
    monkeypatch.setenv("WHISK_DELIVERY_KEY", f["key"])
    seen: dict[str, dict[str, Any]] = {}
    got: list[Delivery] = []
    app = FastAPI()

    @app.post("/hooks/stripe")
    @deliveries.handle(memory_record(seen))
    async def hook(delivery: Delivery) -> None:
        got.append(delivery)

    client = TestClient(app)

    def send(mutate=None, body: bytes | None = None):
        headers = {**f["headers"], "X-Whisk-Delivery-Signature": f["signature"], "X-Whisk-Webhook-Orig-Stripe-Signature": ORIG_SIG}
        if mutate:
            mutate(headers)
        return client.post("/hooks/stripe", content=f["body"] if body is None else body, headers=headers)

    def refused(name: str, mutate=None, body: bytes | None = None) -> None:
        res = send(mutate, body)
        assert res.status_code == 401, f"{name}: {res.status_code} {res.text}"
        err = res.json()["error"]
        assert err["code"] == "DELIVERY_UNVERIFIED" and err["message"] and err["fix"], name

    refused("wrong signature", lambda h: h.update({"X-Whisk-Delivery-Signature": "v1=" + "0" * 64}))
    refused("altered body", None, b" " + f["body"])
    refused("missing signature", lambda h: h.pop("X-Whisk-Delivery-Signature"))
    refused("service app present", lambda h: h.update({"X-Whisk-Service-App": "01OTHERAPP"}))
    assert not got and not seen, "a refused delivery reached the app or the store"

    ok = send()
    assert ok.status_code == 200, ok.text
    assert ok.json() == {"id": f["headers"]["X-Whisk-Webhook-Id"], "kind": "webhook", "duplicate": False}
    assert len(got) == 1
    d = got[0]
    assert d.id == f["headers"]["X-Whisk-Webhook-Id"]
    assert d.source == "stripe"
    assert d.received_at == f["headers"]["X-Whisk-Webhook-Received-At"]
    assert d.body == f["body"]
    assert d.headers["stripe-signature"] == ORIG_SIG
    assert seen[d.id]["verified"] is True, "recorded row is not marked verified"

    dup = send()
    assert dup.status_code == 200 and dup.json()["duplicate"] is True
    assert len(got) == 1, "a duplicate id ran the app's function again"

    monkeypatch.setenv("WHISK_DELIVERY_KEY", "")
    refused("no key")
