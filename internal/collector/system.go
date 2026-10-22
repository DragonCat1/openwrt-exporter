package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SystemCollector 系统健康 stub：负载、uptime、thermal（后续读 /proc 与 sysfs）。
type SystemCollector struct {
	scrapeErrors *prometheus.Desc
}

func NewSystem() *SystemCollector {
	return &SystemCollector{
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "system", "scrape_errors_total"),
			"Stub: system collector scrape errors (always 0 until implemented).",
			nil, nil,
		),
	}
}

func (c *SystemCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.scrapeErrors
}

func (c *SystemCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, 0)
}
