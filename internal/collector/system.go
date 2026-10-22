package collector

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// SystemCollector 系统健康：负载、uptime、温度、CPU/内存使用率、conntrack 连接数。
type SystemCollector struct {
	paths Paths

	load1  *prometheus.Desc
	load5  *prometheus.Desc
	load15 *prometheus.Desc

	uptime *prometheus.Desc

	thermalCelsius *prometheus.Desc

	cpuUsageRatio *prometheus.Desc
	cpuSeconds    *prometheus.Desc // 累计 CPU 秒，便于外部自行差分

	memTotalBytes     *prometheus.Desc
	memAvailableBytes *prometheus.Desc
	memUsageRatio     *prometheus.Desc

	conntrackEntries *prometheus.Desc
	conntrackMax     *prometheus.Desc

	scrapeErrors *prometheus.Desc

	mu       sync.Mutex
	prevCPU  cpuTimes
	prevOK   bool
	prevWall time.Time
}

type cpuTimes struct {
	user, nice, system, idle, iowait, irq, softirq, steal float64
}

func NewSystem() *SystemCollector {
	return NewSystemWithPaths(defaultPaths())
}

func NewSystemWithPaths(p Paths) *SystemCollector {
	return &SystemCollector{
		paths: p,
		load1: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "load", "1"),
			"1-minute load average.",
			nil, nil,
		),
		load5: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "load", "5"),
			"5-minute load average.",
			nil, nil,
		),
		load15: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "load", "15"),
			"15-minute load average.",
			nil, nil,
		),
		uptime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "system", "uptime_seconds"),
			"System uptime in seconds.",
			nil, nil,
		),
		thermalCelsius: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "thermal", "celsius"),
			"Thermal zone temperature in Celsius.",
			[]string{"zone", "type"}, nil,
		),
		cpuUsageRatio: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "cpu", "usage_ratio"),
			"CPU busy ratio since last scrape (0-1), from /proc/stat delta. First scrape is 0.",
			nil, nil,
		),
		cpuSeconds: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "cpu", "seconds_total"),
			"Cumulative CPU time in seconds by mode (from /proc/stat).",
			[]string{"mode"}, nil,
		),
		memTotalBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memory", "total_bytes"),
			"Total physical memory in bytes (MemTotal).",
			nil, nil,
		),
		memAvailableBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memory", "available_bytes"),
			"Available memory in bytes (MemAvailable, or MemFree+Cached+Buffers fallback).",
			nil, nil,
		),
		memUsageRatio: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memory", "usage_ratio"),
			"Memory usage ratio: 1 - available/total (0-1).",
			nil, nil,
		),
		conntrackEntries: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "conntrack", "entries"),
			"Current nf_conntrack entry count.",
			nil, nil,
		),
		conntrackMax: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "conntrack", "max"),
			"nf_conntrack table size limit.",
			nil, nil,
		),
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "system", "scrape_errors_total"),
			"Number of partial scrape failures within the system collector (this Collect).",
			nil, nil,
		),
	}
}

func (c *SystemCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.load1
	ch <- c.load5
	ch <- c.load15
	ch <- c.uptime
	ch <- c.thermalCelsius
	ch <- c.cpuUsageRatio
	ch <- c.cpuSeconds
	ch <- c.memTotalBytes
	ch <- c.memAvailableBytes
	ch <- c.memUsageRatio
	ch <- c.conntrackEntries
	ch <- c.conntrackMax
	ch <- c.scrapeErrors
}

func (c *SystemCollector) Collect(ch chan<- prometheus.Metric) {
	var errs float64

	if err := c.collectLoad(ch); err != nil {
		errs++
	}
	if err := c.collectUptime(ch); err != nil {
		errs++
	}
	if err := c.collectThermal(ch); err != nil {
		errs++
	}
	if err := c.collectCPU(ch); err != nil {
		errs++
	}
	if err := c.collectMemory(ch); err != nil {
		errs++
	}
	if err := c.collectConntrack(ch); err != nil {
		errs++
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
}

func (c *SystemCollector) collectLoad(ch chan<- prometheus.Metric) error {
	b, err := os.ReadFile(filepath.Join(c.paths.Proc, "loadavg"))
	if err != nil {
		return err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 3 {
		return fmt.Errorf("loadavg: unexpected format")
	}
	l1, err1 := strconv.ParseFloat(fields[0], 64)
	l5, err5 := strconv.ParseFloat(fields[1], 64)
	l15, err15 := strconv.ParseFloat(fields[2], 64)
	if err1 != nil || err5 != nil || err15 != nil {
		return fmt.Errorf("loadavg: parse error")
	}
	ch <- prometheus.MustNewConstMetric(c.load1, prometheus.GaugeValue, l1)
	ch <- prometheus.MustNewConstMetric(c.load5, prometheus.GaugeValue, l5)
	ch <- prometheus.MustNewConstMetric(c.load15, prometheus.GaugeValue, l15)
	return nil
}

func (c *SystemCollector) collectUptime(ch chan<- prometheus.Metric) error {
	b, err := os.ReadFile(filepath.Join(c.paths.Proc, "uptime"))
	if err != nil {
		return err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 1 {
		return fmt.Errorf("uptime: unexpected format")
	}
	up, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return err
	}
	ch <- prometheus.MustNewConstMetric(c.uptime, prometheus.GaugeValue, up)
	return nil
}

func (c *SystemCollector) collectThermal(ch chan<- prometheus.Metric) error {
	dir := filepath.Join(c.paths.Sys, "class", "thermal")
	entries, err := os.ReadDir(dir)
	if err != nil {
		// 无 thermal 子系统时不算致命（部分 SoC 节点路径不同）
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var any bool
	var lastErr error
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "thermal_zone") {
			continue
		}
		zoneDir := filepath.Join(dir, name)
		tempRaw, err := os.ReadFile(filepath.Join(zoneDir, "temp"))
		if err != nil {
			lastErr = err
			continue
		}
		milli, err := strconv.ParseFloat(strings.TrimSpace(string(tempRaw)), 64)
		if err != nil {
			lastErr = err
			continue
		}
		typ := name
		if tb, err := os.ReadFile(filepath.Join(zoneDir, "type")); err == nil {
			typ = strings.TrimSpace(string(tb))
		}
		ch <- prometheus.MustNewConstMetric(c.thermalCelsius, prometheus.GaugeValue, milli/1000.0, name, typ)
		any = true
	}
	if !any && lastErr != nil {
		return lastErr
	}
	return nil
}

func (c *SystemCollector) collectCPU(ch chan<- prometheus.Metric) error {
	cur, err := readCPUTimes(filepath.Join(c.paths.Proc, "stat"))
	if err != nil {
		return err
	}
	now := time.Now()

	// 累计秒（jiffies → 秒按 USER_HZ=100 常见值；OpenWrt/Linux 默认 100）
	const hz = 100.0
	modes := []struct {
		name string
		v    float64
	}{
		{"user", cur.user / hz},
		{"nice", cur.nice / hz},
		{"system", cur.system / hz},
		{"idle", cur.idle / hz},
		{"iowait", cur.iowait / hz},
		{"irq", cur.irq / hz},
		{"softirq", cur.softirq / hz},
		{"steal", cur.steal / hz},
	}
	for _, m := range modes {
		ch <- prometheus.MustNewConstMetric(c.cpuSeconds, prometheus.CounterValue, m.v, m.name)
	}

	usage := 0.0
	c.mu.Lock()
	if c.prevOK {
		// 标准做法：busy = total - idle - iowait；对相邻两次 scrape 做差分
		dBusy := (cur.user - c.prevCPU.user) + (cur.nice - c.prevCPU.nice) +
			(cur.system - c.prevCPU.system) + (cur.irq - c.prevCPU.irq) +
			(cur.softirq - c.prevCPU.softirq) + (cur.steal - c.prevCPU.steal)
		dTotal := dBusy + (cur.idle - c.prevCPU.idle) + (cur.iowait - c.prevCPU.iowait)
		if dTotal > 0 {
			usage = dBusy / dTotal
			if usage < 0 {
				usage = 0
			}
			if usage > 1 {
				usage = 1
			}
		}
	}
	c.prevCPU = cur
	c.prevOK = true
	c.prevWall = now
	c.mu.Unlock()

	ch <- prometheus.MustNewConstMetric(c.cpuUsageRatio, prometheus.GaugeValue, usage)
	return nil
}

func readCPUTimes(path string) (cpuTimes, error) {
	f, err := os.Open(path)
	if err != nil {
		return cpuTimes{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// cpu user nice system idle iowait irq softirq steal ...
		if len(fields) < 5 {
			return cpuTimes{}, fmt.Errorf("stat: short cpu line")
		}
		parse := func(i int) float64 {
			if i >= len(fields) {
				return 0
			}
			v, _ := strconv.ParseFloat(fields[i], 64)
			return v
		}
		return cpuTimes{
			user:    parse(1),
			nice:    parse(2),
			system:  parse(3),
			idle:    parse(4),
			iowait:  parse(5),
			irq:     parse(6),
			softirq: parse(7),
			steal:   parse(8),
		}, nil
	}
	if err := sc.Err(); err != nil {
		return cpuTimes{}, err
	}
	return cpuTimes{}, fmt.Errorf("stat: no aggregate cpu line")
}

func (c *SystemCollector) collectMemory(ch chan<- prometheus.Metric) error {
	f, err := os.Open(filepath.Join(c.paths.Proc, "meminfo"))
	if err != nil {
		return err
	}
	defer f.Close()

	vals := map[string]float64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		v, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		// meminfo 单位为 kB
		vals[key] = v * 1024
	}
	if err := sc.Err(); err != nil {
		return err
	}

	total := vals["MemTotal"]
	if total <= 0 {
		return fmt.Errorf("meminfo: MemTotal missing")
	}
	avail, ok := vals["MemAvailable"]
	if !ok {
		// 老内核后备：Free + Cached + Buffers
		avail = vals["MemFree"] + vals["Cached"] + vals["Buffers"]
	}
	usage := 1 - avail/total
	if usage < 0 {
		usage = 0
	}
	if usage > 1 {
		usage = 1
	}

	ch <- prometheus.MustNewConstMetric(c.memTotalBytes, prometheus.GaugeValue, total)
	ch <- prometheus.MustNewConstMetric(c.memAvailableBytes, prometheus.GaugeValue, avail)
	ch <- prometheus.MustNewConstMetric(c.memUsageRatio, prometheus.GaugeValue, usage)
	return nil
}

func (c *SystemCollector) collectConntrack(ch chan<- prometheus.Metric) error {
	// 优先读 nf_conntrack_* sysctl；不可用时返回错误由 scrape_errors 记录（README 说明后备）
	countPath := filepath.Join(c.paths.Proc, "sys", "net", "netfilter", "nf_conntrack_count")
	maxPath := filepath.Join(c.paths.Proc, "sys", "net", "netfilter", "nf_conntrack_max")

	count, err := readFloatFile(countPath)
	if err != nil {
		return fmt.Errorf("conntrack count unavailable (%v); ensure nf_conntrack is loaded", err)
	}
	max, err := readFloatFile(maxPath)
	if err != nil {
		return err
	}
	ch <- prometheus.MustNewConstMetric(c.conntrackEntries, prometheus.GaugeValue, count)
	ch <- prometheus.MustNewConstMetric(c.conntrackMax, prometheus.GaugeValue, max)
	return nil
}

func readFloatFile(path string) (float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
}
