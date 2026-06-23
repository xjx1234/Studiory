# 示例模块：Admin 用户管理

演示如何在 **RBAC + admin 路由** 下编写后台管理类接口：用户列表分页、查看详情、修改角色、启用 / 禁用账号。禁用账号会**即时吊销其登录态**（access token 立即失效，且无法用 refresh token 续期）。

## 涉及文件

| 步骤 | 路径 |
|------|------|
| 迁移 | `backend/migrations/000003_user_status.up.sql`（`users.status` + 创建时间索引） |
| SQL | `backend/internal/repo/sqlc/query/users.sql`（`ListUsers` / `CountUsers` / `UpdateUserRole` / `UpdateUserStatus`） |
| Repo 接口 | `backend/internal/repo/user_repo.go`（`List` / `Count` / `UpdateRole` / `UpdateStatus`） |
| Repo 实现 | `backend/internal/repo/pg/user_repo.go` |
| 业务 | `backend/internal/service/admin/service.go` |
| HTTP | `backend/internal/http/admin.go` |
| 鉴权联动 | `backend/internal/service/auth/service.go`（`issueResult` / `Refresh` 禁用校验） |
| 装配 | `internal/app/app.go`、`internal/http/deps.go`、`router.go` |

## API（需 admin 角色）

请求头：`Authorization: Bearer <access_token>`，且 token 内角色为 `admin`。非 admin 访问返回 `403 err_forbidden`。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/users?page=&page_size=&keyword=&status=` | 用户列表（分页 + 过滤） |
| GET | `/api/v1/admin/users/:id` | 用户详情 |
| PATCH | `/api/v1/admin/users/:id/role` | 修改角色 |
| PATCH | `/api/v1/admin/users/:id/status` | 启用 / 禁用 |

### 列表过滤参数

- `keyword`：对 `phone` / `email` / `nickname` 做模糊匹配（`ILIKE`），为空不过滤
- `status`：`active` / `disabled`，为空不过滤（传非法值返回 `400`）
- `page` / `page_size`：同统一分页约定（`page` 从 1 开始，`page_size` 1~100）

返回结构（统一响应 `data` 字段内）：

```json
{
  "data": {
    "items": [
      {
        "id": "uuid",
        "phone": "13800138000",
        "email": "",
        "nickname": "用户8000",
        "avatar": "",
        "role": "user",
        "status": "active",
        "created_at": "2026-06-23T11:00:00+08:00",
        "updated_at": "2026-06-23T11:00:00+08:00"
      }
    ],
    "page": 1,
    "page_size": 20,
    "total": 1
  }
}
```

### 修改角色

```json
PATCH /api/v1/admin/users/{id}/role
{ "role": "admin" }   // 仅允许 admin / user
```

### 启用 / 禁用

```json
PATCH /api/v1/admin/users/{id}/status
{ "status": "disabled" }   // 仅允许 active / disabled
```

## 安全约束

- **不能操作自己**：管理员不能修改自己的角色，也不能禁用自己（返回 `10012 err_cannot_modify_self`），避免把自己锁在系统外或失去管理入口。
- **禁用即时生效**：`status=disabled` 时写入 `<prefix>:revoke:uid:<id>` 吊销键（TTL=access token 有效期），中间件 `isAccessTokenRevoked` 会立即拦截其现有 access token。
- **禁用后无法续期**：`auth.Refresh` 会重新加载用户，若已禁用则拒绝换发新 token（`10011 err_account_disabled`）；登录入口 `issueResult` 同样拒绝禁用账号。

## 本地验证流程

```bash
# 1. 迁移（up 到 000003）
migrate -path backend/migrations -database "$DATABASE_URL" up

# 2. 初始化一个 admin（见 cmd/seed）
cd backend && go run ./cmd/seed -phone 13900000000 -password 'Admin@123456'

# 3. 以 admin 登录拿到 access_token，再列出用户
curl -s "http://localhost:8080/api/v1/admin/users?page=1&page_size=20" \
  -H "Authorization: Bearer <admin_token>"

# 4. 禁用某用户（其在线会话立即失效）
curl -s -X PATCH "http://localhost:8080/api/v1/admin/users/<uid>/status" \
  -H "Authorization: Bearer <admin_token>" \
  -H 'Content-Type: application/json' \
  -d '{"status":"disabled"}'
```

详细分层说明见 [architecture.md](../architecture.md)。
