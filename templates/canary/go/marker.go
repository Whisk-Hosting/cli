package main

import (
	_ "embed"
	"net/http"
	"strings"
)

// marker is marker.txt, built into the binary. The platform's release canary writes a fresh value
// there in every commit it pushes and reads it back at /diag/marker, so it knows the copy that
// answers was built from that commit (DEPLOYMENT.md "The release canary").
//
//go:embed marker.txt
var marker string

// markerRoute answers the marker as plain text.
func markerRoute(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(strings.TrimSpace(marker) + "\n"))
}
