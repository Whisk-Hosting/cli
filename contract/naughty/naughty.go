// Package naughty is the Big List of Naughty Strings (github.com/minimaxir/big-list-of-naughty-strings,
// MIT, see LICENSE-blns): strings with a history of breaking software that takes text from people,
// such as reserved words, odd Unicode, script and SQL injections, format strings and path tricks.
//
// Every place in Whisk that takes text from outside (a name, a path, a manifest, a header, a
// request body, a query) is tested with each of them: it must answer with a value or one of its
// stable errors, never panic, hang or let the string change what it does (docs/HARNESS.md §8).
// The fuzz targets seed their corpora from the same list.
//
// A change's checks try a sample (every one of Extra and one in 25 of the list); the nightly run
// sets WHISK_TESTS=full and tries them all (DEPLOYMENT.md "Checks").
package naughty

import (
	_ "embed"
	"encoding/json"
	"os"
)

//go:embed blns.json
var raw []byte

var list = func() []string {
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		panic("naughty: blns.json is not a JSON list of strings: " + err.Error())
	}
	return out
}()

// Strings is the naughty strings a test tries, plus the extra ones Whisk adds for its own inputs
// (Extra): every one of the list with WHISK_TESTS=full, one in sampleEvery otherwise. The returned
// slice is a fresh copy, so a test may change it.
func Strings() []string {
	return pick(os.Getenv("WHISK_TESTS") == "full")
}

const sampleEvery = 25

func pick(full bool) []string {
	out := make([]string, 0, len(list)+len(Extra))
	for i, s := range list {
		if full || i%sampleEvery == 0 {
			out = append(out, s)
		}
	}
	return append(out, Extra...)
}

// Extra are inputs the list does not carry that matter to Whisk: an empty and a very long
// string, NUL and other control bytes, invalid UTF-8, path traversal in the forms the platform
// has to refuse, and the shapes of names it reserves.
var Extra = []string{
	"",
	" ",
	"\x00",
	"a\x00b",
	"\r\n",
	"\xff\xfe",
	"\xc3\x28",
	"../../../../etc/passwd",
	"..\\..\\..\\windows\\win.ini",
	"%2e%2e%2f%2e%2e%2f",
	"/",
	"//",
	"~",
	"-",
	"--help",
	"$(id)",
	"`id`",
	"${HOME}",
	"{{.}}",
	"%s%s%s%n",
	"null",
	"undefined",
	"__proto__",
	"constructor",
	"admin",
	"api",
	"www",
	"xn--80ak6aa92e",
	"a.b.c.d.e.f.g",
	longString(70000),
}

func longString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(i%26)
	}
	return string(b)
}
