package manifest

import (
	"strings"
	"testing"
)

// The app's admin, which the Whisk bar links (CONTRACT.md §3.1): a path on the app's own address,
// and an editor link with {path} once; anything a browser could read as another host is refused.
func TestAdmin(t *testing.T) {
	m, err := Parse([]byte("whisk: 1\nname: site\nadmin:\n  path: /app\n  edit: /app/edit?path={path}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Admin == nil || m.Admin.Path != "/app" || m.Admin.Edit != "/app/edit?path={path}" {
		t.Fatalf("admin = %+v", m.Admin)
	}
	if none, _ := Parse([]byte("whisk: 1\nname: site\n")); none.Admin != nil {
		t.Fatalf("no admin = %+v", none.Admin)
	}
	for _, tc := range []struct{ name, yaml, problem string }{
		{"no path", "admin: {edit: '/a/{path}'}", "path"},
		{"another host", "admin: {path: //evil.example}", "/admin/path"},
		{"a backslash", `admin: {path: '/\evil.example'}`, "/admin/path"},
		{"a full address", "admin: {path: 'https://evil.example'}", "admin"},
		{"edit without the page", "admin: {path: /app, edit: /app/edit}", "admin"},
		{"edit on another host", "admin: {path: /app, edit: '//evil.example/{path}'}", "/admin/edit"},
		{"edit twice", "admin: {path: /app, edit: '/a/{path}/{path}'}", "/admin/edit"},
		{"something else", "admin: {path: /app, home: /}", "admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte("whisk: 1\nname: site\n" + tc.yaml + "\n"))
			if err == nil || !strings.Contains(err.Error(), tc.problem) {
				t.Fatalf("Parse = %v, want a problem naming %s", err, tc.problem)
			}
		})
	}
}
