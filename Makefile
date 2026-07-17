BACKEND_DIR := backend
DATABASE_URL ?= postgres://postgres:password@localhost:5432/app?sslmode=disable
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X backend/internal/buildinfo.Version=$(VERSION) -X backend/internal/buildinfo.Commit=$(COMMIT) -X backend/internal/buildinfo.BuildTime=$(BUILD_TIME)

.PHONY: help setup up down logs run build test cover test-integration test-e2e lint vuln sqlc openapi-check migrate-up migrate-down compose-up compose-down fmt tidy seed rename

help:
	@echo "Available targets:"
	@echo "  make setup         一键初始化本地开发环境（复制 .env + 起依赖 + 迁移）"
	@echo "  make up            Start full stack (API + PostgreSQL + Redis + auto migrate)"
	@echo "  make down          Stop full stack"
	@echo "  make logs          Tail API container logs"
	@echo "  make compose-up    Start only PostgreSQL and Redis (for local 'make run')"
	@echo "  make compose-down  Stop local services"
	@echo "  make migrate-up    Run database migrations"
	@echo "  make migrate-down  Roll back one migration"
	@echo "  make sqlc          Generate sqlc code"
	@echo "  make openapi-check Verify OpenAPI spec and router contract"
	@echo "  make seed          Init admin user (set SEED_ADMIN_* env vars)"
	@echo "  make run           Run backend"
	@echo "  make build         Build backend"
	@echo "  make test          Run Go tests"
	@echo "  make cover         Run tests with coverage summary"
	@echo "  make test-integration  Run repo/app integration tests (needs Docker)"
	@echo "  make test-e2e          Run HTTP E2E tests (needs Docker)"
	@echo "  make lint          Run golangci-lint (install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)"
	@echo "  make vuln          Run govulncheck (install: go install golang.org/x/vuln/cmd/govulncheck@latest)"
	@echo "  make fmt           Format Go code"
	@echo "  make tidy          Tidy Go modules"
	@echo "  make rename MODULE=<path>  Rename Go module from 'backend' to <path> (new project bootstrap)"

# 一键初始化本地开发环境：复制 .env → 起依赖 → 迁移
setup:
	@if [ ! -f $(BACKEND_DIR)/.env ]; then \
		cp $(BACKEND_DIR)/.env.example $(BACKEND_DIR)/.env; \
		echo "✅ 已创建 $(BACKEND_DIR)/.env，请按需修改 DATABASE_URL / REDIS_URL / JWT_SECRET"; \
	else \
		echo "ℹ️  $(BACKEND_DIR)/.env 已存在，跳过复制"; \
	fi
	$(MAKE) compose-up
	@echo "⏳ 等待 PostgreSQL 就绪..."
	@sleep 3
	$(MAKE) migrate-up
	@echo "✅ 初始化完成，运行 'make run' 启动后端"

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

openapi-check:
	cd $(BACKEND_DIR) && go test -run 'Test(OpenAPI|LoadDocument)' ./internal/openapi/... ./internal/http/...

run:
	cd $(BACKEND_DIR) && go run .

build:
	cd $(BACKEND_DIR) && go build -trimpath -ldflags "$(LDFLAGS)" -o /tmp/app-api .

test:
	cd $(BACKEND_DIR) && go test ./...

cover:
	cd $(BACKEND_DIR) && go test -race -covermode=atomic -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

test-integration:
	cd $(BACKEND_DIR) && go test -tags=integration ./internal/repo/pg/... ./internal/app/...

test-e2e:
	cd $(BACKEND_DIR) && go test -tags=integration -count=1 ./e2e/...

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

# 新项目开箱第一步：把 module 名从 backend 改成你自己的项目名，
# 并同步替换所有 import 路径 / -ldflags 包路径。
# 用法：make rename MODULE=github.com/acme/myapp
rename:
	@if [ -z "$(MODULE)" ]; then \
		echo "用法: make rename MODULE=github.com/acme/myapp"; \
		exit 1; \
	fi
	@bash scripts/rename-module.sh $(MODULE)
