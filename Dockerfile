# syntax=docker/dockerfile:1.7

FROM node:22-bookworm-slim AS web-builder
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@11.16.0 --activate
COPY web/vben-admin/ ./web/vben-admin/
COPY scripts/ ./scripts/
COPY web/overrides/ ./web/overrides/
RUN ./scripts/apply-vben-overrides.sh
WORKDIR /src/web/vben-admin
RUN pnpm install --frozen-lockfile
RUN pnpm --filter @vben/web-antd build

FROM golang:1.23-alpine AS go-builder
WORKDIR /src
COPY . .
COPY --from=web-builder /src/web/vben-admin/apps/web-antd/dist ./web/dist
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tg-card ./cmd/server

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
