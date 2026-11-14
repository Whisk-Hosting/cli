package gitcmd

import (
	"reflect"
	"testing"
)

// The edges of the parsers: the shortest lines each reads, and lines that must not be read.

func TestSidebandDocs(t *testing.T) {
	sb := ParseSideband("remote: whisk: docs: https://early.example\n" +
		"remote: whisk: {\"error\":{\"code\":\"BUILD_FAILED\",\"message\":\"The build failed.\"}}\n" +
		"remote: whisk: docs: https://skill.whisk.run/errors/BUILD_FAILED\n" +
		"remote: whisk: docs: https://later.example\n")
	if sb.Error == nil || sb.Error.Docs != "https://skill.whisk.run/errors/BUILD_FAILED" {
		t.Fatalf("docs = %+v", sb.Error)
	}
	if got := ParseSideband("remote: whisk: docs: https://alone.example\n"); got.Error != nil {
		t.Fatalf("docs with no error made one: %+v", got.Error)
	}
}

func TestParsePushShortLines(t *testing.T) {
	for _, c := range []struct {
		out     string
		outcome PushOutcome
		summary string
	}{
		{"*\trefs/heads/main:refs/heads/main\n", PushUpdated, ""},
		{"!\n=\trefs/heads/main:refs/heads/main\t[up to date]\n", PushUpToDate, "[up to date]"},
		{"!\n", PushUnknown, ""},
	} {
		if o, s := ParsePush(c.out); o != c.outcome || s != c.summary {
			t.Errorf("ParsePush(%q) = %v %q, want %v %q", c.out, o, s, c.outcome, c.summary)
		}
	}
}

func TestParseStatusShortest(t *testing.T) {
	if got := ParseStatus(" M a\n?? \n"); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("ParseStatus = %q", got)
	}
}

func TestLastLinesExactly(t *testing.T) {
	if got := LastLines("a\nb\n", 2); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("LastLines(2 of 2) = %q", got)
	}
	if got := LastLines("a\nb\nc", 2); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("LastLines(2 of 3) = %q", got)
	}
}

func TestRequestIDIgnoresOtherHeaders(t *testing.T) {
	trace := "<= Recv header: X-Whisk-Request-Id: 01ABC\n<= Recv header: Content-Type: text/plain\n<= Recv header: HTTP/1.1 200 OK\n"
	if got := RequestID(trace); got != "01ABC" {
		t.Errorf("RequestID = %q", got)
	}
}

func TestNetworkedIsANewSlice(t *testing.T) {
	cred := make([]string, 1, 8)
	cred[0] = "-c"
	got := Networked(cred, "push")
	got[0] = "changed"
	if cred[0] != "-c" || len(got) != 1+len(StallArgs)+1 || got[len(got)-1] != "push" {
		t.Errorf("Networked = %q, credential %q", got, cred)
	}
}
