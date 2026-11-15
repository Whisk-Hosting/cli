package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The webhooks routes.
var (
	ListPresets        = New[None, map[string]any]("GET /v1/webhooks/presets")
	ListWebhooks       = New[None, apitypes.List[apitypes.WebhookSource]]("GET /v1/orgs/{org}/apps/{app}/webhooks")
	GetWebhook         = New[None, apitypes.WebhookSource]("GET /v1/orgs/{org}/apps/{app}/webhooks/{name}")
	ListWebhookEvents  = New[ListWebhookEventsQuery, apitypes.List[apitypes.WebhookEvent]]("GET /v1/orgs/{org}/apps/{app}/webhooks/{name}/events")
	ReplayWebhookEvent = New[None, apitypes.WebhookEvent]("POST /v1/orgs/{org}/apps/{app}/webhooks/{name}/events/{id}/replay")

	_ = public.Add(ListPresets, ListWebhooks, GetWebhook, ListWebhookEvents, ReplayWebhookEvent)
)

// ListWebhookEventsQuery is what ListWebhookEvents reads from the query.
type ListWebhookEventsQuery struct {
	Limit int `query:"limit"`
}
