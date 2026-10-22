package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestSystemCollectorFixtures(t *testing.T) {
	root := t.TempDir()
	proc := filepath.Join(root, "proc")
	sys := filepath.Join(root, "sys")
	mustWrite(t, filepath.Join(proc, "loadavg"), "0.10 0.20 0.30 1/100 1\n")
	mustWrite(t, filepath.Join(proc, "uptime"), "123.45 99.00\n")
	mustWrite(t, filepath.Join(proc, "stat"), "cpu  100 0 50 850 0 0 0 0\ncpu0 100 0 50 850 0 0 0 0\n")
	mustWrite(t, filepath.Join(proc, "meminfo"), ""+
		"MemTotal:       1024000 kB\n"+
		"MemFree:         512000 kB\n"+
		"MemAvailable:    768000 kB\n"+
		"Buffers:          10000 kB\n"+
		"Cached:           20000 kB\n")
	mustWrite(t, filepath.Join(proc, "sys", "net", "netfilter", "nf_conntrack_count"), "42\n")
	mustWrite(t, filepath.Join(proc, "sys", "net", "netfilter", "nf_conntrack_max"), "1000\n")
	zone := filepath.Join(sys, "class", "thermal", "thermal_zone0")
	mustWrite(t, filepath.Join(zone, "temp"), "45123\n")
	mustWrite(t, filepath.Join(zone, "type"), "cpu-thermal\n")

	c := NewSystemWithPaths(Paths{Proc: proc, Sys: sys})
	mfs := collectAll(t, c)

	assertGauge(t, mfs, "openwrt_load_1", 0.10)
	assertGauge(t, mfs, "openwrt_system_uptime_seconds", 123.45)
	assertGauge(t, mfs, "openwrt_thermal_celsius", 45.123)
	assertGauge(t, mfs, "openwrt_memory_usage_ratio", 1-768000.0/1024000.0)
	assertGauge(t, mfs, "openwrt_conntrack_entries", 42)
	assertGauge(t, mfs, "openwrt_conntrack_max", 1000)
	assertGauge(t, mfs, "openwrt_cpu_usage_ratio", 0) // 首次 scrape

	// 第二次：busy 增加 → usage > 0
	time.Sleep(5 * time.Millisecond)
	mustWrite(t, filepath.Join(proc, "stat"), "cpu  200 0 100 900 0 0 0 0\n")
	mfs = collectAll(t, c)
	usage := gaugeValue(t, mfs, "openwrt_cpu_usage_ratio")
	if usage <= 0 || usage > 1 {
		t.Fatalf("expected cpu usage in (0,1], got %v", usage)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func collectAll(t *testing.T, c prometheus.Collector) map[string]*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		out[mf.GetName()] = mf
	}
	return out
}

func assertGauge(t *testing.T, mfs map[string]*dto.MetricFamily, name string, want float64) {
	t.Helper()
	got := gaugeValue(t, mfs, name)
	if abs(got-want) > 1e-6 {
		t.Fatalf("%s: got %v want %v", name, got, want)
	}
}

func gaugeValue(t *testing.T, mfs map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()
	mf, ok := mfs[name]
	if !ok {
		var names []string
		for n := range mfs {
			names = append(names, n)
		}
		t.Fatalf("missing metric %s (have %s)", name, strings.Join(names, ","))
	}
	for _, m := range mf.Metric {
		if m.Gauge != nil {
			return m.Gauge.GetValue()
		}
		if m.Counter != nil {
			return m.Counter.GetValue()
		}
		if m.Untyped != nil {
			return m.Untyped.GetValue()
		}
	}
	t.Fatalf("%s: no value", name)
	return 0
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
