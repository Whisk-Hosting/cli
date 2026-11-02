# Builds without a Dockerfile too (Railpack); this one exists to show the shape.
FROM node:24-trixie-slim AS build
WORKDIR /app
COPY package.json package-lock.json ./
# npm's cache persists between builds, so unchanged dependencies are not downloaded again.
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
# Next.js's own cache persists too, so a change rebuilds only what it touches. The build writes
# a standalone server with only the files it runs, and dist/migrate.js beside it.
RUN --mount=type=cache,target=/app/.next/cache npm run build

# The app runs on Google's distroless Node image: Node and the system libraries it needs, with no
# shell, package manager or other tools, so far fewer packages ever need a security update.
FROM gcr.io/distroless/nodejs24-debian13:nonroot
# The image starts node itself; on the PATH it also runs the migrate command from whisk.yaml.
ENV PATH=/nodejs/bin:/usr/local/bin:/usr/bin:/bin
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
# The app flushes its spans and errors on SIGTERM itself (src/instrumentation.node.ts).
ENV NEXT_MANUAL_SIG_HANDLE=true
ENV NEXT_TELEMETRY_DISABLED=1
WORKDIR /app
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/dist ./dist
COPY drizzle ./drizzle
USER nonroot
EXPOSE 8080
CMD ["server.js"]
