package whisk

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/output"
)

// dashboard is the origin the CLI prints links to: WHISK_DASHBOARD, else config.json's
// dashboard, else derived from the API origin (api.<domain> → https://<domain>).
func (s *session) dashboard() string {
	if v := s.env.Getenv("WHISK_DASHBOARD"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if s.cfg.Dashboard != "" {
		return strings.TrimRight(s.cfg.Dashboard, "/")
	}
	base := s.cfg.API
	if cred, ok, err := s.credential(); err == nil && ok && cred.API != "" && s.env.Getenv("WHISK_API") == "" {
		base = cred.API
	}
	return dashboardFrom(base)
}

// dashboardFrom derives the dashboard origin from an API origin. api.whisk.run becomes
// whisk.run; any other host is used as it is.
func dashboardFrom(apiBase string) string {
	u, err := url.Parse(strings.TrimRight(apiBase, "/"))
	if err != nil || u.Host == "" {
		return strings.TrimRight(apiBase, "/")
	}
	host := u.Host
	if rest, ok := strings.CutPrefix(host, "api."); ok && strings.Contains(rest, ".") {
		host = rest
	}
	return u.Scheme + "://" + host
}

// orgPage and appPage are dashboard URLs (DASHBOARD.md §3).
func orgPage(dash, org, page string) string {
	return dash + "/o/" + url.PathEscape(org) + page
}

func appPage(dash, org, app, page string) string {
	return dash + "/o/" + url.PathEscape(org) + "/apps/" + url.PathEscape(app) + page
}

// secretsURL is where a human sets secret values: the app's page, or the org's when app is "".
func secretsURL(dash, org, app string) string {
	if app == "" {
		return orgPage(dash, org, "/secrets")
	}
	return appPage(dash, org, app, "/secrets")
}

// pasteURL is the secrets page with its paste form open on a line for each name, so a person
// pastes every value in one go (DASHBOARD.md §4.7).
func pasteURL(page string, names []string) string {
	if len(names) == 0 {
		return page
	}
	escaped := make([]string, len(names))
	for i, n := range names {
		escaped[i] = url.QueryEscape(n)
	}
	return page + "?set=" + strings.Join(escaped, ",")
}

// needsHumanSecrets is the block printed when declared secrets have no value. Its URL opens
// the paste form ready for those names.
func needsHumanSecrets(names []string, page string) *output.Error {
	url := pasteURL(page, names)
	return &output.Error{
		Code:    "NEEDS_HUMAN",
		Message: fmt.Sprintf("%d secret(s) need a value before the app can use them: %s.", len(names), strings.Join(names, ", ")),
		Fix:     "Ask an owner, admin or developer to open " + url + " and paste the values there. The app restarts automatically when they are set.",
		Docs:    "https://skill.whisk.run/errors/NEEDS_HUMAN",
		Details: map[string]any{"url": url, "names": names},
	}
}

// org resolves the org an org-level command acts on: --org, else the directory's binding, else
// the org the token was approved for, else the one org a login for the whole account can see.
func (s *session) org() (string, error) {
	if s.orgFlag != "" {
		return s.orgFlag, nil
	}
	if b, ok, _ := config.LoadBinding(s.env.Dir); ok {
		return b.Org, nil
	}
	if cred, ok, err := s.credential(); err == nil && ok && cred.Org != "" {
		return cred.Org, nil
	}
	return s.onlyOrg()
}

// onlyOrg is the org to act on when nothing names one: the caller's only org. With several it
// asks for --org, naming them, and with none it says so.
func (s *session) onlyOrg() (string, error) {
	client, _, err := s.client()
	if err != nil {
		return "", err
	}
	me, err := client.Whoami(s.ctx)
	if err != nil {
		return "", wrap(err)
	}
	names := make([]string, len(me.Orgs))
	for i, o := range me.Orgs {
		names[i] = o.Slug
	}
	return pickOrg(names)
}

// pickOrg is the decision for onlyOrg (pure): the one org, or an error naming them all.
func pickOrg(names []string) (string, error) {
	switch len(names) {
	case 0:
		return "", output.New("FORBIDDEN_ROLE", "You belong to no org yet.", "Run whisk login and choose a new business when you approve the code.", nil)
	case 1:
		return names[0], nil
	}
	return "", output.New("INVALID_REQUEST", "You belong to several orgs: "+strings.Join(names, ", ")+". Name the one to act on.",
		"Pass --org <slug>, or run from a directory bound with whisk use <org>/<app>.", map[string]any{"orgs": names})
}

// appIfBound returns the bound app, or "" when the command runs at org level.
func (s *session) appIfBound() string {
	if s.appFlag != "" {
		return s.appFlag
	}
	if b, ok, _ := config.LoadBinding(s.env.Dir); ok {
		return b.App
	}
	return ""
}

// when formats an optional timestamp in local time for human output.
func when(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func at(t time.Time) string { return when(&t) }

// short is the seven-character commit prefix.
func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// parseSince turns --since into a time: a duration ago (30m, 6h, 7d) or an RFC 3339 instant.
func parseSince(v string, now time.Time) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if strings.HasSuffix(v, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		if err != nil {
			return time.Time{}, err
		}
		// More days than a Duration holds would wrap round to a time in the future.
		if days < 0 || int64(days) > int64(math.MaxInt64/(24*time.Hour)) {
			return time.Time{}, fmt.Errorf("%d days is not a duration ago", days)
		}
		return now.Add(-time.Duration(days) * 24 * time.Hour), nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return time.Time{}, err
	}
	if d < 0 {
		return time.Time{}, fmt.Errorf("%s is not a duration ago", v)
	}
	return now.Add(-d), nil
}

func sinceFlagError(v string) error {
	return output.New("INVALID_REQUEST", fmt.Sprintf("--since %q is not a duration or a time.", v), "Pass a duration like 30m, 6h or 7d, or an RFC 3339 time like 2026-09-07T10:00:00Z.", nil)
}
