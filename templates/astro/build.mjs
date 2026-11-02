// Bundles the migration into dist/migrate.js, one file with its dependencies inside, so the
// image runs it without the build's node_modules. `astro build` builds the app itself.
import { build } from "esbuild";

await build({
  entryPoints: ["src/migrate.ts"],
  outdir: "dist",
  bundle: true,
  platform: "node",
  format: "esm",
  target: "node22",
  legalComments: "none",
  logLevel: "warning",
  banner: { js: "import { createRequire as whiskCreateRequire } from 'node:module'; const require = whiskCreateRequire(import.meta.url);" },
});
