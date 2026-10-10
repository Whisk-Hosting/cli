// The migrate step (whisk.yaml): Medusa's migrations, then the shop's setup. It runs with the
// direct database connection and none of the app's other platform environment (CONTRACT.md §5),
// before traffic moves to the new version; SHOP_STEP tells the config it signs nothing here.
// The starting catalogue is added by a job (src/jobs/catalogue.ts), which has the storage.
import { spawnSync } from "node:child_process"

const run = (args: string[]) => {
  const r = spawnSync(process.execPath, [require.resolve("@medusajs/cli/cli.js"), ...args], { stdio: "inherit", env: { ...process.env, SHOP_STEP: "migrate" } })
  if (r.status !== 0) process.exit(r.status ?? 1)
}

run(["db:migrate"])
run(["exec", "./src/scripts/setup.js"])
