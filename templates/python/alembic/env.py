"""Alembic environment: the URL comes from DATABASE_URL, which the platform sets to a direct
connection for the migrate step."""

import os
from logging.config import fileConfig

from alembic import context
from sqlalchemy import create_engine

from app.db import Base

config = context.config
if config.config_file_name:
    fileConfig(config.config_file_name)

url = os.environ["DATABASE_URL"].replace("postgres://", "postgresql+psycopg://", 1)
target_metadata = Base.metadata


def run_migrations_offline() -> None:
    context.configure(url=url, target_metadata=target_metadata, literal_binds=True)
    with context.begin_transaction():
        context.run_migrations()


def run_migrations_online() -> None:
    engine = create_engine(url)
    with engine.connect() as connection:
        context.configure(connection=connection, target_metadata=target_metadata)
        with context.begin_transaction():
            context.run_migrations()
    engine.dispose()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()
