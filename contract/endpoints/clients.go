package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The clients routes.
var (
	ListClients     = New[None, apitypes.Clients]("GET /v1/orgs/{org}/clients")
	ClientsOverview = New[None, apitypes.Page[apitypes.ClientOverview]]("GET /v1/orgs/{org}/clients/overview")
	RemoveClient    = New[None, apitypes.Org]("DELETE /v1/orgs/{org}/clients/{client}")
	HandoverClient  = New[apitypes.HandoverClientRequest, apitypes.HandoverMade]("POST /v1/orgs/{org}/clients/{client}/handover")
	AddClient       = New[apitypes.ClientRequest, apitypes.ClientAdded]("POST /v1/orgs/{org}/clients")
	BillClient      = New[apitypes.ClientRequest, apitypes.ClientAdded]("POST /v1/orgs/{org}/clients/{client}/billing")

	_ = public.Add(ListClients, ClientsOverview, RemoveClient, HandoverClient, AddClient, BillClient)
)
