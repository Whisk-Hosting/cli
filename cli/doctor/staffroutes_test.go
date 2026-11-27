package doctor

import "testing"

// A route named for staff, in an app its customers sign in to, checks who the caller is (W180).
func TestStaffRoutes(t *testing.T) {
	manifest := passing["whisk.yaml"]
	customers := manifest + "customer_identity: app\n"
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"express admin route, no check", map[string]string{"whisk.yaml": customers, "src/admin.ts": "app.get(\"/admin/orders\", (c) => c.json(all()));\n"}, true},
		{"router use under internal", map[string]string{"whisk.yaml": customers, "src/routes.ts": "router.use('/internal/reports', reports);\n"}, true},
		{"fastapi staff route", map[string]string{"whisk.yaml": customers, "app/staff.py": "@app.get(\"/staff/refunds\")\ndef refunds(): ...\n"}, true},
		{"go admin handler", map[string]string{"whisk.yaml": customers, "admin.go": "package main\nfunc init() { mux.HandleFunc(\"GET /admin/users\", users) }\n"}, true},
		{"next page under admin", map[string]string{"whisk.yaml": customers, "app/admin/page.tsx": "export default function Page() { return <Orders /> }\n"}, true},
		{"checks the audience", map[string]string{"whisk.yaml": customers, "src/admin.ts": "if (c.req.header(\"X-Whisk-Audience\") !== \"team\") return c.text(\"\", 403);\napp.get(\"/admin/orders\", h);\n"}, false},
		{"checks a role", map[string]string{"whisk.yaml": customers, "src/admin.ts": "app.get(\"/admin/orders\", (c) => hasRole(identity(h), \"admin\") ? ok() : no());\n"}, false},
		{"python role check", map[string]string{"whisk.yaml": customers, "app/staff.py": "@app.get(\"/staff/refunds\")\ndef refunds(req):\n    if not has_role(who(req), \"admin\"): raise Forbidden()\n"}, false},
		{"a middleware checks", map[string]string{"whisk.yaml": customers, "middleware.ts": "if (req.headers.get('x-whisk-audience') !== 'team') return deny();\n", "app/admin/page.tsx": "export default function Page() {}\n"}, false},
		{"no customers sign in", map[string]string{"src/admin.ts": "app.get(\"/admin/orders\", h);\n"}, false},
		{"not a staff route", map[string]string{"whisk.yaml": customers, "src/orders.ts": "app.get(\"/orders/:id\", h);\n", "app/administration-fees/page.tsx": "x\n"}, false},
		{"a python package named app", map[string]string{"whisk.yaml": customers, "app/admin/models.py": "class Refund: ...\n"}, false},
		{"in browser code", map[string]string{"whisk.yaml": customers, "public/admin.js": "api.get('/admin/orders');\n"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, "W180"); got != c.want {
				t.Errorf("W180 = %v, want %v; findings %+v", got, c.want, rep.Findings)
			}
		})
	}
}
