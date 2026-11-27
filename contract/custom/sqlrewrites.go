//whisk:pure

package custom

// SQLRewrites is each column (table.column) whose type the migration sql changes, at the line of
// the statement that does, for a table the file did not make itself. Postgres writes the whole
// table again for most such changes and holds every read and write of it until that ends, which
// on a large table is long enough for the live app's requests to time out. Read as SQLMoves reads.
func SQLRewrites(sql string) []SQLMove {
	s := &sqlScan{made: map[string]bool{}, was: map[string]string{}}
	var out []SQLMove
	for _, st := range sqlStatements(sqlTokens(upOnly(sql))) {
		c := &sqlCursor{t: st}
		if c.words("alter", "table") {
			out = append(out, s.rewrites(c)...)
		}
		s.statement(st)
	}
	return out
}

// rewrites reads the column type changes of one ALTER TABLE, after its first two words.
func (s *sqlScan) rewrites(c *sqlCursor) []SQLMove {
	line := c.line()
	c.words("if", "exists")
	c.words("only")
	name, ok := c.name()
	if !ok || s.made[name] {
		return nil
	}
	c.skipParens()
	c.punct("*")
	var out []SQLMove
	for _, action := range c.actions() {
		a := &sqlCursor{t: action}
		if !a.words("alter") {
			continue
		}
		a.words("column")
		col, ok := a.ident()
		if !ok {
			continue
		}
		if a.words("type") || a.words("set", "data", "type") {
			out = append(out, SQLMove{Name: name + "." + col, Line: a.lineOr(line)})
		}
	}
	return out
}
