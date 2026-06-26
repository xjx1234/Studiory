// Package openapi 提供 OpenAPI 文档校验与路由契约对比。
package openapi

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

var pathParamPattern = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

// Operation 表示一条 HTTP 接口（方法 + 路径模板）。
type Operation struct {
	Method string
	Path   string
}

// Key 返回用于集合对比的标准键，例如 "GET /api/v1/user/todos/{id}"。
func (o Operation) Key() string {
	return o.Method + " " + NormalizePath(o.Path)
}

// NormalizePath 将 Gin 风格 :id 转为 OpenAPI 风格 {id}。
func NormalizePath(path string) string {
	return pathParamPattern.ReplaceAllString(path, "{$1}")
}

// LoadDocument 加载并校验 OpenAPI 3 文档。
func LoadDocument(path string) (*openapi3.T, error) {
	loader := openapi3.Loader{IsExternalRefsAllowed: false}
	doc, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load openapi: %w", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("validate openapi: %w", err)
	}
	return doc, nil
}

// OperationsFromDocument 提取文档中声明的全部 HTTP 操作。
func OperationsFromDocument(doc *openapi3.T) []Operation {
	if doc == nil || doc.Paths == nil {
		return nil
	}

	var ops []Operation
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for method, op := range item.Operations() {
			if op == nil {
				continue
			}
			ops = append(ops, Operation{
				Method: strings.ToUpper(method),
				Path:   path,
			})
		}
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Path == ops[j].Path {
			return ops[i].Method < ops[j].Method
		}
		return ops[i].Path < ops[j].Path
	})
	return ops
}

// CompareOperations 双向对比路由与 OpenAPI 文档，返回仅在文档或仅在路由一侧出现的操作。
func CompareOperations(document, router []Operation) (onlyInDoc, onlyInRouter []string) {
	docSet := toSet(document)
	routerSet := toSet(router)

	for key := range docSet {
		if _, ok := routerSet[key]; !ok {
			onlyInDoc = append(onlyInDoc, key)
		}
	}
	for key := range routerSet {
		if _, ok := docSet[key]; !ok {
			onlyInRouter = append(onlyInRouter, key)
		}
	}
	sort.Strings(onlyInDoc)
	sort.Strings(onlyInRouter)
	return onlyInDoc, onlyInRouter
}

func toSet(ops []Operation) map[string]struct{} {
	set := make(map[string]struct{}, len(ops))
	for _, op := range ops {
		set[op.Key()] = struct{}{}
	}
	return set
}
