package whisk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/output"
)

func TestStdinValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc\n", "abc"},
		{"abc\r\n", "abc"},
		{"abc", "abc"},
		{"abc\n\n", "abc\n"},
		{"", ""},
		{"\n", ""},
		{" spaced \n", " spaced "},
	}
	for _, c := range cases {
		if got := stdinValue([]byte(c.in)); got != c.want {
			t.Errorf("stdinValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseEnvFile(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []secretValue
		err  bool
	}{
		{"plain lines", "A=1\nB=2\n", []secretValue{{"A", "1"}, {"B", "2"}}, false},
		{"comments, blanks and export", "# a comment\n\nexport A=1\n  B = two  \n", []secretValue{{"A", "1"}, {"B", "two"}}, false},
		{"quotes come off once", "A=\"x y\"\nB='z'\nC=\"'q'\"\nD=\"half\n", []secretValue{{"A", "x y"}, {"B", "z"}, {"C", "'q'"}, {"D", "\"half"}}, false},
		{"an equals sign in the value stays", "A=b=c\n", []secretValue{{"A", "b=c"}}, false},
		{"a name twice keeps the last value in the first place", "A=1\nB=2\nA=3\n", []secretValue{{"A", "3"}, {"B", "2"}}, false},
		{"crlf line ends", "A=1\r\nB=2\r\n", []secretValue{{"A", "1"}, {"B", "2"}}, false},
		{"a line without equals", "A=1\nnope\n", nil, true},
		{"a line without a name", "=1\n", nil, true},
		{"nothing to set", "# only a comment\n", nil, true},
	}
	for _, c := range cases {
		got, err := parseEnvFile([]byte(c.in))
		if (err != nil) != c.err {
			t.Errorf("%s: err %v", c.name, err)
			continue
		}
		if !c.err && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPickSecretValues(t *testing.T) {
	file := []secretValue{{"A", "1"}, {"B", "2"}, {"E", ""}}
	cases := []struct {
		name  string
		names []string
		want  []secretValue
		code  string
	}{
		{"names given, in their order", []string{"B", "A"}, []secretValue{{"B", "2"}, {"A", "1"}}, ""},
		{"a name the file lacks", []string{"A", "C"}, nil, "INVALID_REQUEST"},
		{"an empty value named", []string{"E"}, nil, "INVALID_REQUEST"},
		{"all lines, one of them empty", nil, nil, "INVALID_REQUEST"},
	}
	for _, c := range cases {
		got, err := pickSecretValues(file, c.names)
		var oe *output.Error
		if c.code != "" {
			if !errors.As(err, &oe) || oe.Code != c.code {
				t.Errorf("%s: err %v, want %s", c.name, err, c.code)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v %v, want %v", c.name, got, err, c.want)
		}
	}
	if got, err := pickSecretValues(file[:2], nil); err != nil || len(got) != 2 {
		t.Errorf("every line: %v %v", got, err)
	}
}

// On a terminal each value is typed at a prompt that does not echo, one per name, and nothing
// the CLI prints carries a value.
func TestSecretsSetPromptsOnATerminal(t *testing.T) {
	var sent []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.HasSuffix(r.URL.Path, "/value") {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent = append(sent, body)
		name := strings.Split(r.URL.Path, "/")[5]
		writeJSON(w, 200, map[string]any{"id": "s1", "name": name, "scope": "org", "set": true, "version": 1, "consumers": []string{}, "created_at": "2026-09-08T00:00:00Z"})
	}))
	defer srv.Close()
	typed := []string{"first-value-1234", "second-value-5678"}
	read := 0
	var out, errb bytes.Buffer
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test"}
	e := Env{Stdout: &out, Stderr: &errb, Stdin: strings.NewReader("not read"), Dir: t.TempDir(), ConfigDir: t.TempDir(),
		Store: &memStore{m: map[string]config.Credential{}}, Getenv: func(k string) string { return env[k] },
		ReadHidden: func() ([]byte, error) { v := typed[read]; read++; return []byte(v), nil }}
	code := Run(context.Background(), []string{"secrets", "set", "A_KEY", "B_KEY", "--org", "acme", "--scope", "org"}, e)
	if code != 0 || len(sent) != 2 || sent[0]["value"] != typed[0] || sent[1]["value"] != typed[1] {
		t.Fatalf("exit %d, sent %v, stderr %s", code, sent, errb.String())
	}
	if !strings.Contains(errb.String(), "Value for A_KEY (not shown): ") || !strings.Contains(errb.String(), "Value for B_KEY") {
		t.Fatalf("no prompt: %s", errb.String())
	}
	for _, v := range typed {
		if strings.Contains(out.String()+errb.String(), v) {
			t.Fatalf("a value was printed: %s %s", out.String(), errb.String())
		}
	}
}
