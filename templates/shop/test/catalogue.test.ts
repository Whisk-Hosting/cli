import { test } from "node:test"
import assert from "node:assert/strict"
import { catalogueFrom, categoriesToAdd, handleOf, imageType, productInput, productsToAdd, variantsOf } from "../src/lib/catalogue"

const goggles = { handle: "goggles", title: "Goggles", price: 325, sku: "G", categories: ["Impairment Goggles"], options: [{ title: "Type", values: ["Low", "High"] }], images: ["goggles-1.jpg"] }
const tester = { handle: "tester", title: "Tester", price: 1058, sku: "T", inStock: false, categories: ["Workplace Breathalyser"] }

test("catalogueFrom", () => {
  assert.deepEqual(catalogueFrom({ products: [goggles] }), [goggles])
  const bad: [unknown, RegExp][] = [
    [{}, /needs a products list/],
    [{ products: [{ handle: "Bad Handle", title: "x", price: 1 }] }, /handle must be/],
    [{ products: [{ handle: "a", title: "", price: 1 }] }, /title is missing/],
    [{ products: [{ handle: "a", title: "A", price: "1" }] }, /price must be/],
    [{ products: [{ handle: "a", title: "A", price: 1, options: [{ title: "T", values: [] }] }] }, /needs a title and values/],
    [{ products: [goggles, goggles] }, /goggles is used twice/],
  ]
  for (const [json, want] of bad) assert.throws(() => catalogueFrom(json), want)
})

test("handleOf", () => {
  const cases: [string, string][] = [["Impairment Goggles", "impairment-goggles"], ["Wallmount Breathalyser – Passive Only", "wallmount-breathalyser-passive-only"], ["Workplace Passive/Active Breathalyser", "workplace-passive-active-breathalyser"]]
  for (const [name, want] of cases) assert.equal(handleOf(name), want)
})

test("productsToAdd and categoriesToAdd", () => {
  assert.deepEqual(productsToAdd([goggles, tester], ["goggles"]).map((p) => p.handle), ["tester"])
  assert.deepEqual(categoriesToAdd([goggles, tester, goggles], ["impairment-goggles"]), ["Workplace Breathalyser"])
})

test("variantsOf", () => {
  const cases: [any, { options: string[]; variants: [string, string | undefined][] }][] = [
    [goggles, { options: ["Type"], variants: [["Low", "G-1"], ["High", "G-2"]] }],
    [tester, { options: ["Default"], variants: [["Tester", "T"]] }],
    [{ handle: "x", title: "X", price: 1 }, { options: ["Default"], variants: [["X", undefined]] }],
  ]
  for (const [p, want] of cases) {
    const got = variantsOf(p)
    assert.deepEqual(got.options.map((o) => o.title), want.options)
    assert.deepEqual(got.variants.map((v) => [v.title, v.sku]), want.variants)
  }
})

test("productInput", () => {
  const ctx = { currency: "aud", channelId: "sc_1", profileId: "sp_1", categoryIds: { "impairment-goggles": "pcat_1" }, imageUrls: { "goggles-1.jpg": "https://shop/.whisk/img/1" } }
  const g = productInput(goggles, ctx)
  assert.equal(g.thumbnail, "https://shop/.whisk/img/1")
  assert.deepEqual(g.category_ids, ["pcat_1"])
  assert.deepEqual(g.variants[0].prices, [{ currency_code: "aud", amount: 325 }])
  assert.equal(g.variants[0].manage_inventory, false)
  const t = productInput(tester, ctx)
  assert.equal(t.thumbnail, undefined)
  assert.deepEqual(t.category_ids, [])
  assert.equal(t.variants[0].manage_inventory, true)
})

test("imageType", () => {
  assert.equal(imageType("a.JPG"), "image/jpeg")
  assert.equal(imageType("a.png"), "image/png")
  assert.equal(imageType("a.txt"), undefined)
})
