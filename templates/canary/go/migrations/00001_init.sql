-- +goose Up
create table notes (
    id           bigserial primary key,
    author_id    text not null,
    author_email text not null,
    body         text not null,
    created_at   timestamptz not null default now()
);

create table events (
    id         text primary key,
    kind       text not null,
    payload    jsonb not null,
    created_at timestamptz not null default now()
);

-- +goose Down
drop table events;
drop table notes;
