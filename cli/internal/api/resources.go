package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
	werrors "github.com/whisk-run/contract/errors"
)

// Environment is production or a preview.
type Environment = apitypes.Environment

// StartedPreview is a branch's preview and the deploy following its tip.
type StartedPreview = apitypes.StartedPreview

// Secret is a declared secret: never its value.
type Secret = apitypes.Secret

// SecretVersion is one stored value's metadata.
type SecretVersion = apitypes.SecretVersion

// SecretRead is one delivery of a value to a container (or a break-glass read).
type SecretRead = apitypes.SecretRead

// DeclareSecret is the body of POST /orgs/:org/secrets; the CLI sends values with SetSecretValue.
type DeclareSecret = apitypes.DeclareSecretRequest

func orgPath(org string) string { return "/orgs/" + pathSeg(org) }

// DeclareSecret creates the slot for a secret.
func (c *Client) DeclareSecret(ctx context.Context, org string, req DeclareSecret) (Secret, error) {
	return Send(ctx, c, endpoints.DeclareSecret, req, org)
}

// SecretVersions lists a secret's versions; app narrows to an app-scoped secret.
func (c *Client) SecretVersions(ctx context.Context, org, name, app string) ([]SecretVersion, error) {
	return AllOf(ctx, c, endpoints.SecretVersions, endpoints.SecretVersionsQuery{App: app}, org, name)
}

// RollbackSecret makes an earlier version current.
func (c *Client) RollbackSecret(ctx context.Context, org, name string, version int, app string) (Secret, error) {
	return Send(ctx, c, endpoints.RollbackSecret, apitypes.RollbackSecretRequest{Version: version, AppID: app}, org, name)
}

// SharedSecret answers a share: the secret, now org-scoped, and the apps that start reading it.
type SharedSecret = apitypes.SharedSecret

// ShareSecret makes the value app holds under name the business's, for every app that names it
// and has no value of its own. It never sends or receives a value.
func (c *Client) ShareSecret(ctx context.Context, org, name, app string) (SharedSecret, error) {
	return Send(ctx, c, endpoints.ShareSecret, apitypes.ShareSecretRequest{AppID: app}, org, name)
}

// SecretReads lists deliveries of a secret since a time.
func (c *Client) SecretReads(ctx context.Context, org, name, app string, since time.Time) ([]SecretRead, error) {
	q := endpoints.SecretReadsQuery{App: app, Since: since.UTC().Format(time.RFC3339)}
	return AllOf(ctx, c, endpoints.SecretReads, q, org, name)
}

// DeleteSecret removes a secret and every version.
func (c *Client) DeleteSecret(ctx context.Context, org, name, app string) error {
	_, err := Send(ctx, c, endpoints.DeleteSecret, endpoints.DeleteSecretQuery{App: app}, org, name)
	return err
}

// Domain is a hostname of an app, with the DNS records a custom one needs.
type Domain = apitypes.Domain

// SetMainAddress makes hostname, one of the app's verified custom domains, its main address,
// which its other addresses send browsers to, or makes its Whisk address the main one again with
// "" or that address. The answer is the app's domains.
func (c *Client) SetMainAddress(ctx context.Context, org, app, hostname string) ([]Domain, error) {
	out, err := Send(ctx, c, endpoints.SetMainAddress, apitypes.MainAddressRequest{Hostname: hostname}, org, app)
	return out.Items, err
}

// Member is a membership with the person.
type Member = apitypes.Member

// Group is a named set of members.
type Group = apitypes.Group

// Invite is the body of POST /orgs/:org/members.
type Invite = apitypes.InviteRequest

// PersonRequest is a change an agent asked a person to approve (CONTROL-PLANE.md §4.9).
type PersonRequest = apitypes.PersonRequest

// asking makes a call a person may have to approve: on 202 the API kept it as a request, which
// is answered and out is left as it was; otherwise out is the call's answer.
func (c *Client) asking(ctx context.Context, method, path string, in, out any) (*PersonRequest, error) {
	var raw json.RawMessage
	status, err := c.do(ctx, method, path, in, &raw)
	if err != nil {
		return nil, err
	}
	if status == http.StatusAccepted {
		var asked apitypes.PersonRequestAnswer
		if err := json.Unmarshal(raw, &asked); err != nil || asked.Request.ID == "" {
			return nil, &Error{Status: status, Code: "PLATFORM_UNAVAILABLE", Message: method + " " + path + " answered 202 without the request it kept.", Fix: "Retry with backoff."}
		}
		return &asked.Request, nil
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return nil, &Error{Status: status, Code: "PLATFORM_UNAVAILABLE", Message: method + " " + path + " answered with a body that is not the API's JSON.", Fix: "Retry with backoff."}
		}
	}
	return nil, nil
}

// ask sends a declared route a person may have to approve (asking).
func ask[In, Out any](ctx context.Context, c *Client, r endpoints.Route[In, Out], in In, values ...string) (Out, *PersonRequest, error) {
	var out Out
	path, err := r.URL(values...)
	if err != nil {
		return out, nil, err
	}
	var body, dst any = in, &out
	if _, none := body.(endpoints.None); none {
		body = nil
	}
	if _, none := any(out).(endpoints.None); none {
		dst = nil
	}
	asked, err := c.asking(ctx, r.Method, strings.TrimPrefix(path, "/v1"), body, dst)
	return out, asked, err
}

// InviteMember invites a person; the answer carries the invite URL. From an agent token the
// API keeps it for a person to approve, and the request is answered instead.
func (c *Client) InviteMember(ctx context.Context, org string, req Invite) (Member, *PersonRequest, error) {
	return ask(ctx, c, endpoints.InviteMember, req, org)
}

// RemoveMember removes a membership everywhere at once, or answers the request a person approves.
func (c *Client) RemoveMember(ctx context.Context, org, id string) (*PersonRequest, error) {
	_, asked, err := ask(ctx, c, endpoints.RemoveMember, endpoints.None{}, org, id)
	return asked, err
}

// Grant lets a subject open an app.
type Grant = apitypes.Grant

// Access is an app's grants.
type Access = apitypes.Access

// GetAccess fetches who can open the app.
func (c *Client) GetAccess(ctx context.Context, org, app string) (Access, error) {
	out, err := Call(ctx, c, endpoints.GetAccess, org, app)
	if out.Grants == nil {
		out.Grants = []Grant{}
	}
	return out, err
}

// SetAccess replaces the app's grants, or answers the request a person approves.
func (c *Client) SetAccess(ctx context.Context, org, app string, grants []Grant) (Access, *PersonRequest, error) {
	out, asked, err := ask(ctx, c, endpoints.SetAccess, apitypes.AccessRequest{Grants: grants}, org, app)
	if out.Grants == nil {
		out.Grants = []Grant{}
	}
	return out, asked, err
}

// Token is an agent or deploy token without its value.
type Token = apitypes.Token

// DeployKey is POST /orgs/:org/apps/:app/deploy-keys: the token and its value, shown once.
type DeployKey = apitypes.DeployKeyResponse

// CreateDeployKey mints a deploy token for git push.
func (c *Client) CreateDeployKey(ctx context.Context, org, app, label string) (DeployKey, error) {
	return Send(ctx, c, endpoints.CreateDeployKey, apitypes.CreateDeployKeyRequest{Label: label}, org, app)
}

// ---- workflows (CONTROL-PLANE.md §6.9, CLI.md §5.8) -----------------------------------------

// Trigger is what starts a function.
type Trigger = apitypes.Trigger

// Drift is the difference between a function's declared graph and what its runs did.
type Drift = apitypes.Drift

// Drifted reports whether a function has drifted at all.
func Drifted(d Drift) bool { return len(d.Unexpected) > 0 || len(d.NeverRan) > 0 }

// Function is one declared workflow function.
type Function = apitypes.WorkflowFunction

// Run is one run of a function.
type Run = apitypes.Run

// Platform reports whether an error is Whisk's side rather than the app's.
func Platform(e *werrors.Detail) bool {
	if e == nil {
		return false
	}
	if f, ok := e.Details["fault"].(string); ok {
		return f == "platform"
	}
	return strings.HasPrefix(e.Code, "PLATFORM_")
}

// Approval is a decision a workflow is waiting on.
type Approval = apitypes.Approval

// ListFunctions lists the app's declared functions with their drift.
func (c *Client) ListFunctions(ctx context.Context, org, app string) ([]Function, error) {
	out, err := Call(ctx, c, endpoints.ListFunctions, org, app)
	return out.Items, err
}

// FunctionGraph is one function's declared graph beside what its runs did.
func (c *Client) FunctionGraph(ctx context.Context, org, app, name string) (Function, error) {
	out, err := Call(ctx, c, endpoints.GetFunctionGraph, org, app, name)
	return out.Function, err
}

// SendEvent puts an event on the app's queue and returns the platform's id for it.
func (c *Client) SendEvent(ctx context.Context, org, app, name string, data map[string]any, dedupeKey string) (string, error) {
	out, err := Send(ctx, c, endpoints.EnqueueEvent, apitypes.EventRequest{Name: name, Data: data, DedupeKey: dedupeKey}, org, app)
	return out.ID, err
}

// RunsOfEvent lists the runs one event started.
func (c *Client) RunsOfEvent(ctx context.Context, org, app, eventID string) ([]Run, error) {
	var out struct {
		Items []Run `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, appPath(org, app)+"/runs?event="+url.QueryEscape(eventID), nil, &out)
}

// RunFilter narrows a run listing. A zero time is the platform's default, the last seven days.
type RunFilter struct {
	Function string
	Status   string
	Since    time.Time
}

// ListRuns lists the app's runs, newest first. GET /orgs/:org/apps/:app/runs?function=&status=&since=.
func (c *Client) ListRuns(ctx context.Context, org, app string, f RunFilter) ([]Run, error) {
	q := url.Values{}
	if f.Function != "" {
		q.Set("function", f.Function)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if !f.Since.IsZero() {
		q.Set("since", f.Since.UTC().Format(time.RFC3339))
	}
	path := appPath(org, app) + "/runs"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	return listAll[Run](ctx, c, path)
}

// ListApprovals lists what an app is waiting on.
func (c *Client) ListApprovals(ctx context.Context, org, app, status string) ([]Approval, error) {
	out, err := Send(ctx, c, endpoints.ListAppApprovals, endpoints.ListAppApprovalsQuery{Status: status}, org, app)
	return out.Items, err
}

// ---- customers (CONTROL-PLANE.md §4.6, CLI.md §5.11) ---------------------------------------

// Customer is a person in an app's customer pool.
type Customer = apitypes.Customer

// Customers is the pool: its people and whether strangers may register.
type Customers = apitypes.CustomerList

// ListCustomers lists the app's customers.
func (c *Client) ListCustomers(ctx context.Context, org, app string) (Customers, error) {
	out, err := Call(ctx, c, endpoints.ListCustomers, org, app)
	if out.Items == nil {
		out.Items = []Customer{}
	}
	return out, err
}

// ---- signed media links (CONTROL-PLANE.md §6.13 "Signed links", CLI.md §5.9) ---------------

// MediaLinks and MediaLinkKey are the signed-links answers.
type (
	MediaLinks   = apitypes.MediaLinks
	MediaLinkKey = apitypes.MediaLinkKey
)

// ---- tokens (CONTROL-PLANE.md §5.1 Tokens, CLI.md §5.12) -----------------------------------

// AgentToken is POST /orgs/:org/tokens: a new agent token and its value, shown once.
type AgentToken = apitypes.MintedToken

// CreateAgentToken mints an agent token for CI or a second agent, acting for the caller.
func (c *Client) CreateAgentToken(ctx context.Context, org, label string, scopes []string) (AgentToken, error) {
	return Send(ctx, c, endpoints.CreateAgentToken, apitypes.CreateAgentTokenRequest{Label: label, Scopes: scopes}, org)
}

// ---- webhooks (CONTROL-PLANE.md §6.10, CLI.md §5.8) ------------------------------------------

// Source is one webhook source: the URL a provider posts to and how it is verified.
type Source = apitypes.WebhookSource

// WebhookEvent is one stored delivery.
type WebhookEvent = apitypes.WebhookEvent

// DeliveryError is a failed attempt to hand a webhook event to the app's handler.
type DeliveryError = apitypes.DeliveryError

// ListWebhooks lists the app's webhook sources.
func (c *Client) ListWebhooks(ctx context.Context, org, app string) ([]Source, error) {
	out, err := Call(ctx, c, endpoints.ListWebhooks, org, app)
	return out.Items, err
}

// WebhookEvents lists a source's stored deliveries, newest first.
func (c *Client) WebhookEvents(ctx context.Context, org, app, name string, limit int) ([]WebhookEvent, error) {
	out, err := Send(ctx, c, endpoints.ListWebhookEvents, endpoints.ListWebhookEventsQuery{Limit: limit}, org, app, name)
	return out.Items, err
}

// ReplayWebhookEvent queues a stored delivery again, keeping its id.
func (c *Client) ReplayWebhookEvent(ctx context.Context, org, app, name, eventID string) (WebhookEvent, error) {
	return Call(ctx, c, endpoints.ReplayWebhookEvent, org, app, name, eventID)
}

// ---- inbox (CONTROL-PLANE.md §6.12 "Receiving", CLI.md §5.8) ---------------------------------

// Inbox is the app's address, its own receiving domains and the latest messages.
type Inbox = apitypes.Inbox

// InboxDomain is a business's own domain whose mail reaches the app.
type InboxDomain = apitypes.InboxDomain

// AddInboxDomain registers a domain whose mail reaches the app and answers the records to publish.
func (c *Client) AddInboxDomain(ctx context.Context, org, app, domain string) (InboxDomain, error) {
	return Send(ctx, c, endpoints.AddInboxDomain, apitypes.InboxDomainRequest{Domain: domain}, org, app)
}

// EmailDomain is one of the business's sending domains: where it stands and, while it waits,
// the records to publish (CONTROL-PLANE.md §6.12).
type EmailDomain = apitypes.EmailDomain

// EmailDomains lists the business's sending domains, its apps' own among them.
func (c *Client) EmailDomains(ctx context.Context, org string) ([]EmailDomain, error) {
	return listAll[EmailDomain](ctx, c, orgPath(org)+"/email/domains")
}

// ---- logs and errors (CONTROL-PLANE.md §6.17, CLI.md §5.5) -----------------------------------

// LogLine is one line the app printed.
type LogLine = apitypes.LogLine

// ErrorGroup is one group of errors the app reported.
type ErrorGroup = apitypes.ErrorGroup

// LogQuery is what the caller is asking for.
type LogQuery struct {
	Env   string
	Grep  string
	Since string
	Limit int
}

func (q LogQuery) values() url.Values {
	v := url.Values{}
	if q.Env != "" {
		v.Set("env", q.Env)
	}
	if q.Grep != "" {
		v.Set("q", q.Grep)
	}
	if q.Since != "" {
		v.Set("since", q.Since)
	}
	if q.Limit > 0 {
		v.Set("limit", fmt.Sprint(q.Limit))
	}
	return v
}

// Logs reads a window of the app's lines, oldest first.
func (c *Client) Logs(ctx context.Context, org, app string, q LogQuery) ([]LogLine, error) {
	out, err := Send(ctx, c, endpoints.GetLogs, endpoints.GetLogsQuery{Env: q.Env, Q: q.Grep, Since: q.Since, Limit: max(q.Limit, 0)}, org, app)
	return out.Items, err
}

// TailLogs follows the app's lines, calling fn for each batch until the context ends. When the
// stream ends or drops, it reconnects from the newest line delivered (followLines), and
// returns an error only when the platform refuses or five connections in a row fail.
func (c *Client) TailLogs(ctx context.Context, org, app string, q LogQuery, fn func([]LogLine) error) error {
	return c.followLines(ctx, func(since string) string {
		v := q.values()
		v.Set("stream", "true")
		if since != "" {
			v.Set("since", since)
		}
		return appPath(org, app) + "/logs?" + v.Encode()
	}, q.Since, fn)
}

// readLineEvents parses the log stream: each event's data is a JSON array of lines.
func readLineEvents(r io.Reader, fn func([]LogLine) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	var data []string
	flush := func() error {
		if len(data) == 0 {
			return nil
		}
		raw := strings.Join(data, "\n")
		data = nil
		var batch []LogLine
		if err := json.Unmarshal([]byte(raw), &batch); err != nil {
			return nil
		}
		return fn(batch)
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case line == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := flush(); err != nil {
		return err
	}
	if err := sc.Err(); err != nil {
		return &Unavailable{Err: err}
	}
	return nil
}

// ErrorGroups lists the app's error groups: query is words the error says, since a window like
// 24h or 7d; either may be empty.
func (c *Client) ErrorGroups(ctx context.Context, org, app, query, since string, limit int) ([]ErrorGroup, error) {
	out, err := Send(ctx, c, endpoints.ListErrors, endpoints.ListErrorsQuery{Q: query, Since: since, Limit: limit}, org, app)
	return out.Items, err
}

// ErrorSample is a group as whisk errors show opens it: its newest occurrence with the stack,
// the request it was in, the group, and its recent occurrences.
type ErrorSample = apitypes.ErrorSample

// ErrorFrame is one line of a stack, innermost last.
type ErrorFrame = apitypes.ErrorFrame

// ErrorOccurrence is one time a group happened, with the request it was in.
type ErrorOccurrence = apitypes.ErrorOccurrence

// SetErrorStatus resolves or ignores a group.
func (c *Client) SetErrorStatus(ctx context.Context, org, app, id, status string) error {
	_, err := Send(ctx, c, endpoints.SetErrorStatus, apitypes.SetErrorStatusRequest{Status: status}, org, app, id)
	return err
}

// OrgDomain is a business's own domain: once verified, every app answers at <app>.<domain>.
type OrgDomain = apitypes.OrgDomain

// OrgDomainApp is where one app answers on the business domain.
type OrgDomainApp = apitypes.OrgDomainApp

// LogDestination is one of the org's own log services, which every line of every app is also
// sent to. Headers are names only; their values never come back.
type LogDestination = apitypes.LogDestination

// LogDestinations is the org's destinations and whether its plan includes forwarding.
type LogDestinations = apitypes.LogDestinationList

// TestLogDestination sends one destination a test line now.
func (c *Client) TestLogDestination(ctx context.Context, org, id string) error {
	_, err := Call(ctx, c, endpoints.TestLogDestination, org, id)
	return err
}
