package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// BridgeFDBCollector 有线接入视图：桥 FDB 条目（无每设备速率）。
type BridgeFDBCollector struct {
	paths Paths

	entriesTotal *prometheus.Desc
	entryInfo    *prometheus.Desc
	scrapeErrors *prometheus.Desc
}

func NewBridgeFDB() *BridgeFDBCollector {
	return NewBridgeFDBWithPaths(defaultPaths())
}

func NewBridgeFDBWithPaths(p Paths) *BridgeFDBCollector {
	return &BridgeFDBCollector{
		paths: p,
		entriesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "bridge_fdb", "entries"),
			"Number of non-local bridge FDB entries across all bridges.",
			nil, nil,
		),
		entryInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "bridge_fdb", "entry_info"),
			"Bridge FDB entry present (value 1). Wired attachment only; no per-MAC rates.",
			[]string{"bridge", "mac", "port_no", "is_local"}, nil,
		),
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "bridge_fdb", "scrape_errors_total"),
			"Partial scrape failures in bridge FDB collector (this Collect).",
			nil, nil,
		),
	}
}

func (c *BridgeFDBCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.entriesTotal
	ch <- c.entryInfo
	ch <- c.scrapeErrors
}

func (c *BridgeFDBCollector) Collect(ch chan<- prometheus.Metric) {
	var errs float64
	bridges, err := listBridges(filepath.Join(c.paths.Sys, "class", "net"))
	if err != nil {
		errs++
		ch <- prometheus.MustNewConstMetric(c.entriesTotal, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
		return
	}

	// 同一 bridge/mac/port/is_local 可能在 brforward 中出现多次（VLAN/多路径等），
	// Prometheus 禁止同名同 label 重复 emit，故按 label 键去重后再写出。
	nonLocal := 0
	for _, br := range bridges {
		entries, err := readBrforward(filepath.Join(c.paths.Sys, "class", "net", br, "brforward"))
		if err != nil {
			// 部分内核用 netlink-only，无 brforward；记错误继续
			errs++
			continue
		}
		for _, e := range dedupeFDBEntries(br, entries) {
			key := e.labelKey(br)
			if !e.isLocal {
				nonLocal++
			}
			ch <- prometheus.MustNewConstMetric(
				c.entryInfo, prometheus.GaugeValue, 1,
				key.bridge, key.mac, key.portNo, key.isLocal,
			)
		}
	}
	ch <- prometheus.MustNewConstMetric(c.entriesTotal, prometheus.GaugeValue, float64(nonLocal))
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
}

// fdbLabelKey 与 entry_info 的 label 一一对应，用作去重键。
type fdbLabelKey struct {
	bridge, mac, portNo, isLocal string
}

func (e fdbEntry) labelKey(bridge string) fdbLabelKey {
	isLocal := "0"
	if e.isLocal {
		isLocal = "1"
	}
	return fdbLabelKey{
		bridge:  bridge,
		mac:     e.mac,
		portNo:  fmt.Sprintf("%d", e.portNo),
		isLocal: isLocal,
	}
}

// dedupeFDBEntries 在单桥内按 label 键去重，保留首次出现。
func dedupeFDBEntries(bridge string, entries []fdbEntry) []fdbEntry {
	seen := make(map[fdbLabelKey]struct{}, len(entries))
	out := make([]fdbEntry, 0, len(entries))
	for _, e := range entries {
		k := e.labelKey(bridge)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, e)
	}
	return out
}

func listBridges(netClass string) ([]string, error) {
	ents, err := os.ReadDir(netClass)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		name := e.Name()
		// 存在 bridge/ 子目录即网桥
		if st, err := os.Stat(filepath.Join(netClass, name, "bridge")); err == nil && st.IsDir() {
			out = append(out, name)
		}
	}
	return out, nil
}

// fdbEntry 对应 Linux `__fdb_entry`（16 字节），见 include/uapi/linux/if_bridge.h。
type fdbEntry struct {
	mac     string
	portNo  uint16
	isLocal bool
}

func readBrforward(path string) ([]fdbEntry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	const size = 16
	if len(b)%size != 0 {
		return nil, fmt.Errorf("brforward: length %d not multiple of %d", len(b), size)
	}
	var out []fdbEntry
	for i := 0; i+size <= len(b); i += size {
		rec := b[i : i+size]
		mac := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
			rec[0], rec[1], rec[2], rec[3], rec[4], rec[5])
		portLo := rec[6]
		isLocal := rec[7] != 0
		portHi := rec[12]
		portNo := uint16(portLo) | uint16(portHi)<<8
		out = append(out, fdbEntry{mac: strings.ToLower(mac), portNo: portNo, isLocal: isLocal})

	}
	return out, nil
}
