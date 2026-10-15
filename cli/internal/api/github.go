package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// GitHubRepo is a repository an installation of Whisk's GitHub App reaches, with the app it is
// linked to when it is (CONTROL-PLANE.md §6.26).
type GitHubRepo = apitypes.GitHubRepo

// GitHubInstallation is one GitHub account Whisk's App is installed on for the org.
type GitHubInstallation = apitypes.GitHubInstallation

// GitHubOverview is GET /orgs/:org/github.
type GitHubOverview = apitypes.GitHubOverview

// GitHubBranch is one branch of a link: the commit on each side and whether they agree.
type GitHubBranch = apitypes.GitHubBranch

// GitHubLink is an app's link to a GitHub repository.
type GitHubLink = apitypes.GitHubLink

func githubPath(org, app string) string { return appPath(org, app) + "/github" }

// GitHubOverview lists the org's installations and the repositories each reaches.
func (c *Client) GitHubOverview(ctx context.Context, org string) (GitHubOverview, error) {
	var out GitHubOverview
	return out, c.Do(ctx, http.MethodGet, orgPath(org)+"/github", nil, &out)
}

// GitHubInstallURL asks for the URL where a person installs Whisk's App on GitHub. app names
// the app whose settings page GitHub returns to, or "" for the org's.
func (c *Client) GitHubInstallURL(ctx context.Context, org, app string) (string, error) {
	var out struct {
		InstallURL string `json:"install_url"`
	}
	body := map[string]any{}
	if app != "" {
		body["app"] = app
	}
	err := c.Do(ctx, http.MethodPost, orgPath(org)+"/github/installations", body, &out)
	return out.InstallURL, err
}

// GitHubLink reads the app's link; NOT_FOUND when it has none.
func (c *Client) GitHubLink(ctx context.Context, org, app string) (GitHubLink, error) {
	var out GitHubLink
	return out, c.Do(ctx, http.MethodGet, githubPath(org, app), nil, &out)
}

// LinkGitHub links the app to repo ("owner/name").
func (c *Client) LinkGitHub(ctx context.Context, org, app, repo string) (GitHubLink, error) {
	var out GitHubLink
	return out, c.Do(ctx, http.MethodPut, githubPath(org, app), map[string]string{"repo": repo}, &out)
}

// SyncGitHub queues a sync of the app's link now.
func (c *Client) SyncGitHub(ctx context.Context, org, app string) error {
	return c.Do(ctx, http.MethodPost, githubPath(org, app)+"/sync", map[string]any{}, nil)
}

// UnlinkGitHub removes the app's link; nothing on GitHub or in the app's repository changes.
func (c *Client) UnlinkGitHub(ctx context.Context, org, app string) error {
	return c.Do(ctx, http.MethodDelete, githubPath(org, app), nil, nil)
}
