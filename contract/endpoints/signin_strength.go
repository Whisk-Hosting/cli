package endpoints

// The signin strength routes.
var (
	ResetMemberSignIn = New[None, None]("POST /v1/orgs/{org}/members/{id}/reset-sign-in")

	_ = public.Add(ResetMemberSignIn)
)
