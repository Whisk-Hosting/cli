"""Property tests for who may see and change a record (skill §4). Rather than a few hand-picked
cases, each rule is checked against thousands of generated people and records, including empty
and hostile values, so a change to the rules that lets one person reach another's records fails
here before it ships. The generator is seeded, so a failure repeats exactly."""

import random

from app.whisk import Identity, can_change, can_see, in_scope, scope_for

AUDIENCES = ["anonymous", "team", "customer", "service", "", "Team", "customer ", "admin"]
PEOPLE = [None, "", "01J8ALICE", "01J8BOB", "01J8CAROL", " ", "null", "None", "*", "%", "' OR 1=1 --", "01J8ALICE\x00"]
ROLES = ["owner", "admin", "developer", "billing", "member", "guest", "Admin", "owner ", ""]
N = 20000


def person(rand: random.Random) -> Identity:
    return Identity(audience=rand.choice(AUDIENCES), user_id=rand.choice(PEOPLE), roles=tuple(r for r in ROLES if rand.random() < 0.25))


def cases(seed: int) -> list[tuple[Identity, str]]:
    rand = random.Random(seed)
    return [(person(rand), rand.choice(PEOPLE) or "") for _ in range(N)]


def test_nobody_without_a_signed_in_person_sees_or_changes_anything() -> None:
    for who, owner in cases(1):
        if who.user_id and who.audience in ("team", "customer"):
            continue
        assert not can_see(who, owner), (who, owner)
        assert not can_change(who, owner), (who, owner)


def test_a_customer_sees_and_changes_exactly_their_own_records() -> None:
    for who, owner in cases(2):
        if who.audience != "customer":
            continue
        own = bool(who.user_id) and owner != "" and owner == who.user_id
        assert can_see(who, owner) is own, (who, owner)
        assert can_change(who, owner) is own, (who, owner)


def test_one_customer_never_reaches_another_customers_record() -> None:
    rand = random.Random(3)
    for _ in range(N):
        a, b = rand.choice(PEOPLE) or "", rand.choice(PEOPLE) or ""
        if a == b:
            continue
        other = Identity(audience="customer", user_id=b)
        assert not can_see(other, a), f"{b!r} saw {a!r}'s record"
        assert not can_change(other, a), f"{b!r} changed {a!r}'s record"


def test_on_the_team_only_the_records_owner_or_an_owner_or_admin_changes_it() -> None:
    for who, owner in cases(4):
        if who.audience != "team" or not who.user_id:
            continue
        assert can_see(who, owner)
        assert can_change(who, owner) is (owner == who.user_id or who.has_role("owner", "admin")), (who, owner)


def test_whoever_may_change_a_record_may_also_see_it() -> None:
    for who, owner in cases(5):
        if can_change(who, owner):
            assert can_see(who, owner), (who, owner)


def test_the_database_filter_from_scope_for_agrees_with_can_see() -> None:
    for who, owner in cases(6):
        scope = scope_for(who)
        assert in_scope(scope, owner) is can_see(who, owner), (who, owner, scope)
        if scope.kind == "owner":
            assert scope.owner_id == who.user_id
