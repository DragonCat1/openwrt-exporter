package collector

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DHCPCollector 读取 dnsmasq/odhcpd 风格租约文件。
type DHCPCollector struct {
	leasePaths []string // 按优先级尝试

	leasesTotal *prometheus.Desc
	leaseInfo   *prometheus.Desc
	leaseExpiry *prometheus.Desc
	scrapeErrors *prometheus.Desc
}

func defaultLeasePaths() []string {
	return []string{
		"/tmp/dhcp.leases",
		"/var/dhcp.leases",
		"/tmp/hosts/odhcpd", // odhcpd 主机表（非标准 leases，作后备解析见下）
	}
}

func NewDHCP() *DHCPCollector {
	return NewDHCPWithPaths(defaultLeasePaths())
}

// leaseIndexByMAC 按小写 MAC 建索引，供无线等模块 join；文件皆缺失时返回空 map。
func leaseIndexByMAC(paths []string) map[string]dhcpLease {
	idx := make(map[string]dhcpLease)
	for _, p := range paths {
		leases, err := parseDHCPLeasesFile(p)
		if err != nil {
			continue
		}
		for _, l := range leases {
			mac := strings.ToLower(l.mac)
			if mac == "" {
				continue
			}
			// 同 MAC 多租约时保留第一条有效记录
			if _, ok := idx[mac]; !ok {
				idx[mac] = l
			}
		}
		if len(idx) > 0 {
			return idx
		}
	}
	return idx
}

// stableLeaseLabel 无租约/通配主机名时用 "-"，保证 Prometheus label 稳定。
func stableLeaseLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "*" {
		return "-"
	}
	return s
}

func NewDHCPWithPaths(paths []string) *DHCPCollector {
	return &DHCPCollector{
		leasePaths: paths,
		leasesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "dhcp", "leases"),
			"Number of DHCP leases currently known.",
			nil, nil,
		),
		leaseInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "dhcp", "lease_info"),
			"DHCP lease present (value 1) with identifying labels.",
			[]string{"ip", "mac", "hostname"}, nil,
		),
		leaseExpiry: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "dhcp", "lease_expiry_timestamp_seconds"),
			"Unix timestamp when the lease expires (0 if unknown/infinite).",
			[]string{"ip", "mac", "hostname"}, nil,
		),
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "dhcp", "scrape_errors_total"),
			"Partial scrape failures in DHCP collector (this Collect).",
			nil, nil,
		),
	}
}

func (c *DHCPCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.leasesTotal
	ch <- c.leaseInfo
	ch <- c.leaseExpiry
	ch <- c.scrapeErrors
}

func (c *DHCPCollector) Collect(ch chan<- prometheus.Metric) {
	var errs float64
	leases, path, err := c.readFirstLeases()
	if err != nil {
		errs++
		ch <- prometheus.MustNewConstMetric(c.leasesTotal, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
		return
	}
	_ = path

	ch <- prometheus.MustNewConstMetric(c.leasesTotal, prometheus.GaugeValue, float64(len(leases)))
	for _, l := range leases {
		host := l.hostname
		if host == "" {
			host = "*"
		}
		ch <- prometheus.MustNewConstMetric(c.leaseInfo, prometheus.GaugeValue, 1, l.ip, l.mac, host)
		ch <- prometheus.MustNewConstMetric(c.leaseExpiry, prometheus.GaugeValue, l.expiry, l.ip, l.mac, host)
	}
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
}

type dhcpLease struct {
	expiry   float64
	mac, ip  string
	hostname string
}

func (c *DHCPCollector) readFirstLeases() ([]dhcpLease, string, error) {
	var lastErr error
	for _, p := range c.leasePaths {
		leases, err := parseDHCPLeasesFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				lastErr = err
				continue
			}
			lastErr = err
			continue
		}
		return leases, p, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no lease paths configured")
	}
	return nil, "", lastErr
}

// parseDHCPLeasesFile 支持 dnsmasq：`<expiry> <mac> <ip> <hostname> <clientid>`。
// 亦容忍 odhcpd hosts 风格的弱解析（# 注释与空行跳过）。
func parseDHCPLeasesFile(path string) ([]dhcpLease, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []dhcpLease
	sc := bufio.NewScanner(f)
	now := float64(time.Now().Unix())
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// dnsmasq leases：首字段为 unix expiry
		if exp, err := strconv.ParseFloat(fields[0], 64); err == nil && looksLikeMAC(fields[1]) {
			host := "*"
			if len(fields) >= 4 && fields[3] != "*" {
				host = fields[3]
			}
			ip := fields[2]
			mac := strings.ToLower(fields[1])
			// 过期租约跳过（仍在文件中时）
			if exp > 0 && exp < now {
				continue
			}
			out = append(out, dhcpLease{expiry: exp, mac: mac, ip: ip, hostname: host})
			continue
		}

		// 简单后备：`ip mac` / `mac ip hostname` 无 expiry
		if looksLikeIP(fields[0]) && looksLikeMAC(fields[1]) {
			host := "*"
			if len(fields) >= 3 {
				host = fields[2]
			}
			out = append(out, dhcpLease{expiry: 0, mac: strings.ToLower(fields[1]), ip: fields[0], hostname: host})
		}
	}
	return out, sc.Err()
}

func looksLikeMAC(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 2 {
			return false
		}
		for _, c := range p {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

func looksLikeIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

// LeasePathsForTest 暴露路径拼接辅助（测试用）。
func LeasePathsForTest(dir string) []string {
	return []string{
		filepath.Join(dir, "dhcp.leases"),
	}
}
