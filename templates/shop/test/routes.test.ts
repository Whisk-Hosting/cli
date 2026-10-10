import { test } from "node:test"
import assert from "node:assert/strict"
import { isMedusaPath, limitFor, staticFile, STOREFRONT } from "../src/lib/routes"
import { adminReturn, isStaff, sessionHolds, splitName, personFrom } from "../src/lib/staff"

test("Medusa's paths and the storefront's", () => {
  const cases: [string, boolean][] = [
    ["/store/products", true], ["/admin", true], ["/admin/orders", true], ["/auth/whisk", true], ["/hooks/windcave", true], ["/app", true], ["/app/orders", true], ["/health", true],
    ["/", false], ["/products/beanie", false], ["/storefront", false], ["/application", false], ["/account", false], ["/healthy", false],
  ]
  for (const [p, medusa] of cases) {
    assert.equal(isMedusaPath(p), medusa, p)
    assert.equal(STOREFRONT.test(p), !medusa, p)
  }
})

test("password routes are limited, sign-up and reset more tightly", () => {
  assert.deepEqual(limitFor("/auth/customer/emailpass"), { perEmail: 10, perIp: 60 })
  assert.deepEqual(limitFor("/auth/customer/emailpass/register"), { perEmail: 3, perIp: 20 })
  assert.deepEqual(limitFor("/auth/customer/emailpass/reset-password"), { perEmail: 3, perIp: 20 })
})

test("staticFile serves built files and nothing outside them", () => {
  assert.deepEqual(staticFile("/srv/client", "/_astro/shop.a1b2.css"), { file: "/srv/client/_astro/shop.a1b2.css", type: "text/css; charset=utf-8", cache: "public, max-age=31536000, immutable" })
  assert.deepEqual(staticFile("/srv/client/", "/favicon.svg"), { file: "/srv/client/favicon.svg", type: "image/svg+xml", cache: "public, max-age=3600" })
  const refused = ["/", "/products/beanie", "/../etc/passwd.txt", "/_astro/%2e%2e/x.js", "/.env", "/a/.git/config.json", "/x.php", "/bad%E0%A4%A.js", "/a%00.js"]
  for (const p of refused) assert.equal(staticFile("/srv/client", p), null, p)
})

test("who may run the shop", () => {
  const p = (audience: string, roles: string, id = "u1", email = "a@b.nz") => personFrom({ "x-whisk-audience": audience, "x-whisk-roles": roles, "x-whisk-user-id": id, "x-whisk-email": email })
  assert.equal(isStaff(p("team", "owner")), true)
  assert.equal(isStaff(p("team", "member,billing")), true)
  assert.equal(isStaff(p("team", "member,guest")), true)
  assert.equal(isStaff(p("team", "guest")), false)
  assert.equal(isStaff(p("customer", "")), false)
  assert.equal(isStaff(p("anonymous", "")), false)
  assert.equal(isStaff(p("team", "owner", "")), false)
  assert.equal(isStaff(p("team", "owner", "u1", "")), false)
  assert.equal(sessionHolds("u1", p("team", "owner")), true)
  assert.equal(sessionHolds("u2", p("team", "owner")), false)
  assert.equal(sessionHolds("u1", p("anonymous", "")), false)
  assert.equal(sessionHolds(undefined, p("team", "owner")), false)
})

test("the admin sign-in returns only to the admin", () => {
  const cases: [string, string][] = [
    ["/app", "/app"], ["/app/orders", "/app/orders"], ["/app/orders?q=1", "/app/orders?q=1"], ["/app?x", "/app?x"],
    ["", "/app"], ["/", "/app"], ["/application", "/app"], ["//evil.com", "/app"], ["https://evil.com/app", "/app"], ["/app//evil.com", "/app"], ["/app/\\evil", "/app"],
  ]
  for (const [to, want] of cases) assert.equal(adminReturn(to), want, to)
})

test("names split into first and last", () => {
  assert.deepEqual(splitName("Mere Hohaia"), { first_name: "Mere", last_name: "Hohaia" })
  assert.deepEqual(splitName("  Mary  Ann  Smith "), { first_name: "Mary", last_name: "Ann Smith" })
  assert.deepEqual(splitName(""), { first_name: "", last_name: "" })
})
