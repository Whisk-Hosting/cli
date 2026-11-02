# Builds without a Dockerfile too (Railpack); this one exists to show the shape.
FROM node:24-trixie-slim AS build
WORKDIR /app
COPY package.json package-lock.json ./
# npm's cache persists between builds, so unchanged dependencies are not downloaded again.
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
# The pages and the server into dist/, dist/migrate.js beside them, then only what the server
# runs with is kept.
RUN npm run build
RUN --mount=type=cache,target=/root/.npm npm prune --omit=dev

# The app runs on Google's distroless Node image: Node and the system libraries it needs, with no
# shell, package manager or other tools, so far fewer packages ever need a security update.
FROM gcr.io/distroless/nodejs24-debian13:nonroot
# The image starts node itself; on the PATH it also runs the migrate command from whisk.yaml.
ENV PATH=/nodejs/bin:/usr/local/bin:/usr/bin:/bin
ENV NODE_ENV=production
ENV HOST=0.0.0.0
WORKDIR /app
COPY --from=build /app/dist ./dist
COPY --from=build /app/node_modules ./node_modules
COPY package.json ./
COPY drizzle ./drizzle
USER nonroot
EXPOSE 8080
CMD ["dist/server/entry.mjs"]
