import datetime as dt
from collections.abc import Iterator
from contextlib import AbstractContextManager, contextmanager
from typing import Any, Protocol

from sqlalchemy import Boolean, DateTime, Index, Integer, String, Text, create_engine, false, func, select, text
from sqlalchemy.dialects.postgresql import JSONB, insert
from sqlalchemy.orm import DeclarativeBase, Mapped, Session, mapped_column

from .whisk import Identity, env


class Base(DeclarativeBase):
    pass


class Note(Base):
    __tablename__ = "notes"
    __table_args__ = (Index("notes_author_id", "author_id"),)
    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    author_id: Mapped[str] = mapped_column(String, nullable=False)
    author_email: Mapped[str] = mapped_column(String, nullable=False)
    body: Mapped[str] = mapped_column(Text, nullable=False)
    created_at: Mapped[dt.datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)

    def as_dict(self) -> dict[str, Any]:
        return {"id": self.id, "author_id": self.author_id, "author_email": self.author_email, "body": self.body, "created_at": self.created_at.isoformat()}


class Event(Base):
    """What functions and webhooks did, keyed by the id the platform gave us, so a repeat
    (retry, replay, duplicate event) is a no-op insert."""

    __tablename__ = "events"
    id: Mapped[str] = mapped_column(String, primary_key=True)
    kind: Mapped[str] = mapped_column(String, nullable=False)
    payload: Mapped[dict[str, Any]] = mapped_column(JSONB, nullable=False)
    verified: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default=false())
    created_at: Mapped[dt.datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)

    def as_dict(self) -> dict[str, Any]:
        return {"id": self.id, "kind": self.kind, "payload": self.payload, "verified": self.verified, "created_at": self.created_at.isoformat()}


# DATABASE_URL is pooled in transaction mode: no server-side prepared statements.
engine = create_engine(env("DATABASE_URL").replace("postgres://", "postgresql+psycopg://", 1), pool_pre_ping=True, pool_size=5, connect_args={"prepare_threshold": None})


class Caller(Protocol):
    """Who a query runs as: the request's identity (whisk.identity), or SYSTEM for the app's own
    work."""

    @property
    def audience(self) -> str: ...

    @property
    def user_id(self) -> str | None: ...


SYSTEM = Identity(audience="system")


def caller_settings(caller: Caller) -> dict[str, str]:
    """The two settings the row-level security policies read
    (alembic/versions/0003_row_level_security.py). Pure."""
    return {"audience": caller.audience, "user_id": caller.user_id or ""}


_SET_CALLER = text("select set_config('whisk.audience', :audience, true), set_config('whisk.user_id', :user_id, true)")


@contextmanager
def db_for(caller: Caller) -> Iterator[Session]:
    """A session in one short transaction that first tells Postgres who is asking, so a table
    with a policy answers only that caller's rows even when a query forgets its filter. A plain
    Session(engine) says nothing, and those tables answer it with no rows. Read what you need
    before the block ends (objects expire on commit), and keep slow work (calls to other
    services) outside it: the transaction holds a pooled connection until it ends."""
    with Session(engine) as session, session.begin():
        session.execute(_SET_CALLER, caller_settings(caller))
        yield session


def as_system() -> AbstractContextManager[Session]:
    """db_for for the app's own work, outside any person's request: functions, webhook
    deliveries. It sees every row, so never use it to answer a person."""
    return db_for(SYSTEM)


def record_event(id: str, kind: str, payload: Any, verified: bool = False) -> dict[str, Any]:
    """Insert once per id; verified marks a webhook delivery the deliveries helper proved to
    be the platform's."""
    with Session(engine) as session, session.begin():
        result = session.execute(insert(Event).values(id=id, kind=kind, payload=payload, verified=verified).on_conflict_do_nothing())
        return {"id": id, "kind": kind, "duplicate": result.rowcount == 0}


def list_events(kind: str | None) -> list[dict[str, Any]]:
    stmt = select(Event).order_by(Event.created_at.desc()).limit(100)
    if kind:
        stmt = stmt.where(Event.kind == kind)
    with Session(engine) as session:
        return [e.as_dict() for e in session.scalars(stmt)]


def count_notes() -> int:
    """Every note, for the nightly summary: a function, so it counts as the system."""
    with as_system() as session:
        return session.scalar(select(func.count()).select_from(Note)) or 0
