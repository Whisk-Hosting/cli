// Package webpush holds the scripts Whisk serves on every app's address for push notifications
// (CONTRACT.md §8, SKILL.md §9 "Push notifications"): /.whisk/push.js for a page and
// /.whisk/push-sw.js for its service worker. The edge's gate and the local stub serve the same
// bytes.
package webpush

import _ "embed"

// The paths the scripts and the app's public key are served at.
const (
	PagePath   = "/.whisk/push.js"
	WorkerPath = "/.whisk/push-sw.js"
	KeyPath    = "/.whisk/push/key"
)

// Page is /.whisk/push.js.
//
//go:embed push.js
var Page []byte

// Worker is /.whisk/push-sw.js.
//
//go:embed push-sw.js
var Worker []byte
