package whisk

import "testing"

func TestWebAddress(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://whisk.run/device?code=ABCD-EFGH": true,
		"http://localhost:3000/device":            true,
		"http://127.0.0.1:3000/device":            true,
		"http://whisk.run/device":                 false,
		"file:///C:/Windows/System32/calc.exe":    false,
		`\\attacker\share\run.exe`:                false,
		"javascript:alert(1)":                     false,
		"https://user@whisk.run/":                 false,
		"-a Calculator":                           false,
	} {
		if got := webAddress(raw); got != want {
			t.Errorf("webAddress(%q) = %v, want %v", raw, got, want)
		}
	}
}
