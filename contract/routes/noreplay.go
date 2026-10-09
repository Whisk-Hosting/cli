package routes

import "strings"

// noReplay are the API routes, below /v1, whose answer the platform never keeps for an
// Idempotency-Key, so a key sent to them is ignored: the device code and its poll, and every
// route whose answer carries a credential, a link that acts as one, or the business's data.
// Such an answer must rest nowhere and never be handed out again. A repeated request to one of
// these routes runs again, so a client must not retry a write to it on its own. {name} matches
// any one segment.
var noReplay = []string{
	"/device/code",  // the device code, which the poll trades for a token
	"/device/token", // the poll, and the token it answers
	"/tokens/git",   // a git password

	"/orgs/{org}/tokens",                         // a new agent token's value
	"/orgs/{org}/apps/{app}/deploy-keys",         // a deploy key's value
	"/orgs/{org}/apps/{app}/db/url",              // database connection credentials
	"/orgs/{org}/apps/{app}/db/query",            // query results
	"/orgs/{org}/apps/{app}/db/schema",           // the database's tables
	"/orgs/{org}/apps/{app}/uploads",             // a signed upload URL
	"/orgs/{org}/members",                        // an invitation's join link
	"/orgs/{org}/members/{id}/invite",            // a fresh join link
	"/orgs/{org}/apps/{app}/customers",           // a customer's invitation link
	"/orgs/{org}/clients/{client}/handover",      // a handover link
	"/orgs/{org}/github/installations",           // a GitHub install link with its state
	"/orgs/{org}/billing/portal",                 // a signed-in billing portal link
	"/orgs/{org}/billing/pay/{id}/start",         // a payment's client secret
	"/orgs/{org}/billing/pay/{id}/finish",        // a payment's client secret
	"/orgs/{org}/billing/invoices/{invoice}/pay", // an invoice's payment link
	"/orgs/{org}/billing/payouts",                // a payout onboarding link

	"/operator/tokens",                      // an operator read token's value
	"/operator/identities",                  // a new identity's token value
	"/operator/identities/{id}/credentials", // a rotated identity token's value
	"/operator/nodes",                       // a node's enrolment token
	"/operator/orgs/{org}/breakglass",       // a secret's value
	"/operator/demos",                       // a demo's claim link
	"/operator/github/manifest",             // a GitHub manifest with its state
}

// NoReplayRoutes returns the no-replay route patterns, below /v1. Pure.
func NoReplayRoutes() []string { return append([]string(nil), noReplay...) }

// NoReplay reports whether a path below /v1, concrete ("/orgs/acme/tokens") or a pattern
// ("/orgs/{org}/tokens"), is a no-replay route. A query string is ignored. Pure.
func NoReplay(path string) bool {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	for _, p := range noReplay {
		if patternMatches(p, path) {
			return true
		}
	}
	return false
}

// patternMatches reports whether path has pattern's segments, a {name} segment matching any
// one non-empty segment.
func patternMatches(pattern, path string) bool {
	ps, xs := strings.Split(pattern, "/"), strings.Split(path, "/")
	if len(ps) != len(xs) {
		return false
	}
	for i, s := range ps {
		wild := strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")
		if (wild && xs[i] == "") || (!wild && s != xs[i]) {
			return false
		}
	}
	return true
}
