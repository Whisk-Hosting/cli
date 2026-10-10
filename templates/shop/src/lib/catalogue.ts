// Pure decisions for src/jobs/catalogue.ts: a shop can start with a catalogue (catalogue/
// catalogue.json, its pictures in catalogue/images), and the shop adds the products and
// categories it names that it does not have yet. Matching is by handle, so a product staff
// have changed in the admin is never overwritten.

export type CatalogueProduct = {
  readonly handle: string
  readonly title: string
  readonly subtitle?: string
  readonly description?: string
  readonly price: number // in the currency's main unit, entered as the market enters prices
  readonly sku?: string
  readonly inStock?: boolean // false: listed, but sold out until staff add stock
  readonly categories?: readonly string[]
  readonly options?: readonly { readonly title: string; readonly values: readonly string[] }[]
  readonly images?: readonly string[] // file names in catalogue/images, or addresses (/catalogue/a.jpg) used as they are
}

const HANDLE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

// The catalogue file, checked: every product needs a handle, a title and a price, and handles
// are unique. Throws with every problem found, so one deploy shows them all.
export const catalogueFrom = (json: unknown): CatalogueProduct[] => {
  const products = (json as { products?: unknown })?.products
  if (!Array.isArray(products)) throw new Error("catalogue.json needs a products list")
  const problems = products.flatMap((p: any, i) => [
    ...(typeof p?.handle === "string" && HANDLE.test(p.handle) ? [] : [`product ${i + 1}: handle must be lower-case words joined by hyphens`]),
    ...(typeof p?.title === "string" && p.title.trim() ? [] : [`product ${i + 1}: title is missing`]),
    ...(typeof p?.price === "number" && p.price >= 0 ? [] : [`product ${i + 1}: price must be a number of at least 0`]),
    ...((p?.options ?? []).every((o: any) => o?.title && Array.isArray(o.values) && o.values.length) ? [] : [`product ${i + 1}: every option needs a title and values`]),
  ])
  const handles = products.map((p: any) => p?.handle)
  const dupes = [...new Set(handles.filter((h, i) => handles.indexOf(h) !== i))]
  const all = [...problems, ...dupes.map((h) => `handle ${h} is used twice`)]
  if (all.length) throw new Error(`catalogue.json: ${all.join("; ")}`)
  return products as CatalogueProduct[]
}

// "Impairment Goggles" is impairment-goggles.
export const handleOf = (name: string) => name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")

export const productsToAdd = (products: readonly CatalogueProduct[], existing: readonly string[]) =>
  products.filter((p) => !existing.includes(p.handle))

export const categoriesToAdd = (products: readonly CatalogueProduct[], existing: readonly string[]): string[] =>
  [...new Set(products.flatMap((p) => p.categories ?? []))].filter((c) => !existing.includes(handleOf(c)))

// Each variant: one per combination of option values, or a single one for a product without
// options (Medusa needs an option, so it gets a one-value "Default").
export const variantsOf = (p: CatalogueProduct) => {
  const options = p.options?.length ? p.options : [{ title: "Default", values: ["Default"] }]
  const combos = options.reduce<Record<string, string>[]>((acc, o) => acc.flatMap((c) => o.values.map((v) => ({ ...c, [o.title]: v }))), [{}])
  return {
    options: options.map((o) => ({ title: o.title, values: [...o.values] })),
    variants: combos.map((c, i) => ({
      title: p.options?.length ? Object.values(c).join(" / ") : p.title,
      options: c,
      sku: p.sku ? (combos.length > 1 ? `${p.sku}-${i + 1}` : p.sku) : undefined,
    })),
  }
}

// The input createProductsWorkflow takes for one product.
export const productInput = (
  p: CatalogueProduct,
  ctx: { currency: string; channelId: string; profileId: string; categoryIds: Record<string, string>; imageUrls: Record<string, string> },
) => {
  const { options, variants } = variantsOf(p)
  const images = (p.images ?? []).map((f) => ctx.imageUrls[f]).filter((u): u is string => !!u)
  return {
    title: p.title,
    handle: p.handle,
    subtitle: p.subtitle,
    description: p.description,
    status: "published" as const,
    thumbnail: images[0],
    images: images.map((url) => ({ url })),
    options,
    variants: variants.map((v) => ({
      ...v,
      manage_inventory: p.inStock === false,
      prices: [{ currency_code: ctx.currency, amount: p.price }],
    })),
    sales_channels: [{ id: ctx.channelId }],
    shipping_profile_id: ctx.profileId,
    category_ids: (p.categories ?? []).map((c) => ctx.categoryIds[handleOf(c)]).filter((id): id is string => !!id),
  }
}

const TYPES: Record<string, string> = { ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".gif": "image/gif", ".avif": "image/avif" }
export const imageType = (file: string) => TYPES[/\.[a-z0-9]+$/i.exec(file)?.[0].toLowerCase() ?? ""]
