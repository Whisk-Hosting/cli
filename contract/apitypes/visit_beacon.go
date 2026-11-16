package apitypes

// VisitBeacon is what the public site sends for each page a visitor opens, and, with an action, for
// a call to action a visitor used on that page (CONTROL-PLANE.md §6.25).
type VisitBeacon struct {
	Path string `json:"path"`
	// Entry marks the first page of a load of the site, the one a full page load showed, so a
	// load the CDN answered from its copy, which the edge never logs, is still counted.
	Entry    bool   `json:"entry,omitempty"`
	Referrer string `json:"referrer,omitempty"`
	Source   string `json:"source,omitempty"`
	Campaign string `json:"campaign,omitempty"`
	Action   string `json:"action,omitempty"`
	// Target is the page on another site a link led to, sent with the action outbound.
	Target string `json:"target,omitempty"`
	// Screen is the screen's width in CSS pixels and Language the browser's first language,
	// sent with each page.
	Screen   int    `json:"screen,omitempty"`
	Language string `json:"language,omitempty"`
	// ScreenHeight is the screen's height, and Automated is true when the browser says
	// automation drives it (navigator.webdriver); both only tell a headless browser apart and
	// are not kept.
	ScreenHeight int  `json:"screen_height,omitempty"`
	Automated    bool `json:"automated,omitempty"`
	// Leave marks a reading of the page the visitor is leaving or hiding, not a page view:
	// seconds in view, how far down it was scrolled (percent), and for the first page of a load
	// how long it took to show.
	Leave   bool `json:"leave,omitempty"`
	Seconds int  `json:"seconds,omitempty"`
	Scroll  int  `json:"scroll,omitempty"`
	LoadMS  int  `json:"load_ms,omitempty"`
}

var _ = more.addWire(VisitBeacon{})
