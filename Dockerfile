# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend .
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.23-bookworm AS backend-builder
RUN apt-get update && apt-get install -y --no-install-recommends build-essential gcc-aarch64-linux-gnu gcc-x86-64-linux-gnu libc6-dev-arm64-cross libc6-dev-amd64-cross && rm -rf /var/lib/apt/lists/*
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend .
ARG TARGETPLATFORM
ARG BUILDPLATFORM
RUN CGO_ENABLED=1 GOOS=$(echo $TARGETPLATFORM | cut -d'/' -f1) GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) \
    CC=$(if [ "$(echo $TARGETPLATFORM | cut -d'/' -f2)" = "arm64" ]; then echo "aarch64-linux-gnu-gcc"; else echo "x86_64-linux-gnu-gcc"; fi) \
    go build -o /out/sapp ./cmd/sapp
RUN CGO_ENABLED=1 GOOS=$(echo $TARGETPLATFORM | cut -d'/' -f1) GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) \
    CC=$(if [ "$(echo $TARGETPLATFORM | cut -d'/' -f2)" = "arm64" ]; then echo "aarch64-linux-gnu-gcc"; else echo "x86_64-linux-gnu-gcc"; fi) \
    go build -o /out/migrate ./cmd/migrate

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* && mkdir -p /data
WORKDIR /app
COPY --from=backend-builder /out/sapp /usr/local/bin/sapp
COPY --from=backend-builder /out/migrate /usr/local/bin/migrate
COPY --from=frontend-builder /app/frontend/dist /app/static
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
ENV DATABASE_PATH=/data/sapp.db \
    STATIC_DIR=/app/static \
    PORT=3000
EXPOSE 3000
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["sapp"]
