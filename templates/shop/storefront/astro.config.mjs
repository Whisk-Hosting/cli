// The storefront: server-rendered pages that Medusa's own server hands every address that is not
// its API (src/api/middlewares.ts). The moving service rebuilds a shop's design here.
import { defineConfig } from "astro/config"
import node from "@astrojs/node"

export default defineConfig({
  output: "server",
  adapter: node({ mode: "middleware" }),
  // Forms post to the same address; the edge already refuses cross-site posts (CADDY.md §5.1).
  security: { checkOrigin: false },
  build: { inlineStylesheets: "auto" },
  compressHTML: true,
})
