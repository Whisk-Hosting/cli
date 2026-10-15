# Builds without a Dockerfile too (Railpack); this one exists to show the shape.
FROM node:24-trixie-slim AS build
WORKDIR /app
COPY package.json package-lock.json ./
# npm's cache persists between builds, so unchanged dependencies are not downloaded again.
RUN --mount=type=cache,target=/root/.npm npm ci
COPY tsconfig.json build.mjs ./
COPY src ./src
# The build bundles each entry point into one file with its dependencies, so the app needs no
# node_modules at run time.
RUN npm run build

# The app runs on Google's distroless Node image: Node and the system libraries it needs, with no
# shell, package manager or other tools, so far fewer packages ever need a security update.
FROM gcr.io/distroless/nodejs24-debian13:nonroot
# The image starts node itself; on the PATH it also runs the migrate command from whisk.yaml.
ENV PATH=/nodejs/bin:/usr/local/bin:/usr/bin:/bin
ENV NODE_ENV=production
# Errors point at the TypeScript source, through the source maps next to the bundles.
ENV NODE_OPTIONS=--enable-source-maps
WORKDIR /app
COPY --from=build /app/dist ./dist
COPY package.json ./
COPY drizzle ./drizzle
USER nonroot
EXPOSE 8080
CMD ["dist/index.js"]
