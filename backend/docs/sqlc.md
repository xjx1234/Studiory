# sqlc 使用说明

本项目使用 `sqlc` 生成 PostgreSQL 的强类型访问代码，避免 SQL 散落和运行时字段错误。

## 目录约定

- `migrations/`：数据库迁移（schema 来源）
- `internal/repo/sqlc/query/`：所有 SQL 查询文件（手写）
- `internal/repo/sqlc/gen/`：sqlc 生成代码（自动生成，不手改）
- `sqlc.yaml`：sqlc 配置

## 生成命令

在 `backend/` 目录执行：

```bash
sqlc generate
```

如果本机未安装 `sqlc`：

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

## 如何在代码里使用（示例）

生成后会在 `internal/repo/sqlc/gen` 里产生：

- `type Queries struct { ... }`
- 每个 `-- name:` 对应一个方法，例如 `GetUserByPhone(ctx, phone)`、`CreateUser(...)`

在 repo 层用法示例（伪代码）：

```go
q := sqlcgen.New(pgxtxOrPool)
u, err := q.GetUserByPhone(ctx, phone)
```

> 后续我们会把 `Queries` 封装进 `internal/repo`，让上层 usecase 只依赖接口，不直接依赖 sqlcgen。

