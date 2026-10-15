package whisk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/run/runhttp"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// whisk call sends one request to the app's own address as the person the agent acts for
// (CLI.md §5.9, CONTROL-PLANE.md §4.4 "An agent in an app"). The CLI adds the credential, so an
// agent never writes its token into a request of its own, and it goes only to the app's address.

// callAnswerLimit is the most of an answer the CLI reads.
const callAnswerLimit = 10 << 20

// agentCredentialPrefix starts the credentials the app's address accepts.
const agentCredentialPrefix = "whsk_agent_"

// callMethods are the methods whisk call sends.
var callMethods = map[string]bool{"GET": true, "HEAD": true, "OPTIONS": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// callTarget joins the app's address and a path, refusing anything that would leave the
// address: a full URL, a scheme-relative path or a host of its own. Pure.
func callTarget(base, path string) (*url.URL, error) {
	invalid := func(why string) error {
		return output.New("INVALID_REQUEST", why, "Pass a path on the app that starts with /, such as /cms/api/pages?limit=5.", map[string]any{"path": path})
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, `\`) {
		return nil, invalid(fmt.Sprintf("%q is not a path on the app: whisk call reaches only the app's own address.", path))
	}
	b, err := url.Parse(base)
	if err != nil || b.Host == "" || (b.Scheme != "https" && b.Scheme != "http") {
		return nil, output.New("PLATFORM_UNAVAILABLE", fmt.Sprintf("The platform gave the app no usable address (%q).", base), "Retry; run whisk status to see the app's address.", map[string]any{"url": base})
	}
	p, err := url.Parse(path)
	if err != nil {
		return nil, invalid(fmt.Sprintf("%q is not a path the CLI can send: %s.", path, err.Error()))
	}
	origin := &url.URL{Scheme: b.Scheme, Host: b.Host}
	u := origin.ResolveReference(p)
	if u.Scheme != b.Scheme || u.Host != b.Host {
		return nil, invalid(fmt.Sprintf("%q leaves the app's address.", path))
	}
	if prefix := strings.TrimRight(b.Path, "/"); prefix != "" {
		u.Path, u.RawPath = prefix+u.Path, ""
		if p.RawPath != "" {
			u.RawPath = strings.TrimRight(b.EscapedPath(), "/") + u.EscapedPath()
		}
	}
	return u, nil
}

// callHeaders reads --header values, refusing ones the CLI sets itself. Pure.
func callHeaders(raw []string) (http.Header, error) {
	h := http.Header{}
	for _, line := range raw {
		name, value, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			return nil, output.New("INVALID_REQUEST", fmt.Sprintf("%q is not a header.", line), `Pass each header as --header "Name: value".`, map[string]any{"header": line})
		}
		switch strings.ToLower(name) {
		case "authorization", "cookie", "host":
			return nil, output.New("INVALID_REQUEST", "whisk call sets "+name+" itself.", "Drop the --header for "+name+"; the CLI sends your Whisk credential.", map[string]any{"header": name})
		}
		h.Add(name, strings.TrimSpace(value))
	}
	return h, nil
}

// isJSONType reports whether a Content-Type is JSON. Pure.
func isJSONType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	return ct == "application/json" || strings.HasSuffix(ct, "+json")
}

// callAnswer is the app's answer as the CLI reports it.
type callAnswer struct {
	Status      int
	ContentType string
	RequestID   string
	Location    string
	Body        []byte
	Truncated   bool
}

// bodyValue is the answer's body for JSON output: the JSON itself when it is JSON, else text.
func (a callAnswer) bodyValue() any {
	if len(a.Body) == 0 {
		return nil
	}
	if isJSONType(a.ContentType) && !a.Truncated && json.Valid(a.Body) {
		return json.RawMessage(a.Body)
	}
	return string(a.Body)
}

// platformRefusal reads a Whisk error the edge answered in front of the app (an agent not let
// in, an app that cannot wake): the standard body with a catalogued code and its docs link.
// nil for anything the app itself answered. Pure.
func platformRefusal(a callAnswer) *output.Error {
	if a.Status < 400 || !isJSONType(a.ContentType) {
		return nil
	}
	var b werrors.Body
	if json.Unmarshal(a.Body, &b) != nil || b.Error.Code == "" {
		return nil
	}
	if _, known := werrors.Lookup(b.Error.Code); !known || b.Error.Docs != werrors.DocsBase+b.Error.Code {
		return nil
	}
	return &output.Error{Code: b.Error.Code, Message: b.Error.Message, Fix: b.Error.Fix, Docs: b.Error.Docs, Details: b.Error.Details, Status: a.Status}
}

// callOutcome is what an answer means for the command: nil for a 2xx, the edge's refusal as
// itself, and CALL_FAILED with the app's answer for anything else. Pure.
func callOutcome(method string, u *url.URL, a callAnswer) error {
	if a.Status >= 200 && a.Status < 300 {
		return nil
	}
	if e := platformRefusal(a); e != nil {
		return e
	}
	details := map[string]any{"status": a.Status, "method": method, "url": u.String(), "body": a.bodyValue()}
	if a.RequestID != "" {
		details["request_id"] = a.RequestID
	}
	if a.Location != "" {
		details["location"] = a.Location
	}
	fix := "Read the app's answer in details.body, correct the request and send it again. whisk logs --since 10m shows the app's side of it."
	if a.Status >= 300 && a.Status < 400 {
		fix = "The app answered with a redirect, which whisk call does not follow. Send the request to the path in details.location if it is the app's own."
	}
	return &output.Error{Code: "CALL_FAILED", Message: fmt.Sprintf("The app answered %s %s with %d %s.", method, u.Path, a.Status, http.StatusText(a.Status)),
		Fix: fix, Docs: werrors.DocsBase + "CALL_FAILED", Details: details, Status: a.Status}
}

// callCredential is the token whisk call sends, refusing one the app's address would not
// accept so the agent learns why before anything is sent.
func (s *session) callCredential() (string, error) {
	cred, ok, err := s.credential()
	if err != nil {
		return "", err
	}
	if !ok || cred.Token == "" {
		return "", output.New("AUTH_REQUIRED", "No token for profile "+s.profile+".", "Set WHISK_TOKEN to your agent identity's credential, or ask the person for one from whisk agent-token create.", map[string]any{"profile": s.profile})
	}
	fix := "Set WHISK_TOKEN to an agent identity's credential, or ask the person to run whisk agent-token create --label \"<agent>\" and set it as WHISK_TOKEN. A whisk login stays as it is for every other command."
	if cred.PrivateKey != "" && s.env.Getenv("WHISK_TOKEN") == "" {
		return "", output.New("TOKEN_SCOPE", "This login from whisk login is bound to this computer and signs each request, which an app's address cannot check, so whisk call cannot use it.", fix, map[string]any{"profile": s.profile})
	}
	if !strings.HasPrefix(cred.Token, agentCredentialPrefix) {
		return "", output.New("TOKEN_SCOPE", "Only an agent's credential reaches an app as its person; this token is not one.", fix, nil)
	}
	return cred.Token, nil
}

// appAddress is the address of the app's environment: production unless --env names another.
func (s *session) appAddress(client *api.Client, org, app, env string) (string, string, error) {
	if env == "" || env == "production" {
		a, err := client.GetApp(s.ctx, org, app)
		if err != nil {
			return "", "", wrap(err)
		}
		return a.URL, "production", nil
	}
	envs, err := client.ListEnvironments(s.ctx, org, app)
	if err != nil {
		return "", "", wrap(err)
	}
	names := make([]string, 0, len(envs))
	for _, e := range envs {
		if e.Name == env || e.Name == "preview:"+env {
			return e.URL, e.Name, nil
		}
		names = append(names, e.Name)
	}
	return "", "", output.New("NOT_FOUND", fmt.Sprintf("%s/%s has no environment %q.", org, app, env), "Pass one of: "+strings.Join(names, ", ")+".", map[string]any{"environment": env, "environments": names})
}

func callCmd(s *session) *cobra.Command {
	var data, environment string
	var headers []string
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "call <METHOD> <path> [--data '<json>' | --data @file.json | --data -] [--header 'Name: value'] [--env name]",
		Short: "Send a request to the app's own API as the person you act for",
		Long: `Sends one request to the app's own address with your Whisk credential, so you use the app's
API exactly as the person you act for may: read and change its pages, records and settings
through the app, never its database. The edge lets you in with that person's grants, removes
the credential and tells the app which agent sent it, and the app records the change as theirs.

The credential goes only to the app's address and is never printed. It must be an agent's
credential (WHISK_TOKEN from an agent identity, or from whisk agent-token create); an agent
identity needs read for GET and change for anything else. --data sends a body: JSON text,
@file to read a file, or - for stdin; JSON goes as application/json unless --header sets a
Content-Type. Redirects are not followed.

Prints the app's answer. --json prints {status, url, body, request_id}, with body as JSON when
the app answered JSON. An answer outside 2xx is CALL_FAILED with the answer in details; the
edge's own refusals keep their code (AGENT_CATEGORY, TOKEN_SCOPE, FORBIDDEN_ROLE).`,
		Example: `  whisk call GET /cms/api/product-pages?limit=5
  whisk call PATCH /cms/api/product-pages/4 --data @page.json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(args[0])
			if !callMethods[method] {
				return output.New("INVALID_REQUEST", fmt.Sprintf("%q is not a method whisk call sends.", args[0]), "Pass GET, POST, PUT, PATCH, DELETE, HEAD or OPTIONS.", map[string]any{"method": args[0]})
			}
			if timeout <= 0 || timeout > 10*time.Minute {
				return output.New("INVALID_REQUEST", "--timeout must be more than 0 and at most 10m.", "Pass a duration such as 60s.", map[string]any{"timeout": timeout.String()})
			}
			header, err := callHeaders(headers)
			if err != nil {
				return err
			}
			body, err := s.callBody(data, header)
			if err != nil {
				return err
			}
			token, err := s.callCredential()
			if err != nil {
				return err
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			base, envName, err := s.appAddress(client, org, app, environment)
			if err != nil {
				return err
			}
			u, err := callTarget(base, args[1])
			if err != nil {
				return err
			}
			a, err := s.sendCall(method, u, header, body, token, timeout)
			if err != nil {
				return err
			}
			if err := callOutcome(method, u, a); err != nil {
				return err
			}
			res := map[string]any{"org": org, "app": app, "environment": envName, "method": method, "url": u.String(), "status": a.Status,
				"content_type": a.ContentType, "body": a.bodyValue(), "truncated": a.Truncated}
			if a.RequestID != "" {
				res["request_id"] = a.RequestID
			}
			s.printer.Result(res, func(w io.Writer) { printCallBody(w, a) })
			if !s.printer.Quiet {
				fmt.Fprintf(s.env.Stderr, "%d %s from %s %s\n", a.Status, http.StatusText(a.Status), method, u.String())
			}
			return nil
		},
	}
	c.Flags().StringVar(&data, "data", "", "the body: JSON text, @file to read a file, or - for stdin")
	c.Flags().StringArrayVar(&headers, "header", nil, `a header to send, "Name: value"; repeatable`)
	c.Flags().StringVar(&environment, "env", "", "the environment; production when omitted")
	c.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "how long to wait for the answer, a sleeping app's wake included")
	return c
}

// callBody reads --data and sets its Content-Type, checking JSON before anything is sent.
func (s *session) callBody(data string, header http.Header) ([]byte, error) {
	if data == "" {
		return nil, nil
	}
	body := []byte(data)
	if from, ok := strings.CutPrefix(data, "@"); ok || data == "-" {
		if data == "-" {
			from = "-"
		}
		raw, err := s.readInput(from)
		if err != nil {
			return nil, output.New("INVALID_REQUEST", "The body could not be read: "+err.Error(), "Check the path after @, relative to the working directory.", map[string]any{"data": data})
		}
		body = decodeText(raw)
	}
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/json")
	}
	if isJSONType(header.Get("Content-Type")) && !json.Valid(body) {
		return nil, output.New("INVALID_REQUEST", "The body is not valid JSON.", "Fix the JSON, or set another type with --header \"Content-Type: ...\".", map[string]any{"data": data})
	}
	return body, nil
}

// sendCall sends the request with the credential and reads the answer, within the deadline.
func (s *session) sendCall(method string, u *url.URL, header http.Header, body []byte, token string, timeout time.Duration) (callAnswer, error) {
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return callAnswer{}, output.New("INVALID_REQUEST", "The request could not be made: "+err.Error(), "Check the path.", nil)
	}
	req.Header = header.Clone()
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "whisk-cli/"+Version)
	hc := runhttp.Client(timeout)
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := hc.Do(req)
	if err != nil {
		var ne net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			return callAnswer{}, output.New("PLATFORM_UNAVAILABLE", fmt.Sprintf("%s %s had no answer within %s.", method, u.Path, timeout),
				"Check whether it was made before sending it again (a GET of the same record); raise --timeout for a slow route, or read whisk logs --since 10m.", map[string]any{"url": u.String(), "timeout": timeout.String()})
		}
		return callAnswer{}, output.New("PLATFORM_UNAVAILABLE", "The app's address could not be reached: "+err.Error(), "Retry with backoff; run whisk status to see the app.", map[string]any{"url": u.String()})
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, callAnswerLimit+1))
	if err != nil {
		return callAnswer{}, output.New("PLATFORM_UNAVAILABLE", "The app's answer was cut off: "+err.Error(), "Send the request again; check first whether a change was made.", map[string]any{"url": u.String(), "status": resp.StatusCode})
	}
	a := callAnswer{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), RequestID: resp.Header.Get("X-Whisk-Request-Id"), Location: resp.Header.Get("Location"), Body: raw}
	if len(raw) > callAnswerLimit {
		a.Body, a.Truncated = raw[:callAnswerLimit], true
	}
	return a, nil
}

// printCallBody prints the answer for a person: JSON indented, anything else as it came.
func printCallBody(w io.Writer, a callAnswer) {
	if len(a.Body) == 0 {
		return
	}
	var out bytes.Buffer
	if isJSONType(a.ContentType) && json.Indent(&out, a.Body, "", "  ") == nil {
		fmt.Fprintln(w, out.String())
	} else {
		w.Write(a.Body)
		if !bytes.HasSuffix(a.Body, []byte("\n")) {
			fmt.Fprintln(w)
		}
	}
	if a.Truncated {
		fmt.Fprintf(w, "(answer cut at %d MB)\n", callAnswerLimit>>20)
	}
}
