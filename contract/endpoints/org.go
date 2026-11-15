package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The org routes.
var (
	GetOrg          = New[None, apitypes.Org]("GET /v1/orgs/{org}")
	PatchOrg        = New[apitypes.PatchOrgRequest, apitypes.Org]("PATCH /v1/orgs/{org}")
	ConfirmOrg      = New[apitypes.ConfirmOrgRequest, apitypes.Org]("POST /v1/orgs/{org}/confirm")
	DeleteOrg       = New[None, apitypes.Org]("DELETE /v1/orgs/{org}")
	RestoreOrg      = New[None, apitypes.Org]("POST /v1/orgs/{org}/restore")
	OrgHome         = New[None, apitypes.OrgHome]("GET /v1/orgs/{org}/home")
	ListMembers     = New[None, apitypes.Page[apitypes.Member]]("GET /v1/orgs/{org}/members")
	InviteMember    = New[apitypes.InviteRequest, apitypes.Member]("POST /v1/orgs/{org}/members")
	PatchMember     = New[apitypes.PatchMemberRequest, apitypes.Member]("PATCH /v1/orgs/{org}/members/{id}")
	SetMemberApps   = New[apitypes.PersonAppsRequest, apitypes.PersonAppsRequest]("PUT /v1/orgs/{org}/members/{id}/apps")
	RemoveMember    = New[None, None]("DELETE /v1/orgs/{org}/members/{id}")
	ListGroups      = New[None, apitypes.Page[apitypes.Group]]("GET /v1/orgs/{org}/groups")
	CreateGroup     = New[apitypes.GroupRequest, apitypes.Group]("POST /v1/orgs/{org}/groups")
	PatchGroup      = New[apitypes.GroupRequest, apitypes.Group]("PATCH /v1/orgs/{org}/groups/{id}")
	DeleteGroup     = New[None, None]("DELETE /v1/orgs/{org}/groups/{id}")
	SetGroupMembers = New[apitypes.GroupMembersRequest, apitypes.Group]("PUT /v1/orgs/{org}/groups/{id}/members")

	_ = public.Add(GetOrg, PatchOrg, ConfirmOrg, DeleteOrg, RestoreOrg, OrgHome, ListMembers, InviteMember, PatchMember, SetMemberApps, RemoveMember, ListGroups, CreateGroup, PatchGroup, DeleteGroup, SetGroupMembers)
)
