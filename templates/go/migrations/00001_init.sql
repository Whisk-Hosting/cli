-- +goose Up
create table notes (
    id           bigserial primary key,
    author_id    text not null,
    author_email text not null,
    body         text not null,
    words        integer,
    created_at   timestamptz not null default now()
);

-- +goose Down
drop table notes;
