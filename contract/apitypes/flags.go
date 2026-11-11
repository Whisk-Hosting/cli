package apitypes

import "time"

// An app's own feature flags (CONTRACT.md §3.3, CONTROL-PLANE.md §6.39): the flags its whisk.yaml
// declares, and who each is on for, which a person or agent changes with no deploy.

// FlagKind is what a flag is for, as whisk.yaml declares it.
type FlagKind string

// Flag kinds.
const (
	FlagRelease    FlagKind = "release"
	FlagKillSwitch FlagKind = "kill-switch"
	FlagPermission FlagKind = "permission"
)

// FlagGroup is a group of the business a flag is on for.
type FlagGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AppFlag is one flag the app's whisk.yaml in force declares and who it is on for. Nothing set is
// off for everyone. Overdue is a release flag past its remove_by day.
type AppFlag struct {
	Name     string      `json:"name"`
	Kind     FlagKind    `json:"kind"`
	RemoveBy string      `json:"remove_by,omitempty"`
	Overdue  bool        `json:"overdue"`
	People   []string    `json:"people"`
	Groups   []FlagGroup `json:"groups"`
	Team     bool        `json:"team"`
	Percent  int         `json:"percent"`
	Previews bool        `json:"previews"`
	SetBy    string      `json:"set_by,omitempty"`
	SetAt    *time.Time  `json:"set_at,omitempty"`
}

// AppFlagList is GET /orgs/:org/apps/:app/flags, by name. Included is whether the business's plan
// includes flags; while it does not, every flag is off whatever is set.
type AppFlagList struct {
	Items    []AppFlag `json:"items"`
	Included bool      `json:"included"`
}

// AppFlagRequest is PUT /orgs/:org/apps/:app/flags/:name: who the flag is on for from now,
// replacing what was set. People are emails, groups are the business's group names, team is
// everyone on the business's team, percent is a share of everyone else signed in, and previews
// turns it on in every preview.
type AppFlagRequest struct {
	People   []string `json:"people"`
	Groups   []string `json:"groups"`
	Team     bool     `json:"team"`
	Percent  int      `json:"percent"`
	Previews bool     `json:"previews"`
}

// FlagsOn is GET /orgs/:org/apps/:app/flags/on?user=: the flags on for one person, sorted, for
// an app's own work outside a request (a scheduled job, a queue handler). A person who is not on
// the app's team or among its customers gets the flags on for everyone.
type FlagsOn struct {
	Flags []string `json:"flags"`
}

// VisitorFlags is GET /v1/flags/visitors: Whisk's own feature flags that are on for visitors
// nobody signed in (CONTROL-PLANE.md §6.37), by key, which the public site draws them with.
type VisitorFlags struct {
	Flags []string `json:"flags"`
}

var (
	_ = more.addWire(FlagGroup{}, AppFlag{}, AppFlagList{}, AppFlagRequest{}, FlagsOn{}, VisitorFlags{})
	_ = more.addEnum(enumOf("FlagKind", "FLAG_KINDS", FlagRelease, FlagKillSwitch, FlagPermission))
)
