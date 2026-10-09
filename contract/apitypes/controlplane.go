package apitypes

import "time"

// The one live control plane (CONTROL-PLANE.md §2.1, §6.20): which of its two copies is live,
// how each answers, and whether the operator can switch back.

// ControlPlaneSlot is one of the control plane's two copies.
type ControlPlaneSlot string

const (
	SlotBlue  ControlPlaneSlot = "blue"
	SlotGreen ControlPlaneSlot = "green"
)

// ControlPlaneRecord is the live record: the slot that serves and works, the epoch it went live
// at, and when and by whom it was switched there ("release:git-<sha>", "user:<id>", "whiskd-cli").
type ControlPlaneRecord struct {
	Slot       ControlPlaneSlot `json:"slot"`
	Epoch      int64            `json:"epoch"`
	SwitchedAt time.Time        `json:"switched_at"`
	SwitchedBy string           `json:"switched_by"`
}

// SlotHealth is a slot's health port's answer. A live copy adds when its switch stops being
// reversible, in Unix seconds.
type SlotHealth struct {
	Status            string           `json:"status"`
	Slot              ControlPlaneSlot `json:"slot"`
	Live              bool             `json:"live"`
	Serving           bool             `json:"serving"`
	Epoch             int64            `json:"epoch"`
	RollbackUntilUnix int64            `json:"rollback_until_unix,omitempty"`
}

// SlotSeen is one slot as its health port answered, or why no copy there answered healthy.
type SlotSeen struct {
	Health *SlotHealth `json:"health,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// ControlPlane is GET /operator/control-plane: the live record, each slot, and whether a
// rollback is possible now (within 10 minutes of the switch, the replaced copy idle and healthy).
type ControlPlane struct {
	Record        ControlPlaneRecord            `json:"record"`
	RollbackUntil time.Time                     `json:"rollback_until"`
	CanRollback   bool                          `json:"can_rollback"`
	Slots         map[ControlPlaneSlot]SlotSeen `json:"slots"`
}
