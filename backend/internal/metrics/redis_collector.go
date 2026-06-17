package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// redisCollector 在每次抓取时实时读取 go-redis 的 PoolStats()，无后台 goroutine。
type redisCollector struct {
	client redis.UniversalClient

	hits       *prometheus.Desc
	misses     *prometheus.Desc
	timeouts   *prometheus.Desc
	totalConns *prometheus.Desc
	idleConns  *prometheus.Desc
	staleConns *prometheus.Desc
}

// NewRedisCollector 构造 Redis 连接池指标采集器。
func NewRedisCollector(client redis.UniversalClient) prometheus.Collector {
	return &redisCollector{
		client:     client,
		hits:       prometheus.NewDesc("redis_pool_hits_total", "连接池命中空闲连接的累计次数。", nil, nil),
		misses:     prometheus.NewDesc("redis_pool_misses_total", "连接池未命中（需新建连接）的累计次数。", nil, nil),
		timeouts:   prometheus.NewDesc("redis_pool_timeouts_total", "等待连接超时的累计次数。", nil, nil),
		totalConns: prometheus.NewDesc("redis_pool_total_conns", "连接池当前连接总数。", nil, nil),
		idleConns:  prometheus.NewDesc("redis_pool_idle_conns", "连接池当前空闲连接数。", nil, nil),
		staleConns: prometheus.NewDesc("redis_pool_stale_conns", "被判定为陈旧而移除的累计连接数。", nil, nil),
	}
}

func (c *redisCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.hits
	ch <- c.misses
	ch <- c.timeouts
	ch <- c.totalConns
	ch <- c.idleConns
	ch <- c.staleConns
}

func (c *redisCollector) Collect(ch chan<- prometheus.Metric) {
	if c.client == nil {
		return
	}
	s := c.client.PoolStats()
	ch <- prometheus.MustNewConstMetric(c.hits, prometheus.CounterValue, float64(s.Hits))
	ch <- prometheus.MustNewConstMetric(c.misses, prometheus.CounterValue, float64(s.Misses))
	ch <- prometheus.MustNewConstMetric(c.timeouts, prometheus.CounterValue, float64(s.Timeouts))
	ch <- prometheus.MustNewConstMetric(c.totalConns, prometheus.GaugeValue, float64(s.TotalConns))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(s.IdleConns))
	ch <- prometheus.MustNewConstMetric(c.staleConns, prometheus.CounterValue, float64(s.StaleConns))
}
