# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25.11
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

ARG VERSION
ARG COMMIT
ARG BUILD_TIME

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN build_time="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
    && CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w \
            -X backend/internal/buildinfo.Version=${VERSION} \
            -X backend/internal/buildinfo.Commit=${COMMIT} \
            -X backend/internal/buildinfo.BuildTime=${build_time}" \
        -o /out/app-api \
        .

# 当前保留 alpine 是为了内置 HEALTHCHECK 使用 wget。
# 若后续切到 distroless，需要改为外部健康检查或单独提供探针工具。
FROM alpine:3.22 AS runtime

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app \
    && adduser -S -G app app

COPY --from=builder /out/app-api /app/app-api
COPY config /app/config
# migrations 不打入 API runtime 镜像；数据库迁移应由宿主机、CI/CD
# 或 Kubernetes initContainer/job 独立执行，避免 API 启动时隐式改库。

ENV APP_ENV=production
EXPOSE 8080

USER app

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1

ENTRYPOINT ["/app/app-api"]
