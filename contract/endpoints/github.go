package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The github routes.
var (
	GithubOverview = New[None, apitypes.GitHubOverview]("GET /v1/orgs/{org}/github")
	GithubLink     = New[None, apitypes.GitHubLink]("GET /v1/orgs/{org}/apps/{app}/github")
	GithubSync     = New[None, map[string]any]("POST /v1/orgs/{org}/apps/{app}/github/sync")
	GithubUnlink   = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/github")

	_ = public.Add(GithubOverview, GithubLink, GithubSync, GithubUnlink)
)
