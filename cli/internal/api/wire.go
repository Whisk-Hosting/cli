package api

import "github.com/whisk-run/contract/apitypes"

// Page is one page of a list: items and the cursor for the next page ("" at the end).
type Page[T any] = apitypes.Page[T]

// The API's JSON as the control plane answers it, defined once in contract/apitypes.
type (
	DeviceCode          = apitypes.DeviceCode
	DeviceCodeRequest   = apitypes.DeviceCodeRequest
	DeviceToken         = apitypes.DeviceToken
	Org                 = apitypes.Org
	User                = apitypes.User
	Whoami              = apitypes.Whoami
	WhoamiToken         = apitypes.WhoamiToken
	GitPassword         = apitypes.GitPassword
	App                 = apitypes.App
	AppProblem          = apitypes.AppProblem
	AppStart            = apitypes.AppStart
	AppMemory           = apitypes.AppMemory
	CreateAppRequest    = apitypes.CreateAppRequest
	Validation          = apitypes.Validation
	Phase               = apitypes.Phase
	Deploy              = apitypes.Deploy
	DeployEvent         = apitypes.DeployEvent
	Warning             = apitypes.Warning
	CreateDeployRequest = apitypes.CreateDeployRequest
)
