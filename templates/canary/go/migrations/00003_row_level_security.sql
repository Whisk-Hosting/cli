-- Row-level security (CONTRACT.md §8, SKILL.md §5): each query says who is asking (DB.For and
-- DB.AsSystem in db.go), and Postgres keeps every other person's notes out of its answer. Team
-- members and the app's own work see every note, a customer only their own, anything else none.

-- +goose Up
create index notes_author_id on notes (author_id);
alter table notes enable row level security;
alter table notes force row level security;
create policy notes_by_audience on notes using (
    current_setting('whisk.audience', true) in ('team', 'system')
    or author_id = nullif(current_setting('whisk.user_id', true), '')
);

-- +goose Down
drop policy notes_by_audience on notes;
alter table notes no force row level security;
alter table notes disable row level security;
drop index notes_author_id;
