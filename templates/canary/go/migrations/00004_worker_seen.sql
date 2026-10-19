-- +goose Up
-- What the canary's worker heard on its session connection (HARNESS.md H130).
create table worker_seen (
    marker text primary key,
    worker text not null,
    advisory_locks int not null,
    created_at timestamptz not null default now()
);

-- +goose Down
drop table worker_seen;
