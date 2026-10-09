// Package errors is the error catalogue as data, parsed from errors.md so the document stays
// the single source of truth. The CLI uses it for whisk errors <CODE>; the stub uses it to
// answer with the standard error body.
package errors

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/whisk-run/contract"
)

// Entry is one section of errors.md.
type Entry struct {
	Code string `json:"code"`
	// Status is the HTTP status, or 0 for codes that do not arrive as an HTTP response.
	Status   int      `json:"status"`
	Surfaces []string `json:"surfaces"`
	When     string   `json:"when"`
	Fix      string   `json:"fix"`
	Example  string   `json:"example"`
}

// Body is the wire format of every error.
type Body struct {
	Error Detail `json:"error"`
}

// Detail is the inner object.
type Detail struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Fix     string         `json:"fix"`
	Docs    string         `json:"docs"`
	Details map[string]any `json:"details,omitempty"`
}

// DocsBase is where every code is documented.
const DocsBase = "https://skill.whisk.run/errors/"

// New builds the standard error body for a code. Message and fix default to the catalogue's
// wording when empty.
func New(code, message, fix string, details map[string]any) Body {
	e, _ := Lookup(code)
	if message == "" {
		message = e.When
	}
	if fix == "" {
		fix = e.Fix
	}
	return Body{Error: Detail{Code: code, Message: message, Fix: fix, Docs: DocsBase + code, Details: details}}
}

// JSON renders the body.
func (b Body) JSON() []byte {
	out, _ := json.Marshal(b)
	return out
}

var (
	reHeading = regexp.MustCompile(`(?m)^## ([A-Z][A-Z0-9_]+)\s*$`)
	reStatus  = regexp.MustCompile(`(?m)^Status: (\d{3}|-) · Surface: (.+)$`)
	reWhen    = regexp.MustCompile(`(?s)When: (.+?)\n\n`)
	reFix     = regexp.MustCompile(`(?s)Fix: (.+?)\n\n`)
	reExample = regexp.MustCompile("(?s)```json\n(.+?)\n```")
)

var catalogue = func() map[string]Entry {
	out := map[string]Entry{}
	doc := contract.ErrorsDoc
	locs := reHeading.FindAllStringSubmatchIndex(doc, -1)
	for i, loc := range locs {
		end := len(doc)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		section := doc[loc[1]:end]
		e := Entry{Code: doc[loc[2]:loc[3]]}
		if m := reStatus.FindStringSubmatch(section); m != nil {
			e.Status, _ = strconv.Atoi(m[1])
			for _, s := range strings.Split(m[2], ",") {
				e.Surfaces = append(e.Surfaces, strings.TrimSpace(s))
			}
		}
		if m := reWhen.FindStringSubmatch(section); m != nil {
			e.When = oneLine(m[1])
		}
		if m := reFix.FindStringSubmatch(section); m != nil {
			e.Fix = oneLine(m[1])
		}
		if m := reExample.FindStringSubmatch(section); m != nil {
			e.Example = m[1]
		}
		out[e.Code] = e
	}
	return out
}()

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Lookup returns the catalogue entry for a code.
func Lookup(code string) (Entry, bool) {
	e, ok := catalogue[code]
	return e, ok
}

// Codes returns every code in order.
func Codes() []string {
	out := make([]string, 0, len(catalogue))
	for c := range catalogue {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Section returns the markdown section for a code, for whisk errors <CODE>.
func Section(code string) (string, bool) {
	doc := contract.ErrorsDoc
	locs := reHeading.FindAllStringSubmatchIndex(doc, -1)
	for i, loc := range locs {
		if doc[loc[2]:loc[3]] != code {
			continue
		}
		end := len(doc)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return strings.TrimSpace(doc[loc[0]:end]), true
	}
	return "", false
}
