// Package keyshapes names the shapes of other services' live credentials, so text Whisk keeps or
// shows (an app's logs, its spans, its build output) never holds one an app printed by mistake,
// even one Whisk never delivered. Each pattern is in the syntax Go's regexp and Vector's
// (Rust's regex) both read, so the log pipeline and the control plane mask the same things.
// Test keys and publishable keys are left as they are: they grant nothing, and an app's own
// people read them while debugging.
package keyshapes

import "regexp"

// Shape is one kind of credential: the name a masked value reads as, and its pattern.
type Shape struct {
	Name    string
	Pattern string
}

// Shapes are every credential shape masked, in the order they are applied: a longer prefix
// before one it starts with.
var Shapes = []Shape{
	{"stripe_key", `\b[rs]k_live_[0-9A-Za-z]{16,}`},
	{"stripe_webhook_secret", `\bwhsec_[0-9A-Za-z+/=]{24,}`},
	{"github_token", `\b(?:gh[pousr]_[0-9A-Za-z]{36,}|github_pat_[0-9A-Za-z_]{40,})`},
	{"aws_access_key", `\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`},
	{"slack_token", `\bxox[abposr]-[0-9A-Za-z-]{10,}`},
	{"anthropic_key", `\bsk-ant-[0-9A-Za-z_-]{20,}`},
	{"openai_key", `\bsk-(?:proj-|svcacct-|admin-)?[0-9A-Za-z_-]{32,}`},
	{"google_api_key", `\bAIza[0-9A-Za-z_-]{35}`},
	{"sendgrid_key", `\bSG\.[0-9A-Za-z_-]{16,}\.[0-9A-Za-z_-]{16,}`},
}

// Mask is the text "[REDACTED:<name>]" a value of a shape is replaced by.
func Mask(name string) string { return "[REDACTED:" + name + "]" }

var compiled = func() []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(Shapes))
	for i, s := range Shapes {
		out[i] = regexp.MustCompile(s.Pattern)
	}
	return out
}()

// Text is v with every credential of a known shape masked. Pure.
func Text(v string) string {
	for i, re := range compiled {
		v = re.ReplaceAllLiteralString(v, Mask(Shapes[i].Name))
	}
	return v
}

// Is reports whether v holds a credential of the shape named name. Pure.
func Is(name, v string) bool {
	for i, s := range Shapes {
		if s.Name == name {
			return compiled[i].MatchString(v)
		}
	}
	return false
}
