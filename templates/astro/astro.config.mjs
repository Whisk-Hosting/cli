import node from "@astrojs/node";
import { defineConfig } from "astro/config";

// Every page is drawn for each request by the Node server the build writes (dist/server).
export default defineConfig({
  output: "server",
  adapter: node({ mode: "standalone" }),
  server: { host: "0.0.0.0", port: Number(process.env.PORT ?? 8080) },
  telemetry: false,
});
