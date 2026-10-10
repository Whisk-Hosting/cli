package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
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

	// Public routes (routes.public in whisk.yaml): "/", "/health", "/whoami".
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		id := identityFrom(req.Header)
		greeting := "You are not signed in."
		if id.Audience != "anonymous" {
			who := id.Name
			if who == "" {
				who = id.Email
			}
			greeting = fmt.Sprintf("Signed in as %s (%s, roles: %s).", who, id.Audience, orNone(strings.Join(id.Roles, ", ")))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><title>%[1]s</title><main style="font-family:system-ui;max-width:40rem;margin:4rem auto">
<h1>%[1]s</h1><p>%[2]s</p>
<p><a href="/me">/me</a> · <a href="/notes">/notes</a> · <a href="/.whisk/login?return=/">sign in</a> · <a href="/.whisk/logout?return=/">sign out</a></p></main>`, html.EscapeString(appName), html.EscapeString(greeting))
	})
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		if err := db.Ping(req.Context()); err != nil {
			writeJSON(w, 503, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	r.Get("/whoami", func(w http.ResponseWriter, req *http.Request) { writeJSON(w, 200, whiskHeaders(req.Header)) })

	// Private routes: the platform has already signed the caller in.
	r.Get("/me", func(w http.ResponseWriter, req *http.Request) { writeJSON(w, 200, whiskHeaders(req.Header)) })
	r.Get("/notes", func(w http.ResponseWriter, req *http.Request) {
		notes, err := db.ListNotes(req.Context(), callerOf(identityFrom(req.Header)))
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
		note, err := db.InsertNote(req.Context(), callerOf(id), id.UserID, id.Email, strings.TrimSpace(body.Body))
		if err != nil {
			respond(w, nil, err)
			return
		}
		writeJSON(w, 201, note)
	})
	r.Delete("/notes/{id}", func(w http.ResponseWriter, req *http.Request) {
		who := identityFrom(req.Header)
		if !who.HasRole("owner", "admin") {
			writeJSON(w, 403, map[string]string{"error": "owner or admin role required"})
			return
		}
		id, _ := strconv.ParseInt(chi.URLParam(req, "id"), 10, 64)
		if err := db.DeleteNote(req.Context(), callerOf(who), id); err != nil {
			respond(w, nil, err)
			return
		}
		w.WriteHeader(204)
	})
	r.Get("/events", func(w http.ResponseWriter, req *http.Request) {
		events, err := db.ListEvents(req.Context(), req.URL.Query().Get("kind"))
		respond(w, events, err)
	})

	// Webhook handler: deliveries.Handle proves the delivery is the platform's, records it as
	// verified and dedupes on the webhook id before this function runs (CONTRACT.md §7).
	r.Post("/hooks/stripe", deliveries.Handle(db.RecordEvent, func(req *http.Request, d Delivery) error {
		logger.Info("stripe delivery", "request_id", req.Header.Get("X-Whisk-Request-Id"), "webhook_id", d.ID, "bytes", len(d.Body))
		return nil
	}))
	// Inbound email: each message to the app's address arrives as a delivery whose JSON body names
	// the sender, the subject and where the original and attachments are stored (CONTRACT.md §7).
	r.Post("/inbound/email", deliveries.Handle(db.RecordEvent, func(req *http.Request, d Delivery) error {
		logger.Info("email delivery", "request_id", req.Header.Get("X-Whisk-Request-Id"), "webhook_id", d.ID, "bytes", len(d.Body))
		return nil
	}))

	// Diagnostics used by the platform canary; private, and safe to delete in your own app.
	r.Get("/diag", func(w http.ResponseWriter, req *http.Request) { writeJSON(w, 200, diagReport()) })
	// The notes the caller's row-level security lets through with no filter in the query, counted
	// once as the caller (DB.For) and once saying nothing about who is asking (the pool), which
	// must be none.
	r.Get("/diag/rows", func(w http.ResponseWriter, req *http.Request) {
		scoped, err := db.CountNotes(req.Context(), callerOf(identityFrom(req.Header)))
		if err != nil {
			respond(w, nil, err)
			return
		}
		unscoped, err := db.CountNotesUnscoped(req.Context())
		respond(w, map[string]int{"scoped": scoped, "unscoped": unscoped}, err)
	})
	r.Get("/diag/call", diagCall)
	r.Post("/diag/call", diagPost)
	r.Get("/diag/pg", diagPG)
	r.Post("/diag/pg/share", diagShare)
	r.Get("/diag/pg/read", diagRead)
	r.Post("/diag/pg/guard", diagGuard)
	r.Post("/diag/enqueue", func(w http.ResponseWriter, req *http.Request) {
		// Sends one event through WHISK_QUEUE_URL with the app's own service token, so the
		// platform can prove an app enqueues its own events.
		var in struct {
			Name      string         `json:"name"`
			Data      map[string]any `json:"data"`
			DedupeKey string         `json:"dedupe_key"`
		}
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil || in.Name == "" {
			writeJSON(w, 400, map[string]string{"error": "a JSON body with a name is required"})
			return
		}
		id, err := enqueue(req.Context(), in.Name, in.Data, in.DedupeKey)
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	r.Get("/diag/connect", func(w http.ResponseWriter, req *http.Request) {
		to := req.URL.Query().Get("to")
		conn, err := net.DialTimeout("tcp", to, 2*time.Second)
		if err == nil {
			conn.Close()
		}
		writeJSON(w, 200, map[string]any{"to": to, "connected": err == nil})
	})
	r.Get("/diag/secret", func(w http.ResponseWriter, req *http.Request) {
		v, set := os.LookupEnv("CANARY_SECRET")
		writeJSON(w, 200, map[string]any{"name": "CANARY_SECRET", "set": set, "length": len(v)})
	})
	r.Get("/diag/log-secret", func(w http.ResponseWriter, req *http.Request) {
		// Deliberately prints the secret so the platform's log scrubbing can be proven: the
		// line reaches the log store as [REDACTED:CANARY_SECRET].
		v := os.Getenv("CANARY_SECRET")
		logger.Info("canary secret log line", "value", v)
		writeJSON(w, 200, map[string]any{"logged": true, "length": len(v)})
	})
	r.Get("/diag/connection", diagConnection)
	r.Get("/diag/env", diagEnv)
	r.Post("/diag/domains", diagDomains)
	r.Get("/diag/kv", kvRoute)
	r.Get("/diag/sync", syncRoute)
	r.Get("/diag/boom", func(w http.ResponseWriter, req *http.Request) { panic("deliberate exception for error tracking") })
	// A cacheable answer: the edge answers a repeat for 60 seconds without waking the app.
	// With ?cookie=1 it also sets a cookie, which keeps it out of the edge cache.
	r.Get("/diag/cache", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "private, max-age=60")
		if req.URL.Query().Get("cookie") == "1" {
			http.SetCookie(w, &http.Cookie{Name: "diag_cache", Value: "1", Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
		}
		writeJSON(w, 200, map[string]string{"at": strconv.FormatInt(time.Now().UnixNano(), 10)})
	})

	// The cookies the app receives, by name, and with ?domain= one it sets the way an app might
	// try to share it with its neighbours: on a parent domain and a deeper path. The edge binds
	// it to this host instead (H63).
	r.Get("/diag/cookies", func(w http.ResponseWriter, req *http.Request) {
		if d := req.URL.Query().Get("domain"); d != "" {
			// doctor: allow W091 H63 sets a parent-domain cookie on purpose, to show the edge binds it to this host.
			w.Header().Add("Set-Cookie", "diag_cookie=1; Domain="+d+"; Path=/diag; HttpOnly")
		}
		names := []string{}
		for _, c := range req.Cookies() {
			names = append(names, c.Name)
		}
		sort.Strings(names)
		writeJSON(w, 200, map[string]any{"cookies": names})
	})
	r.Post("/diag/cookies", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, 200, map[string]bool{"reached": true})
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
		logger.Info("request", "request_id", id.RequestID, "method", req.Method, "path", req.URL.Path, "status", rec.status, "audience", id.Audience, "user", id.Email, "ms", time.Since(started).Milliseconds())
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

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".whisk-probe-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
