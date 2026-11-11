//whisk:pure

package custom

import "strings"

// SQLMove is a name a migration removes or renames, at the line of the statement that does: To is
// the name a rename gave it, "" for one removed.
type SQLMove struct {
	Name string `json:"name"`
	To   string `json:"to,omitempty"`
	Line int    `json:"line"`
}

// SQLMoves is each table, column (table.column), view, function, procedure, type or type's value
// (type.value) the migration sql removes or renames, as it stood before the file ran, in order.
// What the file made itself is no move, and neither is a view, function, procedure or type it
// takes away and makes again under the same name, as a migration that changes one does. A table or
// a column made again is still a move: what it held is gone, or means something else. SQL run
// through EXECUTE, inside a function or a DO block, is not read, nor what a migration tool runs only
// to undo the file (goose's -- +goose Down, dbmate's -- migrate:down).
func SQLMoves(sql string) []SQLMove {
	s := &sqlScan{made: map[string]bool{}, was: map[string]string{}}
	for _, st := range sqlStatements(sqlTokens(upOnly(sql))) {
		s.statement(st)
	}
	out := make([]SQLMove, 0, len(s.moves))
	for _, m := range s.moves {
		if !(m.replaceable && s.made[m.Name]) {
			out = append(out, m.SQLMove)
		}
	}
	return out
}

// upOnly is sql with the lines a migration tool runs only to undo it left blank, so every other
// line keeps its number.
func upOnly(sql string) string {
	lines := strings.Split(sql, "\n")
	down := false
	for i, l := range lines {
		switch strings.ToLower(strings.Join(strings.Fields(l), " ")) {
		case "-- +goose down", "-- migrate:down":
			down = true
		case "-- +goose up", "-- migrate:up":
			down = false
		}
		if down {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// sqlScan is what a migration made, renamed and moved so far, statement by statement.
type sqlScan struct {
	made  map[string]bool   // names the file made: tables, views, functions, types and table.column
	was   map[string]string // a name a rename gave, and the name it had before the file ran
	moves []sqlMoveAt
}

type sqlMoveAt struct {
	SQLMove
	replaceable bool // a view, function, procedure or type, which a migration may make again
}

// gone records that the statement at line removed what, or renamed it to to; of is the table, view
// or type what is a column or value of, "" for one of those itself.
func (s *sqlScan) gone(of, what, to string, line int, replaceable bool) {
	name := what
	if of != "" {
		name = of + "." + what
	}
	madeHere := s.made[name] || of != "" && s.made[of]
	delete(s.made, name)
	if madeHere {
		if to != "" {
			s.made[to] = true
		}
		return
	}
	before := s.before(name)
	if of != "" {
		before = s.before(of) + "." + what
		if w, ok := s.was[name]; ok {
			before = w
		}
	}
	if to != "" {
		s.was[to] = before
	}
	s.moves = append(s.moves, sqlMoveAt{SQLMove{before, to, line}, replaceable})
}

// before is the name n had before the file ran.
func (s *sqlScan) before(n string) string {
	if w, ok := s.was[n]; ok {
		return w
	}
	return n
}

func (s *sqlScan) statement(t []sqlToken) {
	c := &sqlCursor{t: t}
	switch {
	case c.words("drop"):
		s.drop(c)
	case c.words("alter"):
		s.alter(c)
	case c.words("create"):
		s.create(c)
	}
}

// replaceableKinds are the kinds of what a migration may take away and make again.
var replaceableKinds = []string{"view", "function", "procedure", "type", "domain"}

// kind reads what a statement acts on: table, view (materialized too), function, procedure, type
// or domain, "" for anything else.
func (c *sqlCursor) kind() string {
	c.words("materialized")
	for _, k := range append([]string{"table"}, replaceableKinds...) {
		if c.words(k) {
			return k
		}
	}
	return ""
}

func (s *sqlScan) drop(c *sqlCursor) {
	line := c.line()
	kind := c.kind()
	if kind == "" {
		return
	}
	c.words("if", "exists")
	for {
		name, ok := c.name()
		if !ok {
			return
		}
		c.skipParens()
		s.gone("", name, "", line, kind != "table")
		if !c.punct(",") {
			return
		}
	}
}

func (s *sqlScan) alter(c *sqlCursor) {
	line := c.line()
	kind := c.kind()
	if kind == "" {
		return
	}
	c.words("if", "exists")
	c.words("only")
	name, ok := c.name()
	if !ok {
		return
	}
	c.skipParens()
	c.punct("*")
	if kind != "table" {
		s.alterOther(c, name, kind, line)
		return
	}
	for _, action := range c.actions() {
		a := &sqlCursor{t: action}
		switch {
		case a.words("drop"):
			if a.words("constraint") {
				continue
			}
			a.words("column")
			a.words("if", "exists")
			if col, ok := a.ident(); ok {
				s.gone(name, col, "", a.lineOr(line), false)
			}
		case a.words("rename"):
			s.renameInTable(a, name, line)
		case a.words("add"):
			if a.words("constraint") || a.words("primary") || a.words("unique") || a.words("foreign") || a.words("check") || a.words("exclude") {
				continue
			}
			a.words("column")
			a.words("if", "not", "exists")
			if col, ok := a.ident(); ok {
				s.made[name+"."+col] = true
			}
		}
	}
}

// renameInTable reads what follows RENAME in ALTER TABLE name: the table's own new name, a column's,
// or a constraint's, which is no move.
func (s *sqlScan) renameInTable(a *sqlCursor, table string, line int) {
	if a.words("to") {
		if to, ok := a.name(); ok {
			s.gone("", table, to, line, false)
		}
		return
	}
	if a.words("constraint") {
		return
	}
	a.words("column")
	at := a.lineOr(line)
	col, ok := a.ident()
	if ok && a.words("to") {
		if to, ok := a.ident(); ok {
			s.gone(table, col, table+"."+to, at, false)
		}
	}
}

// alterOther reads ALTER of a view, function, procedure, type or domain: a rename of it, of a
// view's column or of a type's value.
func (s *sqlScan) alterOther(c *sqlCursor, name, kind string, line int) {
	if !c.words("rename") {
		return
	}
	switch {
	case c.words("to"):
		if to, ok := c.name(); ok {
			s.gone("", name, to, line, true)
		}
	case kind == "type" && c.words("value"):
		from, ok := c.str()
		if ok && c.words("to") {
			if to, ok := c.str(); ok {
				s.gone(name, from, name+"."+to, line, false)
			}
		}
	default:
		_ = c.words("column") || c.words("attribute")
		col, ok := c.ident()
		if ok && c.words("to") {
			if to, ok := c.ident(); ok {
				s.gone(name, col, name+"."+to, line, false)
			}
		}
	}
}

func (s *sqlScan) create(c *sqlCursor) {
	c.words("or", "replace")
	for more := true; more; {
		more = c.words("temp") || c.words("temporary") || c.words("unlogged") || c.words("recursive")
	}
	if c.kind() == "" {
		return
	}
	c.words("if", "not", "exists")
	if name, ok := c.name(); ok {
		s.made[name] = true
	}
}

// sqlToken is a word (lower-case), a quoted name (as written inside its quotes), a string's
// contents, or one character of punctuation, at its line.
type sqlToken struct {
	kind byte // 'w' word, 'q' quoted name, 's' string, 'p' punctuation
	text string
	line int
}

// sqlTokens reads sql as PostgreSQL does, leaving out comments and whitespace.
func sqlTokens(sql string) []sqlToken {
	var out []sqlToken
	line := 1
	for i := 0; i < len(sql); {
		tok, n := sqlNext(sql[i:])
		if tok.kind != 0 {
			tok.line = line
			out = append(out, tok)
		}
		line += strings.Count(sql[i:i+n], "\n")
		i += n
	}
	return out
}

// sqlNext is the token at the start of s, which is not empty, and its length: a token of kind 0
// for whitespace or a comment.
func sqlNext(s string) (sqlToken, int) {
	switch ch := s[0]; {
	case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\f' || ch == '\n':
		return sqlToken{}, 1
	case strings.HasPrefix(s, "--"):
		if end := strings.IndexByte(s, '\n'); end >= 0 {
			return sqlToken{}, end
		}
		return sqlToken{}, len(s)
	case strings.HasPrefix(s, "/*"):
		return sqlToken{}, blockComment(s)
	case ch == '\'':
		text, n := quoted(s, '\'', false)
		return sqlToken{kind: 's', text: text}, n
	case ch == '"':
		text, n := quoted(s, '"', false)
		return sqlToken{kind: 'q', text: text}, n
	case ch == '$' && dollarTag(s) != "":
		return dollarQuoted(s)
	case isWordStart(ch):
		return sqlWord(s)
	}
	return sqlToken{kind: 'p', text: s[:1]}, 1
}

// blockComment is the length of the comment at the start of s, which nests as PostgreSQL's do.
func blockComment(s string) int {
	depth := 0
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "/*"):
			depth, i = depth+1, i+2
		case strings.HasPrefix(s[i:], "*/"):
			depth, i = depth-1, i+2
		default:
			i++
		}
		if depth == 0 {
			return i
		}
	}
	return len(s)
}

// dollarQuoted is the dollar-quoted string at the start of s, such as $$…$$ or $body$…$body$, and
// its length.
func dollarQuoted(s string) (sqlToken, int) {
	tag := dollarTag(s)
	body := s[len(tag):]
	if end := strings.Index(body, tag); end >= 0 {
		return sqlToken{kind: 's', text: body[:end]}, len(tag) + end + len(tag)
	}
	return sqlToken{kind: 's', text: body}, len(s)
}

// sqlWord is the word at the start of s, in lower case, or the string it prefixes (E'…', where a
// backslash escapes, X'…', B'…' or N'…'), and its length.
func sqlWord(s string) (sqlToken, int) {
	j := 1
	for j < len(s) && isWordPart(s[j]) {
		j++
	}
	word := strings.ToLower(s[:j])
	if (word == "e" || word == "x" || word == "b" || word == "n") && strings.HasPrefix(s[j:], "'") {
		text, n := quoted(s[j:], '\'', word == "e")
		return sqlToken{kind: 's', text: text}, j + n
	}
	return sqlToken{kind: 'w', text: word}, j
}

// quoted reads a quoted string or name at the start of s, quoted by q, which doubles inside it, or
// escaped by a backslash where backslash says so: its contents and its length.
func quoted(s string, q byte, backslash bool) (string, int) {
	var b strings.Builder
	i := 1
	for i < len(s) {
		switch {
		case backslash && s[i] == '\\' && i+1 < len(s):
			b.WriteByte(s[i+1])
			i += 2
		case s[i] == q && i+1 < len(s) && s[i+1] == q:
			b.WriteByte(q)
			i += 2
		case s[i] == q:
			return b.String(), i + 1
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String(), len(s)
}

// dollarTag is the tag that opens a dollar-quoted string at the start of s, such as $$ or $body$,
// or "" when s does not start one.
func dollarTag(s string) string {
	for j := 1; j < len(s); j++ {
		switch {
		case s[j] == '$':
			return s[:j+1]
		case !isWordPart(s[j]) || j == 1 && s[j] >= '0' && s[j] <= '9':
			return ""
		}
	}
	return ""
}

func isWordStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isWordPart(c byte) bool { return isWordStart(c) || c >= '0' && c <= '9' || c == '$' }

// sqlStatements splits tokens at each semicolon.
func sqlStatements(t []sqlToken) [][]sqlToken {
	var out [][]sqlToken
	start := 0
	for i, tok := range t {
		if tok.kind == 'p' && tok.text == ";" {
			if i > start {
				out = append(out, t[start:i])
			}
			start = i + 1
		}
	}
	if start < len(t) {
		out = append(out, t[start:])
	}
	return out
}

// sqlCursor reads a statement's tokens in order.
type sqlCursor struct {
	t []sqlToken
	i int
}

func (c *sqlCursor) line() int { return c.lineOr(0) }

// lineOr is the line of the next token, or line when there is none.
func (c *sqlCursor) lineOr(line int) int {
	if c.i < len(c.t) {
		return c.t[c.i].line
	}
	if len(c.t) > 0 {
		return c.t[len(c.t)-1].line
	}
	return line
}

// words takes the words ws when the statement goes on with them, and reports whether it did.
func (c *sqlCursor) words(ws ...string) bool {
	if c.i+len(ws) > len(c.t) {
		return false
	}
	for k, w := range ws {
		if tok := c.t[c.i+k]; tok.kind != 'w' || tok.text != w {
			return false
		}
	}
	c.i += len(ws)
	return true
}

func (c *sqlCursor) punct(p string) bool {
	if c.i < len(c.t) && c.t[c.i].kind == 'p' && c.t[c.i].text == p {
		c.i++
		return true
	}
	return false
}

// ident takes one name, a word or a quoted name, as a database knows it: a word in lower case, a
// quoted name as written inside its quotes.
func (c *sqlCursor) ident() (string, bool) {
	if c.i < len(c.t) && (c.t[c.i].kind == 'w' || c.t[c.i].kind == 'q') {
		c.i++
		return c.t[c.i-1].text, true
	}
	return "", false
}

// str takes a string's contents.
func (c *sqlCursor) str() (string, bool) {
	if c.i < len(c.t) && c.t[c.i].kind == 's' {
		c.i++
		return c.t[c.i-1].text, true
	}
	return "", false
}

// name takes a name with its schema, if any, as a database knows it, without the schema public.
func (c *sqlCursor) name() (string, bool) {
	first, ok := c.ident()
	if !ok {
		return "", false
	}
	parts := []string{first}
	for c.punct(".") {
		next, ok := c.ident()
		if !ok {
			return "", false
		}
		parts = append(parts, next)
	}
	if len(parts) > 1 && parts[0] == "public" {
		parts = parts[1:]
	}
	return strings.Join(parts, "."), true
}

// skipParens takes a parenthesised list, such as a function's arguments, when one comes next.
func (c *sqlCursor) skipParens() {
	if !c.punct("(") {
		return
	}
	for depth := 1; c.i < len(c.t) && depth > 0; c.i++ {
		switch tok := c.t[c.i]; {
		case tok.kind == 'p' && tok.text == "(":
			depth++
		case tok.kind == 'p' && tok.text == ")":
			depth--
		}
	}
}

// actions splits the rest of an ALTER TABLE into its actions, at each comma outside parentheses.
func (c *sqlCursor) actions() [][]sqlToken {
	var out [][]sqlToken
	start, depth := c.i, 0
	for i := c.i; i < len(c.t); i++ {
		tok := c.t[i]
		if tok.kind != 'p' {
			continue
		}
		switch tok.text {
		case "(":
			depth++
		case ")":
			depth--
		case ",":
			if depth == 0 {
				out = append(out, c.t[start:i])
				start = i + 1
			}
		}
	}
	return append(out, c.t[start:])
}
