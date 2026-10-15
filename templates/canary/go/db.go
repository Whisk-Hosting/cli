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

// Caller is who a query runs as: the request's audience and person, or "system" for the app's
// own work.
type Caller struct{ Audience, UserID string }

// callerOf is the caller a request's identity makes.
func callerOf(id Identity) Caller { return Caller{Audience: id.Audience, UserID: id.UserID} }

// systemCaller is the app's own work, outside any person's request: functions, webhook
// deliveries. It sees every row, so never use it to answer a person.
var systemCaller = Caller{Audience: "system"}

// callerSettings are the two settings the row-level security policies read
// (migrations/00003_row_level_security.sql). A caller with no audience is anonymous. Pure.
func callerSettings(c Caller) (audience, userID string) {
	if c.Audience == "" {
		return "anonymous", c.UserID
	}
	return c.Audience, c.UserID
}

// For runs fn in one short transaction that first tells Postgres who is asking, so a table with
// a policy answers only that caller's rows even when a query forgets its filter. A query on the
// pool itself says nothing, and those tables answer it with no rows. Keep slow work (calls to
// other services) outside fn: the transaction holds a pooled connection until it ends.
func (d *DB) For(ctx context.Context, c Caller, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, d.pool, func(tx pgx.Tx) error {
		audience, userID := callerSettings(c)
		if _, err := tx.Exec(ctx, `select set_config('whisk.audience', $1, true), set_config('whisk.user_id', $2, true)`, audience, userID); err != nil {
			return err
		}
		return fn(tx)
	})
}

// AsSystem is For with the system caller, for the app's own work.
func (d *DB) AsSystem(ctx context.Context, fn func(pgx.Tx) error) error {
	return d.For(ctx, systemCaller, fn)
}

func (d *DB) ListNotes(ctx context.Context, c Caller) ([]Note, error) {
	notes := []Note{}
	err := d.For(ctx, c, func(tx pgx.Tx) error {
		rows, _ := tx.Query(ctx, `select id, author_id, author_email, body, created_at from notes order by id desc limit 100`)
		got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Note])
		if got != nil {
			notes = got
		}
		return err
	})
	return notes, err
}

func (d *DB) InsertNote(ctx context.Context, c Caller, authorID, authorEmail, body string) (Note, error) {
	var note Note
	err := d.For(ctx, c, func(tx pgx.Tx) error {
		rows, _ := tx.Query(ctx, `insert into notes (author_id, author_email, body) values ($1, $2, $3) returning id, author_id, author_email, body, created_at`, authorID, authorEmail, body)
		var err error
		note, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[Note])
		return err
	})
	return note, err
}

func (d *DB) DeleteNote(ctx context.Context, c Caller, id int64) error {
	return d.For(ctx, c, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `delete from notes where id = $1`, id)
		return err
	})
}

// CountNotes counts the notes the caller may see, with no filter in the query.
func (d *DB) CountNotes(ctx context.Context, c Caller) (int, error) {
	var n int
	err := d.For(ctx, c, func(tx pgx.Tx) error { return tx.QueryRow(ctx, `select count(*) from notes`).Scan(&n) })
	return n, err
}

// CountNotesUnscoped counts notes on the pool itself, saying nothing about who is asking. The
// policy answers it with none; /diag/rows checks that it does.
func (d *DB) CountNotesUnscoped(ctx context.Context) (int, error) {
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
