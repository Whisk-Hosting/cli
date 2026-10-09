package redirects

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Parsed is a redirects file read into rules, with the line each rule came from.
type Parsed struct {
	Rules []Rule
	Lines []int
}

// LineProblem is a line of the file that is not a rule.
type LineProblem struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func (p LineProblem) String() string { return fmt.Sprintf("line %d: %s", p.Line, p.Message) }

// ParseFile reads a redirects file (CONTRACT.md §3.1): one rule a line, its fields separated by
// spaces or tabs, as from, to, an optional status and an optional query=keep or query=drop. A
// 410 rule is from and 410. Blank lines and lines starting with # are skipped. Each rule is
// then checked by Check, together with the manifest's.
func ParseFile(src []byte) (Parsed, []LineProblem) {
	var out Parsed
	var bad []LineProblem
	if len(src) > MaxFileBytes {
		return out, []LineProblem{{Line: 0, Message: fmt.Sprintf("the file is %d bytes; at most %d", len(src), MaxFileBytes)}}
	}
	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(make([]byte, 0, 64<<10), MaxFileBytes+1)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if n == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r, msg := parseLine(strings.Fields(line))
		if msg != "" {
			bad = append(bad, LineProblem{Line: n, Message: msg})
			continue
		}
		out.Rules = append(out.Rules, r)
		out.Lines = append(out.Lines, n)
	}
	if err := sc.Err(); err != nil {
		bad = append(bad, LineProblem{Message: err.Error()})
	}
	return out, bad
}

func parseLine(f []string) (Rule, string) {
	r := Rule{From: f[0]}
	f = f[1:]
	if len(f) > 0 && f[0] == "410" {
		r.Status, f = 410, f[1:]
	} else if len(f) > 0 {
		r.To, f = f[0], f[1:]
	} else {
		return r, "a rule is from and to, such as /old-page /new-page, or from and 410"
	}
	for _, x := range f {
		switch {
		case strings.HasPrefix(x, "query="):
			r.Query = strings.TrimPrefix(x, "query=")
		case r.Status == 0:
			n, err := strconv.Atoi(x)
			if err != nil {
				return r, x + " is not a status or query=keep or query=drop"
			}
			r.Status = n
		default:
			return r, "unexpected " + x + "; a rule is from, to, an optional status and an optional query=keep or query=drop"
		}
	}
	return r, ""
}

// Format writes rules in the file's form, one a line, so a tool can produce the file from a
// list. ParseFile(Format(rules)) gives the rules back.
func Format(rules []Rule) []byte {
	var b strings.Builder
	for _, r := range rules {
		b.WriteString(r.From)
		if r.To != "" {
			b.WriteString(" " + r.To)
		}
		if r.Status != 0 && (r.Status != DefaultStatus || r.To == "") {
			b.WriteString(" " + strconv.Itoa(r.Status))
		}
		if r.Query != "" {
			b.WriteString(" query=" + r.Query)
		}
		b.WriteString("\n")
	}
	return []byte(b.String())
}
