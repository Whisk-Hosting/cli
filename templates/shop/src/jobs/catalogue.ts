// Adds the shop's starting catalogue (catalogue/catalogue.json, src/lib/catalogue.ts): every minute,
// the categories and products it names that the shop does not have yet, with their pictures
// stored on Whisk. It runs in the shop rather than in the migrate step because the migrate step
// has the database and nothing else (no storage for the pictures). Once everything is in, each
// run is one query. A shop without the file has nothing to do here.
import { existsSync, readFileSync } from "node:fs"
import path from "node:path"
import type { MedusaContainer } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { createProductCategoriesWorkflow, createProductsWorkflow, uploadFilesWorkflow } from "@medusajs/medusa/core-flows"
import { catalogueFrom, categoriesToAdd, handleOf, imageType, productInput, productsToAdd } from "../lib/catalogue"
import { marketFrom } from "../lib/market"

export default async function catalogue(container: MedusaContainer) {
  const logger = container.resolve(ContainerRegistrationKeys.LOGGER)
  const dir = path.resolve(process.cwd(), "catalogue")
  const file = path.join(dir, "catalogue.json")
  if (!existsSync(file)) return
  const products = catalogueFrom(JSON.parse(readFileSync(file, "utf8")))

  const productModule = container.resolve(Modules.PRODUCT)
  const stores = container.resolve(Modules.STORE)
  const fulfillment = container.resolve(Modules.FULFILLMENT)

  const have = await productModule.listProducts({ handle: products.map((p) => p.handle) }, { select: ["handle"], withDeleted: true })
  const add = productsToAdd(products, have.map((p) => p.handle))
  if (!add.length) return

  // Categories the new products name.
  const existing = await productModule.listProductCategories({}, { select: ["id", "handle"] })
  const newCategories = categoriesToAdd(add, existing.map((c) => c.handle))
  const made = newCategories.length
    ? (await createProductCategoriesWorkflow(container).run({ input: { product_categories: newCategories.map((name) => ({ name, handle: handleOf(name), is_active: true })) } })).result
    : []
  const categoryIds = Object.fromEntries([...existing, ...made].map((c) => [c.handle, c.id]))

  // Pictures, stored once per product added.
  const imageUrls: Record<string, string> = {}
  // A picture given as an address (/catalogue/a.jpg, a file of the storefront's) is used as it is.
  for (const name of [...new Set(add.flatMap((p) => p.images ?? []))].filter((n) => /^(\/|https:)/.test(n))) imageUrls[name] = name
  for (const name of [...new Set(add.flatMap((p) => p.images ?? []))].filter((n) => !imageUrls[n])) {
    const at = path.join(dir, "images", name)
    const mimeType = imageType(name)
    if (!mimeType || !existsSync(at)) {
      logger.warn(`catalogue: ${name} is not a picture in catalogue/images, so it is left out`)
      continue
    }
    const { result } = await uploadFilesWorkflow(container).run({ input: { files: [{ filename: name, mimeType, content: readFileSync(at).toString("base64"), access: "public" }] } })
    imageUrls[name] = result[0].url
  }

  const [store] = await stores.listStores({}, { take: 1 })
  const [profile] = await fulfillment.listShippingProfiles({ type: "default" }, { take: 1 })
  const ctx = { currency: marketFrom(process.env).currency, channelId: store.default_sales_channel_id!, profileId: profile.id, categoryIds, imageUrls }
  await createProductsWorkflow(container).run({ input: { products: add.map((p) => productInput(p, ctx)) } })
  logger.info(`catalogue: added ${add.length} products${made.length ? ` and ${made.length} categories` : ""}`)
}

export const config = { name: "catalogue", schedule: "* * * * *" }
