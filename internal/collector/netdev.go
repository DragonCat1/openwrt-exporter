package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

// NetdevCollector 有线/接口级 stub：operstate、RX/TX 字节与包（后续读 /proc/net/dev 与 sysfs）。
type NetdevCollector struct {
	scrapeErrors *prometheus.Desc
}

func NewNetdev() *NetdevCollector {
	return &NetdevCollector{
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "netdev", "scrape_errors_total"),
			"Stub: netdev collector scrape errors (always 0 until implemented).",
			nil, nil,
		),
	}
}

func (c *NetdevCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.scrapeErrors
}

func (c *NetdevCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, 0)
}
