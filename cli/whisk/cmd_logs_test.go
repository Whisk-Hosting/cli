package whisk

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
)

func TestFormatLine(t *testing.T) {
	at := time.Date(2026, 10, 6, 3, 4, 5, 0, time.Local)
	obj := func(s string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	cases := []struct {
		name   string
		line   api.LogLine
		asJSON bool
		want   string
	}{
		{"plain", api.LogLine{At: at, Line: "booted", Stream: "stdout"}, false, "03:04:05.000  booted"},
		{"plain stderr", api.LogLine{At: at, Line: "boom", Stream: "stderr"}, false, "03:04:05.000 stderr  boom"},
		{"request", api.LogLine{At: at, Stream: "stdout", JSON: obj(`{"level":30,"time":1,"app":"crm","request_id":"01M47J70BC2334PEARWVM5X4FK","method":"GET","path":"/reports/trend","status":200,"user":"jo@acme.example","ms":4629,"msg":"request"}`)}, false,
			"03:04:05.000 INFO  GET /reports/trend 200 4.63s jo@acme.example request_id=01M47J70BC2334PEARWVM5X4FK"},
		{"fast request", api.LogLine{At: at, Stream: "stdout", JSON: obj(`{"level":"info","req":{"method":"post","url":"/api"},"res":{"statusCode":502},"responseTime":12,"msg":"request completed"}`)}, false,
			"03:04:05.000 INFO  POST /api 502 12ms"},
		{"message and fields", api.LogLine{At: at, Stream: "stdout", JSON: obj(`{"level":50,"msg":"report failed","err":{"type":"TypeError","message":"x is undefined"},"attempt":2,"tags":["a"]}`)}, false,
			`03:04:05.000 ERROR report failed attempt=2 err="TypeError: x is undefined" tags=["a"]`},
		{"no level", api.LogLine{At: at, Stream: "stdout", JSON: obj(`{"msg":"synced","status":"done"}`)}, false, "03:04:05.000 -     synced status=done"},
		{"json", api.LogLine{At: at, Stream: "stdout", JSON: obj(`{"msg":"x"}`)}, true, `{"msg":"x"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatLine(c.line, c.asJSON); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}
