"""notes: row-level security

Row-level security (CONTRACT.md §8, SKILL.md §5): each query says who is asking (db_for and
as_system in app/db.py), and Postgres keeps every other person's notes out of its answer. Team
members and the app's own work see every note, a customer only their own, anything else none.

Revision ID: 0002
Revises: 0001
Create Date: 2026-10-09
"""

from alembic import op

revision = "0002"
down_revision = "0001"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_index("notes_author_id", "notes", ["author_id"])
    op.execute("alter table notes enable row level security")
    op.execute("alter table notes force row level security")
    op.execute(
        """
        create policy notes_by_audience on notes using (
            current_setting('whisk.audience', true) in ('team', 'system')
            or author_id = nullif(current_setting('whisk.user_id', true), '')
        )
        """
    )


def downgrade() -> None:
    op.execute("drop policy notes_by_audience on notes")
    op.execute("alter table notes no force row level security")
    op.execute("alter table notes disable row level security")
    op.drop_index("notes_author_id", table_name="notes")
