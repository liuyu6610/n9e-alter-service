# syntax=docker/dockerfile:1

FROM node:20-alpine AS web-builder
WORKDIR /src
COPY web/package.json web/package-lock.json web/tsconfig.json web/vite.config.ts ./web/
COPY web/app ./web/app
RUN cd web && npm ci
RUN cd web && npm run build

FROM golang:1.20-alpine AS go-builder
WORKDIR /src
ARG TARGETOS=linux
ARG TARGETARCH=amd64
COPY go.mod go.sum ./
COPY main.go ./
COPY internal ./internal
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod download
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o /out/n9e-alter-service .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=go-builder /out/n9e-alter-service /app/n9e-alter-service
COPY --from=web-builder /src/web/dist /app/web/dist
COPY config.example.json /app/config.example.json
RUN addgroup -S app && adduser -S -G app -u 10001 app \
  && mkdir -p /app/data \
  && chown -R app:app /app
ENV SERVICE_ADDR=:8080
ENV WEB_DIR=/app/web/dist
ENV DATA_DIR=/app/data
EXPOSE 8080
USER 10001

# Optional: Docker runtime healthcheck (Kubernetes should use probes instead).
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/n9e-alter-service"]
CMD ["-config", "/app/config.example.json"]
