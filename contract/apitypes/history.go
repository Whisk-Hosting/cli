package apitypes

import "time"

// The platform's recent past as Prometheus kept it (CONTROL-PLANE.md §6.20): GET
// /operator/platform/history and GET /operator/nodes/:id/history, over a range.

// HistoryRange is a window the history pages offer. Prometheus keeps 15 days, so the longest is
// 14.
type HistoryRange string

const (
	HistorySixHours  HistoryRange = "6h"
	HistoryDay       HistoryRange = "24h"
	HistoryWeek      HistoryRange = "7d"
	HistoryFortnight HistoryRange = "14d"
)

// HistoryState is the state of one stretch of a strip: up, down, or none, a stretch with no
// figures because nothing scraped it or it did not exist yet.
type HistoryState string

const (
	HistoryOK   HistoryState = "ok"
	HistoryDown HistoryState = "down"
	HistoryNone HistoryState = "none"
)

// HistoryUnit is what a chart's figures are in.
type HistoryUnit string

const (
	UnitPercent   HistoryUnit = "percent"
	UnitPerMinute HistoryUnit = "per_minute"
)

// History is what a history route answers: the window, its step, and its strips, charts and
// alerts.
type History struct {
	Range       HistoryRange   `json:"range"`
	From        time.Time      `json:"from"`
	To          time.Time      `json:"to"`
	StepSeconds int            `json:"step_seconds"`
	Strips      []HistoryStrip `json:"strips"`
	Charts      []HistoryChart `json:"charts"`
	Alerts      []HistoryAlert `json:"alerts"`
}

// HistoryStrip is one thing up or down over the window. OKPercent is the share of the stretches
// with figures that were up, null when none had.
type HistoryStrip struct {
	Key       string           `json:"key"`
	OKPercent *float64         `json:"ok_percent"`
	Segments  []HistorySegment `json:"segments"`
}

// HistorySegment is a stretch in one state.
type HistorySegment struct {
	From  time.Time    `json:"from"`
	To    time.Time    `json:"to"`
	State HistoryState `json:"state"`
}

// HistoryChart is figures over the window, one line per series.
type HistoryChart struct {
	Key    string          `json:"key"`
	Unit   HistoryUnit     `json:"unit"`
	Series []HistorySeries `json:"series"`
}

// HistorySeries is one line: [unix seconds, figure] pairs, a missing point left out.
type HistorySeries struct {
	Key    string       `json:"key"`
	Points [][2]float64 `json:"points"`
}

// HistoryAlert is one stretch an alert was firing; To is null while it still is.
type HistoryAlert struct {
	Name     string     `json:"name"`
	Detail   string     `json:"detail"`
	Severity string     `json:"severity"`
	From     time.Time  `json:"from"`
	To       *time.Time `json:"to"`
}

// NodeHistory is GET /operator/nodes/:id/history: the machine as the Machines card shows it, and
// its past over the range.
type NodeHistory struct {
	Node Node `json:"node"`
	History
}
