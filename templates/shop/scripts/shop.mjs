// The shop's build, start and migrate steps, in Node so they run the same in PowerShell,
// Command Prompt and a Unix shell.
//   node scripts/shop.mjs build     the Medusa server and admin, then the storefront, the website
//                                   (site/) and the starting catalogue (catalogue/) beside them
//   node scripts/shop.mjs start     the built shop (.medusa/server)
//   node scripts/shop.mjs migrate   the built shop's migrations and setup
//   node scripts/shop.mjs dev       the storefront and website built once, then Medusa watching
//                                   the source
import { spawnSync } from "node:child_process"
import { cpSync, existsSync, rmSync } from "node:fs"
import { createRequire } from "node:module"
import path from "node:path"
import { fileURLToPath } from "node:url"

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..")
const server = path.join(root, ".medusa", "server")
const require = createRequire(path.join(root, "package.json"))
// Each tool's own command, run with this Node.
const bin = (pkg) => {
  const manifest = require.resolve(`${pkg}/package.json`)
  const { bin } = require(manifest)
  return path.join(path.dirname(manifest), typeof bin === "string" ? bin : Object.values(bin)[0])
}
const medusa = bin("@medusajs/cli")
const astro = bin("astro")

const run = (args, cwd = root) => {
  const r = spawnSync(process.execPath, args, { cwd, stdio: "inherit" })
  if (r.status !== 0) process.exit(r.status ?? 1)
}

const buildStorefront = () => run([astro, "build", "--root", "storefront"])

// The business's website (site/), when there is one: its own Astro app with its own packages,
// built to site/dist, which the shop serves at every address that is not the shop's.
const site = path.join(root, "site")
const npm = (args) => {
  const r = spawnSync("npm", args, { cwd: site, stdio: "inherit", shell: process.platform === "win32" })
  if (r.status !== 0) process.exit(r.status ?? 1)
}
const buildSite = () => {
  if (!existsSync(path.join(site, "package.json"))) return
  npm(existsSync(path.join(site, "package-lock.json")) ? ["ci"] : ["install"])
  npm(["run", "build"])
}

const steps = {
  build: () => {
    run([medusa, "build"])
    buildStorefront()
    rmSync(path.join(server, "storefront"), { recursive: true, force: true })
    cpSync(path.join(root, "storefront", "dist"), path.join(server, "storefront", "dist"), { recursive: true })
    buildSite()
    rmSync(path.join(server, "site"), { recursive: true, force: true })
    for (const part of ["dist", "redirects.json"]) {
      if (existsSync(path.join(site, part))) cpSync(path.join(site, part), path.join(server, "site", part), { recursive: true })
    }
    rmSync(path.join(server, "catalogue"), { recursive: true, force: true })
    if (existsSync(path.join(root, "catalogue"))) cpSync(path.join(root, "catalogue"), path.join(server, "catalogue"), { recursive: true })
  },
  start: () => run([medusa, "start"], server),
  migrate: () => run(["migrate.js"], server),
  dev: () => {
    buildStorefront()
    buildSite()
    run([medusa, "develop"])
  },
}

const step = steps[process.argv[2]]
if (!step) {
  console.error(`usage: node scripts/shop.mjs ${Object.keys(steps).join("|")}`)
  process.exit(2)
}
step()
