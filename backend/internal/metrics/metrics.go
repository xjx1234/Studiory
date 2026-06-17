// Package metrics 提供 Prometheus 指标采集：HTTP 中间件、/metrics handler，
// 以及 pgxpool / Redis 连接池的自定义 Collector。
//
// 设计要点：
//   - 使用独立的 *prometheus.Registry（不污染全局 DefaultRegisterer，便于测试）。
//   - HTTP label 使用路由模板（c.FullPath()）而非真实路径，避免 label 基数爆炸。
//   - QPS / P99 / 错误率由 Prometheus 端用 PromQL 从下列原始指标派生，应用只暴露原始量。
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metricsPath 是指标端点路径，中间件会跳过对它自身的统计。
const metricsPath = "/metrics"

// Metrics 持有注册表与 HTTP 相关指标。
type Metrics struct {
	registry *prometheus.Registry

	reqTotal    *prometheus.CounterVec
	reqDuration *prometheus.HistogramVec
	inFlight    prometheus.Gauge
}

// New 创建 Metrics，注册 Go runtime / process 默认采集器与 HTTP 指标。
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		registry: reg,
		reqTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP 请求总数（按方法、路由模板、状态码）。",
		}, []string{"method", "route", "status"}),
		reqDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP 请求耗时分布（秒）。",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "当前正在处理的 HTTP 请求数。",
		}),
	}
	reg.MustRegister(m.reqTotal, m.reqDuration, m.inFlight)
	return m
}

// Registerer 返回底层注册表，供注册自定义 Collector（pgxpool / Redis）。
func (m *Metrics) Registerer() prometheus.Registerer {
	return m.registry
}

// Handler 返回 /metrics 的 HTTP handler。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Middleware 返回 Gin 中间件，统计请求数、耗时与在途请求。
// 跳过对 /metrics 自身的统计，避免噪声。
func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == metricsPath {
			c.Next()
			return
		}

		start := time.Now()
		m.inFlight.Inc()

		c.Next()

		m.inFlight.Dec()

		// 用路由模板而非真实路径，避免高基数；未匹配路由统一归为 "unmatched"
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())

		m.reqTotal.WithLabelValues(c.Request.Method, route, status).Inc()
		m.reqDuration.WithLabelValues(c.Request.Method, route, status).Observe(time.Since(start).Seconds())
	}
}
