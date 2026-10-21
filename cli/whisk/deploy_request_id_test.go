package whisk

import (
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/gitcmd"
)

// A push the platform refuses without a whisk line names the request it logged, read from
// git's header trace, so the agent can quote it.
func TestDeployFailedPushNamesTheRequest(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git",
		push: gitcmd.Result{ExitCode: 128, Stderr: "fatal: unable to access 'https://git.whisk.run/01J/a1.git/': The requested URL returned error: 503\n"},
		pushTrace: "14:32:01.000000 http.c:756 <= Recv header: HTTP/2 401\n" +
			"14:32:01.000000 http.c:756 <= Recv header: x-whisk-request-id: 01M5E2DFXF7DMV6JD7JYCVM77G\n" +
			"14:32:02.000000 http.c:756 <= Recv header: HTTP/2 503\n" +
			"14:32:02.000000 http.c:756 <= Recv header: X-Whisk-Request-Id: 01M5E2DFXF7DMV6JD7JYCVM77H\n"}
	r := runRemote(t, dir, srv.URL, git, "deploy", "--force", "--json")
	e := r.JSON["error"].(map[string]any)
	if r.Code != 5 || e["code"] != "PLATFORM_UNAVAILABLE" {
		t.Fatalf("a 503 should be PLATFORM_UNAVAILABLE: %+v", r)
	}
	if !strings.HasSuffix(e["message"].(string), "503. Request ID: 01M5E2DFXF7DMV6JD7JYCVM77H.") {
		t.Fatalf("message should end with the last request's ID: %q", e["message"])
	}
	if d := e["details"].(map[string]any); d["request_id"] != "01M5E2DFXF7DMV6JD7JYCVM77H" {
		t.Fatalf("details: %v", d)
	}
	_, env := git.pushCall()
	joined := strings.Join(env, " ")
	if !strings.Contains(joined, "GIT_TRACE_CURL_NO_DATA=1") || !strings.Contains(joined, "GIT_TRACE_REDACT=1") {
		t.Fatalf("the trace must carry headers only, redacted: %v", env)
	}
}
