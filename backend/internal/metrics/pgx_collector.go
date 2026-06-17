package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// pgxPoolCollector 在每次抓取时实时读取 pgxpool.Stat()，无后台 goroutine。
type pgxPoolCollector struct {
	pool *pgxpool.Pool

	totalConns    *prometheus.Desc
	idleConns     *prometheus.Desc
	acquiredConns *prometheus.Desc
	maxConns      *prometheus.Desc
	acquireTotal  *prometheus.Desc
}

// NewPgxPoolCollector 构造 pgxpool 连接池指标采集器。
func NewPgxPoolCollector(pool *pgxpool.Pool) prometheus.Collector {
	return &pgxPoolCollector{
		pool:          pool,
		totalConns:    prometheus.NewDesc("pgxpool_total_conns", "连接池当前连接总数。", nil, nil),
		idleConns:     prometheus.NewDesc("pgxpool_idle_conns", "连接池当前空闲连接数。", nil, nil),
		acquiredConns: prometheus.NewDesc("pgxpool_acquired_conns", "连接池当前已占用连接数。", nil, nil),
		maxConns:      prometheus.NewDesc("pgxpool_max_conns", "连接池允许的最大连接数。", nil, nil),
		acquireTotal:  prometheus.NewDesc("pgxpool_acquire_total", "成功获取连接的累计次数。", nil, nil),
	}
}

func (c *pgxPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalConns
	ch <- c.idleConns
	ch <- c.acquiredConns
	ch <- c.maxConns
	ch <- c.acquireTotal
}

func (c *pgxPoolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.pool == nil {
		return
	}
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.totalConns, prometheus.GaugeValue, float64(s.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.acquiredConns, prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.maxConns, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.acquireTotal, prometheus.CounterValue, float64(s.AcquireCount()))
}
