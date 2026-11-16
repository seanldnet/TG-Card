# syntax=docker/dockerfile:1.7

FROM node:22-bookworm-slim AS web-builder
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@11.16.0 --activate
COPY web/vben-admin/ ./web/vben-admin/
COPY scripts/ ./scripts/
COPY web/overrides/ ./web/overrides/
RUN ./scripts/apply-vben-overrides.sh
WORKDIR /src/web/vben-admin
# The upstream Vben submodule pins npmmirror.com in its project .npmrc.
# Override it inside the image build so GitHub Actions uses the official npm registry.
RUN printf 'registry=https://registry.npmjs.org/\n' > .npmrc
RUN --mount=type=cache,id=tg-card-pnpm-store,target=/root/.local/share/pnpm/store CI=true pnpm install --frozen-lockfile --fetch-retries=5 --fetch-timeout=120000
RUN pnpm --filter @vben/web-antd build

# Build Go for the requested target architecture without emulating the target CPU.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS go-builder
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src
# Use the exact dependency graph validated by Go CI; do not run go mod tidy in
# the image build (it also resolves test-only dependencies and adds an avoidable
# dependency on checksum-database availability during multi-arch builds).
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /src/web/vben-admin/apps/web-antd/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/tg-card ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && addgroup -S tgcard && adduser -S -G tgcard tgcard
WORKDIR /app
COPY --from=go-builder /out/tg-card /app/tg-card
COPY --from=go-builder /src/web/dist /app/web/dist
RUN mkdir -p /app/data && chown -R tgcard:tgcard /app
USER tgcard
ENV APP_ENV=production PORT=38647 DATA_DIR=/app/data WEB_DIST_DIR=/app/web/dist
EXPOSE 38647
VOLUME ["/app/data"]
ENTRYPOINT ["/app/tg-card"]
