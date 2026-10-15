package doctor

import (
	"strings"

	"github.com/whisk-run/cli/internal/stack"
)

// blankComments is the file with every comment replaced by the line breaks it held, so a line
// number read from the result is the file's own and a literal inside a comment does not count.
// It reads string literals as it goes, so "*/3 * * * *", "src/**" or "https://x" in code is
// text, not the start of a comment. Python: # comments; '…', "…" and their triple-quoted forms.
// TypeScript, JavaScript and Go: // and /* */ comments; '…', "…" and `…`, and in TypeScript and
// JavaScript a regular expression literal after an operator or an opening bracket. Pure.
func blankComments(lang stack.Lang, content []byte) []byte {
	var out strings.Builder
	out.Grow(len(content))
	src := string(content)
	py := lang == stack.PY
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case py && c == '#':
			i = skipTo(src, i, "\n", &out)
		case !py && strings.HasPrefix(src[i:], "//"):
			i = skipTo(src, i, "\n", &out)
		case !py && strings.HasPrefix(src[i:], "/*"):
			i = skipTo(src, i+2, "*/", &out)
		case py && (strings.HasPrefix(src[i:], `"""`) || strings.HasPrefix(src[i:], "'''")):
			i = copyString(src, i, src[i:i+3], true, &out)
		case c == '"' || c == '\'':
			i = copyString(src, i, src[i:i+1], true, &out)
		case !py && c == '`':
			i = copyString(src, i, "`", lang != stack.GO, &out)
		case lang == stack.JS && c == '/' && regexCanStart(src[:i]):
			i = copyRegex(src, i, &out)
		default:
			out.WriteByte(c)
			i++
		}
	}
	return []byte(out.String())
}

// stripComments is blankComments; a literal inside a comment does not count.
func stripComments(lang stack.Lang, content []byte) []byte { return blankComments(lang, content) }

// skipTo drops src from i through the end marker (or to the end of the file), writing only the
// line breaks it held; a line comment's own line break is kept. It returns where reading resumes.
func skipTo(src string, i int, end string, out *strings.Builder) int {
	j := strings.Index(src[i:], end)
	stop := len(src)
	if j >= 0 {
		stop = i + j
		if end != "\n" {
			stop += len(end)
		}
	}
	out.WriteString(strings.Repeat("\n", strings.Count(src[i:stop], "\n")))
	return stop
}

// copyString writes the string literal opening at i with quote q and returns the index after it.
// A single-line quote ends at a line break if unclosed, so one stray quote cannot hide the rest
// of the file. escapes says whether a backslash escapes the next character.
func copyString(src string, i int, q string, escapes bool, out *strings.Builder) int {
	multiline := len(q) == 3 || q == "`"
	j := i + len(q)
	for j < len(src) {
		switch {
		case escapes && src[j] == '\\' && j+1 < len(src):
			j += 2
		case strings.HasPrefix(src[j:], q):
			j += len(q)
			out.WriteString(src[i:j])
			return j
		case src[j] == '\n' && !multiline:
			out.WriteString(src[i:j])
			return j
		default:
			j++
		}
	}
	out.WriteString(src[i:])
	return len(src)
}

// regexCanStart reports whether a / after before opens a regular expression literal rather than
// dividing: it follows an operator, an opening bracket, a keyword that takes an expression, or
// nothing on the line.
func regexCanStart(before string) bool {
	t := strings.TrimRight(before, " \t")
	if t == "" || strings.HasSuffix(t, "\n") {
		return true
	}
	if strings.ContainsRune("(,=:[!&|?{};+-*%<>~^", rune(t[len(t)-1])) {
		return true
	}
	for _, kw := range []string{"return", "typeof", "case", "in", "of", "yield", "await", "void", "delete"} {
		if strings.HasSuffix(t, kw) && (len(t) == len(kw) || !isIdent(t[len(t)-len(kw)-1])) {
			return true
		}
	}
	return false
}

func isIdent(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// copyRegex writes the regular expression literal opening at i, through its flags, and returns
// the index after it. A / inside a character class does not close it; a line break does.
func copyRegex(src string, i int, out *strings.Builder) int {
	j, class := i+1, false
	for j < len(src) && src[j] != '\n' {
		switch c := src[j]; {
		case c == '\\' && j+1 < len(src):
			j++
		case c == '[':
			class = true
		case c == ']':
			class = false
		case c == '/' && !class:
			j++
			for j < len(src) && isIdent(src[j]) {
				j++
			}
			out.WriteString(src[i:j])
			return j
		}
		j++
	}
	out.WriteString(src[i:j])
	return j
}
