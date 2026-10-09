"""The identity and deliveries helpers against the Big List of Naughty Strings from
contract/naughty when the contract sits beside this template, otherwise a few of its worst
inline: a copy of the template must still test alone."""

import json
from pathlib import Path

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.whisk import deliveries, identity

BLNS = Path(__file__).resolve().parents[4] / "contract" / "naughty" / "blns.json"
BASE: list[str] = (
    json.loads(BLNS.read_text())
    if BLNS.is_file()
    else ["", "undefined", "__proto__", "<script>alert(1)</script>", "' OR 1=1 --", "%s%n", "‮", "😍", "../../etc/passwd"]
)
NAUGHTY = [*BASE, "", " ", "\x00", "a\x00b", "\r\n", "constructor", "admin", "a" * 70000]

VERDICTS = {"", "no_key", "service_app", "missing", "mismatch"}
KEY = "t/XIoLwasp+aMSftgZasZlBv9QaR0yaNJQmq+JgCphc="  # gitleaks:allow: the public test vector, not a secret
WHEN = "2026-09-23T10:15:30Z"


def test_identity_reads_any_header_and_trusts_only_exact_roles() -> None:
    for s in NAUGHTY:
        names = ["x-whisk-audience", "x-whisk-email", "x-whisk-name", "x-whisk-org", "x-whisk-groups", "x-whisk-roles", "x-whisk-request-id"]
        who = identity({n: s for n in names})
        assert who.email == s and who.name == s
        assert "" not in who.groups and "" not in who.roles
        assert who.has_role("admin") == ("admin" in s.split(","))
        assert not who.has_role("")


def test_verify_answers_a_verdict_and_never_accepts_a_forgery(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("WHISK_DELIVERY_KEY", KEY)
    for s in NAUGHTY:
        cases = [
            {"x-whisk-webhook-id": s, "x-whisk-webhook-received-at": s, "x-whisk-delivery-signature": s},
            {"x-whisk-webhook-id": "01J8", "x-whisk-webhook-received-at": WHEN, "x-whisk-delivery-signature": f"v1={s}"},
            # Header values arrive as Latin-1, so a raw byte 0xE9 reads as "é".
            {"x-whisk-webhook-id": "01J8", "x-whisk-webhook-received-at": WHEN, "x-whisk-delivery-signature": f"v1={s}".ljust(67, "é")[:67]},
            {"x-whisk-webhook-id": "01J8", "x-whisk-webhook-received-at": WHEN, "x-whisk-delivery-signature": "v1=" + "0" * 64, "x-whisk-service-app": s},
        ]
        for headers in cases:
            why = deliveries.verify(headers, s.encode("utf-8", "surrogatepass"))
            assert why in VERDICTS
            assert why != "", f"accepted a forged delivery: {str(headers)[:80]}"


def test_handle_refuses_a_forgery_with_401_not_500(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("WHISK_DELIVERY_KEY", KEY)

    async def record(id: str, kind: str, payload: object, verified: bool) -> dict[str, object]:
        return {"id": id, "kind": kind, "duplicate": False}

    app = FastAPI()

    @app.post("/hooks/x")
    @deliveries.handle(record)
    async def hook(delivery: object) -> None:
        raise AssertionError("a forged delivery ran the app's function")

    client = TestClient(app, raise_server_exceptions=False)
    for sig in ["v1=" + "é" * 64, "v1=" + "0" * 63 + "é", "v1=ÿ" + "0" * 63]:
        headers = {"x-whisk-webhook-id": "01J8", "x-whisk-webhook-received-at": WHEN, "x-whisk-delivery-signature": sig.encode("latin-1")}
        res = client.post("/hooks/x", headers=headers, content=b"{}")  # type: ignore[arg-type]
        assert res.status_code == 401, f"{sig[:8]}: {res.status_code}"
