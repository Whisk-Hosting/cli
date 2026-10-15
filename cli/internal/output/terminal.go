package output

import (
	"io"
	"unicode/utf8"
)

// Terminal wraps a human-facing stream so text from elsewhere (an app's log lines, a server's
// error message, git's sideband) cannot drive the terminal: every control character except tab
// and newline is dropped, and of the escape sequences only colour and style (ESC [ digits ; m)
// pass. An app that logs a visitor's input could otherwise write to the clipboard, retitle the
// window, hide lines or plant a link that reads differently from where it goes.
func Terminal(w io.Writer) io.Writer { return terminal{w} }

type terminal struct{ w io.Writer }

func (t terminal) Write(p []byte) (int, error) {
	if _, err := t.w.Write(Clean(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Clean is the pure filter Terminal applies to one write.
func Clean(p []byte) []byte {
	out := make([]byte, 0, len(p))
	for i := 0; i < len(p); {
		c := p[i]
		switch {
		case c == '\n' || c == '\t':
			out = append(out, c)
			i++
		case c == 0x1b:
			if n := sgrLength(p[i:]); n > 0 {
				out = append(out, p[i:i+n]...)
				i += n
				continue
			}
			i++
		case c < 0x20 || c == 0x7f:
			i++
		case c < utf8.RuneSelf:
			out = append(out, c)
			i++
		default:
			r, size := utf8.DecodeRune(p[i:])
			// C1 controls (U+0080 to U+009F) act like ESC sequences in some terminals.
			if r != utf8.RuneError && (r < 0x80 || r > 0x9f) {
				out = append(out, p[i:i+size]...)
			}
			i += size
		}
	}
	return out
}

// sgrLength is the length of a colour or style sequence at the start of p, or 0.
func sgrLength(p []byte) int {
	if len(p) < 3 || p[1] != '[' {
		return 0
	}
	for j := 2; j < len(p) && j < 32; j++ {
		switch c := p[j]; {
		case c == 'm':
			return j + 1
		case c == ';' || (c >= '0' && c <= '9'):
		default:
			return 0
		}
	}
	return 0
}
