package whisk

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/contract/status"
)

// billingCmd shows the org's plan, this month's usage and the next invoice (CLI.md §5.9).
func billingCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "billing",
		Short: "The org's plan, this month's usage against it, and the next invoice",
		Long: `Shows the org's plan, what it has used this month against each allowance, the overage so far,
the next invoice, and whether a payment has failed and when the apps would stop. Changing the
plan is done from the dashboard's billing page, because a card may be needed. Owners and
billing contacts.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			b, err := client.GetBilling(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(billingResult(org, b), func(w io.Writer) {
				fmt.Fprintln(w, planLine(b))
				s.printer.Table(w, []string{"ALLOWANCE", "USED", "INCLUDED"}, usageRows(b))
				for _, line := range billingNotes(b) {
					fmt.Fprintln(w, line)
				}
			})
			return nil
		},
	}
}

// billingResult is the billing as --json prints it: the API's own shape, with the org named.
func billingResult(org string, b api.Billing) map[string]any {
	out := map[string]any{}
	raw, _ := json.Marshal(b)
	_ = json.Unmarshal(raw, &out)
	out["org"] = org
	return out
}

func planLine(b api.Billing) string {
	price := b.Plan.MonthlyUSD
	per := "month"
	if b.Interval == "year" {
		price, per = b.Plan.AnnualUSD, "year"
	}
	if b.ClientOf != nil && b.Plan.ClientMonthlyUSD == 0 {
		return fmt.Sprintf("Plan: %s, a client business %s has not paid for yet. Usage for %s:", b.Plan.Name, b.ClientOf.Name, b.Period)
	}
	if b.ClientOf != nil {
		// A client business is paid for by its agency at the plan's client price.
		price = b.Plan.ClientMonthlyUSD
		if per == "year" {
			price = b.Plan.ClientAnnualUSD
		}
		return fmt.Sprintf("Plan: %s, $%d a %s, paid for by %s. Usage for %s:", b.Plan.Name, price, per, b.ClientOf.Name, b.Period)
	}
	if price == 0 {
		return fmt.Sprintf("Plan: %s, one app free forever. Usage for %s:", b.Plan.Name, b.Period)
	}
	return fmt.Sprintf("Plan: %s, $%d a %s. Usage for %s:", b.Plan.Name, price, per, b.Period)
}

// usageRows is each allowance with what the org has used of it.
func usageRows(b api.Billing) [][]string {
	rows := [][]string{}
	for _, r := range []struct{ label, usage, limit string }{
		{"members", "members", "members"},
		{"apps", "apps", "apps"},
		{"storage", "storage_bytes", "storage_bytes"},
		{"runs this month", "runs", "runs_per_month"},
		{"emails today", "emails_today", "emails_per_day"},
	} {
		used, limit := b.Usage[r.usage], b.Limits[r.limit]
		format := func(n int64) string { return fmt.Sprintf("%d", n) }
		if r.usage == "storage_bytes" {
			format = billingSize
		}
		included := format(limit)
		if limit <= 0 {
			included = "none"
		}
		rows = append(rows, []string{r.label, format(used), included})
	}
	return rows
}

// billingNotes is what else a person needs to know: overage, the next invoice, the timeline and
// the trial.
func billingNotes(b api.Billing) []string {
	var out []string
	if b.Canary {
		out = append(out, "This is the platform's canary org, which is never billed.")
	}
	if b.OverageCents > 0 {
		out = append(out, fmt.Sprintf("Overage so far this month: %s.", money(b.OverageCents, "usd")))
	}
	if b.NextInvoice != nil && b.NextInvoice.At != nil {
		out = append(out, fmt.Sprintf("Next invoice: %s on %s.", money(b.NextInvoice.AmountCents, b.NextInvoice.Currency), b.NextInvoice.At.Format("2 Jan 2006")))
	}
	t := b.Timeline
	switch {
	case t.StopsAt != nil:
		out = append(out, fmt.Sprintf("A payment failed. Update the card from the dashboard before %s, or the apps stop then.", t.StopsAt.Format("2 Jan 2006")))
	case t.Status == status.OrgFrozen && t.FrozenReason == "payment" && t.ShreddingAt != nil:
		out = append(out, fmt.Sprintf("The apps are paused for an unpaid invoice. Paying it brings them back; the data is kept until %s.", t.ShreddingAt.Format("2 Jan 2006")))
	case t.Status == status.OrgShredding && t.ShreddedAt != nil:
		out = append(out, fmt.Sprintf("The org is being deleted on %s. Run whisk export --wait before then to keep a copy.", t.ShreddedAt.Format("2 Jan 2006")))
	}
	switch tr := b.Trial; {
	case tr.Active && tr.EndsAt != nil:
		out = append(out, fmt.Sprintf("Free trial of Starter: it ends on %s, and Stripe then charges the card. End it before then from the dashboard's billing page and nothing is charged.", tr.EndsAt.Format("2 Jan 2006")))
	case tr.Available:
		out = append(out, fmt.Sprintf("This business can try Starter free for %d days, once. Start it from the dashboard's billing page; a card is needed.", tr.Days))
	}
	if !b.Payments {
		out = append(out, "Payments are not configured on this platform; usage is still measured.")
	}
	return out
}

func money(cents int64, currency string) string {
	if currency == "" {
		currency = "usd"
	}
	return fmt.Sprintf("%s %.2f", strings.ToUpper(currency), float64(cents)/100)
}

func billingSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}
