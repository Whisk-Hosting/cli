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

const noteColumns = `id, author_id, author_email, body, words, created_at`

// ListNotes is the newest notes the scope includes (whisk.go ScopeFor); none for Kind "none".
func (d *DB) ListNotes(ctx context.Context, scope Scope) ([]Note, error) {
	if scope.Kind != "all" && scope.Kind != "owner" {
		return []Note{}, nil
	}
	rows, err := d.pool.Query(ctx, `select `+noteColumns+` from notes where $1 = '' or author_id = $1 order by id desc limit 100`, scope.OwnerID)
	if err != nil {
		return nil, err
	}
	notes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Note])
	if notes == nil {
		notes = []Note{}
	}
	return notes, err
}

// GetNote is one note, or ok false when there is none with that id.
func (d *DB) GetNote(ctx context.Context, id int64) (Note, bool, error) {
	rows, _ := d.pool.Query(ctx, `select `+noteColumns+` from notes where id = $1`, id)
	note, err := pgx.CollectOneRow(rows, pgx.RowToStructByPos[Note])
	if errors.Is(err, pgx.ErrNoRows) {
		return Note{}, false, nil
	}
	return note, err == nil, err
}

func (d *DB) InsertNote(ctx context.Context, authorID, authorEmail, body string) (Note, error) {
	rows, _ := d.pool.Query(ctx, `insert into notes (author_id, author_email, body) values ($1, $2, $3) returning `+noteColumns, authorID, authorEmail, body)
	return pgx.CollectOneRow(rows, pgx.RowToStructByPos[Note])
}

func (d *DB) SetWords(ctx context.Context, id int64, words int) error {
	_, err := d.pool.Exec(ctx, `update notes set words = $2 where id = $1`, id, words)
	return err
}

func (d *DB) DeleteNote(ctx context.Context, id int64) error {
	_, err := d.pool.Exec(ctx, `delete from notes where id = $1`, id)
	return err
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
