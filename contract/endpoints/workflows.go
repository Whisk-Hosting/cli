package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The workflows routes.
var (
	GetRun           = New[None, apitypes.Run]("GET /v1/orgs/{org}/apps/{app}/runs/{id}")
	ReplayRun        = New[None, apitypes.Run]("POST /v1/orgs/{org}/apps/{app}/runs/{id}/replay")
	CancelRun        = New[None, apitypes.Run]("POST /v1/orgs/{org}/apps/{app}/runs/{id}/cancel")
	RunCron          = New[None, apitypes.Run]("POST /v1/orgs/{org}/apps/{app}/cron/{function}/run")
	OrgRuns          = New[OrgRunsQuery, apitypes.RunList]("GET /v1/orgs/{org}/runs")
	ListFunctions    = New[None, apitypes.List[apitypes.WorkflowFunction]]("GET /v1/orgs/{org}/apps/{app}/functions")
	GetFunctionGraph = New[None, apitypes.FunctionGraph]("GET /v1/orgs/{org}/apps/{app}/functions/{name}/graph")

	_ = public.Add(GetRun, ReplayRun, CancelRun, RunCron, OrgRuns, ListFunctions, GetFunctionGraph)
)

// OrgRunsQuery is what OrgRuns reads from the query.
type OrgRunsQuery struct {
	Cursor   string `query:"cursor"`
	Function string `query:"function"`
	Limit    int    `query:"limit"`
	Since    string `query:"since"`
	Status   string `query:"status"`
}
