package apitypes

import "time"

// When Whisk needs another server (CONTROL-PLANE.md §6.33): GET /operator/capacity.

// CapacityStatus is how close the platform is to needing another server: ok, near (60% of the
// app memory it can carry with one server down, or of its disk) or add_server (70%).
type CapacityStatus string

const (
	CapacityOK        CapacityStatus = "ok"
	CapacityNear      CapacityStatus = "near"
	CapacityAddServer CapacityStatus = "add_server"
)

// CapacityPeakSource is where the busiest app memory came from: Prometheus's last seven days
// (history), or the machines' reports now when there is no history (now).
type CapacityPeakSource string

const (
	PeakHistory CapacityPeakSource = "history"
	PeakNow     CapacityPeakSource = "now"
)

// CapacityResource is memory or disk, for which runs out first.
type CapacityResource string

const (
	FirstMemory CapacityResource = "memory"
	FirstDisk   CapacityResource = "disk"
)

// Capacity is GET /operator/capacity: the plan for the servers there are and the next two.
type Capacity struct {
	AsOf       time.Time          `json:"as_of"`
	PeakSource CapacityPeakSource `json:"peak_source"`
	Status     CapacityStatus     `json:"status"`
	// First is the resource with the higher percentage, which the status comes from.
	First CapacityResource `json:"first"`
	// Summary is one line saying what runs out first and when to add a server.
	Summary string `json:"summary"`
	// Servers is the ready machines that reported their memory; 0 leaves Memory and Disk null
	// and Rows empty.
	Servers int `json:"servers"`
	// Failover is whether the servers left carry every app when one fails (two or more).
	Failover bool `json:"failover"`
	// NearPercent and AddPercent are the policy's two thresholds.
	NearPercent float64         `json:"near_percent"`
	AddPercent  float64         `json:"add_percent"`
	Memory      *CapacityMemory `json:"memory"`
	Disk        *CapacityDisk   `json:"disk"`
	Nodes       []CapacityNode  `json:"nodes"`
	Rows        []CapacityRow   `json:"rows"`
	// Prices and Revenue are what the hosted platform's money columns come from. Whisk
	// On-Premise runs on the company's own servers and leaves both out.
	Prices  *CapacityPrices  `json:"prices,omitempty"`
	Revenue *CapacityRevenue `json:"revenue,omitempty"`
}

// CapacityMemory is app memory at its busiest against what the servers carry with one down.
type CapacityMemory struct {
	DemandBytes int64   `json:"demand_bytes"`
	UsableBytes int64   `json:"usable_bytes"`
	AddAtBytes  int64   `json:"add_at_bytes"`
	Percent     float64 `json:"percent"`
}

// CapacityDisk is what the databases and everything else on the Postgres disks need, each
// database counted once per copy, against the disks' total.
type CapacityDisk struct {
	NeededBytes int64   `json:"needed_bytes"`
	TotalBytes  int64   `json:"total_bytes"`
	Copies      int     `json:"copies"`
	Percent     float64 `json:"percent"`
}

// CapacityNode is one machine's part in the plan.
type CapacityNode struct {
	Name          string `json:"name"`
	MemoryBytes   int64  `json:"memory_bytes"`
	PlatformBytes int64  `json:"platform_bytes"`
	UsableBytes   int64  `json:"usable_bytes"`
	AppsBytes     int64  `json:"apps_bytes"`
	PeakAppsBytes int64  `json:"peak_apps_bytes"`
	PeakFromNow   bool   `json:"peak_from_now"`
}

// CapacityRow is one breakpoint: so many servers, what they carry with one down, when to add the
// next, and, on the hosted platform with prices set, what they cost in NZ cents against the
// revenue at that point. A money figure the plan does not have is left out.
type CapacityRow struct {
	Servers      int      `json:"servers"`
	Now          bool     `json:"now"`
	UsableBytes  int64    `json:"usable_bytes"`
	AddAtBytes   int64    `json:"add_at_bytes"`
	MonthlyCents *int64   `json:"monthly_cents,omitempty"`
	PerGBCents   *int64   `json:"per_gb_cents,omitempty"`
	RevenueCents *int64   `json:"revenue_cents,omitempty"`
	SharePercent *float64 `json:"share_percent,omitempty"`
}

// CapacityPrices is where the money columns come from: the monthly price of the server Whisk
// runs, kept in the control plane's code, and the European Central Bank's exchange rates the
// control plane fetches daily. Currency is the server price's currency code. Set is whether the
// costs can be shown in NZ dollars, which needs a rate for that currency. NZDPer is NZ dollars to one unit of each currency the
// plan uses; RatesDate is the oldest bank date among them (YYYY-MM-DD, empty with none) and
// RatesFetchedAt when the oldest of them was fetched.
type CapacityPrices struct {
	Set            bool               `json:"set"`
	Currency       string             `json:"currency"`
	ServerCents    int64              `json:"server_cents"`
	NZDPer         map[string]float64 `json:"nzd_per"`
	RatesDate      string             `json:"rates_date"`
	RatesFetchedAt *time.Time         `json:"rates_fetched_at"`
	Note           string             `json:"note"`
}

// CapacityRevenue is this month's recurring revenue, in US cents and, with a rate, NZ cents.
type CapacityRevenue struct {
	MonthlyUSDCents int64  `json:"monthly_usd_cents"`
	MonthlyNZDCents *int64 `json:"monthly_nzd_cents"`
}
