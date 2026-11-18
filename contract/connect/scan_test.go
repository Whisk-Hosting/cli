package connect

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf8"

	"github.com/whisk-run/contract/naughty"
)

// TestScanBody: the strings at the pointers asked for, from one pass over the body.
func TestScanBody(t *testing.T) {
	big := strings.Repeat("A", 3<<20)
	for _, c := range []struct {
		name     string
		body     string
		pointers []string
		want     map[string]string
	}{
		{"a model call", `{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`, []string{"/model"}, map[string]string{"/model": "gpt-x"}},
		{"the field after a picture", `{"messages":[{"content":"` + big + `"}],"model":"gpt-x"}`, []string{"/model"}, map[string]string{"/model": "gpt-x"}},
		{"inside objects and arrays", `{"params":{"service":"object","args":["db",2,"k","sale.order","create"]}}`,
			[]string{"/params/service", "/params/args/3", "/params/args/4", "/params/args/9"},
			map[string]string{"/params/service": "object", "/params/args/3": "sale.order", "/params/args/4": "create"}},
		{"a value that is not a string is not found", `{"model":7,"a":{"b":null},"c":["x"]}`, []string{"/model", "/a/b", "/c"}, map[string]string{}},
		{"escapes in a key and a string", `{"mod\u0065l":"a\"b\\c\/d\n\u00e9\ud83d\ude00"}`, []string{"/model"}, map[string]string{"/model": "a\"b\\c/d\né😀"}},
		{"a key with a slash and a tilde", `{"a/b":{"c~d":"v"}}`, []string{"/a~1b/c~0d"}, map[string]string{"/a~1b/c~0d": "v"}},
		{"a key repeated off the path", `{"x":{"k":1,"k":2},"model":"m"}`, []string{"/model"}, map[string]string{"/model": "m"}},
		{"a string too long to match anything", `{"model":"` + strings.Repeat("m", maxScanString+1) + `"}`, []string{"/model"}, map[string]string{}},
		{"a scalar document", ` "x" `, []string{"/model"}, map[string]string{}},
		{"numbers of every shape", `[0,-0,1.5,-2e10,3E+2,4e-1,10]`, nil, map[string]string{}},
		{"half a pair where nothing is matched", `{"note":"\ud800","model":"m"}`, []string{"/model"}, map[string]string{"/model": "m"}},
	} {
		// One byte at a time, so nothing depends on where a read ends.
		for _, r := range []io.Reader{strings.NewReader(c.body), iotest.OneByteReader(strings.NewReader(c.body))} {
			if len(c.body) > 1<<20 {
				r = strings.NewReader(c.body)
			}
			got, err := ScanBody(r, c.pointers)
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
				continue
			}
			if len(got) != len(c.want) {
				t.Errorf("%s: found %v, want %v", c.name, got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("%s: %s is %q, want %q", c.name, k, got[k], v)
				}
			}
		}
	}
}

// TestScanBodyRefuses: what is not one JSON document, and what two readers could take two ways
// where an operation matches.
func TestScanBodyRefuses(t *testing.T) {
	for _, c := range []struct {
		name      string
		body      string
		ambiguous string
	}{
		{"nothing", "", ""},
		{"white space alone", " \n", ""},
		{"two documents", `{"model":"a"}{"model":"b"}`, ""},
		{"text after the document", `{"model":"a"} x`, ""},
		{"an object cut short", `{"model":"a"`, ""},
		{"a string cut short", `{"model":"a`, ""},
		{"a trailing comma", `{"model":"a",}`, ""},
		{"a comment", `{"model":"a"/* b */}`, ""},
		{"single quotes", `{'model':'a'}`, ""},
		{"a key without quotes", `{model:"a"}`, ""},
		{"a leading zero", `{"n":01}`, ""},
		{"a bare decimal point", `{"n":1.}`, ""},
		{"an exponent with no digits", `{"n":1e}`, ""},
		{"a plus sign", `{"n":+1}`, ""},
		{"NaN", `{"n":NaN}`, ""},
		{"a control character in a string", "{\"model\":\"a\tb\"}", ""},
		{"an escape JSON has none of", `{"model":"\x41"}`, ""},
		{"a short unicode escape", `{"model":"\u12"}`, ""},
		{"bytes that are not UTF-8", "{\"note\":\"\xff\"}", ""},
		{"an overlong UTF-8 sequence", "{\"note\":\"\xc0\xaf\"}", ""},
		{"a byte order mark", "\xef\xbb\xbf{}", ""},
		{"the matched key twice", `{"model":"allowed","model":"other"}`, "/model"},
		{"the matched key twice, once escaped", `{"model":"allowed","mod\u0065l":"other"}`, "/model"},
		{"a key on the way twice", `{"params":{"service":"object"},"params":{"service":"other"}}`, "/params"},
		{"half a pair in the matched string", `{"model":"a\ud800"}`, "/model"},
		{"half a pair in a key on the way", `{"mod\udc00el":"a"}`, ""},
	} {
		_, err := ScanBody(strings.NewReader(c.body), []string{"/model", "/params/service"})
		var amb *AmbiguousBodyError
		switch {
		case err == nil:
			t.Errorf("%s: read without complaint", c.name)
		case c.ambiguous != "" && (!errors.As(err, &amb) || amb.Pointer != c.ambiguous):
			t.Errorf("%s: %v, want ambiguous at %s", c.name, err, c.ambiguous)
		case c.ambiguous == "" && !errors.Is(err, ErrBodyNotJSON) && !errors.As(err, &amb):
			t.Errorf("%s: %v, want not JSON", c.name, err)
		}
	}
	deep := strings.Repeat("[", maxScanDepth+1) + strings.Repeat("]", maxScanDepth+1)
	if _, err := ScanBody(strings.NewReader(deep), nil); !errors.Is(err, ErrBodyNotJSON) {
		t.Errorf("nesting past the depth: %v", err)
	}
	boom := errors.New("the caller went away")
	if _, err := ScanBody(io.MultiReader(strings.NewReader(`{"model":`), iotest.ErrReader(boom)), nil); !errors.Is(err, boom) || errors.Is(err, ErrBodyNotJSON) {
		t.Errorf("a read that fails: %v, want the read's own error", err)
	}
}

// agrees holds ScanBody to encoding/json: it reads a body exactly when the decoder does, bar
// what it refuses on purpose, and finds the strings the decoder finds.
func agrees(t *testing.T, body string, pointers []string) {
	t.Helper()
	found, err := ScanBody(strings.NewReader(body), pointers)
	doc, decErr := DecodeBody([]byte(body))
	var amb *AmbiguousBodyError
	if errors.As(err, &amb) {
		return // refused either way; it may also stop being JSON further on
	}
	// Refused on purpose where the decoder reads on: bytes that are not UTF-8.
	if err != nil && decErr == nil && !utf8.ValidString(body) {
		return
	}
	if (err == nil) != (decErr == nil) {
		t.Fatalf("%q: scan says %v, the decoder says %v", body, err, decErr)
	}
	if err != nil {
		return
	}
	for _, p := range pointers {
		toks, _ := pointer(p)
		want, isString := valueAt(doc, toks).(string)
		got, ok := found[p]
		// A string past maxScanString is left out, as is one with half a pair (ambiguous above).
		if isString && len(want) > maxScanString {
			isString = false
		}
		if ok != isString || got != want && isString {
			t.Fatalf("%q at %s: scan found %q (%v), the decoder %q (%v)", body, p, got, ok, want, isString)
		}
	}
}

var scanSeeds = []string{
	`{"model":"m","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`,
	`{"params":{"service":"object","args":["db",2,"k","sale.order","create",[{"a":1.5e3}]]}}`,
	`[]`, `{}`, `null`, `true`, `-0.0e-0`, `"\ud83d\ude00"`, `{"a":{"a":{"a":"a"}}}`, `[[[["x"]]]]`,
	`{"model":"\u006d","0":"zero","a":["x","y"]}`, ` { "model" : "m" , "a" : [ 1 , 2 ] } `,
	`{"a":"\"","b":"\\","c":"\/"}`, `{"":""}`, `{"model":""}`, `[1,2`, `{"a"}`, `{"a":}`, `[,]`, `01`, `1.e1`, `"\u00"`, `tru`, `nul`,
}

func TestScanBodyAgreesWithTheDecoder(t *testing.T) {
	pointers := []string{"/model", "/params/service", "/params/args/3", "/a/a/a", "/a/1", "/0", "/"}
	for _, s := range scanSeeds {
		agrees(t, s, pointers)
		for i := range s { // every prefix, and every body with one byte dropped
			agrees(t, s[:i], pointers)
			agrees(t, s[:i]+s[i+1:], pointers)
		}
	}
	for _, s := range naughty.Strings() {
		agrees(t, s, pointers)
		if b, err := json.Marshal(map[string]any{"model": s, s: "x", "a": []string{s, s}}); err == nil {
			agrees(t, string(b), pointers)
		}
	}
}

func FuzzScanBody(f *testing.F) {
	for _, s := range scanSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		agrees(t, body, []string{"/model", "/a/a/a", "/a/1", "/0", "/"})
	})
}

// TestMatchScanned: an operation with body conditions matches a streamed body exactly as it
// matches a decoded one.
func TestMatchScanned(t *testing.T) {
	ops := rpc().Operations
	if got := BodyPointers(ops); strings.Join(got, " ") != "/params/args/3 /params/args/4 /params/service" {
		t.Fatalf("pointers %v", got)
	}
	for _, body := range []string{
		`{"params":{"service":"object","args":["db",2,"k","product.product","search_read"]}}`,
		`{"params":{"service":"object","args":["db",2,"k","sale.order","create"]}}`,
		`{"params":{"service":"object","args":["db",2,"k","sale.order","unlink"]}}`,
		`{"params":{"service":"common","args":["db",2,"k","sale.order","create"]}}`,
		`{}`,
	} {
		doc, _ := DecodeBody([]byte(body))
		want, wantOK := Match(ops, "POST", "/jsonrpc", doc)
		found, err := ScanBody(bytes.NewReader([]byte(body)), BodyPointers(ops))
		if err != nil {
			t.Fatal(err)
		}
		got, ok := MatchScanned(ops, "POST", "/jsonrpc", found)
		if ok != wantOK || got.Name != want.Name {
			t.Errorf("%s: streamed matches %q (%v), decoded %q (%v)", body, got.Name, ok, want.Name, wantOK)
		}
	}
}
