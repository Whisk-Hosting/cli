import datetime as dt
from collections.abc import Iterator
from contextlib import AbstractContextManager, contextmanager
from typing import Any, Protocol

from sqlalchemy import DateTime, Index, Integer, String, Text, create_engine, func, text, update
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
    words: Mapped[int | None] = mapped_column(Integer, nullable=True)
    created_at: Mapped[dt.datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)

    def as_dict(self) -> dict[str, Any]:
        return {"id": self.id, "author_id": self.author_id, "author_email": self.author_email, "body": self.body, "words": self.words, "created_at": self.created_at.isoformat()}


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
    (alembic/versions/0002_row_level_security.py). Pure."""
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


def note_body(note_id: int) -> str | None:
    with as_system() as session:
        note = session.get(Note, note_id)
        return note.body if note else None


def set_words(note_id: int, words: int) -> bool:
    """Safe to run twice: it writes the same count again."""
    with as_system() as session:
        session.execute(update(Note).where(Note.id == note_id).values(words=words))
    return True
