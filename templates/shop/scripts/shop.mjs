// The shop's build, start and migrate steps, in Node so they run the same in PowerShell,
// Command Prompt and a Unix shell.
//   node scripts/shop.mjs build     the Medusa server and admin, then the storefront beside them
//   node scripts/shop.mjs start     the built shop (.medusa/server)
//   node scripts/shop.mjs migrate   the built shop's migrations and setup
//   node scripts/shop.mjs dev       the storefront built once, then Medusa watching the source
import { spawnSync } from "node:child_process"
import { cpSync, rmSync } from "node:fs"
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

const steps = {
  build: () => {
    run([medusa, "build"])
    buildStorefront()
    rmSync(path.join(server, "storefront"), { recursive: true, force: true })
    cpSync(path.join(root, "storefront", "dist"), path.join(server, "storefront", "dist"), { recursive: true })
  },
  start: () => run([medusa, "start"], server),
  migrate: () => run(["migrate.js"], server),
  dev: () => {
    buildStorefront()
    run([medusa, "develop"])
  },
}

const step = steps[process.argv[2]]
if (!step) {
  console.error(`usage: node scripts/shop.mjs ${Object.keys(steps).join("|")}`)
  process.exit(2)
}
step()
