// Windcave's notification that a session changed (GET or POST, with sessionId). It is handed to
// Medusa's payment webhook processing, which asks Windcave for the session itself, so a forged
// call can only make the shop look up a real session.
import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { Modules, PaymentWebhookEvents } from "@medusajs/framework/utils"

const handle = async (req: MedusaRequest, res: MedusaResponse) => {
  const sessionId = String((req.query as any)?.sessionId ?? (req.body as any)?.sessionId ?? "")
  if (!/^[0-9a-zA-Z]{8,64}$/.test(sessionId)) return res.status(400).json({ error: { code: "SESSION_INVALID", message: "No Windcave session named.", fix: "Windcave sends sessionId." } })
  await req.scope.resolve(Modules.EVENT_BUS).emit(
    { name: PaymentWebhookEvents.WebhookReceived, data: { provider: "windcave_windcave", payload: { data: { sessionId }, rawData: "", headers: {} } } },
    { delay: 5000, attempts: 3 },
  )
  return res.sendStatus(200)
}

export const GET = handle
export const POST = handle
