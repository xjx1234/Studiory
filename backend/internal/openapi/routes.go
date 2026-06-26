package openapi

import (
	"net/http"
	"sort"
	"strings"
)

var allowedMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
}

// RouteInfo 是路由采集所需的最小接口（兼容 gin.Engine.Routes()）。
type RouteInfo struct {
	Method string
	Path   string
}

// OperationsFromRoutes 从注册路由提取业务 API 操作。
//
// skipPaths 为精确路径排除列表（如 /metrics）；skipPrefixes 为前缀排除。
func OperationsFromRoutes(routes []RouteInfo, skipPaths, skipPrefixes []string) []Operation {
	pathSkip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		pathSkip[p] = struct{}{}
	}

	var ops []Operation
	for _, route := range routes {
		method := strings.ToUpper(route.Method)
		if _, ok := allowedMethods[method]; !ok {
			continue
		}
		if _, skip := pathSkip[route.Path]; skip {
			continue
		}
		if hasPrefix(route.Path, skipPrefixes) {
			continue
		}
		ops = append(ops, Operation{Method: method, Path: route.Path})
	}

	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Path == ops[j].Path {
			return ops[i].Method < ops[j].Method
		}
		return ops[i].Path < ops[j].Path
	})
	return ops
}

func hasPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
