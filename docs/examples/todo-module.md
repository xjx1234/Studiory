# 示例模块：Todo（待办）

脚手架内置的**完整竖切示例**，演示从表结构到 API 的标准路径。真实项目可复制后改名，或删除示例（见文末）。

## 涉及文件

| 步骤 | 路径 |
|------|------|
| 迁移 | `backend/migrations/000002_example_todos.up.sql` |
| SQL | `backend/internal/repo/sqlc/query/todos.sql` |
| Repo 接口 | `backend/internal/repo/todo_repo.go` |
| Repo 实现 | `backend/internal/repo/pg/todo_repo.go` |
| 业务 | `backend/internal/service/todo/service.go` |
| HTTP | `backend/internal/http/user_todo.go` |
| 装配 | `internal/app/app.go`、`internal/http/deps.go`、`router.go` |

## API（需登录）

请求头：`Authorization: Bearer <access_token>`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/user/todos?page=&page_size=` | 列表（当前用户，分页） |
| POST | `/api/v1/user/todos` | 创建 |
| GET | `/api/v1/user/todos/:id` | 详情 |
| PATCH | `/api/v1/user/todos/:id` | 更新 |
| DELETE | `/api/v1/user/todos/:id` | 删除 |

分页约定：
- `page`：从 1 开始（默认 1）
- `page_size`：1~100（默认 20）
- 返回结构在统一响应 `data` 字段内，包含：`items`、`page`、`page_size`、`total`

示例：

```json
{
  "data": {
    "items": [/* TodoItem[] */],
    "page": 1,
    "page_size": 20,
    "total": 1
  }
}
```

### 创建

```json
POST /api/v1/user/todos
{ "title": "学完第一章" }
```

### 更新

```json
PATCH /api/v1/user/todos/{id}
{ "title": "学完第一章", "done": true }
```

## 本地验证流程

```bash
# 1. 迁移（新库或已有库需 up 到 000002）
migrate -path backend/migrations -database "$DATABASE_URL" up

# 2. 启动
cd backend && go run .

# 3. 注册并登录（开发验证码 123456）
curl -s -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"grant_type":"code","code_type":"sms","phone":"13800138000","code":"123456"}'

# 4. 用返回的 access_token 创建待办
curl -s -X POST http://localhost:8080/api/v1/user/todos \
  -H "Authorization: Bearer <token>" \
  -H 'Content-Type: application/json' \
  -d '{"title":"示例待办"}'
```

## 如何删除示例

若不需要 Todo 演示：

1. `migrate down 1`（回滚 000002）
2. 删除上表所列 todo 相关源码与 `000002_*` 迁移
3. 从 `sqlc.yaml` schema 去掉 `000002`，执行 `sqlc generate`
4. 清理 `app.go` / `deps.go` / `router.go` 中的 Todo 注入与路由

## 复制成真实业务模块

1. 复制 `migrations/000002_*` → `00000N_your_feature.up.sql`
2. 复制 `todos.sql` → `your_table.sql`，改表名与字段
3. 复制 `todo_repo.go`、`pg/todo_repo.go`、`service/todo`、`http/user_todo.go`
4. 全局替换 `Todo` / `todo` 为你的领域名
5. 在 `store.go` 增加 `YourFeature()` 并在 `app.New` 注入

详细分层说明见 [architecture.md](../architecture.md)。
