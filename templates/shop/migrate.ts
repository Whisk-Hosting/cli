// The migrate step (whisk.yaml): Medusa's migrations, then the shop's setup. It runs with the
// direct database connection, before traffic moves to the new version.
import { spawnSync } from "node:child_process"

const run = (args: string[]) => {
  const r = spawnSync(process.execPath, [require.resolve("@medusajs/cli/cli.js"), ...args], { stdio: "inherit", env: process.env })
  if (r.status !== 0) process.exit(r.status ?? 1)
}

run(["db:migrate"])
run(["exec", "./src/scripts/setup.js"])
