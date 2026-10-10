// The customer's emails about their order: confirmed, cancelled, sent and refunded.
import type { SubscriberArgs, SubscriberConfig } from "@medusajs/framework"
import { sendOrderCanceled, sendOrderPlaced, sendRefunded, sendShipped } from "../lib/notify"

export default async function orderEmails({ event, container }: SubscriberArgs<{ id: string; no_notification?: boolean }>) {
  if (event.data.no_notification) return
  switch (event.name) {
    case "order.placed":
      return sendOrderPlaced(container, event.data.id)
    case "order.canceled":
      return sendOrderCanceled(container, event.data.id)
    case "shipment.created":
      return sendShipped(container, event.data.id)
    case "payment.refunded":
      return sendRefunded(container, event.data.id)
  }
}

export const config: SubscriberConfig = {
  event: ["order.placed", "order.canceled", "shipment.created", "payment.refunded"],
}
