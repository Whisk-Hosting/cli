package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Note struct {
	ID          int64     `json:"id"`
	AuthorID    string    `json:"author_id"`
	AuthorEmail string    `json:"author_email"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
}

// Event records what functions and webhooks did, keyed by the id the platform gave us, so a
// repeat (retry, replay, duplicate event) is a no-op insert. Verified is true on a webhook
// delivery the deliveries helper proved to be the platform's.
type Event struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	Verified  bool            `json:"verified"`
	CreatedAt time.Time       `json:"created_at"`
}

type Recorded struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Duplicate bool   `json:"duplicate"`
}

type DB struct{ pool *pgxpool.Pool }

// openDB connects to DATABASE_URL, which is pooled in transaction mode, so no prepared
// statement cache: every query is sent with its parameters.
func openDB(ctx context.Context) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(env("DATABASE_URL", ""))
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	traceQueries(cfg.ConnConfig)
	cfg.MaxConns = 5
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &DB{pool: pool}, nil
}

func (d *DB) Close()                         { d.pool.Close() }
func (d *DB) Ping(ctx context.Context) error { return d.pool.Ping(ctx) }

func (d *DB) ListNotes(ctx context.Context) ([]Note, error) {
	rows, err := d.pool.Query(ctx, `select id, author_id, author_email, body, created_at from notes order by id desc limit 100`)
	if err != nil {
		return nil, err
	}
	notes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Note])
	if notes == nil {
		notes = []Note{}
	}
	return notes, err
}

func (d *DB) InsertNote(ctx context.Context, authorID, authorEmail, body string) (Note, error) {
	rows, _ := d.pool.Query(ctx, `insert into notes (author_id, author_email, body) values ($1, $2, $3) returning id, author_id, author_email, body, created_at`, authorID, authorEmail, body)
	return pgx.CollectOneRow(rows, pgx.RowToStructByPos[Note])
}

func (d *DB) DeleteNote(ctx context.Context, id int64) error {
	_, err := d.pool.Exec(ctx, `delete from notes where id = $1`, id)
	return err
}

func (d *DB) CountNotes(ctx context.Context) (int, error) {
	var n int
	err := d.pool.QueryRow(ctx, `select count(*) from notes`).Scan(&n)
	return n, err
}

func (d *DB) ListEvents(ctx context.Context, kind string) ([]Event, error) {
	rows, err := d.pool.Query(ctx, `select id, kind, payload, verified, created_at from events where ($1 = '' or kind = $1) order by created_at desc limit 100`, kind)
	if err != nil {
		return nil, err
	}
	events, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Event])
	if events == nil {
		events = []Event{}
	}
	return events, err
}

// RecordEvent inserts once per id; verified marks a webhook delivery the deliveries helper
// proved to be the platform's.
func (d *DB) RecordEvent(ctx context.Context, id, kind string, payload any, verified bool) (Recorded, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Recorded{}, err
	}
	tag, err := d.pool.Exec(ctx, `insert into events (id, kind, payload, verified) values ($1, $2, $3::jsonb, $4) on conflict (id) do nothing`, id, kind, string(raw), verified)
	return Recorded{ID: id, Kind: kind, Duplicate: tag.RowsAffected() == 0}, err
}

// migrate applies migrations/*.sql with goose. It is the manifest's migrate command
// ("/app migrate") and runs before traffic switches to a new deploy.
func migrate() error {
	db, err := sql.Open("pgx", env("DATABASE_URL", ""))
	if err != nil {
		return err
	}
	defer db.Close()
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, "migrations")
}
