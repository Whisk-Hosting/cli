// Windcave's hosted payment page as a Medusa payment provider. The customer pays on Windcave's
// page and comes back to /checkout/return/windcave; Windcave also calls /hooks/windcave, and
// either way the shop asks Windcave for the session rather than trusting what arrived.
import { AbstractPaymentProvider, BigNumber, MedusaError, PaymentActions } from "@medusajs/framework/utils"
import type {
  AuthorizePaymentInput, AuthorizePaymentOutput, CancelPaymentInput, CancelPaymentOutput, CapturePaymentInput,
  CapturePaymentOutput, DeletePaymentInput, DeletePaymentOutput, GetPaymentStatusInput, GetPaymentStatusOutput,
  InitiatePaymentInput, InitiatePaymentOutput, ProviderWebhookPayload, RefundPaymentInput, RefundPaymentOutput,
  RetrievePaymentInput, RetrievePaymentOutput, UpdatePaymentInput, UpdatePaymentOutput, WebhookActionResult,
} from "@medusajs/framework/types"
import {
  authHeader, baseUrl, completeRequest, hostedPageUrl, refundRequest, sessionRequest, statusOf, voidRequest,
  type Session, type Transaction, type TransactionRequest, type WindcaveEnv,
} from "../../lib/windcave"

type Options = { username: string; apiKey: string; environment: WindcaveEnv; publicUrl: string; capture?: boolean }

export default class WindcaveProviderService extends AbstractPaymentProvider<Options> {
  static identifier = "windcave"
  protected readonly options_: Options

  static validateOptions(options: Record<string, unknown>) {
    if (!options.username || !options.apiKey) throw new MedusaError(MedusaError.Types.INVALID_DATA, "Windcave needs WINDCAVE_USERNAME and WINDCAVE_API_KEY")
    if (!options.publicUrl) throw new MedusaError(MedusaError.Types.INVALID_DATA, "Windcave needs the shop's public address (WHISK_PUBLIC_URL)")
  }

  constructor(container: Record<string, unknown>, options: Options) {
    super(container, options)
    this.options_ = options
  }

  private async call<T>(method: "GET" | "POST", path: string, body?: unknown): Promise<T> {
    const res = await fetch(baseUrl(this.options_.environment) + path, {
      method,
      headers: { "content-type": "application/json", authorization: authHeader(this.options_.username, this.options_.apiKey) },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(20_000),
    })
    const text = await res.text()
    if (!res.ok) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, `Windcave ${method} ${path} answered ${res.status}: ${text.slice(0, 300)}`)
    return (text ? JSON.parse(text) : {}) as T
  }

  private session = (id: string) => this.call<Session>("GET", `/sessions/${encodeURIComponent(id)}`)

  private async transact(req: TransactionRequest): Promise<Transaction> {
    const t = await this.call<Transaction>("POST", "/transactions", req)
    if (!t.authorised) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, `Windcave refused the ${req.type}: ${t.responseText ?? "declined"}`)
    return t
  }

  private async create(amount: number, currency: string, paymentSessionId: string): Promise<Session> {
    return this.call<Session>("POST", "/sessions", sessionRequest({
      amount, currency, paymentSessionId, publicUrl: this.options_.publicUrl, capture: this.options_.capture !== false,
    }))
  }

  async initiatePayment({ amount, currency_code, data }: InitiatePaymentInput): Promise<InitiatePaymentOutput> {
    const paymentSessionId = String(data?.session_id ?? "")
    const s = await this.create(Number(new BigNumber(amount).numeric), currency_code, paymentSessionId)
    return { id: s.id, status: "pending", data: { id: s.id, session_id: paymentSessionId, redirect_url: hostedPageUrl(s) } }
  }

  // A Windcave session is fixed to its amount, so a changed cart gets a new one.
  async updatePayment({ amount, currency_code, data }: UpdatePaymentInput): Promise<UpdatePaymentOutput> {
    const paymentSessionId = String(data?.session_id ?? "")
    const s = await this.create(Number(new BigNumber(amount).numeric), currency_code, paymentSessionId)
    return { status: "pending", data: { id: s.id, session_id: paymentSessionId, redirect_url: hostedPageUrl(s) } }
  }

  async authorizePayment({ data }: AuthorizePaymentInput): Promise<AuthorizePaymentOutput> {
    return this.getPaymentStatus({ data })
  }

  async getPaymentStatus({ data }: GetPaymentStatusInput): Promise<GetPaymentStatusOutput> {
    const s = await this.session(String(data?.id))
    const { status, transaction } = statusOf(s)
    return { status, data: { ...data, transaction: transaction ?? null } }
  }

  async capturePayment({ data }: CapturePaymentInput): Promise<CapturePaymentOutput> {
    const t = data?.transaction as Transaction | null | undefined
    if (!t) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "This Windcave payment has no approved transaction to capture")
    if (t.type === "purchase") return { data }
    const done = await this.transact(completeRequest(t))
    return { data: { ...data, capture: done } }
  }

  async refundPayment({ data, amount }: RefundPaymentInput): Promise<RefundPaymentOutput> {
    const t = (data?.capture as Transaction | undefined) ?? (data?.transaction as Transaction | undefined)
    if (!t) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "This Windcave payment has nothing to refund")
    const r = await this.transact(refundRequest(t, Number(new BigNumber(amount).numeric)))
    return { data: { ...data, refunds: [...((data?.refunds as Transaction[]) ?? []), r] } }
  }

  async cancelPayment({ data }: CancelPaymentInput): Promise<CancelPaymentOutput> {
    const t = data?.transaction as Transaction | null | undefined
    if (t && t.type === "auth" && !data?.capture) await this.transact(voidRequest(t))
    return { data }
  }

  async deletePayment({ data }: DeletePaymentInput): Promise<DeletePaymentOutput> {
    return { data }
  }

  async retrievePayment({ data }: RetrievePaymentInput): Promise<RetrievePaymentOutput> {
    return { data: { ...data, session: await this.session(String(data?.id)) } }
  }

  // A notification only says which session to look at; the answer comes from Windcave.
  async getWebhookActionAndData(payload: ProviderWebhookPayload["payload"]): Promise<WebhookActionResult> {
    const sessionId = String((payload.data as Record<string, unknown>)?.sessionId ?? "")
    if (!/^[0-9a-zA-Z]{8,64}$/.test(sessionId)) return { action: PaymentActions.NOT_SUPPORTED }
    const s = await this.session(sessionId)
    if (!s.merchantReference) return { action: PaymentActions.NOT_SUPPORTED }
    const { status, transaction } = statusOf(s)
    const amount = new BigNumber(transaction?.amount ?? s.amount ?? 0)
    const data = { session_id: s.merchantReference, amount }
    if (status === "captured") return { action: PaymentActions.SUCCESSFUL, data }
    if (status === "authorized") return { action: PaymentActions.AUTHORIZED, data }
    if (status === "error") return { action: PaymentActions.FAILED, data }
    return { action: PaymentActions.NOT_SUPPORTED }
  }
}
