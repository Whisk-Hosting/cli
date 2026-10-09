import os
import time
from contextlib import asynccontextmanager
from typing import Any

import inngest.fast_api
import sentry_sdk
from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.responses import HTMLResponse, JSONResponse
from sqlalchemy import select, text
from sqlalchemy.orm import Session

from .db import Note, engine
from .functions import functions
from .whisk import client, enqueue, env, identity, log, trace_app, tracing

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
    log.info("request", request_id=who.request_id, method=request.method, path=request.url.path, status=response.status_code, user=who.email, ms=int((time.monotonic() - started) * 1000))
    return response


# Public routes (routes.public in whisk.yaml): "/" and "/health". Everything else is private:
# the platform signs people in before a request reaches it.
@app.get("/", response_class=HTMLResponse)
def home(request: Request) -> str:
    who = identity(request.headers)
    greeting = "You are not signed in." if who.audience == "anonymous" else f"Signed in as {who.name or who.email}."
    return f"""<!doctype html><title>{APP_NAME}</title><main style="font-family:system-ui;max-width:40rem;margin:4rem auto">
<h1>{APP_NAME}</h1><p>{greeting}</p>
<p><a href="/notes">Notes</a> · <a href="/.whisk/login?return=/">Sign in</a> · <a href="/.whisk/logout?return=/">Sign out</a></p></main>"""


@app.get("/health")
def health() -> Any:
    try:
        with engine.connect() as conn:
            conn.execute(text("select 1"))
        return {"ok": True}
    except Exception as exc:  # noqa: BLE001
        return JSONResponse({"ok": False, "error": str(exc)}, status_code=503)


@app.get("/notes")
def list_notes() -> list[dict[str, Any]]:
    with Session(engine) as session:
        return [n.as_dict() for n in session.scalars(select(Note).order_by(Note.id.desc()).limit(100))]


@app.post("/notes", status_code=201)
async def create_note(request: Request) -> dict[str, Any]:
    who = identity(request.headers)
    if not who.user_id:
        raise HTTPException(403, "a signed-in person is required")
    body = await request.json() if request.headers.get("content-type", "").startswith("application/json") else {}
    text_ = str(body.get("body", "")).strip() if isinstance(body, dict) else ""
    if not text_:
        raise HTTPException(400, "body is required")
    with Session(engine) as session, session.begin():
        note = Note(author_id=who.user_id, author_email=who.email or "", body=text_)
        session.add(note)
        session.flush()
        created = note.as_dict()
    # Hand the slow part to a function. A preview runs no functions, so it sends no events.
    if os.environ.get("WHISK_QUEUE_URL") and not os.environ.get("WHISK_ENV", "").startswith("preview:"):
        try:
            enqueue("note.added", {"note_id": created["id"]}, f"note-{created['id']}")
        except Exception as err:  # noqa: BLE001 - the note is saved; the count can wait
            log.warning("note.added not sent", request_id=who.request_id, note_id=created["id"], err=str(err))
    return created


@app.delete("/notes/{note_id}", status_code=204)
def delete_note(note_id: int, request: Request) -> Response:
    if not identity(request.headers).has_role("owner", "admin"):
        raise HTTPException(403, "owner or admin role required")
    with Session(engine) as session, session.begin():
        note = session.get(Note, note_id)
        if note is not None:
            session.delete(note)
    return Response(status_code=204)


# Function runs arrive here from the platform with the service identity (queue.endpoint).
inngest.fast_api.serve(app, client, functions, serve_path="/.whisk/inngest")
