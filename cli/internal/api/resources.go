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
	werrors "github.com/whisk-run/contract/errors"
)

// Environment is production or a preview.
type Environment = apitypes.Environment

// StartedPreview is a branch's preview and the deploy following its tip.
type StartedPreview = apitypes.StartedPreview

// StartPreview starts the preview of a branch already pushed to the app, or answers the deploy
// already following it (CONTROL-PLANE.md §6.3).
func (c *Client) StartPreview(ctx context.Context, org, app, branch string) (StartedPreview, error) {
	var out StartedPreview
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/previews", map[string]string{"branch": branch}, &out)
}

// ListEnvironments lists the app's environments.
func (c *Client) ListEnvironments(ctx context.Context, org, app string) ([]Environment, error) {
	return listAll[Environment](ctx, c, appPath(org, app)+"/environments")
}

// DeleteEnvironment deletes a preview environment by id or name.
func (c *Client) DeleteEnvironment(ctx context.Context, org, app, ref string) error {
	return c.Do(ctx, http.MethodDelete, appPath(org, app)+"/environments/"+pathSeg(ref), nil, nil)
}

// Secret is a declared secret: never its value.
type Secret = apitypes.Secret

// SecretVersion is one stored value's metadata.
type SecretVersion = apitypes.SecretVersion

// SecretRead is one delivery of a value to a container (or a break-glass read).
type SecretRead = apitypes.SecretRead

// DeclareSecret is the body of POST /orgs/:org/secrets. Agents never send a value.
type DeclareSecret struct {
	Name  string `json:"name"`
	Scope string `json:"scope,omitempty"`  // org | app
	AppID string `json:"app_id,omitempty"` // slug or id
}

func orgPath(org string) string { return "/orgs/" + pathSeg(org) }

func secretPath(org, name, suffix, app string) string {
	p := orgPath(org) + "/secrets/" + pathSeg(name) + suffix
	if app != "" {
		p += "?app=" + url.QueryEscape(app)
	}
	return p
}

// ListOrgSecrets lists the org-scoped secrets.
func (c *Client) ListOrgSecrets(ctx context.Context, org string) ([]Secret, error) {
	return listAll[Secret](ctx, c, orgPath(org)+"/secrets")
}

// ListAppSecrets lists the secrets an app can see.
func (c *Client) ListAppSecrets(ctx context.Context, org, app string) ([]Secret, error) {
	return listAll[Secret](ctx, c, appPath(org, app)+"/secrets")
}

// DeclareSecret creates the slot for a secret.
func (c *Client) DeclareSecret(ctx context.Context, org string, req DeclareSecret) (Secret, error) {
	var out Secret
	return out, c.Do(ctx, http.MethodPost, orgPath(org)+"/secrets", req, &out)
}

// SecretVersions lists a secret's versions; app narrows to an app-scoped secret.
func (c *Client) SecretVersions(ctx context.Context, org, name, app string) ([]SecretVersion, error) {
	return listAll[SecretVersion](ctx, c, secretPath(org, name, "/versions", app))
}

// RollbackSecret makes an earlier version current.
func (c *Client) RollbackSecret(ctx context.Context, org, name string, version int, app string) (Secret, error) {
	var out Secret
	body := map[string]any{"version": version}
	if app != "" {
		body["app_id"] = app
	}
	return out, c.Do(ctx, http.MethodPost, secretPath(org, name, "/rollback", ""), body, &out)
}

// SharedSecret answers a share: the secret, now org-scoped, and the apps that start reading it.
type SharedSecret = apitypes.SharedSecret

// ShareSecret makes the value app holds under name the business's, for every app that names it
// and has no value of its own. It never sends or receives a value.
func (c *Client) ShareSecret(ctx context.Context, org, name, app string) (SharedSecret, error) {
	var out SharedSecret
	return out, c.Do(ctx, http.MethodPost, secretPath(org, name, "/share", ""), map[string]any{"app_id": app}, &out)
}

// SecretReads lists deliveries of a secret since a time.
func (c *Client) SecretReads(ctx context.Context, org, name, app string, since time.Time) ([]SecretRead, error) {
	p := secretPath(org, name, "/reads", app)
	sep := "?"
	if app != "" {
		sep = "&"
	}
	p += sep + "since=" + url.QueryEscape(since.UTC().Format(time.RFC3339))
	return listAll[SecretRead](ctx, c, p)
}

// DeleteSecret removes a secret and every version.
func (c *Client) DeleteSecret(ctx context.Context, org, name, app string) error {
	return c.Do(ctx, http.MethodDelete, secretPath(org, name, "", app), nil, nil)
}

// Domain is a hostname of an app, with the DNS records a custom one needs.
type Domain = apitypes.Domain

// ListDomains lists the app's hostnames.
func (c *Client) ListDomains(ctx context.Context, org, app string) ([]Domain, error) {
	return listAll[Domain](ctx, c, appPath(org, app)+"/domains")
}

// AddDomain attaches a custom hostname; the answer carries the records to create.
func (c *Client) AddDomain(ctx context.Context, org, app, hostname string) (Domain, error) {
	var out Domain
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/domains", map[string]string{"hostname": hostname}, &out)
}

// VerifyDomain checks the DNS records; DOMAIN_UNVERIFIED when they are not in place.
func (c *Client) VerifyDomain(ctx context.Context, org, app, id string) (Domain, error) {
	var out Domain
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/domains/"+pathSeg(id)+"/verify", map[string]any{}, &out)
}

// SetDomainRedirect makes a custom domain send every request to another hostname of the app,
// or serve the app again when to is "".
func (c *Client) SetDomainRedirect(ctx context.Context, org, app, id, to string) (Domain, error) {
	var out Domain
	return out, c.Do(ctx, http.MethodPut, appPath(org, app)+"/domains/"+pathSeg(id)+"/redirect", map[string]string{"to": to}, &out)
}

// LiveManifest is the manifest in force: the one of production's live deploy.
func (c *Client) LiveManifest(ctx context.Context, org, app string) (apitypes.Manifest, error) {
	var out apitypes.Manifest
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/manifest", nil, &out)
}

// DeleteDomain detaches a custom hostname.
func (c *Client) DeleteDomain(ctx context.Context, org, app, id string) error {
	return c.Do(ctx, http.MethodDelete, appPath(org, app)+"/domains/"+pathSeg(id), nil, nil)
}

// Member is a membership with the person.
type Member = apitypes.Member

// Group is a named set of members.
type Group = apitypes.Group

// Invite is the body of POST /orgs/:org/members.
type Invite struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	Role  string `json:"role"`
	Kind  string `json:"kind,omitempty"`
}

// ListMembers lists the org's members and guests, invited ones included.
func (c *Client) ListMembers(ctx context.Context, org string) ([]Member, error) {
	return listAll[Member](ctx, c, orgPath(org)+"/members")
}

// InviteMember invites a person; the answer carries the invite URL.
func (c *Client) InviteMember(ctx context.Context, org string, req Invite) (Member, error) {
	var out Member
	return out, c.Do(ctx, http.MethodPost, orgPath(org)+"/members", req, &out)
}

// RemoveMember removes a membership everywhere at once.
func (c *Client) RemoveMember(ctx context.Context, org, id string) error {
	return c.Do(ctx, http.MethodDelete, orgPath(org)+"/members/"+pathSeg(id), nil, nil)
}

// ListGroups lists the org's groups.
func (c *Client) ListGroups(ctx context.Context, org string) ([]Group, error) {
	return listAll[Group](ctx, c, orgPath(org)+"/groups")
}

// Grant lets a subject open an app.
type Grant = apitypes.Grant

// Access is an app's grants.
type Access = apitypes.Access

// GetAccess fetches who can open the app.
func (c *Client) GetAccess(ctx context.Context, org, app string) (Access, error) {
	var out Access
	err := c.Do(ctx, http.MethodGet, appPath(org, app)+"/access", nil, &out)
	if out.Grants == nil {
		out.Grants = []Grant{}
	}
	return out, err
}

// SetAccess replaces the app's grants.
func (c *Client) SetAccess(ctx context.Context, org, app string, grants []Grant) (Access, error) {
	var out Access
	err := c.Do(ctx, http.MethodPut, appPath(org, app)+"/access", map[string]any{"grants": grants}, &out)
	if out.Grants == nil {
		out.Grants = []Grant{}
	}
	return out, err
}

// Token is an agent or deploy token without its value.
type Token = apitypes.Token

// DeployKey is POST /orgs/:org/apps/:app/deploy-keys: the token and its value, shown once.
type DeployKey struct {
	Token  Token  `json:"token"`
	Value  string `json:"value"`
	GitURL string `json:"git_url"`
}

// CreateDeployKey mints a deploy token for git push.
func (c *Client) CreateDeployKey(ctx context.Context, org, app, label string) (DeployKey, error) {
	var out DeployKey
	body := map[string]string{}
	if label != "" {
		body["label"] = label
	}
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/deploy-keys", body, &out)
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
	return listAll[Function](ctx, c, appPath(org, app)+"/functions")
}

// FunctionGraph is one function's declared graph beside what its runs did.
func (c *Client) FunctionGraph(ctx context.Context, org, app, name string) (Function, error) {
	var out struct {
		Function Function `json:"function"`
	}
	return out.Function, c.Do(ctx, http.MethodGet, appPath(org, app)+"/functions/"+pathSeg(name)+"/graph", nil, &out)
}

// SendEvent puts an event on the app's queue and returns the platform's id for it.
func (c *Client) SendEvent(ctx context.Context, org, app, name string, data map[string]any, dedupeKey string) (string, error) {
	body := map[string]any{"name": name, "data": data}
	if dedupeKey != "" {
		body["dedupe_key"] = dedupeKey
	}
	var out struct {
		ID string `json:"id"`
	}
	return out.ID, c.Do(ctx, http.MethodPost, appPath(org, app)+"/events", body, &out)
}

// RunsOfEvent lists the runs one event started.
func (c *Client) RunsOfEvent(ctx context.Context, org, app, eventID string) ([]Run, error) {
	var out struct {
		Items []Run `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, appPath(org, app)+"/runs?event="+url.QueryEscape(eventID), nil, &out)
}

// GetRun fetches one run.
func (c *Client) GetRun(ctx context.Context, org, app, id string) (Run, error) {
	var out Run
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/runs/"+pathSeg(id), nil, &out)
}

// RunFilter narrows a run listing. A zero time means no lower bound.
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

// ReplayRun starts a run again from its first step, or from the parked step when it is
// parked, and returns the new run. POST /orgs/:org/apps/:app/runs/:id/replay.
func (c *Client) ReplayRun(ctx context.Context, org, app, id string) (Run, error) {
	var out Run
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/runs/"+pathSeg(id)+"/replay", map[string]any{}, &out)
}

// RunCron starts one run of a cron function now, the way its schedule would.
// POST /orgs/:org/apps/:app/cron/:function/run.
func (c *Client) RunCron(ctx context.Context, org, app, function string) (Run, error) {
	var out Run
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/cron/"+pathSeg(function)+"/run", map[string]any{}, &out)
}

// CancelRun stops a running or parked run. POST /orgs/:org/apps/:app/runs/:id/cancel.
func (c *Client) CancelRun(ctx context.Context, org, app, id string) (Run, error) {
	var out Run
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/runs/"+pathSeg(id)+"/cancel", map[string]any{}, &out)
}

// ListApprovals lists what an app is waiting on.
func (c *Client) ListApprovals(ctx context.Context, org, app, status string) ([]Approval, error) {
	path := appPath(org, app) + "/approvals"
	if status != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	var out struct {
		Items []Approval `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, path, nil, &out)
}

// ---- customers (CONTROL-PLANE.md §4.6, CLI.md §5.11) ---------------------------------------

// Customer is a person in an app's customer pool.
type Customer = apitypes.Customer

// Customers is the pool: its people and whether strangers may register.
type Customers = apitypes.CustomerList

// ListCustomers lists the app's customers.
func (c *Client) ListCustomers(ctx context.Context, org, app string) (Customers, error) {
	var out Customers
	err := c.Do(ctx, http.MethodGet, appPath(org, app)+"/customers", nil, &out)
	if out.Items == nil {
		out.Items = []Customer{}
	}
	return out, err
}

// InviteCustomer adds a person to the pool; the answer carries the invite URL.
func (c *Client) InviteCustomer(ctx context.Context, org, app, email, name string) (Customer, error) {
	var out Customer
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/customers", map[string]string{"email": email, "name": name}, &out)
}

// SetCustomerStatus blocks (status "blocked") or unblocks (status "active") a customer.
func (c *Client) SetCustomerStatus(ctx context.Context, org, app, id, status string) (Customer, error) {
	var out Customer
	return out, c.Do(ctx, http.MethodPatch, appPath(org, app)+"/customers/"+pathSeg(id), map[string]string{"status": status}, &out)
}

// RemoveCustomer removes a customer and their grant.
func (c *Client) RemoveCustomer(ctx context.Context, org, app, id string) error {
	return c.Do(ctx, http.MethodDelete, appPath(org, app)+"/customers/"+pathSeg(id), nil, nil)
}

// ---- signed media links (CONTROL-PLANE.md §6.13 "Signed links", CLI.md §5.9) ---------------

// MediaLinks and MediaLinkKey are the signed-links answers.
type (
	MediaLinks   = apitypes.MediaLinks
	MediaLinkKey = apitypes.MediaLinkKey
)

// SignMedia asks for signed, expiring links to the app's private images and video: each path
// as a page writes it (/.whisk/img/<id>?w=800, /.whisk/media/<id>), expiresIn seconds (0 is an
// hour).
func (c *Client) SignMedia(ctx context.Context, org, app string, paths []string, expiresIn int64) (MediaLinks, error) {
	var out MediaLinks
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/uploads/links", map[string]any{"paths": paths, "expires_in": expiresIn}, &out)
}

// RotateMediaLinkKey gives the app a new signing key: links already issued work until they
// expire, or with immediately stop now.
func (c *Client) RotateMediaLinkKey(ctx context.Context, org, app string, immediately bool) (MediaLinkKey, error) {
	var out MediaLinkKey
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/uploads/links/rotate", map[string]bool{"immediately": immediately}, &out)
}

// ---- tokens (CONTROL-PLANE.md §5.1 Tokens, CLI.md §5.12) -----------------------------------

// ListTokens lists the org's live agent and deploy tokens, values never included.
func (c *Client) ListTokens(ctx context.Context, org string) ([]Token, error) {
	return listAll[Token](ctx, c, orgPath(org)+"/tokens")
}

// RevokeToken revokes one token by id.
func (c *Client) RevokeToken(ctx context.Context, org, id string) error {
	return c.Do(ctx, http.MethodDelete, orgPath(org)+"/tokens/"+pathSeg(id), nil, nil)
}

// AgentToken is POST /orgs/:org/tokens: a new agent token and its value, shown once.
type AgentToken = apitypes.MintedToken

// CreateAgentToken mints an agent token for CI or a second agent, acting for the caller.
func (c *Client) CreateAgentToken(ctx context.Context, org, label string, scopes []string) (AgentToken, error) {
	body := map[string]any{"label": label}
	if len(scopes) > 0 {
		body["scopes"] = scopes
	}
	var out AgentToken
	return out, c.Do(ctx, http.MethodPost, orgPath(org)+"/tokens", body, &out)
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
	return listAll[Source](ctx, c, appPath(org, app)+"/webhooks")
}

// WebhookEvents lists a source's stored deliveries, newest first.
func (c *Client) WebhookEvents(ctx context.Context, org, app, name string, limit int) ([]WebhookEvent, error) {
	path := fmt.Sprintf("%s/webhooks/%s/events?limit=%d", appPath(org, app), pathSeg(name), limit)
	var out struct {
		Items []WebhookEvent `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, path, nil, &out)
}

// ReplayWebhookEvent queues a stored delivery again, keeping its id.
func (c *Client) ReplayWebhookEvent(ctx context.Context, org, app, name, eventID string) (WebhookEvent, error) {
	var out WebhookEvent
	path := fmt.Sprintf("%s/webhooks/%s/events/%s/replay", appPath(org, app), pathSeg(name), pathSeg(eventID))
	return out, c.Do(ctx, http.MethodPost, path, nil, &out)
}

// ---- inbox (CONTROL-PLANE.md §6.12 "Receiving", CLI.md §5.8) ---------------------------------

// Inbox is the app's address, its own receiving domains and the latest messages.
type Inbox = apitypes.Inbox

// InboxDomain is a business's own domain whose mail reaches the app.
type InboxDomain = apitypes.InboxDomain

// GetInbox reads the app's inbox.
func (c *Client) GetInbox(ctx context.Context, org, app string) (Inbox, error) {
	var out Inbox
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/inbox", nil, &out)
}

// AddInboxDomain registers a domain whose mail reaches the app and answers the records to publish.
func (c *Client) AddInboxDomain(ctx context.Context, org, app, domain string) (InboxDomain, error) {
	var out InboxDomain
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/inbox/domains", apitypes.InboxDomainRequest{Domain: domain}, &out)
}

// VerifyInboxDomain checks the domain's records; it stays pending until they are published.
func (c *Client) VerifyInboxDomain(ctx context.Context, org, app, id string) (InboxDomain, error) {
	var out InboxDomain
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/inbox/domains/"+pathSeg(id)+"/verify", nil, &out)
}

// RemoveInboxDomain stops the domain reaching the app, by id.
func (c *Client) RemoveInboxDomain(ctx context.Context, org, app, id string) error {
	return c.Do(ctx, http.MethodDelete, appPath(org, app)+"/inbox/domains/"+pathSeg(id), nil, nil)
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
	var out struct {
		Items []LogLine `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, appPath(org, app)+"/logs?"+q.values().Encode(), nil, &out)
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
	v := url.Values{}
	if query != "" {
		v.Set("q", query)
	}
	if since != "" {
		v.Set("since", since)
	}
	if limit > 0 {
		v.Set("limit", fmt.Sprint(limit))
	}
	var out struct {
		Items []ErrorGroup `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, appPath(org, app)+"/errors?"+v.Encode(), nil, &out)
}

// SetErrorStatus resolves or ignores a group.
func (c *Client) SetErrorStatus(ctx context.Context, org, app, id, status string) error {
	return c.Do(ctx, http.MethodPost, appPath(org, app)+"/errors/"+pathSeg(id), map[string]string{"status": status}, nil)
}

// OrgDomain is a business's own domain: once verified, every app answers at <app>.<domain>.
type OrgDomain = apitypes.OrgDomain

// OrgDomainApp is where one app answers on the business domain.
type OrgDomainApp = apitypes.OrgDomainApp

// OrgDomains is the business's domain, none or one.
func (c *Client) OrgDomains(ctx context.Context, org string) ([]OrgDomain, error) {
	return listAll[OrgDomain](ctx, c, orgPath(org)+"/domains")
}

// AddOrgDomain records the business's domain; the answer carries the records to create.
func (c *Client) AddOrgDomain(ctx context.Context, org, domain string) (OrgDomain, error) {
	var out OrgDomain
	return out, c.Do(ctx, http.MethodPost, orgPath(org)+"/domains", map[string]string{"domain": domain}, &out)
}

// VerifyOrgDomain checks the records; DOMAIN_UNVERIFIED when they are not in place.
func (c *Client) VerifyOrgDomain(ctx context.Context, org, id string) (OrgDomain, error) {
	var out OrgDomain
	return out, c.Do(ctx, http.MethodPost, orgPath(org)+"/domains/"+pathSeg(id)+"/verify", map[string]any{}, &out)
}

// DeleteOrgDomain removes the business's domain.
func (c *Client) DeleteOrgDomain(ctx context.Context, org, id string) error {
	return c.Do(ctx, http.MethodDelete, orgPath(org)+"/domains/"+pathSeg(id), nil, nil)
}

// LogDestination is one of the org's own log services, which every line of every app is also
// sent to. Headers are names only; their values never come back.
type LogDestination = apitypes.LogDestination

// LogDestinations is the org's destinations and whether its plan includes forwarding.
type LogDestinations = apitypes.LogDestinationList

// ListLogDestinations reads where the org's logs are forwarded.
func (c *Client) ListLogDestinations(ctx context.Context, org string) (LogDestinations, error) {
	var out LogDestinations
	return out, c.Do(ctx, http.MethodGet, orgPath(org)+"/logs/destinations", nil, &out)
}

// TestLogDestination sends one destination a test line now.
func (c *Client) TestLogDestination(ctx context.Context, org, id string) error {
	return c.Do(ctx, http.MethodPost, orgPath(org)+"/logs/destinations/"+pathSeg(id)+"/test", nil, nil)
}
