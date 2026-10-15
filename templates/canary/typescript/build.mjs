// Bundles the app into dist/index.js and dist/migrate.js, each one file with its dependencies
// inside, so the image holds no node_modules and Node starts without reading thousands of
// files. The type check is `npm run typecheck`; this only builds.
import { build } from "esbuild";

await build({
  entryPoints: ["src/index.ts", "src/migrate.ts"],
  outdir: "dist",
  bundle: true,
  platform: "node",
  format: "esm",
  target: "node22",
  sourcemap: "linked",
  legalComments: "none",
  logLevel: "warning",
  // Some dependencies are CommonJS and call require(); an ES module bundle has none of its own.
  // The import is renamed because bundled ES modules (Sentry's) import createRequire themselves.
  banner: { js: "import { createRequire as whiskCreateRequire } from 'node:module'; const require = whiskCreateRequire(import.meta.url);" },
});
