// The shop's name and address, as its emails and pages show them.
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import type { MedusaContainer } from "@medusajs/framework/types"
import { bankAccountOf, type BankAccount } from "./orders"
import { browserKeysFrom } from "./settings"
import { marketFrom, type Market } from "./market"

let cached: { name: string; url: string; at: number } | undefined

export const shopInfo = async (scope: MedusaContainer): Promise<{ name: string; url: string }> => {
  if (cached && Date.now() - cached.at < 60_000) return cached
  const [store] = await scope.resolve(Modules.STORE).listStores({}, { take: 1 })
  cached = { name: store?.name || process.env.WHISK_APP_NAME || "Shop", url: (process.env.WHISK_PUBLIC_URL ?? "").replace(/\/$/, ""), at: Date.now() }
  return cached
}

export type StorefrontContext = {
  api: string
  publishableKey: string
  regionId: string
  currency: string
  name: string
  bank: BankAccount | null
  stripeKey: string | null
  paypalClientId: string | null
  providers: string[]
  market: Market
}

let context: { value: StorefrontContext; at: number } | undefined

// What the storefront needs to know about the shop on every request (storefront/src/lib/medusa.ts),
// read once a minute: the key it calls the store API with, the market's region and the payment
// providers it takes, the shop's name and its bank account for transfers.
export const storefrontContext = async (scope: MedusaContainer): Promise<StorefrontContext> => {
  if (context && Date.now() - context.at < 60_000) return context.value
  const query = scope.resolve(ContainerRegistrationKeys.QUERY)
  const [{ data: stores }, { data: keys }, { data: regions }, enabled] = await Promise.all([
    query.graph({ entity: "store", fields: ["name", "metadata"], pagination: { take: 1 } }),
    query.graph({ entity: "api_key", fields: ["token", "revoked_at", "created_at"], filters: { type: "publishable" } }),
    query.graph({ entity: "region", fields: ["id", "currency_code", "created_at", "payment_providers.id"], filters: { currency_code: marketFrom(process.env).currency } }),
    scope.resolve(Modules.PAYMENT).listPaymentProviders({ is_enabled: true }),
  ])
  const key = (keys as any[]).filter((k) => !k.revoked_at).sort((x, y) => String(x.created_at).localeCompare(String(y.created_at)))[0]
  const region = (regions as any[]).sort((x, y) => String(x.created_at).localeCompare(String(y.created_at)))[0]
  if (!key || !region) throw new Error("the shop is not set up yet: run the migrate step (node migrate.js)")
  const loaded = new Set(enabled.map((p) => p.id))
  const store = (stores as any[])[0]
  const value: StorefrontContext = {
    api: `http://127.0.0.1:${process.env.PORT ?? "9000"}`,
    publishableKey: key.token,
    regionId: region.id,
    currency: region.currency_code,
    name: store?.name || process.env.WHISK_APP_NAME || "Shop",
    bank: bankAccountOf(store?.metadata),
    stripeKey: browserKeysFrom(process.env).stripe,
    paypalClientId: browserKeysFrom(process.env).paypal,
    providers: (region.payment_providers ?? []).map((p: { id: string }) => p.id).filter((id: string) => loaded.has(id)),
    market: marketFrom(process.env),
  }
  context = { value, at: Date.now() }
  return value
}
