package main

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// The cache (CONTRACT.md §8). WHISK_KV_URL is an ordinary Redis URL to a Valkey this app has
// to itself, so keys need no prefix and nothing else can read them. It empties when the app
// sleeps: use it for counters, short caches and locks, never for anything you need back.

// cache is the client, or nil when the manifest did not ask for one.
func cache() *redis.Client {
	url := os.Getenv("WHISK_KV_URL")
	if url == "" {
		return nil
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil
	}
	return redis.NewClient(opts)
}

// kvRoute is the diagnostic the platform's canary drives: write a value, read it back, count
// a visit, and report how many keys the instance holds — which is only ever this app's.
func kvRoute(w http.ResponseWriter, req *http.Request) {
	c := cache()
	if c == nil {
		writeJSON(w, 200, map[string]any{"configured": false})
		return
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	key := req.URL.Query().Get("key")
	if key == "" {
		key = "diag"
	}
	out := map[string]any{"configured": true, "key": key}
	if value := req.URL.Query().Get("value"); value != "" {
		ttl := time.Duration(0)
		if seconds, err := strconv.Atoi(req.URL.Query().Get("ttl")); err == nil && seconds > 0 {
			ttl = time.Duration(seconds) * time.Second
		}
		if err := c.Set(ctx, key, value, ttl).Err(); err != nil {
			writeJSON(w, 500, map[string]any{"configured": true, "error": err.Error()})
			return
		}
		out["ttl_seconds"] = int(ttl.Seconds())
	}
	switch value, err := c.Get(ctx, key).Result(); {
	case err == redis.Nil:
		out["value"] = nil
	case err != nil:
		writeJSON(w, 500, map[string]any{"configured": true, "error": err.Error()})
		return
	default:
		out["value"] = value
	}
	if n, err := c.Incr(ctx, "diag:visits").Result(); err == nil {
		out["visits"] = n
	}
	if n, err := c.DBSize(ctx).Result(); err == nil {
		out["keys"] = n
	}
	writeJSON(w, 200, out)
}
