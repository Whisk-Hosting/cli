package keyshapes

import "testing"

func TestTextMasksLiveCredentialsAndLeavesTheRest(t *testing.T) {
	// The keys are written in two parts, so this file holds nothing a secret scanner reads as one.
	testKey := "test key sk_test_" + "51HxQ2eZvKYlo2C0aBcDeFgHiJ stays"
	publishable := "publishable pk_live_" + "51HxQ2eZvKYlo2C0aBcDeFgHiJ stays"
	cases := []struct{ in, want string }{
		{"charge with sk_live_" + "51HxQ2eZvKYlo2C0aBcDeFgHiJ ok", "charge with [REDACTED:stripe_key] ok"},
		{"restricted rk_live_" + "abcdefghijklmnopqrstu", "restricted [REDACTED:stripe_key]"},
		{testKey, testKey},
		{publishable, publishable},
		{"secret=whsec_" + "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", "secret=[REDACTED:stripe_webhook_secret]"},
		{"token ghp_" + "aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789", "token [REDACTED:github_token]"},
		{"pat github_pat_" + "11ABCDEFG0123456789_abcdefghijklmnopqrstuvwxyz", "pat [REDACTED:github_token]"},
		{"aws AKIA" + "IOSFODNN7EXAMPLE used", "aws [REDACTED:aws_access_key] used"},
		{"slack xoxb-" + "1234567890-abcdefghij", "slack [REDACTED:slack_token]"},
		{"Bearer sk-ant-" + "api03-abcdefghijklmnopqrstuvwxyz", "Bearer [REDACTED:anthropic_key]"},
		{"key sk-proj-" + "abcdefghijklmnopqrstuvwxyz0123456789", "key [REDACTED:openai_key]"},
		{"maps AIza" + "SyD-abcdefghijklmnopqrstuvwxyz01234", "maps [REDACTED:google_api_key]"},
		{"mail SG." + "abcdefghijklmnop.qrstuvwxyz012345678", "mail [REDACTED:sendgrid_key]"},
		{"order sk-12 shipped to the desk", "order sk-12 shipped to the desk"},
		{"nothing secret here", "nothing secret here"},
	}
	for _, tc := range cases {
		if got := Text(tc.in); got != tc.want {
			t.Errorf("Text(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEveryShapeHasANameAndCompiles(t *testing.T) {
	seen := map[string]bool{}
	for i, s := range Shapes {
		if s.Name == "" || s.Pattern == "" || compiled[i] == nil {
			t.Errorf("shape %d is incomplete: %+v", i, s)
		}
		seen[s.Name] = true
	}
	if len(seen) != len(Shapes) {
		t.Error("two shapes share a name")
	}
}
