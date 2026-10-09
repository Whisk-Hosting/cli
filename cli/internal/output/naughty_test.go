package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/whisk-run/contract/naughty"
)

// After Clean, text from elsewhere can only print: no control byte but tab and newline, no
// escape sequence but colour and style, no C1 control, and cleaning again changes nothing.
func TestNaughtyClean(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, in := range []string{s, "\x1b[31m" + s + "\x1b[0m", "\x1b]52;c;" + s + "\x07", "\x1b[" + s + "m", "\u009b" + s} {
			out := Clean([]byte(in))
			if !bytes.Equal(Clean(out), out) {
				t.Errorf("Clean(%q) is not stable: %q then %q", in, out, Clean(out))
			}
			for i := 0; i < len(out); i++ {
				c := out[i]
				switch {
				case c == 0x1b:
					n := sgrLength(out[i:])
					if n == 0 {
						t.Errorf("Clean(%q) kept a bare escape: %q", in, out)
						continue
					}
					i += n - 1
				case c == '\n' || c == '\t':
				case c < 0x20 || c == 0x7f:
					t.Errorf("Clean(%q) kept control byte %#x", in, c)
				}
			}
			for _, r := range string(out) {
				if r >= 0x80 && r <= 0x9f {
					t.Errorf("Clean(%q) kept C1 control %U", in, r)
				}
			}
		}
	}
}

// Results and errors made of naughty text are one valid JSON object with the text intact, and
// the human forms neither panic nor lose a row.
func TestNaughtyPrinter(t *testing.T) {
	for _, s := range naughty.Strings() {
		var out, errb bytes.Buffer
		p := Printer{JSON: true, Out: &out, Err: &errb, Version: "test"}
		p.Result(map[string]any{"value": s}, nil)
		var got map[string]any
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Errorf("Result(%q) is not JSON: %v", s, err)
		} else if utf8.ValidString(s) && got["value"] != s {
			t.Errorf("Result(%q) read back as %q", s, got["value"])
		}
		out.Reset()
		if code := p.Fail(New(s, s, s, map[string]any{"url": s})); code != ExitError && code != ExitCode(&Error{Code: s}) {
			t.Errorf("Fail(%q) exit %d", s, code)
		}
		if !json.Valid(out.Bytes()) || strings.Count(strings.TrimSpace(out.String()), "\n") != 0 {
			t.Errorf("Fail(%q) did not print one JSON line: %q", s, out.String())
		}
		out.Reset()
		p.Fail(errors.New(s))
		if !strings.Contains(out.String(), `"CLI_ERROR"`) {
			t.Errorf("a plain error %q is not CLI_ERROR: %s", s, out.String())
		}
		human := Printer{Out: &out, Err: &errb}
		out.Reset()
		human.Table(&out, []string{"NAME", "VALUE"}, [][]string{{s, s}, {"x", s}})
		if lines := strings.Count(out.String(), "\n") - strings.Count(s, "\n")*3; lines != 3 {
			t.Errorf("Table with %q printed %d lines", s, lines)
		}
		human.Block(&errb, New("INVALID_REQUEST", s, s, map[string]any{"url": s}))
		human.Line(nil, s)
		if Outdated(s, s) || Outdated("dev", s) {
			t.Errorf("Outdated(%q, itself) or from dev is true", s)
		}
	}
}
