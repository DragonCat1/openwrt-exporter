package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

// DHCPCollector DHCP 租约 stub（后续读 /tmp/dhcp.leases 或 ubus/odhcpd）。
type DHCPCollector struct {
	scrapeErrors *prometheus.Desc
}

func NewDHCP() *DHCPCollector {
	return &DHCPCollector{
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "dhcp", "scrape_errors_total"),
			"Stub: DHCP collector scrape errors (always 0 until implemented).",
			nil, nil,
		),
	}
}

func (c *DHCPCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.scrapeErrors
}

func (c *DHCPCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, 0)
}
