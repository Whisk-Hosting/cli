package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Note is one row of the notes table, the one table to start from. Change it with a new file in
// migrations/; whisk deploy runs it before traffic switches.
type Note struct {
	ID          int64     `json:"id"`
	AuthorID    string    `json:"author_id"`
	AuthorEmail string    `json:"author_email"`
	Body        string    `json:"body"`
	Words       *int      `json:"words"` // filled in by the note-added function
	CreatedAt   time.Time `json:"created_at"`
}

// Recorded is what the webhook deliveries helper in whisk.go answers for a delivery: its id, its
// kind and whether it had already been handled.
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
// (migrations/00002_row_level_security.sql). A caller with no audience is anonymous. Pure.
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

const noteColumns = `id, author_id, author_email, body, words, created_at`

// ListNotes is the newest notes the scope includes (whisk.go ScopeFor); none for Kind "none".
func (d *DB) ListNotes(ctx context.Context, c Caller, scope Scope) ([]Note, error) {
	notes := []Note{}
	if scope.Kind != "all" && scope.Kind != "owner" {
		return notes, nil
	}
	err := d.For(ctx, c, func(tx pgx.Tx) error {
		rows, _ := tx.Query(ctx, `select `+noteColumns+` from notes where $1 = '' or author_id = $1 order by id desc limit 100`, scope.OwnerID)
		got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Note])
		if got != nil {
			notes = got
		}
		return err
	})
	return notes, err
}

// GetNote is one note, or ok false when there is none with that id the caller may see.
func (d *DB) GetNote(ctx context.Context, c Caller, id int64) (Note, bool, error) {
	var note Note
	err := d.For(ctx, c, func(tx pgx.Tx) error {
		rows, _ := tx.Query(ctx, `select `+noteColumns+` from notes where id = $1`, id)
		var err error
		note, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[Note])
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Note{}, false, nil
	}
	return note, err == nil, err
}

func (d *DB) InsertNote(ctx context.Context, c Caller, authorID, authorEmail, body string) (Note, error) {
	var note Note
	err := d.For(ctx, c, func(tx pgx.Tx) error {
		rows, _ := tx.Query(ctx, `insert into notes (author_id, author_email, body) values ($1, $2, $3) returning `+noteColumns, authorID, authorEmail, body)
		var err error
		note, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[Note])
		return err
	})
	return note, err
}

func (d *DB) SetWords(ctx context.Context, c Caller, id int64, words int) error {
	return d.For(ctx, c, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update notes set words = $2 where id = $1`, id, words)
		return err
	})
}

func (d *DB) DeleteNote(ctx context.Context, c Caller, id int64) error {
	return d.For(ctx, c, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `delete from notes where id = $1`, id)
		return err
	})
}

// migrate applies migrations/*.sql with goose. It is the manifest's migrate command
// ("./server migrate") and runs before traffic switches to a new deploy.
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
