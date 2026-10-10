// The platform calls the shop makes with its service token (CONTRACT.md §8): email and uploads.
// Every call has a deadline and a refusal is raised as a PlatformError with the platform's code.

export const env = (name: string, fallback?: string): string => {
  const v = process.env[name] ?? fallback
  if (v === undefined) throw new Error(`${name} is not set`)
  return v
}

// The app's own routes on the platform API, .../v1/orgs/<org>/apps/<app>, read from
// WHISK_QUEUE_URL, which is that address plus /events.
const platformUrl = () => env("WHISK_QUEUE_URL").replace(/\/events$/, "")

export class PlatformError extends Error {
  readonly status: number
  readonly code: string
  readonly fix: string
  readonly details: Record<string, unknown>
  constructor(status: number, code: string, message: string, fix: string, details: Record<string, unknown> = {}) {
    super(message)
    this.name = "PlatformError"
    this.status = status
    this.code = code
    this.fix = fix
    this.details = details
  }
}

// The service token is read on every call, since it rotates.
const platformCall = async <T>(method: string, path: string, waitMs: number, body?: unknown): Promise<T> => {
  const res = await fetch(platformUrl() + path, {
    method,
    headers: { "content-type": "application/json", authorization: `Bearer ${env("WHISK_SERVICE_TOKEN")}` },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(waitMs),
  })
  const text = await res.text()
  if (!res.ok) {
    const e = ((): { code?: string; message?: string; fix?: string; details?: Record<string, unknown> } => {
      try {
        return (JSON.parse(text) as { error?: object }).error ?? {}
      } catch {
        return {}
      }
    })()
    if (!e.code) throw new Error(`${method} ${path}: ${res.status} ${text.slice(0, 300)}`)
    throw new PlatformError(res.status, e.code, e.message ?? "", e.fix ?? "", e.details ?? {})
  }
  return (text ? JSON.parse(text) : undefined) as T
}

export type Attachment = { filename: string; content_type?: string; content: string }
export type Email = { to: string; subject: string; html: string; text: string; from?: string; attachments?: Attachment[] }

// sendEmail sends one transactional email from the shop's own address. A 429 carries
// Retry-After; it is raised for the caller (a subscriber, which Medusa retries, or the sweep).
export const sendEmail = (mail: Email): Promise<{ id: string }> => platformCall("POST", "/email/send", 20_000, mail)

export type Upload = { id: string; key: string; url: string; fields: Record<string, string>; expires_at: string }

// uploadFile stores one file through the platform's upload form (the bucket is the business's,
// the object is virus-scanned) and answers its id. Product pictures are public: anyone may see
// them, at /.whisk/img/<id>.
export const uploadFile = async (
  file: { filename: string; contentType: string; content: Buffer },
  visibility: "public" | "private",
): Promise<string> => {
  const form = await platformCall<Upload>("POST", "/uploads", 15_000, {
    filename: file.filename,
    content_type: file.contentType,
    max_bytes: Math.max(file.content.length, 1),
    visibility,
  })
  const body = new FormData()
  for (const [k, v] of Object.entries(form.fields)) body.append(k, v)
  body.append("file", new Blob([new Uint8Array(file.content)], { type: file.contentType }), file.filename)
  const res = await fetch(form.url, { method: "POST", body, signal: AbortSignal.timeout(120_000) })
  if (!res.ok) throw new Error(`upload ${file.filename}: ${res.status} ${(await res.text()).slice(0, 300)}`)
  return form.id
}

// setVisibility makes an upload public or private. An app cannot delete an upload (only a
// person can), so a picture the shop no longer uses is made private: it stops being served.
export const setVisibility = (id: string, visibility: "public" | "private"): Promise<unknown> =>
  platformCall("PATCH", `/uploads/${encodeURIComponent(id)}`, 15_000, { visibility })

export const readUpload = (id: string): Promise<{ url: string }> =>
  platformCall("GET", `/uploads/${encodeURIComponent(id)}`, 15_000)
