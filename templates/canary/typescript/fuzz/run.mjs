// Runs the Jazzer.js fuzz targets in fuzz/targets, coverage-guided fuzzing of the code that takes
// input from outside. Each target is bundled with esbuild into fuzz/.build (packages stay
// external, so Jazzer.js instruments only this app's own code) and handed to the jazzer CLI.
//
//   node fuzz/run.mjs                 fuzz every target for FUZZ_SECONDS (default 30) each
//   node fuzz/run.mjs --regression    run every target once over its seed corpus, no fuzzing
//   node fuzz/run.mjs whisk           only the targets named
//
// The seeds are the Big List of Naughty Strings (contract/naughty/blns.json when the contract sits
// beside this template, otherwise a few of its worst inline) plus the inputs in
// fuzz/corpus/<target>/, where every crash a run found is kept. New inputs a fuzzing run finds go
// to fuzz/.build/corpus/<target>/ and crashes to fuzz/.build/crashes/.

import { spawn } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { availableParallelism } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { build } from "esbuild";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..");
const out = path.join(here, ".build");
const require = createRequire(import.meta.url);
const jazzer = require.resolve("@jazzer.js/core/dist/cli.js");

const args = process.argv.slice(2);
const regression = args.includes("--regression");
const only = args.filter((a) => !a.startsWith("--"));
const seconds = Number(process.env.FUZZ_SECONDS ?? 30);
if (!Number.isFinite(seconds) || seconds <= 0) {
  console.error(`FUZZ_SECONDS must be a positive number of seconds, not "${process.env.FUZZ_SECONDS}".`);
  process.exit(2);
}

const targets = readdirSync(path.join(here, "targets"))
  .filter((f) => /\.fuzz\.tsx?$/.test(f))
  .map((f) => ({ name: f.replace(/\.fuzz\.tsx?$/, ""), file: path.join(here, "targets", f) }))
  .filter((t) => only.length === 0 || only.includes(t.name));
if (targets.length === 0) {
  console.error(`No fuzz target named ${only.join(", ")}.`);
  process.exit(2);
}

// The naughty strings, one file each, written once per run.
const blns = path.join(out, "seeds", "blns");
mkdirSync(blns, { recursive: true });
const list = path.resolve(root, "..", "..", "..", "contract", "naughty", "blns.json");
const naughty = existsSync(list)
  ? JSON.parse(readFileSync(list, "utf8"))
  : ["", "undefined", "__proto__", "<script>alert(1)</script>", "' OR 1=1 --", "%s%n", "\u202e", "Ω≈ç√∫", "😍", "../../etc/passwd"];
naughty.forEach((s, i) => writeFileSync(path.join(blns, String(i).padStart(4, "0")), s));

await build({
  entryPoints: targets.map((t) => t.file),
  outdir: out,
  outExtension: { ".js": ".cjs" },
  entryNames: "[name]",
  bundle: true,
  platform: "node",
  format: "cjs",
  target: "node22",
  packages: "external",
  sourcemap: "inline",
  tsconfig: path.join(root, "tsconfig.json"),
  logLevel: "warning",
});

// run starts one target and answers whether it failed. A run over the seed corpus keeps its
// output unless it fails; a fuzzing run shows libFuzzer's progress as it goes.
const run = (t) =>
  new Promise((resolve) => {
    const kept = path.join(here, "corpus", t.name);
    const found = path.join(out, "corpus", t.name);
    const crashes = path.join(out, "crashes");
    mkdirSync(found, { recursive: true });
    mkdirSync(crashes, { recursive: true });
    const corpus = [...(regression ? [] : [found]), ...(existsSync(kept) ? [kept] : []), blns];
    const engine = [
      `-artifact_prefix=${crashes}/${t.name}-`,
      ...(regression ? [] : [`-max_total_time=${seconds}`, "-max_len=8192", "-print_final_stats=1"]),
    ];
    // A target that awaits runs in Jazzer.js's async mode, which settles promises between runs.
    const sync = /export const fuzz = async/.test(readFileSync(t.file, "utf8")) ? [] : ["--sync"];
    if (!regression) console.log(`== fuzz ${t.name} (${seconds}s)`);
    const child = spawn(
      process.execPath,
      [jazzer, path.join(out, `${t.name}.fuzz.cjs`), ...corpus, ...sync, "--timeout=5000", `--mode=${regression ? "regression" : "fuzzing"}`, "--", ...engine],
      { cwd: root, stdio: regression ? ["ignore", "pipe", "pipe"] : "inherit" },
    );
    const output = [];
    child.stdout?.on("data", (d) => output.push(d));
    child.stderr?.on("data", (d) => output.push(d));
    child.on("close", (code, signal) => {
      if (regression) console.log(`== fuzz ${t.name} (seed corpus): ${code === 0 ? "ok" : "FAILED"}`);
      if (code !== 0) {
        process.stderr.write(Buffer.concat(output));
        console.error(`fuzz ${t.name} failed (exit ${code ?? signal})`);
      }
      resolve(code !== 0);
    });
  });

// Seed corpus runs are short and run side by side, one per CPU; fuzzing runs one at a time.
const lanes = regression ? Math.max(1, availableParallelism()) : 1;
const queue = [...targets];
const results = [];
await Promise.all(
  Array.from({ length: lanes }, async () => {
    for (let t = queue.shift(); t; t = queue.shift()) results.push({ name: t.name, failed: await run(t) });
  }),
);

const failed = results.filter((r) => r.failed).map((r) => r.name);
if (failed.length) {
  console.error(`Fuzz targets failed: ${failed.join(", ")}`);
  process.exit(1);
}
