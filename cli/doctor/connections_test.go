package doctor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/contract/connect"
	"github.com/whisk-run/contract/manifest"
)

// The connection fixture: erp signs with ERP_ID (sent) and ERP_KEY (only signs); crm and erp
// both read SHARED_ID.
func connectionsManifest() manifest.Manifest {
	return manifest.Manifest{Connections: map[string]connect.Connection{
		"erp": {URL: "https://api.example-erp.com/v2", Auth: connect.Auth{Headers: map[string]string{
			"api-auth-id":        "{secret.ERP_ID}",
			"api-auth-signature": "{base64(hmac_sha256(secret.ERP_KEY, request.query))}",
			"x-shared":           "{secret.SHARED_ID}",
		}}, Operations: []connect.Operation{{Name: "Read stock levels", Method: "GET", Path: "/stock/**"}}},
		"crm": {URL: "https://api.example-crm.com", Auth: connect.Auth{Headers: map[string]string{
			"Authorization": "Bearer {secret.SHARED_ID}",
		}}, Operations: []connect.Operation{{Name: "Read contacts", Method: "GET", Path: "/contacts"}}},
		"open": {URL: "https://api.example-open.com", Operations: []connect.Operation{{Name: "Read", Method: "GET", Path: "/**"}}},
	}}
}

func TestConnectionKeyOwners(t *testing.T) {
	cases := []struct {
		name string
		m    manifest.Manifest
		want map[string][]string
	}{
		{"no connections", manifest.Manifest{}, map[string][]string{}},
		{"each key with the connections that read it, sorted", connectionsManifest(), map[string][]string{
			"ERP_ID": {"erp"}, "ERP_KEY": {"erp"}, "SHARED_ID": {"crm", "erp"},
		}},
	}
	for _, c := range cases {
		if got := connectionKeyOwners(c.m); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestConnectionKeyReads(t *testing.T) {
	owners := map[string][]string{"ERP_KEY": {"erp"}, "SHARED_ID": {"crm", "erp"}}
	cases := []struct {
		name string
		hits []hit
		want []hit
	}{
		{"nothing read", nil, nil},
		{"other names are not connection keys", []hit{{"API_KEY", "a.ts", 1}, {"WHISK_CONNECTION_ERP_URL", "a.ts", 2}}, nil},
		{"one finding per name, at its first read", []hit{
			{"ERP_KEY", "a.ts", 3}, {"API_KEY", "a.ts", 4}, {"ERP_KEY", "b.ts", 1}, {"SHARED_ID", "b.ts", 9},
		}, []hit{{"ERP_KEY", "a.ts", 3}, {"SHARED_ID", "b.ts", 9}}},
	}
	for _, c := range cases {
		if got := connectionKeyReads(owners, c.hits); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestConnectionKeyMessage(t *testing.T) {
	cases := []struct {
		name  string
		conns []string
		want  string
	}{
		{"ERP_KEY", []string{"erp"}, "ERP_KEY is read from the environment, but it is a key of the connection erp: Whisk's broker holds it and it never reaches the app."},
		{"SHARED_ID", []string{"crm", "erp"}, "SHARED_ID is read from the environment, but it is a key of the connections crm, erp: Whisk's broker holds it and it never reaches the app."},
	}
	for _, c := range cases {
		if got := connectionKeyMessage(c.name, c.conns); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

const erpConnection = `connections:
  erp:
    url: https://api.example-erp.com/v2
    auth:
      headers:
        Authorization: "Bearer {secret.ERP_API_KEY}"
    operations:
      - { name: Read stock levels, method: GET, path: "/stock/**" }
`

// Reading a connection's key is W032, never W030: the connection declares it. Calling the
// connection's address is neither.
func TestW032(t *testing.T) {
	cases := []struct {
		name     string
		code     string
		want032  bool
		wantLine int
	}{
		{"reads the key", "const url = process.env.WHISK_CONNECTION_ERP_URL;\nexport const k = process.env.ERP_API_KEY;\n", true, 2},
		{"reads it in Python", "import os\nkey = os.environ.get(\"ERP_API_KEY\")\n", true, 2},
		{"calls through the broker", "export const url = process.env.WHISK_CONNECTION_ERP_URL;\nexport const t = process.env.WHISK_SERVICE_TOKEN;\n", false, 0},
		{"names it only as a literal", "export const label = \"ERP_API_KEY\";\n", false, 0},
	}
	for _, c := range cases {
		file := "src/erp.ts"
		if strings.HasPrefix(c.code, "import os") {
			file = "erp.py"
		}
		rep := runDoctor(t, with(passing, map[string]string{"whisk.yaml": passing["whisk.yaml"] + erpConnection, file: c.code}), false)
		if has(rep, "W030") {
			t.Errorf("%s: W030 reported a connection's key: %+v", c.name, rep.Findings)
		}
		if got := has(rep, "W032"); got != c.want032 {
			t.Errorf("%s: W032 = %v, want %v; findings %+v; skipped %v", c.name, got, c.want032, rep.Findings, rep.Skipped)
			continue
		}
		for _, f := range rep.Findings {
			if f.Rule == "W032" && (f.File != file || f.Line != c.wantLine || f.Level != "warning" || !strings.Contains(f.Fix, "WHISK_CONNECTION_")) {
				t.Errorf("%s: finding %+v", c.name, f)
			}
		}
	}
}
