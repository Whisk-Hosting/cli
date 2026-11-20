package doctor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
)

// Go code is read from its syntax tree, by the parser the Go compiler uses, so an import is an
// import and a call is a call: neither a comment nor a string that only looks like one counts.
// TypeScript and Python are read by patterns over the file with its comments blanked
// (comments.go), because no parser for them runs inside this program without C.

// goTree is the file's syntax tree, as much of it as parses, and where its lines are.
func goTree(f File) (*token.FileSet, *ast.File) {
	fset := token.NewFileSet()
	tree, _ := parser.ParseFile(fset, f.Path, f.Content, parser.SkipObjectResolution)
	return fset, tree
}

// goImports is each package the file imports whose whole path matches re, named by its path.
func goImports(f File, re *regexp.Regexp) []hit {
	fset, tree := goTree(f)
	if tree == nil {
		return nil
	}
	var out []hit
	for _, imp := range tree.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err == nil && re.MatchString(p) {
			out = append(out, hit{Name: p, File: f.Path, Line: fset.Position(imp.Pos()).Line})
		}
	}
	return out
}

// goText is the text of a string literal, and whether the expression is one.
func goText(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// goJoined is the operands of a chain of +, left to right.
func goJoined(e ast.Expr) []ast.Expr {
	if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.ADD {
		return append(goJoined(b.X), b.Y)
	}
	return []ast.Expr{e}
}

var (
	statementStart = regexp.MustCompile(`(?is)^\s*` + sqlShape)
	valueVerb      = regexp.MustCompile(`%[svdq]`)
)

// goConstants is every name the files declare with const. A constant is text the program holds,
// never a value a person typed.
func goConstants(files []File) map[string]bool {
	out := map[string]bool{}
	for _, f := range files {
		_, tree := goTree(f)
		if tree == nil {
			continue
		}
		ast.Inspect(tree, func(n ast.Node) bool {
			if d, ok := n.(*ast.GenDecl); ok && d.Tok == token.CONST {
				for _, spec := range d.Specs {
					for _, name := range spec.(*ast.ValueSpec).Names {
						out[name.Name] = true
					}
				}
			}
			return true
		})
	}
	return out
}

// goBuildsSQL reports whether the expression makes a statement out of values: fmt.Sprintf with
// a statement for its format and a verb in it, or a statement joined with + to something that
// is neither a literal nor one of the constants.
func goBuildsSQL(e ast.Expr, constants map[string]bool) bool {
	switch n := e.(type) {
	case *ast.CallExpr:
		fn, ok := n.Fun.(*ast.SelectorExpr)
		if !ok || fn.Sel.Name != "Sprintf" || len(n.Args) < 2 {
			return false
		}
		if pkg, ok := fn.X.(*ast.Ident); !ok || pkg.Name != "fmt" {
			return false
		}
		format, ok := goText(n.Args[0])
		return ok && statementStart.MatchString(format) && valueVerb.MatchString(format)
	case *ast.BinaryExpr:
		parts := goJoined(n)
		first, ok := goText(parts[0])
		if !ok || !statementStart.MatchString(first) {
			return false
		}
		for _, p := range parts[1:] {
			if _, literal := goText(p); literal {
				continue
			}
			if name, ok := p.(*ast.Ident); !ok || !constants[name.Name] {
				return true
			}
		}
	}
	return false
}

// goSQLFromStrings is each place a Go file builds SQL by joining text (W096).
func goSQLFromStrings(f File, constants map[string]bool) []hit {
	fset, tree := goTree(f)
	if tree == nil {
		return nil
	}
	var out []hit
	ast.Inspect(tree, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok && goBuildsSQL(e, constants) {
			out = append(out, hit{File: f.Path, Line: fset.Position(e.Pos()).Line})
		}
		return true
	})
	return out
}
