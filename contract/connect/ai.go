package connect

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// AI makes a connection an AI model's (CONTRACT.md §3.1 "AI models"): the spend cap a person
// grants with it, where each answer says what it cost, and the operations that cost nothing.
type AI struct {
	SpendCap *SpendCap  `json:"spend_cap"`
	Cost     []CostPart `json:"cost"`
	// Free names operations whose success answers never cost anything (listing models), so an
	// answer without a cost is not unreadable.
	Free []string `json:"free,omitempty"`
}

// SpendCap is what the app may spend on the model, in US dollars, in each window it names.
type SpendCap struct {
	PerDay   float64 `json:"per_day,omitempty"`
	PerWeek  float64 `json:"per_week,omitempty"`
	PerMonth float64 `json:"per_month,omitempty"`
	Total    float64 `json:"total,omitempty"`
}

// CostPart is one number in an answer and its price: tokens priced per million, or a number
// priced each (a cost already in dollars is usd_each: 1).
type CostPart struct {
	At            Pointers `json:"at"`
	USDPerMillion float64  `json:"usd_per_million,omitempty"`
	USDEach       float64  `json:"usd_each,omitempty"`
}

// Pointers is one JSON pointer or a list of them; written as a list.
type Pointers []string

// UnmarshalJSON takes a single pointer or a list.
func (p *Pointers) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*p = Pointers{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return errors.New("a JSON pointer or a list of them")
	}
	*p = many
	return nil
}

// Spend windows, as the spend cap and the control plane name them.
const (
	PeriodDay   = "day"
	PeriodWeek  = "week"
	PeriodMonth = "month"
	PeriodTotal = "total"
)

// Periods are the windows in the order they are checked and shown.
var Periods = []string{PeriodDay, PeriodWeek, PeriodMonth, PeriodTotal}

// Bounds of an AI connection (CONTRACT.md §3.1 "Rules for AI connections").
const (
	MinCap          = 0.01
	MaxCap          = 1000000
	MaxCostParts    = 8
	MaxCostPointers = 4
	MaxPrice        = 1000000
)

// Caps is the cap in dollars for each window it names, by period. Pure.
func (s SpendCap) Caps() map[string]float64 {
	out := map[string]float64{}
	for p, v := range map[string]float64{PeriodDay: s.PerDay, PeriodWeek: s.PerWeek, PeriodMonth: s.PerMonth, PeriodTotal: s.Total} {
		if v != 0 {
			out[p] = v
		}
	}
	return out
}

// Micros is a dollar amount in millionths of a dollar, rounded to the nearest.
func Micros(usd float64) int64 { return int64(math.Round(usd * 1e6)) }

// USD is millionths of a dollar as dollars.
func USD(micros int64) float64 { return float64(micros) / 1e6 }

// IsAI reports whether a connection is an AI model's.
func IsAI(c Connection) bool { return c.AI != nil }

// checkAI adds the problems with a connection's ai block, and requires one for a known AI
// provider's address.
func checkAI(c Connection, add func(string, string, ...any)) {
	a := c.AI
	if a == nil {
		if host := hostOf(c.URL); AIHost(host) {
			add("/ai", "%s is an AI model's API, so this is an AI connection: add ai with its spend_cap and cost (CONTRACT.md §3.1 AI models)", host)
		}
		return
	}
	if a.SpendCap == nil || len(a.SpendCap.Caps()) == 0 {
		add("/ai/spend_cap", "required: name per_day, per_week, per_month or total in US dollars. Ask your person how much this app may spend on the model and walk them through it: what one use costs, how many uses a day you expect, and that calls stop until the window ends when the cap is reached. Never choose it yourself")
	} else {
		for _, p := range Periods {
			v, ok := a.SpendCap.Caps()[p]
			key := map[string]string{PeriodDay: "per_day", PeriodWeek: "per_week", PeriodMonth: "per_month", PeriodTotal: "total"}[p]
			if ok && (v < MinCap || v > MaxCap || math.IsNaN(v)) {
				add("/ai/spend_cap/"+key, "between %.2f and %.0f US dollars", MinCap, float64(MaxCap))
			}
		}
	}
	if len(a.Cost) == 0 {
		add("/ai/cost", "name where each answer says what it cost: token counts with usd_per_million, or a cost with usd_each (CONTRACT.md §3.1 AI models lists each provider's)")
	}
	if len(a.Cost) > MaxCostParts {
		add("/ai/cost", "at most %d parts", MaxCostParts)
	}
	for i, part := range a.Cost {
		p := fmt.Sprintf("/ai/cost/%d", i)
		if len(part.At) == 0 || len(part.At) > MaxCostPointers {
			add(p+"/at", "1 to %d JSON pointers into the answer, such as /usage/prompt_tokens", MaxCostPointers)
		}
		for j, ptr := range part.At {
			if _, err := pointer(ptr); err != nil {
				add(fmt.Sprintf("%s/at/%d", p, j), "%s", err.Error())
			}
		}
		priced := 0
		for _, v := range []float64{part.USDPerMillion, part.USDEach} {
			if v != 0 {
				priced++
			}
			if v < 0 || v > MaxPrice || math.IsNaN(v) {
				add(p, "a price is above 0 and at most %d US dollars", MaxPrice)
			}
		}
		if priced != 1 {
			add(p, "exactly one price: usd_per_million for tokens, or usd_each")
		}
	}
	names := map[string]bool{}
	for _, o := range c.Operations {
		names[strings.ToLower(o.Name)] = true
	}
	for i, f := range a.Free {
		if !names[strings.ToLower(f)] {
			add(fmt.Sprintf("/ai/free/%d", i), "%q is not one of the connection's operations", f)
		}
	}
}

// aiHosts are the AI model providers' API hosts a connection must carry ai to reach.
var aiHosts = map[string]bool{
	"api.openai.com": true, "api.anthropic.com": true, "generativelanguage.googleapis.com": true,
	"aiplatform.googleapis.com": true, "openrouter.ai": true, "api.mistral.ai": true,
	"api.groq.com": true, "api.x.ai": true, "api.deepseek.com": true, "api.together.xyz": true,
	"api.fireworks.ai": true, "api.perplexity.ai": true, "api.cohere.com": true, "api.cohere.ai": true,
}

// AIHost reports whether host is a known AI model provider's API. Pure.
func AIHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return aiHosts[host] || strings.HasSuffix(host, ".openai.azure.com") || strings.HasSuffix(host, "-aiplatform.googleapis.com")
}

// IsFree reports whether an operation's answers never cost anything.
func (a AI) IsFree(operation string) bool {
	for _, f := range a.Free {
		if strings.EqualFold(f, operation) {
			return true
		}
	}
	return false
}

// Price is an answer's cost in millionths of a dollar from the number found for each part (by
// index), rounded up; found is false when no part was. Pure.
func (a AI) Price(values map[int]float64) (micros int64, found bool) {
	total := 0.0
	for i, part := range a.Cost {
		v, ok := values[i]
		if !ok {
			continue
		}
		found = true
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		// Tokens times dollars per million tokens is millionths of a dollar.
		total += v*part.USDPerMillion + v*part.USDEach*1e6
	}
	if total > math.MaxInt64/2 {
		return math.MaxInt64 / 2, found
	}
	return int64(math.Ceil(total - 1e-9)), found
}

// AISummary is the grant screen's account of an AI connection: its caps and each price, in
// the order the parts are listed.
type AISummary struct {
	Caps  map[string]float64 `json:"caps"`
	Parts []AIPart           `json:"parts"`
	Free  []string           `json:"free"`
}

// AIPart is one priced number.
type AIPart struct {
	At            []string `json:"at"`
	USDPerMillion float64  `json:"usd_per_million,omitempty"`
	USDEach       float64  `json:"usd_each,omitempty"`
}

func describeAI(a *AI) *AISummary {
	if a == nil {
		return nil
	}
	s := &AISummary{Caps: map[string]float64{}, Parts: []AIPart{}, Free: append([]string{}, a.Free...)}
	if a.SpendCap != nil {
		s.Caps = a.SpendCap.Caps()
	}
	for _, p := range a.Cost {
		s.Parts = append(s.Parts, AIPart{At: append([]string{}, p.At...), USDPerMillion: p.USDPerMillion, USDEach: p.USDEach})
	}
	return s
}

// meterTarget maps an unescaped JSON pointer path to a part and the pointer's place in it.
type meterTarget struct{ part, rank int }

func targets(a AI) map[string][]meterTarget {
	out := map[string][]meterTarget{}
	for i, part := range a.Cost {
		for r, p := range part.At {
			toks, err := pointer(p)
			if err != nil {
				continue
			}
			k := strings.Join(toks, "\x00")
			out[k] = append(out[k], meterTarget{i, r})
		}
	}
	return out
}

// numberOf reads a JSON number, or a string holding one.
func numberOf(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	}
	return 0, false
}

// Unreadable reports whether an AI connection's answer that named no cost part stops the
// deploy's calls (CONTRACT.md §3.1 "Rules for AI connections"): a success, to a call other than
// GET or HEAD, of an operation not listed as free, that ended rather than the app stopping
// reading it. Pure.
func (a AI) Unreadable(operation, method string, status int, found, complete bool) bool {
	return !found && complete && status/100 == 2 && method != "GET" && method != "HEAD" && !a.IsFree(operation)
}
