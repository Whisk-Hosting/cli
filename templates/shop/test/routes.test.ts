import { test } from "node:test"
import assert from "node:assert/strict"
import { anyFilled, formFields, formName, formReturn, isMedusaPath, isShopPath, limitFor, redirectFor, siteFiles, staticFile, STOREFRONT, variantFor } from "../src/lib/routes"
import { siteAddress } from "../storefront/src/lib/site"
import { adminReturn, isStaff, sessionHolds, splitName, personFrom } from "../src/lib/staff"

test("Medusa's paths and the storefront's", () => {
  const cases: [string, boolean][] = [
    ["/store/products", true], ["/admin", true], ["/admin/orders", true], ["/auth/whisk", true], ["/hooks/windcave", true], ["/app", true], ["/app/orders", true], ["/health", true], ["/erp-link/v1/orders", true],
    ["/", false], ["/products/beanie", false], ["/storefront", false], ["/application", false], ["/account", false], ["/healthy", false], ["/erp-linked", false],
  ]
  for (const [p, medusa] of cases) {
    assert.equal(isMedusaPath(p), medusa, p)
    assert.equal(STOREFRONT.test(p), !medusa, p)
  }
})

test("the shop's own pages, which the website cannot take over", () => {
  const shop = ["/shop", "/shop/", "/products/beanie", "/categories/hats/wool", "/collections/winter", "/cart", "/cart/add", "/checkout", "/checkout/payment", "/account", "/account/orders", "/order/order_1", "/search", "/sitemap.xml", "/robots.txt"]
  const website = ["/", "/about", "/about/", "/shopping-guide", "/products-we-love", "/cartography", "/orders", "/_astro/site.a1b2.css", "/favicon.svg", "/sitemap_index.xml", "/forms/contact", "/brochure.pdf"]
  for (const p of shop) assert.equal(isShopPath(p), true, p)
  for (const p of website) assert.equal(isShopPath(p), false, p)
})

test("siteFiles tries each form a page may have been built in, and nothing outside the website", () => {
  const page = (file: string) => ({ file, type: "text/html; charset=utf-8", cache: "public, max-age=300" })
  const cases: [string, ReturnType<typeof siteFiles>][] = [
    ["/", [page("/srv/site/index.html")]],
    ["/about", [page("/srv/site/about.html"), page("/srv/site/about/index.html")]],
    ["/about/", [page("/srv/site/about/index.html")]],
    ["/services/fleet", [page("/srv/site/services/fleet.html"), page("/srv/site/services/fleet/index.html")]],
    ["/v1.2/notes", [page("/srv/site/v1.2/notes.html"), page("/srv/site/v1.2/notes/index.html")]],
    ["/about.html", [page("/srv/site/about.html")]],
    ["/brochure.pdf", [{ file: "/srv/site/brochure.pdf", type: "application/pdf", cache: "public, max-age=3600" }]],
    ["/Team%20Photo.JPG", [{ file: "/srv/site/Team Photo.JPG", type: "image/jpeg", cache: "public, max-age=3600" }]],
    ["/_astro/site.a1b2.css", [{ file: "/srv/site/_astro/site.a1b2.css", type: "text/css; charset=utf-8", cache: "public, max-age=31536000, immutable" }]],
    ["/sitemap_index.xml", [{ file: "/srv/site/sitemap_index.xml", type: "application/xml", cache: "public, max-age=3600" }]],
  ]
  for (const [p, want] of cases) assert.deepEqual(siteFiles("/srv/site/", p), want, p)
  const refused = ["/../etc/passwd", "/a/%2e%2e/b", "/.env", "/.git/config", "/x.php", "/bad%E0%A4%A", "/a%00", "/a\\..\\b", "relative"]
  for (const p of refused) assert.deepEqual(siteFiles("/srv/site", p), [], p)
})

test("the website's old addresses answer in one hop", () => {
  const list = [
    { from: "/old", to: "/new" },
    { from: "/gone", status: 410 },
    { from: "/page?id=7", to: "/seven", status: 301 },
    { from: "/page", to: "/pages/" },
    { from: "/temp", to: "/later", status: 302 },
    { from: "/odd", to: "/x", status: 200 },
    { from: "/broken" },
  ]
  const cases: [string, string, ReturnType<typeof redirectFor>][] = [
    ["/old", "", { status: 301, location: "/new" }],
    ["/old", "?utm=1", { status: 301, location: "/new" }],
    ["/gone", "", { status: 410 }],
    ["/page", "?id=7", { status: 301, location: "/seven" }],
    ["/page", "?id=8", { status: 301, location: "/pages/" }],
    ["/temp", "", { status: 302, location: "/later" }],
    ["/odd", "", { status: 301, location: "/x" }],
    ["/broken", "", null],
    ["/new", "", null],
  ]
  for (const [p, q, want] of cases) assert.deepEqual(redirectFor(list, p, q), want, p + q)
  assert.equal(redirectFor([], "/", ""), null)
})

test("the smallest copy of a website file the browser takes", () => {
  const has = (...files: string[]) => (f: string) => files.includes(f)
  assert.deepEqual(variantFor("/s/a.png", "image/avif,image/webp,*/*", "br", has("/s/a.png.webp")), { file: "/s/a.png.webp", type: "image/webp", vary: "Accept" })
  assert.deepEqual(variantFor("/s/a.jpg", "image/*;q=0.8, image/webp;q=0", "", has("/s/a.jpg.webp")), { file: "/s/a.jpg", vary: "Accept" })
  assert.deepEqual(variantFor("/s/a.png", "image/webp", "", has()), { file: "/s/a.png" })
  assert.deepEqual(variantFor("/s/i.html", "", "gzip, deflate, br", has("/s/i.html.br", "/s/i.html.gz")), { file: "/s/i.html.br", encoding: "br", vary: "Accept-Encoding" })
  assert.deepEqual(variantFor("/s/i.html", "", "gzip", has("/s/i.html.br", "/s/i.html.gz")), { file: "/s/i.html.gz", encoding: "gzip", vary: "Accept-Encoding" })
  assert.deepEqual(variantFor("/s/i.html", "", "br;q=0", has("/s/i.html.br")), { file: "/s/i.html", vary: "Accept-Encoding" })
  assert.deepEqual(variantFor("/s/i.html", "", "br", has()), { file: "/s/i.html" })
})

test("the website's forms", () => {
  assert.equal(formName("/forms/contact"), "contact")
  assert.equal(formName("/forms/get-a-quote-2"), "get-a-quote-2")
  for (const p of ["/forms/", "/forms/Contact", "/forms/a/b", "/forms/" + "x".repeat(41), "/form/contact"]) assert.equal(formName(p), null, p)
  assert.deepEqual(formFields("application/x-www-form-urlencoded", "name=Mere&email=mere%40example.nz"), { name: "Mere", email: "mere@example.nz" })
  assert.deepEqual(formFields("application/json", '{"name":"Mere"}'), { name: "Mere" })
  assert.equal(formFields("application/json", "[1]"), null)
  assert.equal(formFields("application/json", "{"), null)
  assert.equal(anyFilled({ name: " ", altcha: "" }), false)
  assert.equal(anyFilled({}), false)
  assert.equal(anyFilled({ name: "Mere" }), true)
  assert.equal(formReturn("https://lifeloc.co.nz/contact?x=1", "contact"), "/contact?x=1&sent=contact")
  assert.equal(formReturn(undefined, "contact"), "/?sent=contact")
  assert.equal(formReturn("https://lifeloc.co.nz//evil.com/x", "contact"), "/?sent=contact")
  assert.equal(formReturn("https://evil.com/x", "contact"), "/x?sent=contact")
})

test("the website's pages in the sitemap", () => {
  const cases: [string, string | null][] = [
    ["index.html", "/"], ["about.html", "/about"], ["about/index.html", "/about/"], ["services/fleet/index.html", "/services/fleet/"],
    ["404.html", null], ["500.html", null], ["about/404.html", "/about/404"], ["logo.png", null], ["_astro/x.html", null], [".well-known/a.html", null], ["index.html.br", null],
  ]
  for (const [file, want] of cases) assert.equal(siteAddress(file), want, file)
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
