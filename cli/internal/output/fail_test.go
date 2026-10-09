package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// Fail finds the error object however deep it is wrapped; anything else is CLI_ERROR.
func TestFailUnwraps(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
		exit int
	}{
		{"the object", New("PLATFORM_UNAVAILABLE", "m", "f", nil), "PLATFORM_UNAVAILABLE", ExitCode(&Error{Code: "PLATFORM_UNAVAILABLE"})},
		{"wrapped once", fmt.Errorf("following: %w", New("AUTH_REQUIRED", "m", "f", nil)), "AUTH_REQUIRED", ExitCode(&Error{Code: "AUTH_REQUIRED"})},
		{"joined", errors.Join(errors.New("first"), New("NOT_FOUND", "m", "f", nil)), "NOT_FOUND", ExitCode(&Error{Code: "NOT_FOUND"})},
		{"plain", errors.New("disk full"), "CLI_ERROR", ExitError},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		p := Printer{JSON: true, Out: &out, Err: &out}
		exit := p.Fail(tc.err)
		var body struct {
			Error struct{ Code, Fix string } `json:"error"`
		}
		if err := json.Unmarshal(out.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v: %s", tc.name, err, out.String())
		}
		if body.Error.Code != tc.code || body.Error.Fix == "" || exit != tc.exit {
			t.Errorf("%s: code %q fix %q exit %d; want %q exit %d", tc.name, body.Error.Code, body.Error.Fix, exit, tc.code, tc.exit)
		}
	}
}
