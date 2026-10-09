import os
import socket
import time
from contextlib import asynccontextmanager
from typing import Any

import inngest.fast_api
import sentry_sdk
from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.responses import HTMLResponse, JSONResponse
from sqlalchemy import func, select, text
from sqlalchemy.orm import Session

from .db import Note, db_for, engine, list_events, record_event
from .functions import functions
from .diag import diag_call, diag_pg, diag_post, diag_read, diag_report, diag_share
from .kv import diag_kv
from .sync import diag_sync
from .whisk import Delivery, client, deliveries, enqueue, env, identity, log, trace_app, tracing, whisk_headers

if os.environ.get("SENTRY_DSN"):
    sentry_sdk.init(dsn=os.environ["SENTRY_DSN"], environment=os.environ.get("WHISK_ENV"))


@asynccontextmanager
async def lifespan(_: FastAPI):
    log.info("listening", port=os.environ.get("PORT"))
    yield
    engine.dispose()  # SIGTERM: uvicorn stops accepting, finishes in-flight requests, then this runs.
    if tracing:
        tracing.shutdown()  # flushes the spans not yet sent


app = FastAPI(lifespan=lifespan, docs_url=None, redoc_url=None)
trace_app(app, engine)
APP_NAME = env("WHISK_APP_NAME", "whisk-python")


@app.middleware("http")
async def log_requests(request: Request, call_next):
    """Every request: identity from the platform headers, one JSON log line with the request id."""
    started = time.monotonic()
    who = identity(request.headers)
    sentry_sdk.set_tag("request_id", who.request_id)  # an error links to its trace
    response = await call_next(request)
    log.info("request", request_id=who.request_id, method=request.method, path=request.url.path, status=response.status_code, audience=who.audience, user=who.email, ms=int((time.monotonic() - started) * 1000))
    return response


# Public routes (routes.public in whisk.yaml): "/", "/health", "/whoami".
@app.get("/", response_class=HTMLResponse)
def home(request: Request) -> str:
    who = identity(request.headers)
    greeting = "You are not signed in." if who.audience == "anonymous" else f"Signed in as {who.name or who.email} ({who.audience}, roles: {', '.join(who.roles) or 'none'})."
    return f"""<!doctype html><title>{APP_NAME}</title><main style="font-family:system-ui;max-width:40rem;margin:4rem auto">
<h1>{APP_NAME}</h1><p>{greeting}</p>
<p><a href="/me">/me</a> · <a href="/notes">/notes</a> · <a href="/.whisk/login?return=/">sign in</a> · <a href="/.whisk/logout?return=/">sign out</a></p></main>"""


@app.get("/health")
def health() -> Any:
    try:
        with engine.connect() as conn:
            conn.execute(text("select 1"))
        return {"ok": True}
    except Exception as exc:  # noqa: BLE001
        return JSONResponse({"ok": False, "error": str(exc)}, status_code=503)


@app.get("/whoami")
def whoami(request: Request) -> dict[str, str]:
    return whisk_headers(request.headers)


# Private routes: the platform has already signed the caller in.
@app.get("/me")
def me(request: Request) -> dict[str, str]:
    return whisk_headers(request.headers)


@app.get("/notes")
def list_notes(request: Request) -> list[dict[str, Any]]:
    with db_for(identity(request.headers)) as session:
        return [n.as_dict() for n in session.scalars(select(Note).order_by(Note.id.desc()).limit(100))]


@app.post("/notes", status_code=201)
async def create_note(request: Request) -> dict[str, Any]:
    who = identity(request.headers)
    if not who.user_id:
        raise HTTPException(403, "a signed-in person is required")
    body = await request.json() if request.headers.get("content-type", "").startswith("application/json") else {}
    text_ = str(body.get("body", "")).strip()
    if not text_:
        raise HTTPException(400, "body is required")
    with db_for(who) as session:
        note = Note(author_id=who.user_id, author_email=who.email or "", body=text_)
        session.add(note)
        session.flush()
        return note.as_dict()


@app.delete("/notes/{note_id}", status_code=204)
def delete_note(note_id: int, request: Request) -> Response:
    who = identity(request.headers)
    if not who.has_role("owner", "admin"):
        raise HTTPException(403, "owner or admin role required")
    with db_for(who) as session:
        session.delete(session.get(Note, note_id))
    return Response(status_code=204)


@app.get("/events")
def events(kind: str | None = None) -> list[dict[str, Any]]:
    return list_events(kind)


# Webhook handler: deliveries.handle proves the delivery is the platform's, records it as
# verified and dedupes on the webhook id before this function runs (CONTRACT.md section 7).
@app.post("/hooks/stripe")
@deliveries.handle(record_event)
def stripe_hook(delivery: Delivery) -> None:
    log.info("stripe delivery", webhook_id=delivery.id, bytes=len(delivery.body))


# Inbound email: each message to the app's address arrives as a delivery whose JSON body names
# the sender, the subject and where the original and attachments are stored (CONTRACT.md §7).
@app.post("/inbound/email")
@deliveries.handle(record_event)
def inbound_email(delivery: Delivery) -> None:
    log.info("email delivery", webhook_id=delivery.id, bytes=len(delivery.body))


# Diagnostics used by the platform canary; private, and safe to delete in your own app.
@app.get("/diag")
def diag() -> dict[str, Any]:
    return diag_report()


@app.get("/diag/rows")
def diag_rows(request: Request) -> dict[str, int]:
    """The notes the caller's row-level security lets through with no filter in the query,
    counted once as the caller (db_for) and once saying nothing about who is asking (a plain
    Session), which must be none."""
    count = select(func.count()).select_from(Note)
    with db_for(identity(request.headers)) as session:
        scoped = session.scalar(count) or 0
    with Session(engine) as session:
        unscoped = session.scalar(count) or 0
    return {"scoped": scoped, "unscoped": unscoped}


@app.get("/diag/call")
def diag_call_route(to: str, token: str | None = None, via: str | None = None) -> dict[str, Any]:
    return diag_call(to, with_token=token != "none", via=via)


@app.post("/diag/call")
async def diag_post_route(request: Request) -> Any:
    try:
        data = await request.json()
    except ValueError:
        data = None
    out = diag_post(data.get("app_id"), data.get("path"), data.get("headers"), data.get("body", "")) if isinstance(data, dict) else None
    if out is None:
        return JSONResponse({"error": "a JSON body with app_id and a path starting with / is required"}, status_code=400)
    return out


@app.post("/diag/enqueue")
async def diag_enqueue_route(request: Request) -> Any:
    """Send one event through WHISK_QUEUE_URL with the app's own service token, so the platform
    can prove an app enqueues its own events."""
    try:
        data = await request.json()
    except ValueError:
        data = None
    if not isinstance(data, dict) or not isinstance(data.get("name"), str) or not data["name"]:
        return JSONResponse({"error": "a JSON body with a name is required"}, status_code=400)
    try:
        return enqueue(data["name"], data.get("data") or {}, data.get("dedupe_key"))
    except Exception as err:  # noqa: BLE001 - the caller reads the queue's answer
        detail = err.read().decode(errors="replace") if hasattr(err, "read") else ""
        return JSONResponse({"error": f"{err} {detail}".strip()}, status_code=502)


@app.get("/diag/pg")
def diag_pg_route(role: str | None = None, database: str | None = None) -> dict[str, Any]:
    return diag_pg(role, database)


@app.post("/diag/pg/share")
async def diag_share_route(request: Request) -> Any:
    try:
        data = await request.json()
    except ValueError:
        data = None
    out = diag_share(data.get("to"), data.get("marker")) if isinstance(data, dict) else None
    if out is None:
        return JSONResponse({"error": "a JSON body with to (an app role) and marker is required"}, status_code=400)
    return out


@app.get("/diag/pg/read")
def diag_read_route(schema: str = "", marker: str = "") -> Any:
    out = diag_read(schema, marker)
    if out is None:
        return JSONResponse({"error": "schema (an app role) and marker are required"}, status_code=400)
    return out


@app.get("/diag/connect")
def diag_connect(to: str) -> dict[str, Any]:
    host, _, port = to.partition(":")
    try:
        with socket.create_connection((host, int(port)), timeout=2):
            return {"to": to, "connected": True}
    except OSError:
        return {"to": to, "connected": False}


@app.get("/diag/secret")
def diag_secret() -> dict[str, Any]:
    value = os.environ.get("CANARY_SECRET")
    return {"name": "CANARY_SECRET", "set": value is not None, "length": len(value or "")}


@app.get("/diag/log-secret")
def diag_log_secret() -> dict[str, Any]:
    """Deliberately prints the secret so the platform's log scrubbing can be proven: the line
    reaches the log store as [REDACTED:CANARY_SECRET]."""
    value = os.environ.get("CANARY_SECRET", "")
    log.info("canary secret log line", value=value)
    return {"logged": True, "length": len(value)}


@app.get("/diag/kv")
async def diag_kv_route(key: str = "diag", value: str | None = None, ttl: int | None = None):
    return await diag_kv(key, value, ttl)


@app.get("/diag/sync")
def diag_sync_route(tree: str = "diag", file: str = "diag.txt", value: str | None = None) -> JSONResponse:
    status, body = diag_sync(tree, file, value)
    return JSONResponse(body, status_code=status)


@app.get("/diag/boom")
def diag_boom() -> None:
    raise RuntimeError("deliberate exception for error tracking")


@app.get("/diag/cache")
def diag_cache(cookie: str | None = None) -> JSONResponse:
    """A cacheable answer: the edge answers a repeat for 60 seconds without waking the app.
    With ?cookie=1 it also sets a cookie, which keeps it out of the edge cache."""
    response = JSONResponse({"at": str(time.time_ns())}, headers={"Cache-Control": "private, max-age=60"})
    if cookie == "1":
        response.set_cookie("diag_cache", "1", path="/", httponly=True, secure=True, samesite="lax")
    return response


# Function runs arrive here from the platform with the service identity (queue.endpoint).
inngest.fast_api.serve(app, client, functions, serve_path="/.whisk/inngest")
