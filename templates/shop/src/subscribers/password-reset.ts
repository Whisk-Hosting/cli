// The email with a link to choose a password, for a customer who asked for one at
// /account/password. Staff have no password: they sign in through Whisk.
import type { SubscriberArgs, SubscriberConfig } from "@medusajs/framework"
import { Modules } from "@medusajs/framework/utils"
import { shopInfo } from "../lib/shop"

export default async function passwordReset({ event, container }: SubscriberArgs<{ entity_id: string; token: string; actor_type: string }>) {
  if (event.data.actor_type !== "customer") return
  const shop = await shopInfo(container)
  const link = `${shop.url}/account/password?token=${encodeURIComponent(event.data.token)}&email=${encodeURIComponent(event.data.entity_id)}`
  await container.resolve(Modules.NOTIFICATION).createNotifications({
    to: event.data.entity_id,
    channel: "email",
    template: "passwordReset",
    data: { shop: shop.name, shopUrl: shop.url, link, minutes: 15 },
  })
}

export const config: SubscriberConfig = { event: "auth.password_reset" }
