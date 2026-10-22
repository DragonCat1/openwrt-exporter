package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

// WirelessCollector 无线 station stub（后续 ubus hostapd / nl80211；含每站速率）。
type WirelessCollector struct {
	scrapeErrors *prometheus.Desc
}

func NewWireless() *WirelessCollector {
	return &WirelessCollector{
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "scrape_errors_total"),
			"Stub: wireless collector scrape errors (always 0 until implemented).",
			nil, nil,
		),
	}
}

func (c *WirelessCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.scrapeErrors
}

func (c *WirelessCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, 0)
}
