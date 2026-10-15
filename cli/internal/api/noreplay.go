package api

import "github.com/whisk-run/contract/routes"

// NoReplay reports whether a write to path, below /v1, goes without an Idempotency-Key and is
// never retried: one of the platform's no-replay routes (routes.NoReplay). Pure.
func NoReplay(path string) bool { return routes.NoReplay(path) }
