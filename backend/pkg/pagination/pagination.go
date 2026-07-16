// Package pagination 提供列表接口通用的分页参数解析与响应结构。
package pagination

import (
	"math"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Query 分页查询参数。
type Query struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

// Offset 返回 SQL OFFSET。
func (q Query) Offset() int {
	return (q.Page - 1) * q.PageSize
}

// LimitInt32 返回适合传给数据库层（如 sqlc 生成代码）的 LIMIT 值。
// PageSize 已由 ParseQuery clamp 到 [1, MaxPageSize]，这里的 clamp 是面向
// 直接构造 Query（不经过 ParseQuery）场景的兜底防线。
func (q Query) LimitInt32() int32 {
	return clampToInt32(q.PageSize)
}

// OffsetInt32 返回适合传给数据库层的 OFFSET 值，避免 Page 异常偏大时
// int → int32 转换溢出（G115）。
func (q Query) OffsetInt32() int32 {
	return clampToInt32(q.Offset())
}

// clampToInt32 将 int 安全收窄到 int32 范围，避免溢出。
func clampToInt32(n int) int32 {
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32
	case n < 0:
		return 0
	default:
		return int32(n)
	}
}

// List 统一分页响应结构。
type List[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// NewList 构造分页响应。
func NewList[T any](items []T, q Query, total int64) List[T] {
	if items == nil {
		items = []T{}
	}
	return List[T]{
		Items:    items,
		Page:     q.Page,
		PageSize: q.PageSize,
		Total:    total,
	}
}

// ParseQuery 从 Query 参数解析分页，自动修正非法值。
func ParseQuery(c *gin.Context) Query {
	q := Query{
		Page:     DefaultPage,
		PageSize: DefaultPageSize,
	}

	if raw := c.Query("page"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			q.Page = n
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			q.PageSize = n
		}
	}
	if q.PageSize > MaxPageSize {
		q.PageSize = MaxPageSize
	}
	return q
}
