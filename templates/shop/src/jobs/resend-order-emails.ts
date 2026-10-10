// Every fifteen minutes, sends any order confirmation from the last two days that has not gone
// out: one whose event was lost in a restart, or whose send failed. Already-sent ones are
// skipped by their key.
import type { MedusaContainer } from "@medusajs/framework/types"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { sendOrderPlaced } from "../lib/notify"

export default async function resendOrderEmails(container: MedusaContainer) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const logger = container.resolve(ContainerRegistrationKeys.LOGGER)
  const since = new Date(Date.now() - 2 * 24 * 3600 * 1000)
  const { data } = await query.graph({ entity: "order", fields: ["id"], filters: { created_at: { $gte: since } } as any })
  for (const o of data as { id: string }[]) {
    try {
      await sendOrderPlaced(container, o.id)
    } catch (e) {
      logger.warn(`order ${o.id}: confirmation not sent: ${(e as Error).message}`)
    }
  }
}

export const config = { name: "resend-order-emails", schedule: "*/15 * * * *" }
