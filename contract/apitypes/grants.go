package apitypes

import "time"

// Grants beyond connections: what else a production deploy waits on a person to approve, and
// the one approval that answers a blocked deploy (CONTROL-PLANE.md §6.5 "What waits on a
// person").

// GrantRef names one entry of a blocked deploy's grants_needed exactly as the person was shown
// it: its kind, name and hash.
type GrantRef struct {
	Kind GrantKind `json:"kind"`
	Name string    `json:"name"`
	Hash string    `json:"hash,omitempty"`
}

// ApproveDeployRequest is POST /orgs/:org/apps/:app/deploys/:id/approve: the entries of the
// deploy's grants_needed the person approves, each as it was shown to them. An entry that is
// no longer what the deploy waits on is 409 CONFLICT, so an approval never lands on something
// nobody saw.
type ApproveDeployRequest struct {
	Grants []GrantRef `json:"grants"`
}

// ApprovedDeploy answers an approval: what was granted, and the deploy queued again in the
// blocked one's place.
type ApprovedDeploy struct {
	Approved []GrantRef `json:"approved"`
	Deploy   Deploy     `json:"deploy"`
}

// ConfirmCodeRequest is POST /me/confirm/code: email a code that confirms the calling session
// (CONTROL-PLANE.md §4.2), for a decision the person is making on the page they are on. A code
// already on its way to this session is answered again rather than a second one sent, unless
// Again asks for a new one.
type ConfirmCodeRequest struct {
	Again bool `json:"again,omitempty"`
}

// ConfirmCode answers a code request: the code's flow, which POST /me/confirm sends back with
// the code, and the address it went to.
type ConfirmCode struct {
	Flow  string `json:"flow"`
	Email string `json:"email"`
}

// ConfirmRequest is POST /me/confirm: the code the person typed for the flow.
type ConfirmRequest struct {
	Flow string `json:"flow"`
	Code string `json:"code"`
}

// Confirmed answers a right code: the session is confirmed until then.
type Confirmed struct {
	ConfirmedUntil time.Time `json:"confirmed_until"`
}
