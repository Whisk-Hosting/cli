"""notes

Revision ID: 0001
Revises:
Create Date: 2026-10-06
"""

import sqlalchemy as sa
from alembic import op

revision = "0001"
down_revision = None
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "notes",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("author_id", sa.String, nullable=False),
        sa.Column("author_email", sa.String, nullable=False),
        sa.Column("body", sa.Text, nullable=False),
        sa.Column("words", sa.Integer, nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
    )


def downgrade() -> None:
    op.drop_table("notes")
