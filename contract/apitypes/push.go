package apitypes

import "time"

// PushSubscription is what a browser's PushManager.subscribe answers, as its toJSON gives it.
type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// AddPushDeviceRequest is the body of POST /v1/orgs/{org}/apps/{app}/push/devices: one device a
// person turned notifications on for, under the key the app knows them by.
type AddPushDeviceRequest struct {
	User         string           `json:"user"`
	Subscription PushSubscription `json:"subscription"`
	// Label names the device for the person, such as "Chrome on Android"; optional.
	Label string `json:"label,omitempty"`
}

// PushDevice is one device a person gets an app's notifications on.
type PushDevice struct {
	ID        string     `json:"id"`
	User      string     `json:"user"`
	Label     string     `json:"label,omitempty"`
	Service   string     `json:"service"`
	CreatedAt time.Time  `json:"created_at"`
	LastOKAt  *time.Time `json:"last_ok_at,omitempty"`
}

// PushDeviceQuery is GET /v1/orgs/{org}/apps/{app}/push/devices?user=.
type PushDeviceQuery struct {
	User string `query:"user"`
}

// PushDevicesRemoved is what DELETE …/push/devices answers.
type PushDevicesRemoved struct {
	Removed int `json:"removed"`
}

// PushDeviceRemoveQuery is DELETE /v1/orgs/{org}/apps/{app}/push/devices?user=&endpoint=: a
// person's devices, or the one with that endpoint, as a browser turning notifications off says.
type PushDeviceRemoveQuery struct {
	User     string `query:"user"`
	Endpoint string `query:"endpoint"`
}

// SendPushRequest is the body of POST /v1/orgs/{org}/apps/{app}/push/send. To is one user key or
// a list of them.
type SendPushRequest struct {
	To    any    `json:"to"`
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	// URL is the page a tap opens, a path on the app's own address or a full https URL.
	URL string `json:"url,omitempty"`
	// Tag replaces an earlier notification with the same tag on the device instead of adding one.
	Tag  string `json:"tag,omitempty"`
	Icon string `json:"icon,omitempty"`
	// TTL is how many seconds a push service keeps trying to reach a device that is off.
	TTL int `json:"ttl,omitempty"`
	// Urgency is very-low, low, normal or high.
	Urgency string `json:"urgency,omitempty"`
	// Data is handed to the page a tap opens, as the notification's data.
	Data map[string]any `json:"data,omitempty"`
	// FallbackEmail is mailed the title, body and link when no device took the notification.
	FallbackEmail string `json:"fallback_email,omitempty"`
}

// PushSent is what POST …/push/send answers: the send's id and how many devices it is going to.
type PushSent struct {
	ID      string `json:"id"`
	Devices int    `json:"devices"`
	// Status is queued while devices remain, no_devices when nobody named has one.
	Status string `json:"status"`
}

// PushDelivery is what became of one device's copy of a send.
type PushDelivery struct {
	Device   string     `json:"device"`
	User     string     `json:"user"`
	Status   string     `json:"status"`
	Attempts int        `json:"attempts"`
	Detail   string     `json:"detail,omitempty"`
	At       *time.Time `json:"at,omitempty"`
}

// PushSend is GET …/push/sends/{id}: the send and each device's copy.
type PushSend struct {
	ID         string         `json:"id"`
	Status     string         `json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
	Emailed    bool           `json:"emailed"`
	Deliveries []PushDelivery `json:"deliveries"`
}

var _ = more.addWire(PushSubscription{}, AddPushDeviceRequest{}, PushDevice{}, PushDeviceQuery{}, PushDevicesRemoved{},
	PushDeviceRemoveQuery{}, SendPushRequest{}, PushSent{}, PushDelivery{}, PushSend{})
