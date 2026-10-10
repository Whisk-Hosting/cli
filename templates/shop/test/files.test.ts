import { test } from "node:test"
import assert from "node:assert/strict"
import { decodeContent, imageUrl, isUploadKey, uploadId, uploadKey } from "../src/lib/files"
import { missingProviders, storeName } from "../src/lib/setup"

test("uploads are told apart from bucket keys", () => {
  assert.equal(uploadKey("01J"), "upload:01J")
  assert.equal(isUploadKey("upload:01J"), true)
  assert.equal(isUploadKey("products/a.png"), false)
  assert.equal(uploadId(uploadKey("01J")), "01J")
})

test("pictures are served resized, other files by their media address", () => {
  assert.equal(imageUrl("https://kiwi.whisk.page/", "01J", "image/png"), "https://kiwi.whisk.page/.whisk/img/01J")
  assert.equal(imageUrl("https://kiwi.whisk.page", "01J", "application/pdf"), "https://kiwi.whisk.page/.whisk/media/01J")
})

test("decodeContent reads base64, text and binary strings", () => {
  assert.deepEqual(decodeContent(Buffer.from("hello").toString("base64"), "image/png"), Buffer.from("hello"))
  assert.deepEqual(decodeContent("a,b\n1,2", "text/csv"), Buffer.from("a,b\n1,2"))
  assert.deepEqual(decodeContent("ÿ\u0001!", "image/png"), Buffer.from([0xff, 0x01, 0x21]))
})

test("setup names a new store after its app and adds missing providers", () => {
  const cases: [string | null, string | undefined, string][] = [
    ["Medusa Store", "acme-tools", "Acme Tools"],
    [null, "kiwi-goods", "Kiwi Goods"],
    ["Our Shop", "kiwi-goods", "Our Shop"],
    ["Medusa Store", undefined, "Medusa Store"],
    [null, undefined, "Shop"],
  ]
  for (const [current, app, want] of cases) assert.equal(storeName(current, app), want)
  assert.deepEqual(missingProviders(["pp_system_default"], ["pp_system_default", "pp_stripe_stripe"]), ["pp_stripe_stripe"])
  assert.deepEqual(missingProviders(["pp_x", "pp_system_default"], ["pp_system_default"]), [])
})
