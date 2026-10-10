package connect

import (
	"reflect"
	"testing"
)

func TestFillSettings(t *testing.T) {
	c := Connection{URL: "https://{setting.HOST}/b1s/v2", Auth: Auth{Headers: map[string]string{
		"X-Db": "{setting.DATABASE}", "Authorization": "{basic(secret.USER, secret.PASSWORD)}"}}}
	if got := Settings(c); !reflect.DeepEqual(got, []string{"DATABASE", "HOST"}) {
		t.Errorf("Settings = %v", got)
	}
	out, err := FillSettings(c, map[string]string{"HOST": "erp.example.com", "DATABASE": "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if out.URL != "https://erp.example.com/b1s/v2" || out.Auth.Headers["X-Db"] != "acme" ||
		out.Auth.Headers["Authorization"] != "{basic(secret.USER, secret.PASSWORD)}" {
		t.Errorf("filled = %+v", out)
	}
	for name, values := range map[string]map[string]string{
		"missing":     {"HOST": "erp.example.com"},
		"empty":       {"HOST": "erp.example.com", "DATABASE": ""},
		"a brace":     {"HOST": "erp.example.com", "DATABASE": "{secret.USER}"},
		"a quote":     {"HOST": "erp.example.com", "DATABASE": "a'b"},
		"a space":     {"HOST": "erp.example.com", "DATABASE": "a b"},
		"a json char": {"HOST": `x"y`, "DATABASE": "acme"},
	} {
		if _, err := FillSettings(c, values); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	plain := Connection{URL: "https://api.example.com"}
	if out, err := FillSettings(plain, nil); err != nil || out.URL != plain.URL {
		t.Errorf("a connection with no settings: %v %v", out, err)
	}
}
