# syntax=docker/dockerfile:1

FROM node:20-alpine AS web-builder
WORKDIR /src
COPY web/package.json web/tsconfig.json web/vite.config.ts ./web/
COPY web/app ./web/app
RUN cd web && npm install
RUN cd web && npm run build

FROM golang:1.20-alpine AS go-builder
WORKDIR /src
COPY go.mod ./
COPY main.go ./
COPY internal ./internal
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/n9e-alter-service .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=go-builder /out/n9e-alter-service /app/n9e-alter-service
COPY --from=web-builder /src/web/dist /app/web/dist
COPY config.example.json /app/config.json
COPY config.example.json /app/config.example.json
RUN mkdir -p /app/data
ENV SERVICE_ADDR=:8080
ENV WEB_DIR=/app/web/dist
ENV DATA_DIR=/app/data
EXPOSE 8080
ENTRYPOINT ["/app/n9e-alter-service"]
CMD ["-config", "/app/config.json"]
