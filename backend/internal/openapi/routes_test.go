package openapi

import "testing"

func TestOperationsFromRoutes_SkipsMetrics(t *testing.T) {
	routes := []RouteInfo{
		{Method: "GET", Path: "/health"},
		{Method: "GET", Path: "/metrics"},
		{Method: "HEAD", Path: "/health"},
	}
	ops := OperationsFromRoutes(routes, []string{"/metrics"}, nil)
	if len(ops) != 1 || ops[0].Key() != "GET /health" {
		t.Fatalf("unexpected ops: %+v", ops)
	}
}
