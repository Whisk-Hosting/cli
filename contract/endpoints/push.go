package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The push notification routes, which an app calls with its own service token.
var (
	AddPushDevice     = New[apitypes.AddPushDeviceRequest, apitypes.PushDevice]("POST /v1/orgs/{org}/apps/{app}/push/devices")
	ListPushDevices   = New[apitypes.PushDeviceQuery, apitypes.List[apitypes.PushDevice]]("GET /v1/orgs/{org}/apps/{app}/push/devices")
	RemovePushDevices = New[apitypes.PushDeviceRemoveQuery, apitypes.PushDevicesRemoved]("DELETE /v1/orgs/{org}/apps/{app}/push/devices")
	SendPush          = New[apitypes.SendPushRequest, apitypes.PushSent]("POST /v1/orgs/{org}/apps/{app}/push/send")
	GetPushSend       = New[None, apitypes.PushSend]("GET /v1/orgs/{org}/apps/{app}/push/sends/{id}")

	_ = public.Add(AddPushDevice, ListPushDevices, RemovePushDevices, SendPush, GetPushSend)
)
