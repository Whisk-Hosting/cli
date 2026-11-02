import assert from "node:assert/strict";
import { test } from "node:test";
import { canChange, canSee, hasRole, inScope, scopeFor, type Identity } from "../src/whisk.js";

// Property tests for who may see and change a record (skill §4). Rather than a few hand-picked
// cases, each rule is checked against thousands of generated people and records, including
// empty and hostile values, so a change to the rules that lets one person reach another's
// records fails here before it ships. The generator is seeded, so a failure repeats exactly.

const seeded = (seed: number) => () => {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
};

const AUDIENCES = ["anonymous", "team", "customer", "service", "", "Team", "customer ", "admin"];
const PEOPLE = [undefined, "", "01J8ALICE", "01J8BOB", "01J8CAROL", " ", "null", "undefined", "*", "%", "' OR 1=1 --", "01J8ALICE\u0000"];
const ROLES = ["owner", "admin", "developer", "billing", "member", "guest", "Admin", "owner ", ""];

const pick = <T>(rand: () => number, xs: readonly T[]) => xs[Math.floor(rand() * xs.length)];

const person = (rand: () => number): Identity => ({
  audience: pick(rand, AUDIENCES) as Identity["audience"],
  userId: pick(rand, PEOPLE),
  groups: [],
  roles: ROLES.filter(() => rand() < 0.25),
  requestId: "",
});

const cases = (n: number, seed = 1) => {
  const rand = seeded(seed);
  return Array.from({ length: n }, () => ({ id: person(rand), ownerId: pick(rand, PEOPLE) ?? "" }));
};

const N = 20000;

test("nobody without a signed-in person sees or changes anything", () => {
  for (const { id, ownerId } of cases(N)) {
    if (id.userId && (id.audience === "team" || id.audience === "customer")) continue;
    assert.equal(canSee(id, ownerId), false, JSON.stringify({ id, ownerId }));
    assert.equal(canChange(id, ownerId), false, JSON.stringify({ id, ownerId }));
  }
});

test("a customer sees and changes exactly their own records", () => {
  for (const { id, ownerId } of cases(N, 2)) {
    if (id.audience !== "customer") continue;
    const own = !!id.userId && ownerId !== "" && ownerId === id.userId;
    assert.equal(canSee(id, ownerId), own, JSON.stringify({ id, ownerId }));
    assert.equal(canChange(id, ownerId), own, JSON.stringify({ id, ownerId }));
  }
});

test("one customer never reaches another customer's record", () => {
  const rand = seeded(3);
  for (let i = 0; i < N; i++) {
    const a = pick(rand, PEOPLE) ?? "";
    const b = pick(rand, PEOPLE) ?? "";
    if (a === b) continue;
    const other: Identity = { audience: "customer", userId: b, groups: [], roles: [], requestId: "" };
    assert.equal(canSee(other, a), false, `${b} saw ${a}'s record`);
    assert.equal(canChange(other, a), false, `${b} changed ${a}'s record`);
  }
});

test("on the team, only the record's owner or an owner or admin changes it", () => {
  for (const { id, ownerId } of cases(N, 4)) {
    if (id.audience !== "team" || !id.userId) continue;
    assert.equal(canSee(id, ownerId), true);
    assert.equal(canChange(id, ownerId), ownerId === id.userId || hasRole(id, "owner", "admin"), JSON.stringify({ id, ownerId }));
  }
});

test("whoever may change a record may also see it", () => {
  for (const { id, ownerId } of cases(N, 5)) {
    if (canChange(id, ownerId)) assert.equal(canSee(id, ownerId), true, JSON.stringify({ id, ownerId }));
  }
});

test("the database filter from scopeFor agrees with canSee for every record", () => {
  for (const { id, ownerId } of cases(N, 6)) {
    const scope = scopeFor(id);
    assert.equal(inScope(scope, ownerId), canSee(id, ownerId), JSON.stringify({ id, ownerId, scope }));
    if (scope.kind === "owner") assert.equal(scope.ownerId, id.userId);
  }
});
