// Pure helpers for the file provider: how an upload is named in Medusa and where it is seen.

// A platform upload is recorded in Medusa as "upload:<id>", so the provider can tell it from an
// object key in the bucket, which never has a colon.
export const uploadKey = (id: string) => `upload:${id}`
export const isUploadKey = (key: string) => key.startsWith("upload:")
export const uploadId = (key: string) => key.slice("upload:".length)

// Pictures are served resized by the edge; anything else public by its media address.
export const imageUrl = (publicUrl: string, id: string, mimeType: string) =>
  `${publicUrl.replace(/\/$/, "")}${mimeType.startsWith("image/") ? "/.whisk/img/" : "/.whisk/media/"}${id}`

const TEXT = /^text\/|csv|json|xml/

// Medusa hands a provider the file as a string: base64, or for text the text itself, or a
// "binary" string of bytes. The same rule as Medusa's own S3 provider.
export const decodeContent = (content: string, mimeType: string): Buffer => {
  const b64 = Buffer.from(content, "base64")
  if (b64.toString("base64") === content) return b64
  return TEXT.test(mimeType ?? "") ? Buffer.from(content, "utf8") : Buffer.from(content, "binary")
}
