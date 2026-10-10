// Shaping an order for the customer's emails. Pure; the subscribers fetch the order with
// ORDER_FIELDS and pass it here.
import type { Address, OrderView } from "./emails"

export const ORDER_FIELDS = [
  "id",
  "display_id",
  "email",
  "currency_code",
  "metadata",
  "item_total",
  "item_subtotal",
  "shipping_total",
  "shipping_subtotal",
  "discount_total",
  "tax_total",
  "total",
  "items.title",
  "items.variant_title",
  "items.quantity",
  "items.total",
  "items.subtotal",
  "items.thumbnail",
  "items.is_tax_inclusive",
  "shipping_methods.name",
  "shipping_address.*",
  "payment_collections.payment_sessions.provider_id",
  "payment_collections.payments.provider_id",
]

type Num = number | string | { numeric?: number } | null | undefined
const num = (v: Num): number => (v == null ? 0 : typeof v === "object" ? Number(v.numeric ?? 0) : Number(v))

export type BankAccount = { accountName: string; accountNumber: string }

export type OrderLike = {
  id: string
  display_id?: number | null
  email?: string | null
  currency_code: string
  metadata?: Record<string, unknown> | null
  item_total?: Num
  item_subtotal?: Num
  shipping_total?: Num
  shipping_subtotal?: Num
  discount_total?: Num
  tax_total?: Num
  total?: Num
  items?: { title: string; variant_title?: string | null; quantity: Num; total?: Num; subtotal?: Num; thumbnail?: string | null; is_tax_inclusive?: boolean }[] | null
  shipping_methods?: { name: string }[] | null
  shipping_address?: {
    first_name?: string | null
    last_name?: string | null
    company?: string | null
    address_1?: string | null
    address_2?: string | null
    city?: string | null
    province?: string | null
    postal_code?: string | null
    country_code?: string | null
  } | null
  payment_collections?: { payment_sessions?: { provider_id: string }[] | null; payments?: { provider_id: string }[] | null }[] | null
}

export const displayId = (o: { display_id?: number | null; id: string }) => (o.display_id ? `#${o.display_id}` : o.id)

export const providerOf = (o: OrderLike): string => {
  const pc = o.payment_collections ?? []
  const fromPayments = pc.flatMap((c) => c.payments ?? []).map((p) => p.provider_id)
  const fromSessions = pc.flatMap((c) => c.payment_sessions ?? []).map((p) => p.provider_id)
  return fromPayments[0] ?? fromSessions[0] ?? ""
}

export const paymentMethodOf = (providerId: string): OrderView["paymentMethod"] => {
  if (providerId === "pp_system_default") return "bank_transfer"
  if (providerId.startsWith("pp_stripe") || providerId.startsWith("pp_windcave")) return "card"
  if (providerId.startsWith("pp_paypal")) return "paypal"
  if (providerId.startsWith("pp_account")) return "account"
  return "other"
}

const addressOf = (a: OrderLike["shipping_address"]): Address | null => {
  if (!a || !a.address_1) return null
  const name = [a.first_name, a.last_name].filter(Boolean).join(" ")
  const cityLine = [a.city, a.postal_code].filter(Boolean).join(" ")
  return {
    name,
    company: a.company ?? null,
    lines: [a.address_1, a.address_2 ?? "", a.province ?? "", cityLine, (a.country_code ?? "").toUpperCase()].filter(Boolean),
  }
}

export const orderView = (o: OrderLike, shop: { name: string; url: string }, bank: BankAccount | null): OrderView => {
  const method = paymentMethodOf(providerOf(o))
  const id = displayId(o)
  const items = o.items ?? []
  // Where the tax is added on top, lines, the subtotal and shipping are shown before it, and the
  // tax has its own line above the total (storefront/src/lib/format.ts, sums).
  const taxIncluded = items.length === 0 || items.every((i) => i.is_tax_inclusive !== false)
  const before = (withTax: Num, withoutTax: Num) => num(taxIncluded || withoutTax == null ? withTax : withoutTax)
  return {
    shop: shop.name,
    shopUrl: shop.url,
    displayId: id,
    email: o.email ?? "",
    currency: o.currency_code,
    lines: items.map((i) => ({ title: i.title, variant: i.variant_title ?? null, quantity: num(i.quantity), total: before(i.total, i.subtotal), thumbnail: i.thumbnail ?? null })),
    subtotal: before(o.item_total, o.item_subtotal),
    shipping: before(o.shipping_total, o.shipping_subtotal),
    discount: num(o.discount_total),
    tax: num(o.tax_total),
    total: num(o.total),
    taxIncluded,
    shippingMethod: o.shipping_methods?.[0]?.name ?? null,
    shippingAddress: addressOf(o.shipping_address),
    paymentMethod: method,
    bankTransfer: method === "bank_transfer" && bank ? { ...bank, reference: id.replace(/^#/, "") } : null,
    poNumber: typeof o.metadata?.po_number === "string" ? o.metadata.po_number : null,
  }
}

// The business's bank account, from the store's details in the admin (Settings, Store,
// Metadata: bank_account_name and bank_account_number). Without both, bank transfer is not
// offered at checkout.
export const bankAccountOf = (metadata: Record<string, unknown> | null | undefined): BankAccount | null => {
  const name = typeof metadata?.bank_account_name === "string" ? metadata.bank_account_name.trim() : ""
  const number = typeof metadata?.bank_account_number === "string" ? metadata.bank_account_number.trim() : ""
  return name && number ? { accountName: name, accountNumber: number } : null
}
