package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := migrate(); err != nil {
			logger.Error("migrate failed", "err", err)
			os.Exit(1)
		}
		logger.Info("migrations applied")
		return
	}
	if err := run(); err != nil {
		logger.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		if err := sentry.Init(sentry.ClientOptions{Dsn: dsn, Environment: os.Getenv("WHISK_ENV")}); err != nil {
			return err
		}
		defer sentry.Flush(2 * time.Second)
	}
	stopTracing, err := startTracing(ctx)
	if err != nil {
		return err
	}
	db, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	client, err := newInngest()
	if err != nil {
		return err
	}
	if err := registerFunctions(client, db); err != nil {
		return err
	}

	appName := env("WHISK_APP_NAME", "whisk-go")
	r := chi.NewRouter()
	r.Use(traceRequests, logRequests, recoverer)

	// Public routes (routes.public in whisk.yaml): "/" and "/health". Everything else is
	// private: the platform signs people in before a request reaches it.
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		id := identityFrom(req.Header)
		greeting := "You are not signed in."
		if id.Audience != "anonymous" {
			who := id.Name
			if who == "" {
				who = id.Email
			}
			greeting = fmt.Sprintf("Signed in as %s.", who)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><title>%[1]s</title><main style="font-family:system-ui;max-width:40rem;margin:4rem auto">
<h1>%[1]s</h1><p>%[2]s</p>
<p><a href="/notes">Notes</a> · <a href="/.whisk/login?return=/">Sign in</a> · <a href="/.whisk/logout?return=/">Sign out</a></p></main>`, html.EscapeString(appName), html.EscapeString(greeting))
	})
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		if err := db.Ping(req.Context()); err != nil {
			writeJSON(w, 503, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// Each note belongs to its author (AuthorID). The team sees every note; a customer, when the
	// app has customer_identity, sees only their own (whisk.go ScopeFor).
	r.Get("/notes", func(w http.ResponseWriter, req *http.Request) {
		notes, err := db.ListNotes(req.Context(), ScopeFor(identityFrom(req.Header)))
		respond(w, notes, err)
	})
	r.Post("/notes", func(w http.ResponseWriter, req *http.Request) {
		id := identityFrom(req.Header)
		if id.UserID == "" {
			writeJSON(w, 403, map[string]string{"error": "a signed-in person is required"})
			return
		}
		var body struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		if strings.TrimSpace(body.Body) == "" {
			writeJSON(w, 400, map[string]string{"error": "body is required"})
			return
		}
		note, err := db.InsertNote(req.Context(), id.UserID, id.Email, strings.TrimSpace(body.Body))
		if err != nil {
			respond(w, nil, err)
			return
		}
		// Hand the slow part to a function. A preview runs no functions, so it sends no events.
		if os.Getenv("WHISK_QUEUE_URL") != "" && !strings.HasPrefix(os.Getenv("WHISK_ENV"), "preview:") {
			key := "note-" + strconv.FormatInt(note.ID, 10)
			if _, err := enqueue(req.Context(), "note.added", map[string]any{"note_id": note.ID}, key); err != nil {
				logger.Warn("note.added not sent", "request_id", id.RequestID, "note_id", note.ID, "err", err)
			}
		}
		writeJSON(w, 201, note)
	})
	r.Delete("/notes/{id}", func(w http.ResponseWriter, req *http.Request) {
		who := identityFrom(req.Header)
		id, _ := strconv.ParseInt(chi.URLParam(req, "id"), 10, 32)
		note, ok, err := db.GetNote(req.Context(), id)
		if err != nil {
			respond(w, nil, err)
			return
		}
		// A note the person may not see is answered as if it did not exist.
		if !ok || !who.CanSee(note.AuthorID) {
			writeJSON(w, 404, map[string]string{"error": "no such note"})
			return
		}
		if !who.CanChange(note.AuthorID) {
			writeJSON(w, 403, map[string]string{"error": "only its author or an owner or admin can delete it"})
			return
		}
		if err := db.DeleteNote(req.Context(), id); err != nil {
			respond(w, nil, err)
			return
		}
		w.WriteHeader(204)
	})

	// Function runs arrive here from the platform with the service identity (queue.endpoint).
	r.Handle("/.whisk/inngest", client.Serve())

	srv := &http.Server{Addr: "0.0.0.0:" + env("PORT", "8080"), Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server", "err", err)
			stop()
		}
	}()
	<-ctx.Done()

	// SIGTERM: stop accepting, finish in-flight requests, exit within the grace period.
	grace, _ := strconv.Atoi(env("WHISK_STOP_GRACE", "28"))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(grace-2)*time.Second)
	defer cancel()
	logger.Info("shutting down")
	return errors.Join(srv.Shutdown(shutdownCtx), stopTracing(shutdownCtx))
}

// logRequests writes one JSON line per request with the platform request id.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		started := time.Now()
		rec := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, req)
		id := identityFrom(req.Header)
		logger.Info("request", "request_id", id.RequestID, "method", req.Method, "path", req.URL.Path, "status", rec.status, "user", id.Email, "ms", time.Since(started).Milliseconds())
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				err := fmt.Errorf("%v", p)
				hub := sentry.CurrentHub().Clone()
				hub.Scope().SetTag("request_id", req.Header.Get("X-Whisk-Request-Id")) // an error links to its trace
				hub.CaptureException(err)
				logger.Error("unhandled error", "request_id", req.Header.Get("X-Whisk-Request-Id"), "err", err)
				writeJSON(w, 500, map[string]string{"error": "internal error", "request_id": req.Header.Get("X-Whisk-Request-Id")})
			}
		}()
		next.ServeHTTP(w, req)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, v)
}
