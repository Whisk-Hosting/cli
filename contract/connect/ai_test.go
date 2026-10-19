package connect

import (
	"encoding/json"
	"strings"
	"testing"
)

func aiConn(a *AI) Connection {
	return Connection{
		URL:        "https://api.openai.com/v1",
		Auth:       Auth{Headers: map[string]string{"Authorization": "Bearer {secret.OPENAI_API_KEY}"}},
		Operations: []Operation{{Name: "Write replies", Method: "POST", Path: "/chat/completions"}, {Name: "List models", Method: "GET", Path: "/models"}},
		AI:         a,
	}
}

func openAIChat() *AI {
	return &AI{
		SpendCap: &SpendCap{PerDay: 5, PerMonth: 50},
		Cost: []CostPart{
			{At: Pointers{"/usage/prompt_tokens"}, USDPerMillion: 2.5},
			{At: Pointers{"/usage/completion_tokens"}, USDPerMillion: 10},
		},
		Free: []string{"List models"},
	}
}

func TestCheckAI(t *testing.T) {
	cases := []struct {
		name string
		c    Connection
		want []string // problem paths, in order
	}{
		{"a full AI connection", aiConn(openAIChat()), nil},
		{"an AI provider without ai", aiConn(nil), []string{"/ai"}},
		{"Azure OpenAI without ai", func() Connection { c := aiConn(nil); c.URL = "https://acme.openai.azure.com/openai"; return c }(), []string{"/ai"}},
		{"another API needs no ai", func() Connection { c := aiConn(nil); c.URL = "https://api.example-erp.com/v2"; return c }(), nil},
		{"no spend cap: the person sets it", aiConn(&AI{Cost: openAIChat().Cost}), nil},
		{"an empty spend cap", aiConn(&AI{SpendCap: &SpendCap{}, Cost: openAIChat().Cost}), []string{"/ai/spend_cap"}},
		{"a cap too small", aiConn(&AI{SpendCap: &SpendCap{PerWeek: 0.001}, Cost: openAIChat().Cost}), []string{"/ai/spend_cap/per_week"}},
		{"a cap too large", aiConn(&AI{SpendCap: &SpendCap{Total: 2000000}, Cost: openAIChat().Cost}), []string{"/ai/spend_cap/total"}},
		{"no cost", aiConn(&AI{SpendCap: &SpendCap{PerDay: 1}}), []string{"/ai/cost"}},
		{"two prices", aiConn(&AI{SpendCap: &SpendCap{PerDay: 1}, Cost: []CostPart{{At: Pointers{"/usage/cost"}, USDEach: 1, USDPerMillion: 1}}}), []string{"/ai/cost/0"}},
		{"no price", aiConn(&AI{SpendCap: &SpendCap{PerDay: 1}, Cost: []CostPart{{At: Pointers{"/usage/cost"}}}}), []string{"/ai/cost/0"}},
		{"no pointer", aiConn(&AI{SpendCap: &SpendCap{PerDay: 1}, Cost: []CostPart{{USDEach: 1}}}), []string{"/ai/cost/0/at"}},
		{"not a pointer", aiConn(&AI{SpendCap: &SpendCap{PerDay: 1}, Cost: []CostPart{{At: Pointers{"usage.cost"}, USDEach: 1}}}), []string{"/ai/cost/0/at/0"}},
		{"a free operation that is not one", aiConn(&AI{SpendCap: &SpendCap{PerDay: 1}, Cost: openAIChat().Cost, Free: []string{"Delete files"}}), []string{"/ai/free/0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps, _ := Check(c.c.WithDefaults())
			var got []string
			for _, p := range ps {
				if strings.HasPrefix(p.Path, "/ai") {
					got = append(got, p.Path)
				}
			}
			if strings.Join(got, " ") != strings.Join(c.want, " ") {
				t.Fatalf("problems %v, want %v (%v)", got, c.want, ps)
			}
		})
	}
}

func TestAIIsPartOfTheGrant(t *testing.T) {
	granted := aiConn(openAIChat())
	raised := aiConn(openAIChat())
	raised.AI.SpendCap.PerMonth = 100
	if changed, _ := Gaps(&granted, raised); changed {
		t.Fatal("the agent's suggested cap is not the person's: changing it waits for nobody")
	}
	if e := Effective(&granted, raised); e == nil || e.AI.SpendCap.PerMonth != 50 {
		t.Fatalf("a deploy runs under the cap the person set, whatever whisk.yaml suggests: %+v", e)
	}
	cheaper := aiConn(openAIChat())
	cheaper.AI.Cost[0].USDPerMillion = 0.1
	if changed, _ := Gaps(&granted, cheaper); !changed {
		t.Fatal("a changed price must wait for a person")
	}
	if Hash(granted) != Hash(raised) {
		t.Fatal("the cap is not part of the version a person allows")
	}
	if !Covers(&granted, aiConn(openAIChat())) {
		t.Fatal("the same AI connection is covered")
	}
	s := Describe(granted)
	if s.AI == nil || s.AI.Caps[PeriodMonth] != 50 || s.AI.Caps[PeriodDay] != 5 || len(s.AI.Parts) != 2 || s.AI.Free[0] != "List models" {
		t.Fatalf("the grant screen shows the cap and prices: %+v", s.AI)
	}
}

func TestPrice(t *testing.T) {
	a := openAIChat()
	cases := []struct {
		name   string
		values map[int]float64
		micros int64
		found  bool
	}{
		{"nothing found", map[int]float64{}, 0, false},
		{"tokens", map[int]float64{0: 1000, 1: 500}, 2500 + 5000, true},
		{"one part", map[int]float64{1: 1}, 10, true},
		{"rounded up", map[int]float64{0: 1}, 3, true},
		{"a negative number counts nothing", map[int]float64{0: -1000}, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, f := a.Price(c.values)
			if m != c.micros || f != c.found {
				t.Fatalf("got %d %v, want %d %v", m, f, c.micros, c.found)
			}
		})
	}
	dollars := AI{Cost: []CostPart{{At: Pointers{"/usage/cost"}, USDEach: 1}}}
	if m, _ := dollars.Price(map[int]float64{0: 0.0123}); m != 12300 {
		t.Fatalf("a cost in dollars: got %d micros", m)
	}
	ticks := AI{Cost: []CostPart{{At: Pointers{"/usage/cost_in_usd_ticks"}, USDPerMillion: 0.0001}}}
	if m, _ := ticks.Price(map[int]float64{0: 1e10}); m != 1000000 {
		t.Fatalf("ten billion ticks of a ten-billionth of a dollar is a dollar: got %d", m)
	}
}

func TestPointersReadOneOrMany(t *testing.T) {
	var p struct{ At Pointers }
	for src, want := range map[string]string{`{"At":"/usage/cost"}`: "/usage/cost", `{"At":["/a","/b"]}`: "/a /b"} {
		if err := jsonUnmarshal(src, &p); err != nil || strings.Join(p.At, " ") != want {
			t.Fatalf("%s: %v %v", src, p.At, err)
		}
	}
	if Canonical(aiConn(openAIChat())) == "" || !strings.Contains(Canonical(aiConn(openAIChat())), `"at":["/usage/prompt_tokens"]`) {
		t.Fatal("pointers are written as a list")
	}
}

func meter(a *AI, contentType string, chunks ...string) map[int]float64 {
	m := NewMeter(*a, contentType)
	for _, c := range chunks {
		_, _ = m.Write([]byte(c))
	}
	return m.Close()
}

func TestMeter(t *testing.T) {
	anthropic := &AI{SpendCap: &SpendCap{PerDay: 1}, Cost: []CostPart{
		{At: Pointers{"/usage/input_tokens", "/message/usage/input_tokens"}, USDPerMillion: 3},
		{At: Pointers{"/usage/output_tokens", "/message/usage/output_tokens"}, USDPerMillion: 15},
	}}
	gemini := &AI{SpendCap: &SpendCap{PerDay: 1}, Cost: []CostPart{
		{At: Pointers{"/usageMetadata/promptTokenCount"}, USDPerMillion: 1},
		{At: Pointers{"/usageMetadata/candidatesTokenCount"}, USDPerMillion: 4},
	}}
	responses := &AI{SpendCap: &SpendCap{PerDay: 1}, Cost: []CostPart{
		{At: Pointers{"/usage/input_tokens", "/response/usage/input_tokens"}, USDPerMillion: 2},
		{At: Pointers{"/usage/output_tokens", "/response/usage/output_tokens"}, USDPerMillion: 8},
	}}
	openrouter := &AI{SpendCap: &SpendCap{PerDay: 1}, Cost: []CostPart{{At: Pointers{"/usage/cost"}, USDEach: 1}}}
	cases := []struct {
		name   string
		ai     *AI
		ctype  string
		chunks []string
		want   map[int]float64
	}{
		{"OpenAI JSON", openAIChat(), "application/json", []string{`{"id":"x","choices":[{"message":{"content":"hi"}}],"usage":{"prompt_tokens":12,"completion_tokens":34,"total_tokens":46}}`}, map[int]float64{0: 12, 1: 34}},
		{"OpenAI JSON split anywhere", openAIChat(), "application/json; charset=utf-8", []string{`{"usage":{"prompt_`, `tokens":1`, `2,"completion_tokens":3`, `4}}`}, map[int]float64{0: 12, 1: 34}},
		{"OpenAI stream with usage", openAIChat(), "text/event-stream", []string{
			"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}],\"usage\":null}\n\n",
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":9}}\n\n",
			"data: [DONE]\n\n"}, map[int]float64{0: 7, 1: 9}},
		{"OpenAI stream without usage", openAIChat(), "text/event-stream", []string{"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"}, map[int]float64{}},
		{"Anthropic stream, running totals", anthropic, "text/event-stream; charset=utf-8", []string{
			"event: message_start\r\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":25,\"output_tokens\":1}}}\r\n\r\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"Hello\"}}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"}, map[int]float64{0: 25, 1: 15}},
		{"Anthropic JSON", anthropic, "application/json", []string{`{"content":[{"type":"text","text":"x"}],"usage":{"input_tokens":10,"output_tokens":20}}`}, map[int]float64{0: 10, 1: 20}},
		{"Gemini streamed as one JSON array", gemini, "application/json", []string{
			`[{"candidates":[],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":2}},`,
			`{"candidates":[],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":40}}]`}, map[int]float64{0: 8, 1: 40}},
		{"OpenAI Responses stream", responses, "text/event-stream", []string{
			"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n",
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":50}}}\n\n"}, map[int]float64{0: 100, 1: 50}},
		{"OpenRouter's own cost", openrouter, "application/json", []string{`{"usage":{"prompt_tokens":5,"cost":0.00042}}`}, map[int]float64{0: 0.00042}},
		{"a cost written as a string", openrouter, "application/json", []string{`{"usage":{"cost":"0.5"}}`}, map[int]float64{0: 0.5}},
		{"not JSON", openAIChat(), "text/plain", []string{"upstream error"}, map[int]float64{}},
		{"an event stream ending without a newline", openAIChat(), "text/event-stream", []string{"data: {\"usage\":{\"prompt_tokens\":3}}"}, map[int]float64{0: 3}},
		{"a deeper match is not the pointer", openAIChat(), "application/json", []string{`{"data":{"usage":{"prompt_tokens":99}}}`}, map[int]float64{}},
		{"newline-delimited documents, the last counting", openAIChat(), "application/x-ndjson", []string{"{\"usage\":{\"prompt_tokens\":1}}\n{\"usage\":{\"prompt_tokens\":2}}\n"}, map[int]float64{0: 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := meter(c.ai, c.ctype, c.chunks...)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestMeterFirstPointerWinsWithinAnEvent(t *testing.T) {
	a := &AI{Cost: []CostPart{{At: Pointers{"/a", "/b"}, USDEach: 1}}}
	if got := meter(a, "application/json", `{"b":2,"a":1}`); got[0] != 1 {
		t.Fatalf("the part's first pointer counts within one event: %v", got)
	}
}

func TestMeterBounds(t *testing.T) {
	long := "data: {\"pad\":\"" + strings.Repeat("x", MaxEventLine) + "\",\"usage\":{\"prompt_tokens\":5}}\n\ndata: {\"usage\":{\"completion_tokens\":6}}\n\n"
	got := meter(openAIChat(), "text/event-stream", long)
	if _, ok := got[0]; ok || got[1] != 6 {
		t.Fatalf("an overlong line is skipped and the next read: %v", got)
	}
	// A huge answer is read without holding it whole; the usage after it still counts.
	m := NewMeter(*openAIChat(), "application/json")
	_, _ = m.Write([]byte(`{"data":[`))
	chunk := []byte(strings.Repeat(`[0.1,0.2,0.3,0.4],`, 1000))
	for i := 0; i < 200; i++ {
		_, _ = m.Write(chunk)
	}
	_, _ = m.Write([]byte(`[0]],"usage":{"prompt_tokens":4096}}`))
	if got := m.Close(); got[0] != 4096 {
		t.Fatalf("a large answer's usage: %v", got)
	}
	if got := meter(openAIChat(), "application/json"); len(got) != 0 {
		t.Fatalf("an empty answer finds nothing: %v", got)
	}
}

func jsonUnmarshal(src string, v any) error { return json.Unmarshal([]byte(src), v) }
