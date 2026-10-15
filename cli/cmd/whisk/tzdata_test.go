package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The binary must carry the zone database: Windows has none for time.LoadLocation to read.
func TestBinaryEmbedsTimeZoneData(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if pkg == "time/tzdata" {
			return
		}
	}
	t.Fatal("cmd/whisk does not import time/tzdata")
}
