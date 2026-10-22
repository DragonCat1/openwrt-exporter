package collector

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// NetdevCollector 接口级状态与计数；速率交给 Prometheus rate()。
type NetdevCollector struct {
	paths Paths

	up      *prometheus.Desc
	carrier *prometheus.Desc
	speed   *prometheus.Desc

	rxBytes   *prometheus.Desc
	txBytes   *prometheus.Desc
	rxPackets *prometheus.Desc
	txPackets *prometheus.Desc
	rxErrs    *prometheus.Desc
	txErrs    *prometheus.Desc
	rxDrop    *prometheus.Desc
	txDrop    *prometheus.Desc

	scrapeErrors *prometheus.Desc
}

func NewNetdev() *NetdevCollector {
	return NewNetdevWithPaths(defaultPaths())
}

func NewNetdevWithPaths(p Paths) *NetdevCollector {
	labels := []string{"device"}
	return &NetdevCollector{
		paths: p,
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "up"),
			"1 if operstate is up, else 0.",
			labels, nil,
		),
		carrier: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "carrier"),
			"Link carrier (1=present) from sysfs.",
			labels, nil,
		),
		speed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "speed_mbps"),
			"Negotiated link speed in Mbps (-1 if unknown).",
			labels, nil,
		),
		rxBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "receive_bytes_total"),
			"Network device receive bytes (Counter; use rate()).",
			labels, nil,
		),
		txBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "transmit_bytes_total"),
			"Network device transmit bytes (Counter; use rate()).",
			labels, nil,
		),
		rxPackets: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "receive_packets_total"),
			"Network device receive packets.",
			labels, nil,
		),
		txPackets: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "transmit_packets_total"),
			"Network device transmit packets.",
			labels, nil,
		),
		rxErrs: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "receive_errs_total"),
			"Network device receive errors.",
			labels, nil,
		),
		txErrs: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "transmit_errs_total"),
			"Network device transmit errors.",
			labels, nil,
		),
		rxDrop: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "receive_drop_total"),
			"Network device receive drops.",
			labels, nil,
		),
		txDrop: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "network", "transmit_drop_total"),
			"Network device transmit drops.",
			labels, nil,
		),
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "netdev", "scrape_errors_total"),
			"Partial scrape failures in netdev collector (this Collect).",
			nil, nil,
		),
	}
}

func (c *NetdevCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.carrier
	ch <- c.speed
	ch <- c.rxBytes
	ch <- c.txBytes
	ch <- c.rxPackets
	ch <- c.txPackets
	ch <- c.rxErrs
	ch <- c.txErrs
	ch <- c.rxDrop
	ch <- c.txDrop
	ch <- c.scrapeErrors
}

func (c *NetdevCollector) Collect(ch chan<- prometheus.Metric) {
	var errs float64

	stats, err := parseProcNetDev(filepath.Join(c.paths.Proc, "net", "dev"))
	if err != nil {
		errs++
		ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
		return
	}

	for device, s := range stats {
		ch <- prometheus.MustNewConstMetric(c.rxBytes, prometheus.CounterValue, s.rxBytes, device)
		ch <- prometheus.MustNewConstMetric(c.txBytes, prometheus.CounterValue, s.txBytes, device)
		ch <- prometheus.MustNewConstMetric(c.rxPackets, prometheus.CounterValue, s.rxPackets, device)
		ch <- prometheus.MustNewConstMetric(c.txPackets, prometheus.CounterValue, s.txPackets, device)
		ch <- prometheus.MustNewConstMetric(c.rxErrs, prometheus.CounterValue, s.rxErrs, device)
		ch <- prometheus.MustNewConstMetric(c.txErrs, prometheus.CounterValue, s.txErrs, device)
		ch <- prometheus.MustNewConstMetric(c.rxDrop, prometheus.CounterValue, s.rxDrop, device)
		ch <- prometheus.MustNewConstMetric(c.txDrop, prometheus.CounterValue, s.txDrop, device)

		if err := c.collectSysfsState(ch, device); err != nil {
			errs++
		}
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
}

type netdevStats struct {
	rxBytes, txBytes     float64
	rxPackets, txPackets float64
	rxErrs, txErrs       float64
	rxDrop, txDrop       float64
}

func parseProcNetDev(path string) (map[string]netdevStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]netdevStats)
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		if lineNo <= 2 {
			continue // 跳过表头
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// "eth0:  bytes packets ..."
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		dev := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			return nil, fmt.Errorf("net/dev: short line for %s", dev)
		}
		num := func(i int) float64 {
			v, _ := strconv.ParseFloat(fields[i], 64)
			return v
		}
		out[dev] = netdevStats{
			rxBytes:   num(0),
			rxPackets: num(1),
			rxErrs:    num(2),
			rxDrop:    num(3),
			txBytes:   num(8),
			txPackets: num(9),
			txErrs:    num(10),
			txDrop:    num(11),
		}
	}
	return out, sc.Err()
}

func (c *NetdevCollector) collectSysfsState(ch chan<- prometheus.Metric, device string) error {
	base := filepath.Join(c.paths.Sys, "class", "net", device)

	up := 0.0
	if b, err := os.ReadFile(filepath.Join(base, "operstate")); err == nil {
		if strings.TrimSpace(string(b)) == "up" {
			up = 1
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, up, device)

	carrier := 0.0
	if b, err := os.ReadFile(filepath.Join(base, "carrier")); err == nil {
		if strings.TrimSpace(string(b)) == "1" {
			carrier = 1
		}
	}
	// carrier 在口 down 时可能 ENODEV，忽略
	ch <- prometheus.MustNewConstMetric(c.carrier, prometheus.GaugeValue, carrier, device)

	speed := -1.0
	if b, err := os.ReadFile(filepath.Join(base, "speed")); err == nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
			speed = v
		}
	}
	ch <- prometheus.MustNewConstMetric(c.speed, prometheus.GaugeValue, speed, device)
	return nil
}
