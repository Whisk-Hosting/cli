import { test } from "node:test"
import assert from "node:assert/strict"
import { addressFrom, fromPrice, money, picture, robotsTxt, safeReturn, sitemap, variantFor } from "../storefront/src/lib/format"
import { mergedCookies, query } from "../storefront/src/lib/medusa"
import { passwordProblem, registerProblem } from "../storefront/src/lib/account"

test("money and pictures", () => {
  assert.equal(money(45, "nzd"), "$45.00")
  assert.equal(money(null, "nzd"), "")
  assert.deepEqual(picture("https://k.whisk.page/.whisk/img/01J?w=1", 300), {
    src: "https://k.whisk.page/.whisk/img/01J?w=300&format=auto",
    srcset: "https://k.whisk.page/.whisk/img/01J?w=300&format=auto 1x, https://k.whisk.page/.whisk/img/01J?w=600&format=auto 2x",
  })
  assert.deepEqual(picture("https://elsewhere/a.png", 300), { src: "https://elsewhere/a.png" })
  assert.equal(picture(null, 300), null)
})

test("fromPrice is the lowest price, and says when it is a sale or varies", () => {
  const p = (calculated: number, original = calculated) => ({ calculated_price: { calculated_amount: calculated, original_amount: original, currency_code: "nzd" } })
  assert.equal(fromPrice([]), null)
  assert.deepEqual(fromPrice([p(45)]), { amount: 45, original: 45, currency: "nzd", sale: false, varies: false })
  assert.deepEqual(fromPrice([p(50), p(40, 45)]), { amount: 40, original: 45, currency: "nzd", sale: true, varies: true })
})

test("variantFor matches every option chosen", () => {
  const vs = [
    { id: "a", options: [{ option_id: "colour", value: "Red" }, { option_id: "size", value: "S" }] },
    { id: "b", options: [{ option_id: "colour", value: "Red" }, { option_id: "size", value: "M" }] },
  ]
  assert.equal(variantFor(vs, { colour: "Red", size: "M" })?.id, "b")
  assert.equal(variantFor(vs, { colour: "Blue" }), null)
  assert.equal(variantFor(vs, {})?.id, "a")
})

test("addressFrom reads a form or names what is missing", () => {
  const form = new FormData()
  for (const [k, v] of Object.entries({ first_name: " Kiri ", last_name: "Tane", address_1: "1 Queen Street", city: "Auckland", postal_code: "1010" })) form.set(k, v)
  assert.deepEqual(addressFrom(form), { address: { first_name: "Kiri", last_name: "Tane", company: undefined, address_1: "1 Queen Street", address_2: undefined, city: "Auckland", postal_code: "1010", province: undefined, country_code: "nz", phone: undefined } })
  form.delete("city")
  assert.deepEqual(addressFrom(form), { missing: ["city"] })
})

test("forms send people back only to this shop", () => {
  const cases: [string | null, string][] = [["/checkout", "/checkout"], ["/account?x=1", "/account?x=1"], ["//evil.com", "/"], ["https://evil.com", "/"], ["", "/"], [null, "/"]]
  for (const [to, want] of cases) assert.equal(safeReturn(to), want, String(to))
})

test("cookies the API sets in a request are laid over the browser's", () => {
  assert.equal(mergedCookies("a=1; shop_cart=c1", ["shop_session=s2; Path=/; HttpOnly"]), "a=1; shop_cart=c1; shop_session=s2")
  assert.equal(mergedCookies("shop_session=old; a=1", ["shop_session=new; Path=/"]), "shop_session=new; a=1")
  assert.equal(mergedCookies("shop_session=old; a=1", ["shop_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT"]), "a=1")
  assert.equal(mergedCookies("", []), "")
})

test("query leaves out empty values and repeats lists", () => {
  assert.equal(query({ a: 1, b: "", c: undefined, d: null, id: ["x", "y"] }), "?a=1&id=x&id=y")
  assert.equal(query({}), "")
})

test("passwords and new accounts", () => {
  assert.match(passwordProblem("short") ?? "", /10 characters/)
  assert.equal(passwordProblem("long enough pass"), null)
  assert.equal(registerProblem({ email: "kiri@example.nz", password: "long enough pass", first_name: "Kiri", last_name: "Tane" }), null)
  assert.match(registerProblem({ email: "nope", password: "long enough pass", first_name: "K", last_name: "T" }) ?? "", /email/)
  assert.match(registerProblem({ email: "k@e.nz", password: "long enough pass", first_name: "", last_name: "T" }) ?? "", /name/)
})

test("the sitemap lists each page with its date, escaped", () => {
  const xml = sitemap("https://k.whisk.page", [{ path: "/" }, { path: "/products/a&b", updated: "2026-10-09T00:00:00Z" }])
  assert.match(xml, /<loc>https:\/\/k\.whisk\.page\/<\/loc>/)
  assert.match(xml, /<loc>https:\/\/k\.whisk\.page\/products\/a&amp;b<\/loc><lastmod>2026-10-09T00:00:00.000Z<\/lastmod>/)
})

test("robots.txt keeps the website's lines first, then the shop's", () => {
  const shop = "User-agent: *\nDisallow: /cart\nDisallow: /checkout\nDisallow: /account\nDisallow: /order/\nDisallow: /app\n\nSitemap: https://k.nz/sitemap.xml\n"
  assert.equal(robotsTxt("https://k.nz", undefined), shop)
  assert.equal(robotsTxt("https://k.nz", "  \n"), shop)
  assert.equal(robotsTxt("https://k.nz", "User-agent: *\nDisallow: /wp-admin/\n\n"), "User-agent: *\nDisallow: /wp-admin/\n\n# The shop\n" + shop)
})
