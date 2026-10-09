"""events.verified: a webhook delivery the deliveries helper proved to be the platform's

Revision ID: 0002
Revises: 0001
Create Date: 2026-09-23
"""

import sqlalchemy as sa
from alembic import op

revision = "0002"
down_revision = "0001"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.add_column("events", sa.Column("verified", sa.Boolean, nullable=False, server_default=sa.false()))


def downgrade() -> None:
    op.drop_column("events", "verified")
