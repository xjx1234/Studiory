package metrics

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

type sqlDBCollector struct {
	db                                         *sql.DB
	open, inUse, idle, waitCount, waitDuration *prometheus.Desc
}

func NewSQLDBCollector(db *sql.DB) prometheus.Collector {
	return &sqlDBCollector{
		db:           db,
		open:         prometheus.NewDesc("database_sql_open_connections", "SQL pool open connections.", nil, nil),
		inUse:        prometheus.NewDesc("database_sql_in_use_connections", "SQL pool connections in use.", nil, nil),
		idle:         prometheus.NewDesc("database_sql_idle_connections", "SQL pool idle connections.", nil, nil),
		waitCount:    prometheus.NewDesc("database_sql_wait_count_total", "Total waits for a SQL pool connection.", nil, nil),
		waitDuration: prometheus.NewDesc("database_sql_wait_duration_seconds_total", "Total time waiting for a SQL pool connection.", nil, nil),
	}
}

func (c *sqlDBCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.open
	ch <- c.inUse
	ch <- c.idle
	ch <- c.waitCount
	ch <- c.waitDuration
}

func (c *sqlDBCollector) Collect(ch chan<- prometheus.Metric) {
	if c.db == nil {
		return
	}
	s := c.db.Stats()
	ch <- prometheus.MustNewConstMetric(c.open, prometheus.GaugeValue, float64(s.OpenConnections))
	ch <- prometheus.MustNewConstMetric(c.inUse, prometheus.GaugeValue, float64(s.InUse))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.Idle))
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(s.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.waitDuration, prometheus.CounterValue, s.WaitDuration.Seconds())
}
