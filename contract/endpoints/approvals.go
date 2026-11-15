package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The approvals routes.
var (
	ListAppApprovals = New[ListAppApprovalsQuery, apitypes.List[apitypes.Approval]]("GET /v1/orgs/{org}/apps/{app}/approvals")
	ListOrgApprovals = New[ListOrgApprovalsQuery, apitypes.List[apitypes.Approval]]("GET /v1/orgs/{org}/approvals")
	GetApproval      = New[None, apitypes.Approval]("GET /v1/approvals/{id}")

	_ = public.Add(ListAppApprovals, ListOrgApprovals, GetApproval)
)

// ListAppApprovalsQuery is what ListAppApprovals reads from the query.
type ListAppApprovalsQuery struct {
	Status string `query:"status"`
}

// ListOrgApprovalsQuery is what ListOrgApprovals reads from the query.
type ListOrgApprovalsQuery struct {
	Status string `query:"status"`
}
