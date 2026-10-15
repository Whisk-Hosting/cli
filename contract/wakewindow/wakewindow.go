// Package wakewindow holds the three spans that together keep the edge's wake registry from
// ever leading one app's traffic to another app's container (docs/CADDY.md §5.2,
// docs/NODE-AGENT.md §A5). The edge's whisk_wake module and the node agent both read them from
// here, so they cannot drift apart.
package wakewindow

import "time"

// Confirm is how long the edge dials an awake address after the node last sent or confirmed
// it, while the node can be reached. Past it the edge asks the node again first.
const Confirm = 60 * time.Second

// Grace is how long the edge keeps dialling an awake address after the node last confirmed it
// when the node cannot be asked (its agent restarting for a release, crashed or slow): the
// containers keep running without their agent, so an outage of the agent is not an outage of
// the apps. Half of SubnetQuarantine, which leaves five minutes of margin.
const Grace = SubnetQuarantine / 2

// SubnetQuarantine is how long the node keeps a freed app subnet out of use. It must exceed
// Grace with room to spare: an address the edge may still dial belongs to a subnet freed no
// earlier than the edge's last confirmation, so no address is dialled once its subnet can
// belong to another app.
const SubnetQuarantine = 10 * time.Minute
