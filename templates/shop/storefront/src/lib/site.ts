// The business's website, built from site/ into site/dist and served beside the shop on one
// address (src/api/middlewares.ts): which addresses its pages answer, for the sitemap.
import { promises as fs } from "node:fs"
import path from "node:path"

// The address a built page answers: index.html is /, about.html is /about, about/index.html is
// /about/. Error pages and files that are not pages have none.
export const siteAddress = (file: string): string | null => {
  const rel = file.split(path.sep).join("/").replace(/^\/+/, "")
  if (!/\.html?$/i.test(rel) || /^(404|500)\.html?$/i.test(rel) || rel.split("/").some((part) => part.startsWith(".") || part === "_astro")) return null
  if (/(^|\/)index\.html?$/i.test(rel)) return "/" + rel.replace(/index\.html?$/i, "")
  return "/" + rel.replace(/\.html?$/i, "")
}

// Every page address the website answers, sorted; none when there is no website.
export const siteAddresses = async (root = path.resolve(process.cwd(), "site/dist")): Promise<string[]> => {
  const files = (await fs.readdir(root, { recursive: true }).catch(() => [] as string[])) as string[]
  return files.map(siteAddress).filter((a): a is string => a !== null).sort()
}
