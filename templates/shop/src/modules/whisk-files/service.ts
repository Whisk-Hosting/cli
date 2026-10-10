// Files on Whisk. Public files (product and category pictures) go through the platform's
// uploads, so each is virus-scanned and served at /.whisk/img/<id> in whatever size the page
// asks for, cached at the edge. Private files (imports and exports) stay in the business's own
// bucket under the app's prefix, through the S3 provider Medusa ships.
import { S3FileService } from "@medusajs/file-s3/dist/services/s3-file"
import type { FileTypes } from "@medusajs/framework/types"
import { Readable } from "node:stream"
import { readUpload, setVisibility, uploadFile } from "../../lib/whisk"
import { decodeContent, imageUrl, isUploadKey, uploadId, uploadKey } from "../../lib/files"

type Options = Record<string, unknown> & { public_url: string }

export default class WhiskFileService extends S3FileService {
  static identifier = "whisk"
  private readonly publicUrl_: string

  constructor(deps: { logger: any }, options: Options) {
    super(deps as any, { ...options, acl: false } as any)
    this.publicUrl_ = options.public_url
  }

  async upload(file: FileTypes.ProviderUploadFileDTO): Promise<FileTypes.ProviderFileResultDTO> {
    if (file.access !== "public") return super.upload(file)
    const id = await uploadFile(
      { filename: file.filename, contentType: file.mimeType, content: decodeContent(file.content, file.mimeType) },
      "public",
    )
    return { key: uploadKey(id), url: imageUrl(this.publicUrl_, id, file.mimeType) }
  }

  async delete(files: FileTypes.ProviderDeleteFileDTO | FileTypes.ProviderDeleteFileDTO[]): Promise<void> {
    const all = Array.isArray(files) ? files : [files]
    const uploads = all.filter((f) => isUploadKey(f.fileKey))
    const objects = all.filter((f) => !isUploadKey(f.fileKey))
    // Whisk keeps uploads until a person removes them; one the shop no longer uses stops
    // being served.
    await Promise.all(uploads.map((f) => setVisibility(uploadId(f.fileKey), "private")))
    if (objects.length) await super.delete(objects)
  }

  async getPresignedDownloadUrl(file: FileTypes.ProviderGetFileDTO): Promise<string> {
    if (!isUploadKey(file.fileKey)) return super.getPresignedDownloadUrl(file)
    return (await readUpload(uploadId(file.fileKey))).url
  }

  async getAsBuffer(file: FileTypes.ProviderGetFileDTO): Promise<Buffer> {
    if (!isUploadKey(file.fileKey)) return super.getAsBuffer(file)
    const res = await fetch(await this.getPresignedDownloadUrl(file), { signal: AbortSignal.timeout(60_000) })
    if (!res.ok) throw new Error(`read ${file.fileKey}: ${res.status}`)
    return Buffer.from(await res.arrayBuffer())
  }

  async getDownloadStream(file: FileTypes.ProviderGetFileDTO): Promise<Readable> {
    if (!isUploadKey(file.fileKey)) return super.getDownloadStream(file)
    return Readable.from(await this.getAsBuffer(file))
  }
}
