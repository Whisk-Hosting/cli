package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5"
)

// The canary's worker (CONTRACT.md §3.2, HARNESS.md H130): `./server worker`, which whisk-init
// runs beside the app when the canary is a Promoted app that declares it with database: direct.
// It holds an advisory lock and listens on its session connection, both of which the
// transaction pool would lose between statements, and records each notification it hears. A
// notification saying "exit" ends it, so the platform's restart can be watched. Safe to delete
// in your own app; it is on this template alone.

// workerChannel is what the worker listens on and POST /diag/worker notifies.
const workerChannel = "canary_worker"

// workerLock is the advisory lock the worker holds for as long as its connection is open.
const workerLock = 4129

func runWorker() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	conn, err := pgx.Connect(ctx, env("DATABASE_URL", ""))
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	// doctor: allow W024 the worker runs on a direct connection, which keeps both; H02 deploys the canary without declaring it.
	for _, stmt := range []string{fmt.Sprintf("select pg_advisory_lock(%d)", workerLock), "listen " + workerChannel} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	logger.Info("canary worker listening", "worker", os.Getenv("WHISK_WORKER"))
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if n.Payload == "exit" {
			logger.Info("canary worker exiting on request")
			os.Exit(3)
		}
		var locks int
		if err := conn.QueryRow(ctx, "select count(*) from pg_locks where locktype = 'advisory' and pid = pg_backend_pid()").Scan(&locks); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, "insert into worker_seen (marker, worker, advisory_locks) values ($1, $2, $3) on conflict (marker) do nothing", n.Payload, os.Getenv("WHISK_WORKER"), locks); err != nil {
			return err
		}
		logger.Info("canary worker heard", "marker", n.Payload)
	}
}

// diagWorkerNotify is POST /diag/worker with {marker}: a notification to the worker, sent on the
// app's pooled connection.
func diagWorkerNotify(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var in struct {
			Marker string `json:"marker"`
		}
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil || in.Marker == "" {
			writeJSON(w, 400, map[string]string{"error": "send {marker}"})
			return
		}
		_, err := db.pool.Exec(req.Context(), "select pg_notify($1, $2)", workerChannel, in.Marker)
		respond(w, map[string]bool{"sent": err == nil}, err)
	}
}

// diagWorkerSeen is GET /diag/worker?marker=<m>: whether the worker heard it, as which worker and
// holding how many advisory locks, and what the web process itself was given: no worker name and
// no session connection.
func diagWorkerSeen(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		out := map[string]any{
			"web_worker":  os.Getenv("WHISK_WORKER"),
			"web_session": os.Getenv("WHISK_WORKER_DATABASE_URL") != "",
			"seen":        false,
		}
		var worker string
		var locks int
		err := db.pool.QueryRow(req.Context(), "select worker, advisory_locks from worker_seen where marker = $1", req.URL.Query().Get("marker")).Scan(&worker, &locks)
		if errors.Is(err, pgx.ErrNoRows) {
			respond(w, out, nil)
			return
		}
		out["seen"], out["worker"], out["advisory_locks"] = err == nil, worker, locks
		respond(w, out, err)
	}
}
