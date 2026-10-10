// Sends the shop's emails through Whisk's email API, from the shop's own address on Whisk's
// domain unless SHOP_EMAIL_FROM names a mailbox on a domain the business has verified.
import { AbstractNotificationProviderService, MedusaError } from "@medusajs/framework/utils"
import type { NotificationTypes } from "@medusajs/framework/types"
import { templates, type TemplateName } from "../../lib/emails"
import { sendEmail } from "../../lib/whisk"

export default class WhiskEmailService extends AbstractNotificationProviderService {
  static identifier = "whisk-email"

  async send(n: NotificationTypes.ProviderSendNotificationDTO): Promise<NotificationTypes.ProviderSendNotificationResultsDTO> {
    const render = templates[n.template as TemplateName] as ((data: any) => { subject: string; html: string; text: string }) | undefined
    if (!render) throw new MedusaError(MedusaError.Types.INVALID_DATA, `No email template named ${n.template}`)
    const mail = render(n.data ?? {})
    const from = process.env.SHOP_EMAIL_FROM || undefined
    const { id } = await sendEmail({ to: n.to, from, ...mail })
    return { id }
  }
}
