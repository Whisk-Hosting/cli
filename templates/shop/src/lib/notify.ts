// Sending the customer's emails about an order. Each is keyed so it goes once: a subscriber
// that runs twice, or the sweep (src/jobs/resend-order-emails.ts) catching one that failed,
// sends nothing more.
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import type { MedusaContainer } from "@medusajs/framework/types"
import { bankAccountOf, displayId, ORDER_FIELDS, orderView, type OrderLike } from "./orders"
import { shopInfo } from "./shop"

const orderOf = async (scope: MedusaContainer, id: string): Promise<OrderLike | null> => {
  const { data } = await scope.resolve(ContainerRegistrationKeys.QUERY).graph({ entity: "order", fields: ORDER_FIELDS, filters: { id } })
  return (data[0] as OrderLike | undefined) ?? null
}

const send = (scope: MedusaContainer, to: string, template: string, data: Record<string, unknown>, key: string) =>
  scope.resolve(Modules.NOTIFICATION).createNotifications({ to, channel: "email", template, data, idempotency_key: key })

export const sendOrderPlaced = async (scope: MedusaContainer, orderId: string) => {
  const order = await orderOf(scope, orderId)
  if (!order?.email) return
  const [store] = await scope.resolve(Modules.STORE).listStores({}, { take: 1 })
  const view = orderView(order, await shopInfo(scope), bankAccountOf(store?.metadata))
  await send(scope, order.email, "orderPlaced", view, `order-placed:${order.id}`)
}

export const sendOrderCanceled = async (scope: MedusaContainer, orderId: string) => {
  const order = await orderOf(scope, orderId)
  if (!order?.email) return
  const shop = await shopInfo(scope)
  await send(scope, order.email, "orderCanceled", { shop: shop.name, shopUrl: shop.url, displayId: displayId(order) }, `order-canceled:${order.id}`)
}

export const sendShipped = async (scope: MedusaContainer, fulfillmentId: string) => {
  const query = scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data } = await query.graph({
    entity: "fulfillment",
    fields: ["id", "labels.tracking_number", "labels.tracking_url", "order.id", "order.display_id", "order.email"],
    filters: { id: fulfillmentId },
  })
  const f = data[0] as any
  if (!f?.order?.email) return
  const shop = await shopInfo(scope)
  const tracking = (f.labels ?? []).map((l: any) => ({ number: l.tracking_number ?? "", url: l.tracking_url && /^https:\/\//.test(l.tracking_url) ? l.tracking_url : null }))
  await send(scope, f.order.email, "orderShipped", { shop: shop.name, shopUrl: shop.url, displayId: displayId(f.order), tracking }, `shipped:${f.id}`)
}

// payment.refunded names the payment; the refund it is about is that payment's newest.
export const sendRefunded = async (scope: MedusaContainer, paymentId: string) => {
  const query = scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data } = await query.graph({
    entity: "payment",
    fields: ["id", "currency_code", "refunds.id", "refunds.amount", "refunds.created_at", "payment_collection.order.id", "payment_collection.order.display_id", "payment_collection.order.email"],
    filters: { id: paymentId },
  })
  const p = data[0] as any
  const order = p?.payment_collection?.order
  const refund = [...(p?.refunds ?? [])].sort((a: any, b: any) => String(a.created_at).localeCompare(String(b.created_at))).slice(-1)[0]
  if (!order?.email || !refund) return
  const shop = await shopInfo(scope)
  await send(
    scope, order.email, "refunded",
    { shop: shop.name, shopUrl: shop.url, displayId: displayId(order), amount: Number(refund.amount?.numeric ?? refund.amount), currency: p.currency_code },
    `refunded:${refund.id}`,
  )
}
