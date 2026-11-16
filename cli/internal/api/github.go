package api

import (
	"context"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
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

// GitHubInstallURL asks for the URL where a person installs Whisk's App on GitHub. app names
// the app whose settings page GitHub returns to, or "" for the org's.
func (c *Client) GitHubInstallURL(ctx context.Context, org, app string) (string, error) {
	out, err := Send(ctx, c, endpoints.GithubInstall, apitypes.GithubInstallRequest{App: app}, org)
	return out.InstallURL, err
}

// SyncGitHub queues a sync of the app's link now.
func (c *Client) SyncGitHub(ctx context.Context, org, app string) error {
	_, err := Call(ctx, c, endpoints.GithubSync, org, app)
	return err
}
