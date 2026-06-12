# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25.8

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/app-api \
    .

FROM alpine:3.22 AS runtime

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app \
    && adduser -S -G app app

COPY --from=builder /out/app-api /app/app-api
COPY config /app/config

ENV APP_ENV=production
EXPOSE 8080

USER app

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1

ENTRYPOINT ["/app/app-api"]
