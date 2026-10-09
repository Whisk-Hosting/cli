-- +goose Up
alter table events add column verified boolean not null default false;

-- +goose Down
alter table events drop column verified;
