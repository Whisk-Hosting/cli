# Builds without a Dockerfile too (Railpack); this one exists to show the shape.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
# The module and build caches persist between builds, so a change to the app's own code
# compiles in seconds rather than rebuilding every dependency.
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server .

FROM gcr.io/distroless/static-debian13:nonroot
WORKDIR /app
COPY --from=build /out/server ./server
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/server"]
