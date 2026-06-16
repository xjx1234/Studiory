BACKEND_DIR := backend
DATABASE_URL ?= postgres://postgres:password@localhost:5432/app?sslmode=disable
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X backend/internal/buildinfo.Version=$(VERSION) -X backend/internal/buildinfo.Commit=$(COMMIT) -X backend/internal/buildinfo.BuildTime=$(BUILD_TIME)

.PHONY: help up down logs run build test cover lint vuln sqlc migrate-up migrate-down compose-up compose-down fmt tidy seed

help:
	@echo "Available targets:"
	@echo "  make up            Start full stack (API + PostgreSQL + Redis + auto migrate)"
	@echo "  make down          Stop full stack"
	@echo "  make logs          Tail API container logs"
	@echo "  make compose-up    Start only PostgreSQL and Redis (for local 'make run')"
	@echo "  make compose-down  Stop local services"
	@echo "  make migrate-up    Run database migrations"
	@echo "  make migrate-down  Roll back one migration"
	@echo "  make sqlc          Generate sqlc code"
	@echo "  make seed          Init admin user (set SEED_ADMIN_* env vars)"
	@echo "  make run           Run backend"
	@echo "  make build         Build backend"
	@echo "  make test          Run Go tests"
	@echo "  make cover         Run tests with coverage summary"
	@echo "  make lint          Run golangci-lint (install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)"
	@echo "  make vuln          Run govulncheck (install: go install golang.org/x/vuln/cmd/govulncheck@latest)"
	@echo "  make fmt           Format Go code"
	@echo "  make tidy          Tidy Go modules"

# 全栈一键启动：构建 API 镜像、自动跑迁移、启动 API+PG+Redis
up:
	docker compose --profile full up -d --build

down:
	docker compose --profile full down

logs:
	docker compose logs -f api

compose-up:
	docker compose up -d postgres redis

compose-down:
	docker compose down

migrate-up:
	migrate -path $(BACKEND_DIR)/migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path $(BACKEND_DIR)/migrations -database "$(DATABASE_URL)" down 1

sqlc:
	cd $(BACKEND_DIR) && sqlc generate

run:
	cd $(BACKEND_DIR) && go run .

build:
	cd $(BACKEND_DIR) && go build -trimpath -ldflags "$(LDFLAGS)" -o /tmp/app-api .

test:
	cd $(BACKEND_DIR) && go test ./...

cover:
	cd $(BACKEND_DIR) && go test -race -covermode=atomic -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

lint:
	cd $(BACKEND_DIR) && golangci-lint run ./...

vuln:
	cd $(BACKEND_DIR) && govulncheck ./...

fmt:
	cd $(BACKEND_DIR) && gofmt -w .

tidy:
	cd $(BACKEND_DIR) && go mod tidy

seed:
	cd $(BACKEND_DIR) && go run ./cmd/seed
