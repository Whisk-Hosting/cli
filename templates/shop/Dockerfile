# Medusa's server and admin, and the storefront beside them, in one image.

# Trade ordering: empty here, so this is a plain public shop. Whisk fills this stage with its
# private @whisk/shop-b2b plugin for a Promoted app whose whisk.yaml says b2b: true.
FROM scratch AS whisk-shop-b2b

FROM node:24-trixie-slim AS build
WORKDIR /app
COPY package.json package-lock.json ./
# npm's cache persists between builds, so unchanged dependencies are not downloaded again.
RUN --mount=type=cache,target=/root/.npm npm ci
COPY --from=whisk-shop-b2b / node_modules/@whisk/shop-b2b/
COPY . .
RUN --mount=type=cache,target=/root/.npm npm run build
# The built server (.medusa/server) carries its own package.json and lock: install only what it
# runs with.
WORKDIR /app/.medusa/server
RUN --mount=type=cache,target=/root/.npm npm ci --omit=dev
COPY --from=whisk-shop-b2b / node_modules/@whisk/shop-b2b/

# The shop runs on Google's distroless Node image: Node and the system libraries it needs, with
# no shell, package manager or other tools.
FROM gcr.io/distroless/nodejs24-debian13:nonroot
# The image starts node itself; on the PATH it also runs the migrate command from whisk.yaml.
ENV PATH=/nodejs/bin:/usr/local/bin:/usr/bin:/bin
ENV NODE_ENV=production
WORKDIR /app
COPY --from=build /app/.medusa/server ./
USER nonroot
EXPOSE 8080
CMD ["node_modules/@medusajs/cli/cli.js", "start"]
