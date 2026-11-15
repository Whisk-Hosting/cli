package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The crawlers routes.
var (
	GetCrawlers       = New[GetCrawlersQuery, apitypes.AppCrawlers]("GET /v1/orgs/{org}/apps/{app}/analytics/crawlers")
	GetCrawlerJourney = New[None, apitypes.CrawlerJourneyDetail]("GET /v1/orgs/{org}/apps/{app}/analytics/crawlers/journeys/{journey}")

	_ = public.Add(GetCrawlers, GetCrawlerJourney)
)

// GetCrawlersQuery is what GetCrawlers reads from the query.
type GetCrawlersQuery struct {
	Crawler string `query:"crawler"`
	Period  string `query:"period"`
	Status  string `query:"status"`
}
