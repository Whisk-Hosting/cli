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

// Catalogue is an error catalogue parsed from a document in errors.md's shape: one section per
// code, each with its Status, Surface, When, Fix and example lines.
type Catalogue struct {
	doc     string
	entries map[string]Entry
}

// Parse reads a catalogue from doc. Pure.
func Parse(doc string) Catalogue {
	out := Catalogue{doc: doc, entries: map[string]Entry{}}
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
		out.entries[e.Code] = e
	}
	return out
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Lookup returns the catalogue's entry for a code.
func (c Catalogue) Lookup(code string) (Entry, bool) {
	e, ok := c.entries[code]
	return e, ok
}

// Codes returns every code in the catalogue, in order.
func (c Catalogue) Codes() []string {
	out := make([]string, 0, len(c.entries))
	for code := range c.entries {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// Section returns the markdown section for a code.
func (c Catalogue) Section(code string) (string, bool) {
	locs := reHeading.FindAllStringSubmatchIndex(c.doc, -1)
	for i, loc := range locs {
		if c.doc[loc[2]:loc[3]] != code {
			continue
		}
		end := len(c.doc)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return strings.TrimSpace(c.doc[loc[0]:end]), true
	}
	return "", false
}

// public is errors.md, the catalogue every client reads.
var public = Parse(contract.ErrorsDoc)

// Lookup returns errors.md's entry for a code.
func Lookup(code string) (Entry, bool) { return public.Lookup(code) }

// Codes returns every code in errors.md, in order.
func Codes() []string { return public.Codes() }

// Section returns errors.md's markdown section for a code, for whisk errors <CODE>.
func Section(code string) (string, bool) { return public.Section(code) }
