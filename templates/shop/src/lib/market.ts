// Where the shop sells: its country, currency, tax and how it writes money and dates. One preset
// per country, chosen by SHOP_COUNTRY (New Zealand when unset), with SHOP_PRICES_INCLUDE_TAX and
// SHOP_TIME_ZONE to adjust it. Pure, so setup, the storefront and the emails agree.

export type Market = {
  readonly country: string // ISO 3166 alpha-2, lower case
  readonly currency: string // ISO 4217, lower case
  readonly name: string // the region's name
  readonly locale: string
  readonly timeZone: string
  readonly taxRate: number // percent
  readonly taxName: string
  readonly taxInclusive: boolean // prices entered and shown with tax included
  readonly delivery: number // the first standard delivery price, in the currency's main unit
}

export const MARKETS: Readonly<Record<string, Market>> = {
  nz: { country: "nz", currency: "nzd", name: "New Zealand", locale: "en-NZ", timeZone: "Pacific/Auckland", taxRate: 15, taxName: "GST", taxInclusive: true, delivery: 10 },
  au: { country: "au", currency: "aud", name: "Australia", locale: "en-AU", timeZone: "Australia/Sydney", taxRate: 10, taxName: "GST", taxInclusive: true, delivery: 15 },
}

export const marketFrom = (env: Record<string, string | undefined>): Market => {
  const key = (env.SHOP_COUNTRY || "nz").trim().toLowerCase()
  const base = MARKETS[key]
  if (!base) throw new Error(`SHOP_COUNTRY "${env.SHOP_COUNTRY}" is not one the shop sells in; use one of ${Object.keys(MARKETS).join(", ")}`)
  const inclusive = (env.SHOP_PRICES_INCLUDE_TAX ?? "").trim().toLowerCase()
  return {
    ...base,
    ...(env.SHOP_TIME_ZONE?.trim() ? { timeZone: env.SHOP_TIME_ZONE.trim() } : {}),
    ...(inclusive === "true" || inclusive === "false" ? { taxInclusive: inclusive === "true" } : {}),
  }
}

// "Includes GST" under a total when prices carry the tax, "GST" when it is added on top.
export const taxLabel = (m: Pick<Market, "taxName" | "taxInclusive">) => (m.taxInclusive ? `Includes ${m.taxName}` : m.taxName)
