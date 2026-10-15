// Package ownsignin decides when an app runs its own sign-in without being a Promoted app. An app
// runs its own sign-in when every route is public (routes.public covers /**), so the edge lets
// every request through, and it asks for a password itself: its own login then decides who gets
// in, which only a Promoted app may do (CONTROL-PLANE.md §6.15 "Promoted apps"). A website whose every
// page is public asks for no password and is left alone, and a shop keeps its customers' sign-in
// on any plan. Doctor reports the rest as W106 and a deploy as the warning
// OWN_SIGNIN_NOT_PROMOTED; neither stops anything. Everything here is pure.
package ownsignin

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"github.com/whisk-run/contract/routes"
)

const (
	// Rule is doctor's rule (doctor-rules.md).
	Rule = "W106"
	// Code is the deploy warning's code (errors.md).
	Code = "OWN_SIGNIN_NOT_PROMOTED"
)

// Keeps reports whether routes.public makes every route public: the patterns together match
// every path (routes.CoversAll).
func Keeps(public []string) bool { return routes.CoversAll(public) }

// Warn reports whether an app warrants the warning: every route is public, it asks for a
// password, it is not a Promoted app (promoted is true when it is one in force or its whisk.yaml
// says promoted: true) and it is not a shop.
func Warn(public []string, asksPassword, promoted, shop bool) bool {
	return asksPassword && !promoted && !shop && Keeps(public)
}

// PasswordPatterns find a password field the app renders itself: an HTML or JSX input, an input
// built in script, or a Python form library's password field. Doctor's W090 and W106 and the
// deploy's check read files with these.
var PasswordPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\btype\s*[=:]\s*\{?\s*["'` + "`" + `]?password\b`),
	regexp.MustCompile(`\.type\s*=\s*["'` + "`" + `]password["'` + "`" + `]`),
	regexp.MustCompile(`\b(?:PasswordField|PasswordInput)\b`),
}

// AsksPassword reports whether a file's content renders a password field.
func AsksPassword(content []byte) bool {
	for _, re := range PasswordPatterns {
		if re.Match(content) {
			return true
		}
	}
	return false
}

// pageExts are the code, template and markup files a password field can live in, the same that
// doctor reads for W090.
var pageExts = map[string]bool{
	".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".mts": true, ".cts": true,
	".py": true, ".go": true,
	".html": true, ".htm": true, ".jinja": true, ".jinja2": true, ".j2": true, ".ejs": true,
	".hbs": true, ".handlebars": true, ".njk": true, ".svelte": true, ".vue": true, ".astro": true,
	".tmpl": true, ".gohtml": true, ".templ": true, ".erb": true, ".mustache": true,
}

// Page reports whether a repository path is one of the app's own pages or code, which the
// deploy's check reads: a code, template or markup file outside tests and vendored folders, as
// doctor reads them.
func Page(p string) bool {
	if !pageExts[strings.ToLower(path.Ext(p))] {
		return false
	}
	base := path.Base(p)
	if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") ||
		strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return false
	}
	for _, seg := range strings.Split(path.Dir(p), "/") {
		switch seg {
		case "test", "tests", "__tests__", "spec", "fuzz",
			"node_modules", ".venv", "venv", "vendor", "dist", "build", ".next", "__pycache__", ".whisk", ".git", "site-packages":
			return false
		}
	}
	return true
}

// MedusaShop reports whether a package.json (nil when the app has none) lists @medusajs/medusa
// among its dependencies, as the shop template (CONTRACT.md §11) does. That is how doctor and
// the platform tell a shop, whose customers sign in to the shop itself on any plan.
func MedusaShop(packageJSON []byte) bool {
	var pkg struct {
		Dependencies map[string]any `json:"dependencies"`
	}
	if packageJSON == nil || json.Unmarshal(packageJSON, &pkg) != nil {
		return false
	}
	_, ok := pkg.Dependencies["@medusajs/medusa"]
	return ok
}

// Message is what doctor and the deploy say of app.
func Message(app string) string {
	return "Every route of " + app + " is public (routes.public covers /**) and it asks for a password, so it runs its own sign-in, which only a Promoted app may do; it still deploys."
}

// Fix is the deploy warning's fix: promote is where an owner makes app a Promoted app.
func Fix(app, promote string) string {
	return "If " + app + " must keep its own sign-in, set promoted: true in whisk.yaml and ask an owner or billing contact to make it a Promoted app at " + promote + " (it is charged, so it is their choice). Otherwise remove the password field and list under routes.public only the routes that must be public, and the platform signs people in."
}
