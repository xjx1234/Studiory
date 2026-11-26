// Package reqid 在 context 中传递 HTTP 请求的 request id。
//
// 目的：让 service 层日志带上与访问日志相同的 request_id，
// 排查时可以用一个 id 把「HTTP 入口 → 业务处理 → 数据层错误」串起来。
// 该包不依赖 gin，避免 service 层反向依赖 HTTP 框架。
package reqid

import "context"

type ctxKey struct{}

// WithID 返回携带 request id 的派生 context；id 为空时返回原 context。
func WithID(ctx context.Context, id string) context.Context {
	if ctx == nil || id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext 读取 request id；不存在时返回空串。
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}
