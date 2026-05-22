// Package pagination 提供列表接口通用的分页参数解析与响应结构。
package pagination

import (
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
