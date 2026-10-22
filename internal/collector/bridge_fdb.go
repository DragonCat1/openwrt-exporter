package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

// BridgeFDBCollector 桥 FDB stub：有线接入视图（无每设备速率）。
type BridgeFDBCollector struct {
	scrapeErrors *prometheus.Desc
}

func NewBridgeFDB() *BridgeFDBCollector {
	return &BridgeFDBCollector{
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "bridge_fdb", "scrape_errors_total"),
			"Stub: bridge FDB collector scrape errors (always 0 until implemented).",
			nil, nil,
		),
	}
}

func (c *BridgeFDBCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.scrapeErrors
}

func (c *BridgeFDBCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, 0)
}
