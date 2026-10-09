package whisk

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
)

func billingFrom(t *testing.T, raw string) api.Billing {
	t.Helper()
	var b api.Billing
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUsageRows(t *testing.T) {
	b := billingFrom(t, `{"usage":{"members":3,"apps":5,"storage_bytes":1610612736,"runs":3200,"emails_today":12},
		"limits":{"members":10,"apps":20,"storage_bytes":10737418240,"runs_per_month":10000,"emails_per_day":0}}`)
	rows := usageRows(b)
	want := [][]string{
		{"members", "3", "10"}, {"apps", "5", "20"}, {"storage", "1.5 GB", "10.0 GB"},
		{"runs this month", "3200", "10000"}, {"emails today", "12", "none"},
	}
	for i := range want {
		if strings.Join(rows[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d = %v, want %v", i, rows[i], want[i])
		}
	}
}

func TestBillingNotesSayWhatComesNext(t *testing.T) {
	pastDue := billingFrom(t, `{"payments":true,"overage_cents":1550,"timeline":{"status":"active","past_due_at":"2026-09-01T00:00:00Z","stops_at":"2026-09-08T00:00:00Z"},
		"next_invoice":{"amount_cents":9900,"currency":"usd","at":"2026-10-01T00:00:00Z"}}`)
	notes := strings.Join(billingNotes(pastDue), "\n")
	for _, want := range []string{"Overage so far this month: USD 15.50.", "Next invoice: USD 99.00 on 1 Oct 2026.", "before 8 Sep 2026"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes do not say %q:\n%s", want, notes)
		}
	}
	shredding := billingFrom(t, `{"payments":true,"timeline":{"status":"shredding","shredded_at":"2026-11-10T00:00:00Z"}}`)
	if notes := strings.Join(billingNotes(shredding), "\n"); !strings.Contains(notes, "whisk export --wait before then") {
		t.Errorf("a shredding org is not pointed at the export:\n%s", notes)
	}
	unconfigured := billingFrom(t, `{"payments":false,"timeline":{"status":"active"}}`)
	if notes := strings.Join(billingNotes(unconfigured), "\n"); !strings.Contains(notes, "Payments are not configured") {
		t.Errorf("notes %q", notes)
	}
}

func TestBillingNotesSayWhenTheTrialEnds(t *testing.T) {
	running := billingFrom(t, `{"payments":true,"timeline":{"status":"active"},"trial":{"plan":"starter","days":30,"active":true,"ends_at":"2026-11-01T00:00:00Z"}}`)
	if notes := strings.Join(billingNotes(running), "\n"); !strings.Contains(notes, "ends on 1 Nov 2026") || !strings.Contains(notes, "nothing is charged") {
		t.Errorf("a running trial: %s", notes)
	}
	offered := billingFrom(t, `{"payments":true,"timeline":{"status":"active"},"trial":{"plan":"starter","days":30,"available":true}}`)
	if notes := strings.Join(billingNotes(offered), "\n"); !strings.Contains(notes, "try Starter free for 30 days") {
		t.Errorf("an offered trial: %s", notes)
	}
	used := billingFrom(t, `{"payments":true,"timeline":{"status":"active"},"trial":{"plan":"starter","days":30}}`)
	if notes := strings.Join(billingNotes(used), "\n"); strings.Contains(notes, "trial") {
		t.Errorf("a used trial is still mentioned: %s", notes)
	}
}

func TestPlanLine(t *testing.T) {
	team := billingFrom(t, `{"plan":{"name":"Team","monthly_usd":99,"annual_usd":990},"interval":"year","period":"2026-09"}`)
	if got := planLine(team); got != "Plan: Team, $990 a year. Usage for 2026-09:" {
		t.Errorf("planLine = %q", got)
	}
	free := billingFrom(t, `{"plan":{"name":"Free"},"period":"2026-09"}`)
	if got := planLine(free); got != "Plan: Free, one app free forever. Usage for 2026-09:" {
		t.Errorf("planLine = %q", got)
	}
	client := billingFrom(t, `{"plan":{"name":"Agency","client_monthly_usd":59,"client_annual_usd":590},"client_of":{"slug":"studio","name":"Studio"},"period":"2026-09"}`)
	if got := planLine(client); got != "Plan: Agency, $59 a month, paid for by Studio. Usage for 2026-09:" {
		t.Errorf("planLine = %q", got)
	}
	unpaid := billingFrom(t, `{"plan":{"name":"Free"},"client_of":{"slug":"studio","name":"Studio"},"period":"2026-09"}`)
	if got := planLine(unpaid); got != "Plan: Free, a client business Studio has not paid for yet. Usage for 2026-09:" {
		t.Errorf("planLine = %q", got)
	}
}
