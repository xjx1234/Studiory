# 业务错误码（errcode）

后端统一错误响应结构见 `backend/pkg/resp/response.go`：

```json
{
  "code": 10001,
  "message": "err_unauthorized",
  "data": null,
  "request_id": "..."
}
```

错误码分段：

- `0`：成功
- `1xxxx`：认证/权限
- `2xxxx`：请求参数/校验
- `3xxxx`：业务资源
- `5xxxx`：服务器内部错误

## 具体错误码

| Code | MsgID | HTTPStatus |
|---:|---|---:|
| 10001 | `err_unauthorized` | 401 |
| 10002 | `err_invalid_token` | 401 |
| 10003 | `err_token_expired` | 401 |
| 10004 | `err_invalid_credentials` | 401 |
| 10005 | `err_invalid_code` | 400 |
| 10006 | `err_unsupported_grant` | 400 |
| 10007 | `err_forbidden` | 403 |
| 10008 | `err_wrong_password` | 401 |
| 10009 | `err_same_password` | 400 |
| 10010 | `err_account_locked` | 429 |
| 10011 | `err_account_disabled` | 403 |
| 10012 | `err_cannot_modify_self` | 400 |
| 20001 | `err_bad_request` | 400 |
| 20002 | `err_validation` | 400 |
| 20003 | `err_too_many_requests` | 429 |
| 30001 | `err_not_found` | 404 |
| 30002 | `err_already_exists` | 409 |
| 50001 | `err_internal` | 500 |
| 50002 | `err_service_unavailable` | 503 |

> `message` 字段在运行时会根据 `locales/*` 做 i18n 翻译；这里只列出稳定的 `MsgID`。

## 同一 code 下的不同 MsgID

`code` 是稳定的机器可读契约，`message` 只用于人读，因此个别场景会在不改 `code` 的前提下换 `MsgID`：

| Code | MsgID | HTTPStatus | 触发条件 |
|---:|---|---:|---|
| 20001 | `err_body_too_large` | 400 | 请求体超过 `Safe` 中间件的 2 MB 上限（`http.MaxBytesReader`） |

## 关于 message 的约定

绑定/解析类错误（非法 JSON、类型不匹配、body 超限）**不会**把 Go 侧的原始错误文本回显给客户端，
客户端只会看到上表中的固定文案；原文通过 `c.Error(err)` 进入访问日志（`gin.Logger` 链路的 `error` 字段），
排障时按 `request_id` 查日志即可。这样既避免泄露运行时实现细节，也不损失可观测性。

