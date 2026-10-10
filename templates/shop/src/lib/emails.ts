// The emails the shop sends its customers, as pure functions from what happened to a subject,
// an HTML body and a plain-text body. Subscribers shape the data (orderView below); the
// whisk-email provider sends what these return. Restyle by editing layout().

export type Rendered = { subject: string; html: string; text: string }

export type Line = { title: string; variant?: string | null; quantity: number; total: number; thumbnail?: string | null }
export type Address = {
  name: string
  company?: string | null
  lines: string[]
}
export type OrderView = {
  shop: string
  shopUrl: string
  displayId: string
  email: string
  currency: string
  lines: Line[]
  subtotal: number
  shipping: number
  discount: number
  tax: number
  total: number
  taxIncluded: boolean
  shippingMethod?: string | null
  shippingAddress?: Address | null
  paymentMethod: "bank_transfer" | "card" | "paypal" | "account" | "other"
  bankTransfer?: { accountName: string; accountNumber: string; reference: string } | null
  poNumber?: string | null
}

const LOCALES: Record<string, string> = { nzd: "en-NZ", aud: "en-AU" }
export const money = (amount: number, currency: string) =>
  new Intl.NumberFormat(LOCALES[currency.toLowerCase()] ?? "en-NZ", { style: "currency", currency: currency.toUpperCase() }).format(amount)

const esc = (s: string) =>
  s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!)

const layout = (shop: string, shopUrl: string, title: string, body: string) => `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>${esc(title)}</title></head>
<body style="margin:0;padding:0;background:#f6f6f4;color:#1c1c1a;font:16px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:24px 12px">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#fff;border-radius:8px">
<tr><td style="padding:24px 24px 0"><a href="${esc(shopUrl)}" style="color:#1c1c1a;text-decoration:none;font-weight:600;font-size:18px">${esc(shop)}</a></td></tr>
<tr><td style="padding:16px 24px 24px">${body}</td></tr>
</table></td></tr></table></body></html>`

const h1 = (s: string) => `<h1 style="font-size:22px;line-height:1.3;margin:0 0 12px">${esc(s)}</h1>`
const p = (s: string) => `<p style="margin:0 0 12px">${s}</p>`
const button = (href: string, label: string) =>
  `<p style="margin:16px 0"><a href="${esc(href)}" style="display:inline-block;background:#1c1c1a;color:#fff;text-decoration:none;padding:12px 20px;border-radius:6px;font-weight:600">${esc(label)}</a></p>`

const row = (label: string, value: string, strong = false) =>
  `<tr><td style="padding:4px 0;${strong ? "font-weight:600" : "color:#555"}">${esc(label)}</td><td align="right" style="padding:4px 0;${strong ? "font-weight:600" : ""}">${esc(value)}</td></tr>`

const linesTable = (o: OrderView) =>
  `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-top:1px solid #e6e6e2;margin:12px 0">${o.lines
    .map(
      (l) =>
        `<tr><td style="padding:8px 0;border-bottom:1px solid #e6e6e2">${esc(l.title)}${l.variant ? `<br><span style="color:#555;font-size:14px">${esc(l.variant)}</span>` : ""}<br><span style="color:#555;font-size:14px">Qty ${l.quantity}</span></td><td align="right" style="padding:8px 0;border-bottom:1px solid #e6e6e2">${esc(money(l.total, o.currency))}</td></tr>`,
    )
    .join("")}</table>`

const totalsTable = (o: OrderView) =>
  `<table role="presentation" width="100%" cellpadding="0" cellspacing="0">${[
    row("Subtotal", money(o.subtotal, o.currency)),
    o.discount > 0 ? row("Discount", `-${money(o.discount, o.currency)}`) : "",
    row(o.shippingMethod ? `Shipping (${o.shippingMethod})` : "Shipping", money(o.shipping, o.currency)),
    o.taxIncluded ? "" : row("GST", money(o.tax, o.currency)),
    row("Total", money(o.total, o.currency), true),
    o.taxIncluded ? row("Includes GST", money(o.tax, o.currency)) : "",
  ].join("")}</table>`

const addressBlock = (a?: Address | null) =>
  a ? p(`<strong>Delivering to</strong><br>${[a.name, a.company ?? "", ...a.lines].filter(Boolean).map(esc).join("<br>")}`) : ""

const linesText = (o: OrderView) => o.lines.map((l) => `${l.quantity} x ${l.title}${l.variant ? ` (${l.variant})` : ""}  ${money(l.total, o.currency)}`).join("\n")
const totalsText = (o: OrderView) =>
  [
    `Subtotal ${money(o.subtotal, o.currency)}`,
    o.discount > 0 ? `Discount -${money(o.discount, o.currency)}` : "",
    `Shipping ${money(o.shipping, o.currency)}`,
    o.taxIncluded ? "" : `GST ${money(o.tax, o.currency)}`,
    `Total ${money(o.total, o.currency)}`,
    o.taxIncluded ? `Includes GST ${money(o.tax, o.currency)}` : "",
  ]
    .filter(Boolean)
    .join("\n")

const bankText = (o: OrderView) =>
  o.paymentMethod === "bank_transfer" && o.bankTransfer
    ? `Please pay ${money(o.total, o.currency)} by bank transfer to ${o.bankTransfer.accountName}, ${o.bankTransfer.accountNumber}, with the reference ${o.bankTransfer.reference}. We send your order once it arrives.`
    : ""

export const orderPlaced = (o: OrderView): Rendered => {
  const bank = bankText(o)
  const account = o.paymentMethod === "account" ? "This order is on your account and will be invoiced." : ""
  return {
    subject: `Order ${o.displayId} confirmed`,
    html: layout(
      o.shop,
      o.shopUrl,
      `Order ${o.displayId}`,
      h1(`Thanks for your order`) +
        p(`Order <strong>${esc(o.displayId)}</strong>${o.poNumber ? `, PO ${esc(o.poNumber)}` : ""}.`) +
        (bank ? p(esc(bank)) : "") +
        (account ? p(esc(account)) : "") +
        linesTable(o) +
        totalsTable(o) +
        addressBlock(o.shippingAddress),
    ),
    text: [`Thanks for your order ${o.displayId}${o.poNumber ? `, PO ${o.poNumber}` : ""}.`, bank, account, linesText(o), totalsText(o)]
      .filter(Boolean)
      .join("\n\n"),
  }
}

export type ShipmentView = { shop: string; shopUrl: string; displayId: string; tracking: { number: string; url?: string | null }[] }

export const orderShipped = (s: ShipmentView): Rendered => {
  const tracks = s.tracking.filter((t) => t.number)
  return {
    subject: `Order ${s.displayId} is on its way`,
    html: layout(
      s.shop,
      s.shopUrl,
      `Order ${s.displayId} shipped`,
      h1("Your order is on its way") +
        p(`Order <strong>${esc(s.displayId)}</strong> has been sent.`) +
        tracks.map((t) => (t.url ? button(t.url, `Track ${t.number}`) : p(`Tracking number ${esc(t.number)}`))).join(""),
    ),
    text: [`Order ${s.displayId} has been sent.`, ...tracks.map((t) => `Tracking ${t.number}${t.url ? `: ${t.url}` : ""}`)].join("\n"),
  }
}

export type CancelView = { shop: string; shopUrl: string; displayId: string }

export const orderCanceled = (c: CancelView): Rendered => ({
  subject: `Order ${c.displayId} cancelled`,
  html: layout(c.shop, c.shopUrl, `Order ${c.displayId} cancelled`, h1("Your order was cancelled") + p(`Order <strong>${esc(c.displayId)}</strong> has been cancelled. Any payment taken is refunded to the way you paid.`)),
  text: `Order ${c.displayId} has been cancelled. Any payment taken is refunded to the way you paid.`,
})

export type RefundView = { shop: string; shopUrl: string; displayId: string; amount: number; currency: string }

export const refunded = (r: RefundView): Rendered => ({
  subject: `Refund for order ${r.displayId}`,
  html: layout(r.shop, r.shopUrl, `Refund`, h1("We've refunded you") + p(`${esc(money(r.amount, r.currency))} for order <strong>${esc(r.displayId)}</strong> is on its way back to the way you paid. Banks can take a few days to show it.`)),
  text: `${money(r.amount, r.currency)} for order ${r.displayId} is on its way back to the way you paid. Banks can take a few days to show it.`,
})

export type CodeView = { shop: string; shopUrl: string; code: string; minutes: number }

export const signInCode = (c: CodeView): Rendered => ({
  subject: `${c.code} is your ${c.shop} sign-in code`,
  html: layout(
    c.shop,
    c.shopUrl,
    "Sign-in code",
    h1("Your sign-in code") +
      `<p style="margin:0 0 12px;font-size:32px;letter-spacing:6px;font-weight:700">${esc(c.code)}</p>` +
      p(`It works for ${c.minutes} minutes. If you didn't ask for it, you can ignore this email.`),
  ),
  text: `Your sign-in code is ${c.code}. It works for ${c.minutes} minutes. If you didn't ask for it, you can ignore this email.`,
})

export type ResetView = { shop: string; shopUrl: string; link: string; minutes: number }

export const passwordReset = (r: ResetView): Rendered => ({
  subject: `Set your ${r.shop} password`,
  html: layout(
    r.shop,
    r.shopUrl,
    "Set your password",
    h1("Set your password") + button(r.link, "Choose a password") + p(`The link works for ${r.minutes} minutes. If you didn't ask for it, you can ignore this email.`),
  ),
  text: `Choose a password: ${r.link}\nThe link works for ${r.minutes} minutes. If you didn't ask for it, you can ignore this email.`,
})

// An email a plugin rendered itself, such as trade ordering's approval requests.
export const rendered = (r: Rendered): Rendered => ({ subject: String(r.subject), html: String(r.html), text: String(r.text) })

export const templates = { orderPlaced, orderShipped, orderCanceled, refunded, signInCode, passwordReset, rendered }
export type TemplateName = keyof typeof templates
