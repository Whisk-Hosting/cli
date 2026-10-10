package manifest

import (
	"strings"
	"testing"
)

const withConnection = `whisk: 1
name: shop
secrets: [OTHER]
connections:
  erp:
    url: https://api.example.com/v2
    auth:
      headers:
        Authorization: "Bearer {secret.ERP_KEY}"
    operations:
      - { name: Read stock, method: get, path: "/stock/**" }
`

func TestConnections(t *testing.T) {
	m, err := Parse([]byte(withConnection))
	if err != nil {
		t.Fatalf("problems %v", err)
	}
	c := m.Connections["erp"]
	if c.Operations[0].Method != "GET" {
		t.Fatalf("method not upper-cased: %+v", c.Operations[0])
	}
	if got := ConnectionSecrets(m); strings.Join(got, ",") != "ERP_KEY" {
		t.Fatalf("connection secrets %v", got)
	}
	for _, c := range []struct{ yaml, path string }{
		{strings.Replace(withConnection, "secrets: [OTHER]", "secrets: [ERP_KEY]", 1), "/connections/erp/auth"},
		{strings.Replace(withConnection, "secret.ERP_KEY", "secret.WHISK_KEY", 1), "/connections/erp/auth"},
		{strings.Replace(withConnection, "https://api.example.com/v2", "https://api.example.com/v2?x=1", 1), "/connections/erp/url"},
		{strings.Replace(withConnection, "{secret.ERP_KEY}", "{secret.ERP_KEY", 1), "/connections/erp/auth/headers/Authorization"},
		{strings.Replace(withConnection, "  erp:", "  Erp:", 1), "/connections"},
	} {
		_, err := Parse([]byte(c.yaml))
		ps, _ := err.(Problems)
		hit := false
		for _, p := range ps {
			hit = hit || strings.HasPrefix(p.Path, c.path)
		}
		if !hit {
			t.Errorf("no problem under %s: %v", c.path, ps)
		}
	}
	if m, err := Parse([]byte("whisk: 1\nname: plain\n")); err != nil || m.Connections == nil {
		t.Fatalf("no connections: %v %v", m.Connections, err)
	}
}
