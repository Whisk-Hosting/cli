package main

import (
	"fmt"
	"math/rand"
	"testing"
)

// Property tests for who may see and change a record (skill §4). Rather than a few hand-picked
// cases, each rule is checked against thousands of generated people and records, including
// empty and hostile values, so a change to the rules that lets one person reach another's
// records fails here before it ships. The generator is seeded, so a failure repeats exactly.

var (
	accessAudiences = []string{"anonymous", "team", "customer", "service", "", "Team", "customer ", "admin"}
	accessPeople    = []string{"", "01J8ALICE", "01J8BOB", "01J8CAROL", " ", "null", "nil", "*", "%", "' OR 1=1 --", "01J8ALICE\x00"}
	accessRoles     = []string{"owner", "admin", "developer", "billing", "member", "guest", "Admin", "owner ", ""}
)

const accessN = 20000

type accessCase struct {
	id    Identity
	owner string
}

func (c accessCase) String() string { return fmt.Sprintf("%+v owner=%q", c.id, c.owner) }

func accessCases(seed int64) []accessCase {
	rnd := rand.New(rand.NewSource(seed))
	pick := func(xs []string) string { return xs[rnd.Intn(len(xs))] }
	out := make([]accessCase, accessN)
	for i := range out {
		var roles []string
		for _, r := range accessRoles {
			if rnd.Float64() < 0.25 {
				roles = append(roles, r)
			}
		}
		out[i] = accessCase{Identity{Audience: pick(accessAudiences), UserID: pick(accessPeople), Roles: roles}, pick(accessPeople)}
	}
	return out
}

func TestAccessNobodyWithoutASignedInPersonSeesAnything(t *testing.T) {
	for _, c := range accessCases(1) {
		if c.id.UserID != "" && (c.id.Audience == "team" || c.id.Audience == "customer") {
			continue
		}
		if c.id.CanSee(c.owner) || c.id.CanChange(c.owner) {
			t.Fatalf("reached a record: %s", c)
		}
	}
}

func TestAccessACustomerSeesAndChangesExactlyTheirOwn(t *testing.T) {
	for _, c := range accessCases(2) {
		if c.id.Audience != "customer" {
			continue
		}
		own := c.id.UserID != "" && c.owner != "" && c.owner == c.id.UserID
		if c.id.CanSee(c.owner) != own || c.id.CanChange(c.owner) != own {
			t.Fatalf("own=%v but CanSee=%v CanChange=%v: %s", own, c.id.CanSee(c.owner), c.id.CanChange(c.owner), c)
		}
	}
}

func TestAccessOneCustomerNeverReachesAnothersRecord(t *testing.T) {
	rnd := rand.New(rand.NewSource(3))
	for i := 0; i < accessN; i++ {
		a, b := accessPeople[rnd.Intn(len(accessPeople))], accessPeople[rnd.Intn(len(accessPeople))]
		if a == b {
			continue
		}
		other := Identity{Audience: "customer", UserID: b}
		if other.CanSee(a) || other.CanChange(a) {
			t.Fatalf("%q reached %q's record", b, a)
		}
	}
}

func TestAccessOnTheTeamOnlyTheOwnerOrAnAdminChanges(t *testing.T) {
	for _, c := range accessCases(4) {
		if c.id.Audience != "team" || c.id.UserID == "" {
			continue
		}
		want := c.owner == c.id.UserID || c.id.HasRole("owner", "admin")
		if !c.id.CanSee(c.owner) || c.id.CanChange(c.owner) != want {
			t.Fatalf("want CanChange=%v: %s", want, c)
		}
	}
}

func TestAccessWhoeverMayChangeMaySee(t *testing.T) {
	for _, c := range accessCases(5) {
		if c.id.CanChange(c.owner) && !c.id.CanSee(c.owner) {
			t.Fatalf("changes but cannot see: %s", c)
		}
	}
}

func TestAccessTheDatabaseFilterAgreesWithCanSee(t *testing.T) {
	for _, c := range accessCases(6) {
		s := ScopeFor(c.id)
		if s.Includes(c.owner) != c.id.CanSee(c.owner) {
			t.Fatalf("scope %+v disagrees with CanSee: %s", s, c)
		}
		if s.Kind == "owner" && s.OwnerID != c.id.UserID {
			t.Fatalf("scope names %q, not the person: %s", s.OwnerID, c)
		}
	}
}
