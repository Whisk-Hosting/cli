"""The Whisk conventions in one file: identity from headers, JSON logging with the request id,
tracing, the Inngest client wired to the platform, the approval helper and the webhook
deliveries helper. Copy this file as-is."""

import base64
import binascii
import datetime as dt
import hashlib
import hmac
import inspect
import json
import logging
import os
import sys
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable, Mapping

import inngest
import structlog
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.sqlalchemy import SQLAlchemyInstrumentor
from opentelemetry.instrumentation.urllib import URLLibInstrumentor
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from sqlalchemy import Engine


def env(name: str, default: str | None = None) -> str:
    value = os.environ.get(name, default)
    if value is None:
        raise RuntimeError(f"{name} is not set")
    return value


structlog.configure(
    processors=[structlog.processors.add_log_level, structlog.processors.TimeStamper(fmt="iso"), structlog.processors.JSONRenderer()],
    wrapper_class=structlog.make_filtering_bound_logger(logging.INFO),
    logger_factory=structlog.PrintLoggerFactory(sys.stdout),
)
log = structlog.get_logger().bind(app=os.environ.get("WHISK_APP_NAME"))


# Tracing starts when the platform sets OTEL_EXPORTER_OTLP_TRACES_ENDPOINT (whisk dev does not);
# the provider and exporter read the other OTEL_* variables themselves. urllib calls are traced.
# Call tracing.shutdown() on the way out to flush spans.
tracing = TracerProvider() if os.environ.get("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") else None
if tracing:
    tracing.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
    trace.set_tracer_provider(tracing)
    URLLibInstrumentor().instrument()


def trace_app(app: FastAPI, engine: Engine) -> None:
    """Server spans for the app's requests, continuing the edge's traceparent so a trace's id is
    the request id, and client spans for the engine's SQL. Nothing without tracing."""
    if tracing:
        FastAPIInstrumentor.instrument_app(app, tracer_provider=tracing, exclude_spans=["receive", "send"])
        SQLAlchemyInstrumentor().instrument(engine=engine, tracer_provider=tracing)


@dataclass(frozen=True)
class Identity:
    """Who the platform says is calling. Only the X-Whisk-* headers are trusted."""

    audience: str = "anonymous"
    user_id: str | None = None
    email: str | None = None
    name: str | None = None
    org: str | None = None
    groups: tuple[str, ...] = field(default_factory=tuple)
    roles: tuple[str, ...] = field(default_factory=tuple)
    request_id: str = ""

    def has_role(self, *roles: str) -> bool:
        return any(r in self.roles for r in roles)


def _list(value: str | None) -> tuple[str, ...]:
    return tuple(p for p in (value or "").split(",") if p)


def identity(headers: Mapping[str, str]) -> Identity:
    h = {k.lower(): v for k, v in headers.items()}
    return Identity(
        audience=h.get("x-whisk-audience", "anonymous"),
        user_id=h.get("x-whisk-user-id"),
        email=h.get("x-whisk-email"),
        name=h.get("x-whisk-name"),
        org=h.get("x-whisk-org"),
        groups=_list(h.get("x-whisk-groups")),
        roles=_list(h.get("x-whisk-roles")),
        request_id=h.get("x-whisk-request-id", ""),
    )


def whisk_headers(headers: Mapping[str, str]) -> dict[str, str]:
    return {k.lower(): v for k, v in headers.items() if k.lower().startswith("x-whisk-")}


# The platform delivers runs to queue.endpoint signed with WHISK_INNGEST_SIGNING_KEY; events
# sent from inside a function go to WHISK_INNGEST_URL. Event names are plain everywhere
# ("po.created"): the platform keeps each app's events to itself.

client = inngest.Inngest(
    app_id=env("WHISK_APP_NAME", "whisk-python"),
    event_key=os.environ.get("WHISK_INNGEST_EVENT_KEY"),
    signing_key=os.environ.get("WHISK_INNGEST_SIGNING_KEY"),
    api_base_url=os.environ.get("WHISK_INNGEST_URL"),
    event_api_base_url=os.environ.get("WHISK_INNGEST_URL"),
    is_production=os.environ.get("WHISK_DEV") != "1",
)


def approval(step: inngest.StepSync, run_id: str, name: str, *, to: str, title: str, data: Any = None, timeout: dt.timedelta = dt.timedelta(days=7)) -> dict[str, Any]:
    """Pause the run until a human decides. Sends whisk/approval.requested in a step named
    `<name>/request`, waits in a step named `name` for whisk/approval.decided with the same
    approval id. Returns {approval_id, decision, actor, at, note}; decision is expired on timeout."""
    approval_id = f"{run_id}:{name}"
    step.send_event(f"{name}/request", inngest.Event(name="whisk/approval.requested", data={"approval_id": approval_id, "to": to, "title": title, "data": data, "run_id": run_id}))
    decided = step.wait_for_event(name, event="whisk/approval.decided", if_exp=f'async.data.approval_id == "{approval_id}"', timeout=timeout)
    return dict(decided.data) if decided else {"approval_id": approval_id, "decision": "expired"}


def enqueue(name: str, data: Any, dedupe_key: str | None = None) -> dict[str, Any]:
    """Send an event through the platform queue with the service token."""
    body = json.dumps({"name": name, "data": data, "dedupe_key": dedupe_key}).encode()
    req = urllib.request.Request(env("WHISK_QUEUE_URL"), data=body, method="POST", headers={"Content-Type": "application/json", "Authorization": f"Bearer {env('WHISK_SERVICE_TOKEN')}"})
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.loads(resp.read())


@dataclass(frozen=True)
class Delivery:
    """One webhook delivery, proven to be the platform's: the id to dedupe on, the source name
    from the manifest, when the platform received it, the provider's raw body and the provider's
    own headers (the x-whisk-webhook-orig- prefix removed)."""

    id: str
    source: str
    received_at: str
    body: bytes
    headers: Mapping[str, str]


RecordFn = Callable[[str, str, Any, bool], dict[str, Any]]


def _delivery_signature(key: bytes, id: str, received_at: str, body: bytes) -> str:
    """"v1=" + hex(HMAC-SHA256(key, id "\\n" received_at "\\n" body))."""
    return "v1=" + hmac.new(key, f"{id}\n{received_at}\n".encode() + body, hashlib.sha256).hexdigest()


class _Deliveries:
    """Wraps a webhook handler (CONTRACT.md section 7): reads the raw body, checks
    X-Whisk-Delivery-Signature under WHISK_DELIVERY_KEY in constant time, refuses an app-to-app
    call (X-Whisk-Service-App) or an unsigned request with 401 DELIVERY_UNVERIFIED, records the
    delivery with verified=True and dedupes on X-Whisk-Webhook-Id, and only then runs the app's
    function. A repeat (retry, replay) answers the recorded row without running it."""

    @staticmethod
    def verify(headers: Mapping[str, str], body: bytes) -> str:
        """"" when the request is a platform delivery, else why not: no_key, service_app, missing, mismatch."""
        h = {k.lower(): v for k, v in headers.items()}
        try:
            key = base64.b64decode(os.environ.get("WHISK_DELIVERY_KEY", ""), validate=True)
        except (binascii.Error, ValueError):
            key = b""
        id, received_at, sig = h.get("x-whisk-webhook-id", ""), h.get("x-whisk-webhook-received-at", ""), h.get("x-whisk-delivery-signature", "")
        if not key:
            return "no_key"
        if h.get("x-whisk-service-app"):
            return "service_app"
        if not id or not received_at or not sig.startswith("v1="):
            return "missing"
        # Compared as bytes: a header can carry Latin-1 characters, and compare_digest refuses a
        # str that is not ASCII.
        expected = _delivery_signature(key, id, received_at, body)
        return "" if hmac.compare_digest(sig.encode("utf-8", "surrogateescape"), expected.encode()) else "mismatch"

    @staticmethod
    def handle(record: RecordFn) -> Callable[[Callable[[Delivery], Any]], Callable[[Request], Awaitable[Any]]]:
        """Decorator for a FastAPI webhook handler: `@app.post(path)` over `@deliveries.handle(record_event)`
        over `async def fn(delivery: Delivery)`. fn runs once per delivery id; an exception answers
        500 so the platform retries."""

        def wrap(fn: Callable[[Delivery], Any]) -> Callable[[Request], Awaitable[Any]]:
            async def handler(request: Request) -> Any:
                body = await request.body()
                why = _Deliveries.verify(request.headers, body)
                if why:
                    return JSONResponse({"error": {"code": "DELIVERY_UNVERIFIED", "message": f"This request is not a signed platform delivery ({why}).", "fix": "Only the platform delivers to a webhook handler, signing each delivery under WHISK_DELIVERY_KEY. Send the webhook through the source URL, not to the handler directly."}}, status_code=401)
                orig = {k.lower()[len("x-whisk-webhook-orig-"):]: v for k, v in request.headers.items() if k.lower().startswith("x-whisk-webhook-orig-")}
                delivery = Delivery(id=request.headers["x-whisk-webhook-id"], source=request.headers.get("x-whisk-webhook-source", ""), received_at=request.headers["x-whisk-webhook-received-at"], body=body, headers=orig)
                try:
                    payload = json.loads(body)
                except ValueError:
                    payload = {"raw": "not json"}
                rec = record(delivery.id, "webhook", {"source": delivery.source, "payload": payload}, True)
                if not rec["duplicate"]:
                    result = fn(delivery)
                    if inspect.isawaitable(result):
                        await result
                return rec

            handler.__name__ = getattr(fn, "__name__", "delivery_handler")
            return handler

        return wrap


deliveries = _Deliveries()
