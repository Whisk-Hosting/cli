import datetime as dt
from typing import Any

from sqlalchemy import Boolean, DateTime, Integer, String, Text, create_engine, false, func, select
from sqlalchemy.dialects.postgresql import JSONB, insert
from sqlalchemy.orm import DeclarativeBase, Mapped, Session, mapped_column

from .whisk import env


class Base(DeclarativeBase):
    pass


class Note(Base):
    __tablename__ = "notes"
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
    with Session(engine) as session:
        return session.scalar(select(func.count()).select_from(Note)) or 0
