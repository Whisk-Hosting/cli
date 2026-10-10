// PayPal as a Medusa payment provider. The storefront shows PayPal's buttons for the order this
// provider made; once the buyer approves, completing the cart captures it (or authorises it,
// with PAYPAL_CAPTURE=manual). A webhook only names an order; the answer comes from PayPal.
import { AbstractPaymentProvider, BigNumber, MedusaError, PaymentActions } from "@medusajs/framework/utils"
import type {
  AuthorizePaymentInput, AuthorizePaymentOutput, CancelPaymentInput, CancelPaymentOutput, CapturePaymentInput,
  CapturePaymentOutput, DeletePaymentInput, DeletePaymentOutput, GetPaymentStatusInput, GetPaymentStatusOutput,
  InitiatePaymentInput, InitiatePaymentOutput, ProviderWebhookPayload, RefundPaymentInput, RefundPaymentOutput,
  RetrievePaymentInput, RetrievePaymentOutput, UpdatePaymentInput, UpdatePaymentOutput, WebhookActionResult,
} from "@medusajs/framework/types"
import {
  amountOf, authorizationsOf, baseUrl, capturesOf, orderIdOfEvent, orderRequest, sessionIdOf, statusOf, value,
  type Capture, type Order, type PaypalEnv,
} from "../../lib/paypal"

type Options = { clientId: string; clientSecret: string; environment: PaypalEnv; capture?: boolean }

export default class PaypalProviderService extends AbstractPaymentProvider<Options> {
  static identifier = "paypal"
  protected readonly options_: Options
  private token_: { value: string; until: number } | null = null

  static validateOptions(options: Record<string, unknown>) {
    if (!options.clientId || !options.clientSecret) throw new MedusaError(MedusaError.Types.INVALID_DATA, "PayPal needs PAYPAL_CLIENT_ID and PAYPAL_CLIENT_SECRET")
  }

  constructor(container: Record<string, unknown>, options: Options) {
    super(container, options)
    this.options_ = options
  }

  private async token(): Promise<string> {
    if (this.token_ && this.token_.until > Date.now()) return this.token_.value
    const res = await fetch(`${baseUrl(this.options_.environment)}/v1/oauth2/token`, {
      method: "POST",
      headers: {
        "content-type": "application/x-www-form-urlencoded",
        authorization: `Basic ${Buffer.from(`${this.options_.clientId}:${this.options_.clientSecret}`).toString("base64")}`,
      },
      body: "grant_type=client_credentials",
      signal: AbortSignal.timeout(15_000),
    })
    if (!res.ok) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, `PayPal refused the shop's credentials: ${res.status}`)
    const out = (await res.json()) as { access_token: string; expires_in: number }
    this.token_ = { value: out.access_token, until: Date.now() + Math.max(out.expires_in - 60, 30) * 1000 }
    return out.access_token
  }

  private async call<T>(method: "GET" | "POST" | "PATCH", path: string, body?: unknown, requestId?: string): Promise<T> {
    const headers: Record<string, string> = { "content-type": "application/json", authorization: `Bearer ${await this.token()}`, prefer: "return=representation" }
    if (requestId) headers["paypal-request-id"] = requestId
    const res = await fetch(baseUrl(this.options_.environment) + path, {
      method, headers, body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(20_000),
    })
    const text = await res.text()
    if (!res.ok) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, `PayPal ${method} ${path} answered ${res.status}: ${text.slice(0, 300)}`)
    return (text ? JSON.parse(text) : {}) as T
  }

  private order = (id: string) => this.call<Order>("GET", `/v2/checkout/orders/${encodeURIComponent(id)}`)

  private async create(amount: number, currency: string, paymentSessionId: string, key?: string): Promise<Order> {
    return this.call<Order>("POST", "/v2/checkout/orders", orderRequest({ amount, currency, paymentSessionId, capture: this.options_.capture !== false }), key)
  }

  async initiatePayment({ amount, currency_code, data, context }: InitiatePaymentInput): Promise<InitiatePaymentOutput> {
    const paymentSessionId = String(data?.session_id ?? "")
    const o = await this.create(Number(new BigNumber(amount).numeric), currency_code, paymentSessionId, context?.idempotency_key)
    return { id: o.id, status: "requires_more", data: { id: o.id, session_id: paymentSessionId, intent: this.options_.capture !== false ? "CAPTURE" : "AUTHORIZE" } }
  }

  async updatePayment({ amount, currency_code, data }: UpdatePaymentInput): Promise<UpdatePaymentOutput> {
    const id = String(data?.id)
    await this.call("PATCH", `/v2/checkout/orders/${encodeURIComponent(id)}`, [
      { op: "replace", path: "/purchase_units/@reference_id=='default'/amount", value: { currency_code: currency_code.toUpperCase(), value: value(Number(new BigNumber(amount).numeric), currency_code) } },
    ])
    return { status: "requires_more", data }
  }

  // Called when the cart completes, after the buyer approved in PayPal's window.
  async authorizePayment({ data }: AuthorizePaymentInput): Promise<AuthorizePaymentOutput> {
    const id = String(data?.id)
    let o = await this.order(id)
    if (o.status === "APPROVED") {
      const step = o.intent === "AUTHORIZE" ? "authorize" : "capture"
      o = await this.call<Order>("POST", `/v2/checkout/orders/${encodeURIComponent(id)}/${step}`, {}, `${id}-${step}`)
    }
    return { status: statusOf(o), data: { ...data, order: o } }
  }

  async getPaymentStatus({ data }: GetPaymentStatusInput): Promise<GetPaymentStatusOutput> {
    const o = await this.order(String(data?.id))
    return { status: statusOf(o), data: { ...data, order: o } }
  }

  async capturePayment({ data }: CapturePaymentInput): Promise<CapturePaymentOutput> {
    const o = await this.order(String(data?.id))
    if (capturesOf(o).some((c) => c.status === "COMPLETED")) return { data: { ...data, order: o } }
    const auth = authorizationsOf(o).find((a) => a.status === "CREATED")
    if (!auth) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "This PayPal payment has nothing to capture")
    await this.call("POST", `/v2/payments/authorizations/${encodeURIComponent(auth.id)}/capture`, {}, `${auth.id}-capture`)
    return { data: { ...data, order: await this.order(o.id) } }
  }

  async refundPayment({ data, amount }: RefundPaymentInput): Promise<RefundPaymentOutput> {
    const o = await this.order(String(data?.id))
    const capture = capturesOf(o).find((c: Capture) => c.status === "COMPLETED" || c.status === "PARTIALLY_REFUNDED")
    if (!capture) throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "This PayPal payment has nothing to refund")
    const currency = capture.amount?.currency_code ?? "NZD"
    await this.call("POST", `/v2/payments/captures/${encodeURIComponent(capture.id)}/refund`, {
      amount: { currency_code: currency, value: value(Number(new BigNumber(amount).numeric), currency) },
    })
    return { data: { ...data, order: await this.order(o.id) } }
  }

  async cancelPayment({ data }: CancelPaymentInput): Promise<CancelPaymentOutput> {
    const o = await this.order(String(data?.id))
    const auth = authorizationsOf(o).find((a) => a.status === "CREATED")
    if (auth) await this.call("POST", `/v2/payments/authorizations/${encodeURIComponent(auth.id)}/void`, {}, `${auth.id}-void`)
    return { data }
  }

  async deletePayment({ data }: DeletePaymentInput): Promise<DeletePaymentOutput> {
    return { data }
  }

  async retrievePayment({ data }: RetrievePaymentInput): Promise<RetrievePaymentOutput> {
    return { data: { ...data, order: await this.order(String(data?.id)) } }
  }

  async getWebhookActionAndData(payload: ProviderWebhookPayload["payload"]): Promise<WebhookActionResult> {
    const orderId = orderIdOfEvent((payload.data ?? {}) as any)
    if (!orderId || !/^[0-9A-Z]{10,40}$/.test(orderId)) return { action: PaymentActions.NOT_SUPPORTED }
    const o = await this.order(orderId)
    const sessionId = sessionIdOf(o)
    if (!sessionId) return { action: PaymentActions.NOT_SUPPORTED }
    const data = { session_id: sessionId, amount: new BigNumber(amountOf(o)) }
    switch (statusOf(o)) {
      case "captured":
        return { action: PaymentActions.SUCCESSFUL, data }
      case "authorized":
        return { action: PaymentActions.AUTHORIZED, data }
      default:
        return { action: PaymentActions.NOT_SUPPORTED }
    }
  }
}
