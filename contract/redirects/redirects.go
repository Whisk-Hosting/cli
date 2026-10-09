// Package redirects is the redirect rule an app declares in whisk.yaml and in its redirects
// file (CONTRACT.md §3.1): how a rule is written, checked, and matched against a request. The
// manifest validator, the CLI's doctor and the edge's whisk_redirect handler all use it, so a
// rule means the same thing at every step. Everything here is pure.
package redirects

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Rule is one redirect: a request for From is answered Status with Location To.
type Rule struct {
	// From is the old address: an absolute path, optionally with a query naming the parameters
	// a request must carry with those values (/product.php?id=12), a pattern whose segments
	// :name match any one segment (/blog/:year/:slug), or a prefix ending in /* (/blog/*)
	// that matches the path without the star and everything below it.
	From string `json:"from"`
	// To is the new address: a path on the same host or an absolute http(s) URL. A prefix
	// rule's To may end in *, which takes what the request had after the prefix, and a
	// pattern's To may use each :name its From has. Empty only for 410.
	To string `json:"to,omitempty"`
	// Status is 301 (default), 302, 307, 308 or 410.
	Status int `json:"status,omitempty"`
	// Query is what happens to the request's query: keep (added to To) or drop. The default is
	// keep, except for a rule whose From names a query, which drops it.
	Query string `json:"query,omitempty"`
}

// Limits keep the edge's work per request constant and its configuration small enough to load
// at every change (CADDY.md §5.6).
const (
	MaxRules  = 10_000
	MaxLength = 2_000
	MaxParams = 10
	// MaxPatterns bounds the rules with :name segments, the only ones a request is compared
	// with one by one.
	MaxPatterns  = 1_000
	MaxFileBytes = 2 << 20
	// maxHops bounds the walk that finds chains and loops.
	maxHops = 20
)

const (
	QueryKeep = "keep"
	QueryDrop = "drop"
)

// DefaultStatus is a permanent redirect, the one search engines move a page's ranking along.
const DefaultStatus = 301

// Problem is one thing wrong with a rule. Index is the rule's position in the list checked.
type Problem struct {
	Index   int    `json:"index"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	return fmt.Sprintf("redirect %d %s: %s", p.Index, p.Field, p.Message)
}

// Normal returns the rule with its defaults filled in.
func (r Rule) Normal() Rule {
	if r.Status == 0 {
		r.Status = DefaultStatus
	}
	if r.Query == "" {
		r.Query = QueryKeep
		if strings.Contains(r.From, "?") {
			r.Query = QueryDrop
		}
	}
	return r
}

// Prefix reports whether the rule matches a path and everything below it.
func (r Rule) Prefix() bool { return strings.HasSuffix(r.From, "/*") }

// Pattern reports whether the rule has :name segments.
func (r Rule) Pattern() bool { return len(names(r.From)) > 0 }

var reName = regexp.MustCompile(`^:[A-Za-z_][A-Za-z0-9_]*$`)
var reToName = regexp.MustCompile(`:[A-Za-z_][A-Za-z0-9_]*`)

// names are a from's :name segments, in order.
func names(from string) []string {
	path, _, _ := strings.Cut(from, "?")
	var out []string
	for _, seg := range strings.Split(path, "/") {
		if reName.MatchString(seg) {
			out = append(out, seg[1:])
		}
	}
	return out
}

// Check lists every problem in rules, the manifest's and the file's together: each rule on its
// own, the limits, duplicates and loops. A chain (a rule whose target another rule redirects
// again) is not a problem here; Chains reports it for doctor.
func Check(rules []Rule) []Problem {
	var out []Problem
	if len(rules) > MaxRules {
		out = append(out, Problem{Index: MaxRules, Field: "from", Message: fmt.Sprintf("%d redirects; an app may have at most %d. Replace runs of rules with a prefix rule (/old/* to /new/*)", len(rules), MaxRules)})
		return out
	}
	patterns := 0
	for _, r := range rules {
		if r.Pattern() {
			patterns++
		}
	}
	if patterns > MaxPatterns {
		out = append(out, Problem{Index: MaxRules, Field: "from", Message: fmt.Sprintf("%d rules with :name segments; at most %d. Use a prefix rule (/old/* to /new/*) where the rest of the path is kept", patterns, MaxPatterns)})
		return out
	}
	seen := map[string]int{}
	for i, r := range rules {
		ps := checkOne(r)
		for j := range ps {
			ps[j].Index = i
		}
		out = append(out, ps...)
		if len(ps) > 0 {
			continue
		}
		key := matchKey(r)
		if j, dup := seen[key]; dup {
			out = append(out, Problem{Index: i, Field: "from", Message: fmt.Sprintf("%s is already redirected by redirect %d", r.From, j)})
			continue
		}
		seen[key] = i
	}
	if len(out) > 0 {
		return out
	}
	m := Compile(rules)
	for i := range rules {
		if hops, loop := m.follow(i); loop {
			out = append(out, Problem{Index: i, Field: "to", Message: "redirects in a loop: " + strings.Join(hops, " to ")})
		}
	}
	return out
}

func checkOne(r Rule) []Problem {
	var out []Problem
	bad := func(field, msg string) { out = append(out, Problem{Field: field, Message: msg}) }
	r = r.Normal()
	switch {
	case r.From == "":
		bad("from", "required: the old path, such as /about-us")
	case len(r.From) > MaxLength:
		bad("from", fmt.Sprintf("longer than %d characters", MaxLength))
	case !strings.HasPrefix(r.From, "/") || strings.HasPrefix(r.From, "//"):
		bad("from", r.From+" is not a path; write it from the first slash, such as /about-us")
	case strings.ContainsAny(r.From, "# \t\r\n"):
		bad("from", r.From+" has a space or #; percent-encode it")
	default:
		path, query, hasQuery := strings.Cut(r.From, "?")
		if star := strings.Index(path, "*"); star >= 0 && (star != len(path)-1 || !strings.HasSuffix(path, "/*")) {
			bad("from", "* may only end the path, after a slash, such as /blog/*")
		}
		if _, err := url.PathUnescape(path); err != nil {
			bad("from", r.From+" has a broken percent-encoding")
		}
		if under(path, "/.whisk") || under(path, "/.well-known/acme-challenge") {
			bad("from", path+" is the platform's own; it cannot be redirected")
		}
		if ns := names(path); len(ns) > 0 {
			seen := map[string]bool{}
			for _, n := range ns {
				if seen[n] {
					bad("from", ":"+n+" appears twice; give each segment its own name")
				}
				seen[n] = true
			}
			if strings.HasSuffix(path, "*") || hasQuery {
				bad("from", "a rule with :name segments cannot also end in /* or name a query")
			}
		}
		if hasQuery {
			switch vals, err := url.ParseQuery(query); {
			case strings.HasSuffix(path, "*"):
				bad("from", "a prefix rule cannot name a query")
			case err != nil || query == "":
				bad("from", "the query "+query+" does not parse; write it as ?name=value&other=value")
			case len(vals) > MaxParams:
				bad("from", fmt.Sprintf("names %d query parameters; at most %d", len(vals), MaxParams))
			}
		}
	}
	switch r.Status {
	case 301, 302, 307, 308:
		checkTo(r, bad)
	case 410:
		if r.To != "" {
			bad("to", "a 410 answers that the page is gone for good; remove to")
		}
	default:
		bad("status", strconv.Itoa(r.Status)+" is not a redirect status; use 301, 302, 307, 308 or 410")
	}
	if r.Query != QueryKeep && r.Query != QueryDrop {
		bad("query", r.Query+" is not keep or drop")
	}
	return out
}

func checkTo(r Rule, bad func(field, msg string)) {
	to := r.To
	switch {
	case to == "":
		bad("to", "required: the new path or URL, such as /about")
		return
	case len(to) > MaxLength:
		bad("to", fmt.Sprintf("longer than %d characters", MaxLength))
		return
	case strings.ContainsAny(to, " \t\r\n"):
		bad("to", to+" has a space; percent-encode it")
		return
	}
	if strings.Contains(to, "*") {
		if !r.Prefix() {
			bad("to", "* in to takes the rest of the path, so from must end in /* too")
		} else if strings.Count(to, "*") > 1 || !strings.HasSuffix(strings.SplitN(to, "?", 2)[0], "*") {
			bad("to", "* may only end the path in to, such as /news/*")
		}
	}
	if strings.HasPrefix(to, "/") && !strings.HasPrefix(to, "//") {
		return
	}
	u, err := url.Parse(strings.ReplaceAll(to, "*", "x"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		bad("to", to+" is neither a path (/new) nor an http or https address")
	}
}

func under(path, dir string) bool { return path == dir || strings.HasPrefix(path, dir+"/") }

// matchKey identifies what a rule matches, so two rules matching the same requests are found.
func matchKey(r Rule) string {
	path, query, _ := strings.Cut(r.From, "?")
	if r.Pattern() {
		segs := strings.Split(path, "/")
		for i, seg := range segs {
			if reName.MatchString(seg) {
				segs[i] = ":"
			}
		}
		path = strings.Join(segs, "/")
	}
	path = clean(path)
	if query == "" {
		return path
	}
	vals, _ := url.ParseQuery(query)
	return path + "?" + vals.Encode()
}

// clean is a path as matched: unescaped, without a trailing slash (other than the root's).
func clean(p string) string {
	if u, err := url.PathUnescape(p); err == nil {
		p = u
	}
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// Matcher answers a request from compiled rules in constant work per request: one map lookup
// for the exact path and one per path segment for the prefixes.
type Matcher struct {
	rules  []Rule
	exact  map[string][]int // cleaned path → rules, those naming more parameters first
	params []url.Values     // a rule's query parameters, by index
	prefix map[string]int   // cleaned prefix without /* → rule
	// patterns are the :name rules by their number of segments, those with more literal
	// segments first.
	patterns map[int][]pattern
}

// pattern is a compiled :name rule: each segment is a literal or, when name is set, any one.
type pattern struct {
	rule     int
	segs     []string
	names    []string // by segment; "" for a literal
	literals int
}

// Compile builds a matcher. Rules Check refuses are left out, so a matcher never panics on
// what it was given.
func Compile(rules []Rule) *Matcher {
	m := &Matcher{exact: map[string][]int{}, prefix: map[string]int{}, params: make([]url.Values, len(rules)), patterns: map[int][]pattern{}}
	for i, r := range rules {
		r = r.Normal()
		m.rules = append(m.rules, r)
		if len(checkOne(r)) > 0 {
			continue
		}
		path, query, _ := strings.Cut(r.From, "?")
		if r.Pattern() {
			segs := segments(clean(path))
			p := pattern{rule: i, segs: segs, names: make([]string, len(segs))}
			for j, seg := range segs {
				if reName.MatchString(seg) {
					p.names[j] = seg[1:]
				} else {
					p.literals++
				}
			}
			m.patterns[len(segs)] = append(m.patterns[len(segs)], p)
			continue
		}
		if r.Prefix() {
			key := clean(strings.TrimSuffix(path, "*"))
			if _, dup := m.prefix[key]; !dup {
				m.prefix[key] = i
			}
			continue
		}
		key := clean(path)
		if query != "" {
			m.params[i], _ = url.ParseQuery(query)
		}
		m.exact[key] = append(m.exact[key], i)
	}
	for _, list := range m.exact {
		sort.SliceStable(list, func(a, b int) bool { return len(m.params[list[a]]) > len(m.params[list[b]]) })
	}
	for _, list := range m.patterns {
		sort.SliceStable(list, func(a, b int) bool { return list[a].literals > list[b].literals })
	}
	return m
}

// segments splits a cleaned path into its segments; the root has none.
func segments(p string) []string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// hit is the rule a request matched, with the rest of the path after a prefix and the
// segments a pattern's names took.
type hit struct {
	rule int
	rest string
	vars map[string]string
}

// Len is the number of rules the matcher holds.
func (m *Matcher) Len() int { return len(m.rules) }

// Answer is what a matching rule says to send.
type Answer struct {
	Rule     int    // the rule's index
	Status   int    // the status to answer
	Location string // empty for 410; a path, or an absolute URL when the rule or base made it so
}

// Match finds the rule for a request's path and raw query, and the answer it gives. base,
// when set (https://example.com), is put in front of a path target: a host that redirects
// as a whole sends a rule's path to its new host in the same single hop.
func (m *Matcher) Match(path, rawQuery, base string) (Answer, bool) {
	h, ok := m.find(path, rawQuery)
	if !ok {
		return Answer{}, false
	}
	r := m.rules[h.rule]
	if r.Status == 410 {
		return Answer{Rule: h.rule, Status: 410}, true
	}
	return Answer{Rule: h.rule, Status: r.Status, Location: location(r, h, rawQuery, base)}, true
}

// find is the rule matching path. Precedence: an exact path naming the most parameters the
// request carries, then an exact path naming none, then a pattern with the most literal
// segments, then the longest prefix.
func (m *Matcher) find(path, rawQuery string) (hit, bool) {
	p := clean(path)
	if list, ok := m.exact[p]; ok {
		var q url.Values
		for _, i := range list {
			want := m.params[i]
			if len(want) == 0 {
				return hit{rule: i}, true
			}
			if q == nil {
				q, _ = url.ParseQuery(rawQuery)
			}
			if carries(q, want) {
				return hit{rule: i}, true
			}
		}
	}
	if len(m.patterns) > 0 {
		segs := segments(p)
		for _, pt := range m.patterns[len(segs)] {
			if vars, ok := pt.match(segs); ok {
				return hit{rule: pt.rule, vars: vars}, true
			}
		}
	}
	for key := p; ; {
		if i, ok := m.prefix[key]; ok {
			rest := strings.TrimPrefix(strings.TrimPrefix(p, key), "/")
			if strings.HasSuffix(path, "/") && rest != "" {
				rest += "/"
			}
			return hit{rule: i, rest: rest}, true
		}
		if key == "/" {
			return hit{}, false
		}
		cut := strings.LastIndex(key, "/")
		if cut <= 0 {
			key = "/"
		} else {
			key = key[:cut]
		}
	}
}

// match compares a request's segments with the pattern's, taking each :name's segment.
func (pt pattern) match(segs []string) (map[string]string, bool) {
	for j, seg := range segs {
		if pt.names[j] == "" && seg != pt.segs[j] || seg == "" {
			return nil, false
		}
	}
	vars := make(map[string]string, len(segs)-pt.literals)
	for j, n := range pt.names {
		if n != "" {
			vars[n] = segs[j]
		}
	}
	return vars, true
}

// carries reports whether q holds each wanted parameter with the wanted value.
func carries(q, want url.Values) bool {
	for k, vs := range want {
		for _, v := range vs {
			if !slices.Contains(q[k], v) {
				return false
			}
		}
	}
	return true
}

// location builds the Location for a rule: the target, the rest of a prefix match in place of
// its *, a pattern's segments in place of their :names, the request's query when the rule keeps it, and base before a path target.
func location(r Rule, h hit, rawQuery, base string) string {
	to := r.To
	if strings.Contains(to, "*") {
		// find leaves no leading slash on rest, so /news/* and a/b make /news/a/b.
		to = strings.Replace(to, "*", (&url.URL{Path: h.rest}).EscapedPath(), 1)
	}
	if len(h.vars) > 0 {
		to = fill(to, func(n string) (string, bool) {
			v, ok := h.vars[n]
			return url.PathEscape(v), ok
		})
	}
	if r.Query == QueryKeep && rawQuery != "" {
		if strings.Contains(to, "?") {
			to += "&" + rawQuery
		} else {
			to += "?" + rawQuery
		}
	}
	if base != "" && strings.HasPrefix(to, "/") {
		to = strings.TrimSuffix(base, "/") + to
	}
	return to
}

// follow walks rule i's target through the rules: the addresses a request passes, from the
// rule's own to the last, and whether it comes back to a rule already passed. Only path targets are followed; an absolute URL leaves the app.
func (m *Matcher) follow(i int) ([]string, bool) {
	hops := []string{m.rules[i].From}
	visited := map[int]bool{i: true}
	cur := i
	for range maxHops {
		r := m.rules[cur]
		if r.Status == 410 || !strings.HasPrefix(r.To, "/") {
			return hops, false
		}
		target := sample(r)
		path, query, _ := strings.Cut(target, "?")
		hops = append(hops, target)
		h, ok := m.find(path, query)
		if !ok {
			return hops, false
		}
		next := h.rule
		if visited[next] {
			return hops, true
		}
		visited[next] = true
		cur = next
	}
	return hops, true
}

// sample is a path a rule's target stands for: its * and its :names each filled by one
// segment.
func sample(r Rule) string {
	to := strings.Replace(r.To, "*", "whisk-sample", 1)
	defined := names(r.From)
	return fill(to, func(n string) (string, bool) { return "whisk-sample", slices.Contains(defined, n) })
}

// fill replaces each :name in a target's path that value knows; anything else stays as written.
func fill(to string, value func(string) (string, bool)) string {
	path, rest, hasRest := strings.Cut(to, "?")
	scheme := ""
	if i := strings.Index(path, "://"); i >= 0 {
		scheme, path = path[:i+3], path[i+3:]
	}
	path = reToName.ReplaceAllStringFunc(path, func(tok string) string {
		if v, ok := value(tok[1:]); ok {
			return v
		}
		return tok
	})
	if hasRest {
		return scheme + path + "?" + rest
	}
	return scheme + path
}

// Chain is a rule whose target another rule redirects again: two hops where one would do.
type Chain struct {
	Index int      `json:"index"`
	Hops  []string `json:"hops"`
}

// Chains lists every rule whose target is redirected again, with the hops a request takes.
// Search engines follow a few hops but each one is slower and may lose ranking, so each rule
// should point straight at the final address.
func Chains(rules []Rule) []Chain {
	m := Compile(rules)
	var out []Chain
	for i := range rules {
		if hops, loop := m.follow(i); !loop && len(hops) > 2 {
			out = append(out, Chain{Index: i, Hops: hops})
		}
	}
	return out
}

// ForMethod is the status to answer a request of method with: a 301 or 302 answers anything
// but GET and HEAD with 308 or 307, which keep the method and body, since browsers resend a
// POST after a 301 or 302 as a GET without its body.
func ForMethod(status int, method string) int {
	if method == "GET" || method == "HEAD" {
		return status
	}
	switch status {
	case 301:
		return 308
	case 302:
		return 307
	}
	return status
}
