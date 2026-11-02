import type { NextConfig } from "next";

const config: NextConfig = {
  // The build writes a server with only the files it runs, which the image copies.
  output: "standalone",
  // Loaded from node_modules at run time rather than bundled: they read files of their own or
  // start workers.
  serverExternalPackages: ["pino", "@sentry/node", "@opentelemetry/sdk-trace-node", "@opentelemetry/instrumentation-undici"],
  poweredByHeader: false,
};

export default config;
