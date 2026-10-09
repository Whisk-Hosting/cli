// Package doctor is the doctor rule table as data, parsed from doctor-rules.md. The CLI's
// doctor command and the pre-receive hook take rule ids, levels and fix text from here so the
// document and the implementations cannot disagree.
package doctor

import (
	"regexp"
	"sort"
	"strings"

	"github.com/whisk-run/contract"
)

// Level is error or warning.
type Level string

const (
	Error   Level = "error"
	Warning Level = "warning"
)

// Rule is one section of doctor-rules.md.
type Rule struct {
	ID      string `json:"id"`
	Level   Level  `json:"level"`
	Check   string `json:"check"`
	Fix     string `json:"fix"`
	SafeFix bool   `json:"safe_fix"`
}

// Finding is one doctor result, the shape whisk doctor --json prints.
type Finding struct {
	Rule    string `json:"rule"`
	Level   Level  `json:"level"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// ManifestRules are the rule ids that also run in the git pre-receive hook.
var ManifestRules = []string{"W001", "W002", "W003", "W040", "W051", "W052", "W070"}

var (
	reHeading = regexp.MustCompile(`(?m)^## (W\d{3})\s*$`)
	reLevel   = regexp.MustCompile(`(?m)^Level: (error|warning)\s*$`)
	reCheck   = regexp.MustCompile(`(?m)^Check: (.+)$`)
	reFix     = regexp.MustCompile(`(?m)^Fix: (.+)$`)
	reSafe    = regexp.MustCompile(`(?m)^Safe fix: (yes|no)`)
)

var table = func() map[string]Rule {
	out := map[string]Rule{}
	doc := contract.DoctorRulesDoc
	locs := reHeading.FindAllStringSubmatchIndex(doc, -1)
	for i, loc := range locs {
		end := len(doc)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		section := doc[loc[1]:end]
		r := Rule{ID: doc[loc[2]:loc[3]]}
		if m := reLevel.FindStringSubmatch(section); m != nil {
			r.Level = Level(m[1])
		}
		if m := reCheck.FindStringSubmatch(section); m != nil {
			r.Check = strings.TrimSpace(m[1])
		}
		if m := reFix.FindStringSubmatch(section); m != nil {
			r.Fix = strings.TrimSpace(m[1])
		}
		if m := reSafe.FindStringSubmatch(section); m != nil {
			r.SafeFix = m[1] == "yes"
		}
		out[r.ID] = r
	}
	return out
}()

// Lookup returns a rule by id.
func Lookup(id string) (Rule, bool) {
	r, ok := table[id]
	return r, ok
}

// IDs returns every rule id in order.
func IDs() []string {
	out := make([]string, 0, len(table))
	for id := range table {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// NewFinding builds a finding for a rule with the rule's level and fix text.
func NewFinding(id, file string, line int, message string) Finding {
	r := table[id]
	return Finding{Rule: id, Level: r.Level, File: file, Line: line, Message: message, Fix: r.Fix}
}
