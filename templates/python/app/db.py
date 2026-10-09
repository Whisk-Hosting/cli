import datetime as dt
from typing import Any

from sqlalchemy import DateTime, Integer, String, Text, create_engine, func, update
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
    words: Mapped[int | None] = mapped_column(Integer, nullable=True)
    created_at: Mapped[dt.datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)

    def as_dict(self) -> dict[str, Any]:
        return {"id": self.id, "author_id": self.author_id, "author_email": self.author_email, "body": self.body, "words": self.words, "created_at": self.created_at.isoformat()}


# DATABASE_URL is pooled in transaction mode: no server-side prepared statements.
engine = create_engine(env("DATABASE_URL").replace("postgres://", "postgresql+psycopg://", 1), pool_pre_ping=True, pool_size=5, connect_args={"prepare_threshold": None})


def note_body(note_id: int) -> str | None:
    with Session(engine) as session:
        note = session.get(Note, note_id)
        return note.body if note else None


def set_words(note_id: int, words: int) -> bool:
    """Safe to run twice: it writes the same count again."""
    with Session(engine) as session, session.begin():
        session.execute(update(Note).where(Note.id == note_id).values(words=words))
    return True
