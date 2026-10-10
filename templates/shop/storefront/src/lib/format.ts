// Pure helpers for showing the catalogue: prices, pictures and addresses.

// Money as the currency's own country writes it: $1,058.00 in Australian dollars on an
// Australian shop, $10.00 in New Zealand dollars on a New Zealand one.
const LOCALES: Record<string, string> = { nzd: "en-NZ", aud: "en-AU" }
export const money = (amount: number | null | undefined, currency: string) =>
  amount == null ? "" : new Intl.NumberFormat(LOCALES[currency.toLowerCase()] ?? "en-NZ", { style: "currency", currency: currency.toUpperCase() }).format(amount)

// A picture stored on Whisk is resized by the edge: ask for the width the layout shows, and
// twice that for sharp screens. Any other address is used as it is.
export const picture = (url: string | null | undefined, width: number): { src: string; srcset?: string } | null => {
  if (!url) return null
  if (!url.includes("/.whisk/img/")) return { src: url }
  const base = url.split("?")[0]
  return { src: `${base}?w=${width}&format=auto`, srcset: `${base}?w=${width}&format=auto 1x, ${base}?w=${width * 2}&format=auto 2x` }
}

export type Price = { calculated_amount: number; original_amount: number; currency_code: string } | null | undefined

// The lowest price among a product's variants, and whether it is a sale price.
export const fromPrice = (variants: { calculated_price?: Price }[] | null | undefined) => {
  const prices = (variants ?? []).map((v) => v.calculated_price).filter((p): p is NonNullable<Price> => !!p)
  if (!prices.length) return null
  const low = prices.reduce((a, b) => (b.calculated_amount < a.calculated_amount ? b : a))
  const varies = prices.some((p) => p.calculated_amount !== low.calculated_amount)
  return { amount: low.calculated_amount, original: low.original_amount, currency: low.currency_code, sale: low.calculated_amount < low.original_amount, varies }
}

// The variant whose options match every choice made, by option id.
export const variantFor = <V extends { id: string; options?: { option_id?: string | null; value: string }[] | null }>(
  variants: V[],
  chosen: Record<string, string>,
): V | null =>
  variants.find((v) => Object.entries(chosen).every(([optionId, value]) => (v.options ?? []).some((o) => o.option_id === optionId && o.value === value))) ?? null

export type AddressInput = {
  first_name: string
  last_name: string
  company?: string
  address_1: string
  address_2?: string
  city: string
  postal_code: string
  province?: string
  country_code: string
  phone?: string
}

const field = (form: FormData, name: string) => String(form.get(name) ?? "").trim()

// An address from a checkout or account form, or the fields still missing.
export const addressFrom = (form: FormData, prefix = "", country = "nz"): { address: AddressInput } | { missing: string[] } => {
  const a: AddressInput = {
    first_name: field(form, `${prefix}first_name`),
    last_name: field(form, `${prefix}last_name`),
    company: field(form, `${prefix}company`) || undefined,
    address_1: field(form, `${prefix}address_1`),
    address_2: field(form, `${prefix}address_2`) || undefined,
    city: field(form, `${prefix}city`),
    postal_code: field(form, `${prefix}postal_code`),
    province: field(form, `${prefix}province`) || undefined,
    country_code: (field(form, `${prefix}country_code`) || country).toLowerCase(),
    phone: field(form, `${prefix}phone`) || undefined,
  }
  const missing = (["first_name", "last_name", "address_1", "city", "postal_code"] as const).filter((k) => !a[k])
  return missing.length ? { missing } : { address: a }
}

// Where a form may send someone after it: a path on this shop, never another site.
export const safeReturn = (to: string | null | undefined, fallback = "/") => (to && /^\/(?!\/)/.test(to) ? to : fallback)

const xml = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;")

// A sitemap of the shop's pages.
export const sitemap = (origin: string, entries: { path: string; updated?: string | null }[]) =>
  [
    `<?xml version="1.0" encoding="UTF-8"?>`,
    `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`,
    ...entries.map((e) => `<url><loc>${xml(origin + e.path)}</loc>${e.updated ? `<lastmod>${xml(new Date(e.updated).toISOString())}</lastmod>` : ""}</url>`),
    `</urlset>`,
    "",
  ].join("\n")

// robots.txt: the website's own first when it has one, then the shop's lines, which keep crawlers
// out of the cart, checkout, account and admin and name the one sitemap of both.
export const robotsTxt = (origin: string, siteRobots: string | undefined): string => {
  const shop = ["User-agent: *", "Disallow: /cart", "Disallow: /checkout", "Disallow: /account", "Disallow: /order/", "Disallow: /app"]
  const ours = [...shop, "", `Sitemap: ${origin}/sitemap.xml`, ""]
  return (siteRobots?.trim() ? [siteRobots.replace(/\s*$/, ""), "", "# The shop", ...ours] : ours).join("\n")
}

// A cart's or order's sums as the market writes them. Where prices include the tax, lines and the
// subtotal carry it and a note under the total says how much was in it; where the tax is added on
// top, lines and the subtotal are before it and the tax is its own line above the total.
type Sums = { item_total?: number; item_subtotal?: number; shipping_total?: number; shipping_subtotal?: number; discount_total?: number; tax_total?: number; total?: number }
export const lineAmount = (line: { total?: number; subtotal?: number }, taxInclusive: boolean) =>
  taxInclusive ? line.total : line.subtotal ?? line.total
export const sums = (o: Sums, taxInclusive: boolean) => ({
  subtotal: taxInclusive ? o.item_total : o.item_subtotal ?? o.item_total,
  shipping: taxInclusive ? o.shipping_total : o.shipping_subtotal ?? o.shipping_total,
  discount: Number(o.discount_total ?? 0),
  tax: o.tax_total,
  total: o.total,
})
