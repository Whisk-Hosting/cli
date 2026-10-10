// Windcave's REST API, as pure functions: the requests the shop makes and how it reads the
// answers. The provider in src/modules/windcave sends them. Reference:
// https://www.windcave.com/developer-ecommerce-api-rest

export type WindcaveEnv = "test" | "live"

export const baseUrl = (env: WindcaveEnv) => (env === "live" ? "https://sec.windcave.com/api/v1" : "https://uat.windcave.com/api/v1")

export const authHeader = (username: string, apiKey: string) => `Basic ${Buffer.from(`${username}:${apiKey}`).toString("base64")}`

// Windcave takes amounts as decimal strings in the currency's own unit.
const DECIMALS: Record<string, number> = { jpy: 0, krw: 0, vnd: 0, clp: 0, isk: 0 }
export const amountString = (amount: number, currency: string) => amount.toFixed(DECIMALS[currency.toLowerCase()] ?? 2)

export type SessionRequest = {
  type: "purchase" | "auth"
  methods: string[]
  amount: string
  currency: string
  merchantReference: string
  language: string
  callbackUrls: { approved: string; declined: string; cancelled: string }
  notificationUrl: string
}

// sessionRequest is the hosted payment page session for one Medusa payment session. The
// merchant reference is the Medusa payment session's id, which is how a notification finds it.
export const sessionRequest = (input: {
  amount: number
  currency: string
  paymentSessionId: string
  publicUrl: string
  capture: boolean
}): SessionRequest => {
  const back = (result: string) => `${input.publicUrl.replace(/\/$/, "")}/checkout/return/windcave?result=${result}`
  return {
    type: input.capture ? "purchase" : "auth",
    methods: ["card"],
    amount: amountString(input.amount, input.currency),
    currency: input.currency.toUpperCase(),
    merchantReference: input.paymentSessionId,
    language: "en",
    callbackUrls: { approved: back("approved"), declined: back("declined"), cancelled: back("cancelled") },
    notificationUrl: `${input.publicUrl.replace(/\/$/, "")}/hooks/windcave`,
  }
}

export type Link = { href: string; rel: string; method?: string }
export type Transaction = {
  id: string
  authorised: boolean
  type: string
  amount: string
  currency: string
  responseText?: string
  reCo?: string
}
export type Session = {
  id: string
  state?: string
  merchantReference?: string
  links?: Link[]
  transactions?: Transaction[]
  amount?: string
  currency?: string
}

export const hostedPageUrl = (s: Session) => s.links?.find((l) => l.rel === "hpp")?.href ?? null

export type Status = "authorized" | "captured" | "pending" | "error" | "canceled"

// statusOf reads a session: an authorised purchase is captured, an authorised auth is
// authorized, a declined one is an error the customer can retry, and no transaction yet is
// pending (the customer has not paid, or Windcave is still deciding).
export const statusOf = (s: Session): { status: Status; transaction?: Transaction } => {
  const txs = s.transactions ?? []
  const ok = txs.find((t) => t.authorised && (t.type === "purchase" || t.type === "auth"))
  if (ok) return { status: ok.type === "purchase" ? "captured" : "authorized", transaction: ok }
  if (txs.length) return { status: "error", transaction: txs[txs.length - 1] }
  if (s.state === "cancelled" || s.state === "expired") return { status: "canceled" }
  return { status: "pending" }
}

export type TransactionRequest = { type: "complete" | "refund" | "void"; amount?: string; currency?: string; transactionId: string }

export const completeRequest = (t: Transaction): TransactionRequest => ({ type: "complete", amount: t.amount, currency: t.currency, transactionId: t.id })
export const refundRequest = (t: Transaction, amount: number): TransactionRequest => ({
  type: "refund",
  amount: amountString(amount, t.currency),
  currency: t.currency,
  transactionId: t.id,
})
export const voidRequest = (t: Transaction): TransactionRequest => ({ type: "void", transactionId: t.id })
