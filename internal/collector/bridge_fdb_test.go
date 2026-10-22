package collector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestBridgeFDBParse(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "sys")
	br := filepath.Join(sys, "class", "net", "br-lan")
	mustWrite(t, filepath.Join(br, "bridge", "stp_state"), "0\n") // 标记为 bridge

	// 两条 16 字节 __fdb_entry：一条远端，一条 local
	rec := make([]byte, 32)
	copy(rec[0:6], []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x01})
	rec[6] = 2
	rec[7] = 0
	copy(rec[16:22], []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66})
	rec[16+6] = 0
	rec[16+7] = 1
	if err := os.MkdirAll(br, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(br, "brforward"), rec, 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewBridgeFDBWithPaths(Paths{Sys: sys, Proc: root})
	mfs := collectAll(t, c)
	assertGauge(t, mfs, "openwrt_bridge_fdb_entries", 1) // 仅非 local

	info := mfs["openwrt_bridge_fdb_entry_info"]
	if info == nil || len(info.Metric) != 2 {
		t.Fatalf("want 2 entry_info, got %#v", info)
	}
}

func TestBridgeFDBDedupDuplicateRows(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "sys")
	br := filepath.Join(sys, "class", "net", "br-lan")
	mustWrite(t, filepath.Join(br, "bridge", "stp_state"), "0\n")

	// 同一 local MAC/port 重复 3 次（模拟真实 brforward 重复行）
	mac := []byte{0x54, 0x4d, 0xd4, 0x67, 0x10, 0x09}
	rec := make([]byte, 48)
	for i := 0; i < 3; i++ {
		off := i * 16
		copy(rec[off:off+6], mac)
		rec[off+6] = 5 // port_no
		rec[off+7] = 1 // is_local
	}
	if err := os.MkdirAll(br, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(br, "brforward"), rec, 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewBridgeFDBWithPaths(Paths{Sys: sys, Proc: root})
	// Gather 若重复 emit 会返回 error（Prometheus duplicate metric）
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed (duplicate metrics?): %v", err)
	}
	var infoCount int
	var entries float64
	for _, mf := range mfs {
		switch mf.GetName() {
		case "openwrt_bridge_fdb_entry_info":
			infoCount = len(mf.Metric)
		case "openwrt_bridge_fdb_entries":
			entries = mf.Metric[0].Gauge.GetValue()
		}
	}
	if infoCount != 1 {
		t.Fatalf("want 1 deduped entry_info, got %d", infoCount)
	}
	if entries != 0 {
		t.Fatalf("local-only duplicates should not count as non-local entries, got %v", entries)
	}
}

func TestDedupeFDBEntries(t *testing.T) {
	in := []fdbEntry{
		{mac: "aa:bb:cc:dd:ee:ff", portNo: 1, isLocal: false},
		{mac: "aa:bb:cc:dd:ee:ff", portNo: 1, isLocal: false}, // 重复
		{mac: "aa:bb:cc:dd:ee:ff", portNo: 2, isLocal: false}, // 不同 port，保留
		{mac: "11:22:33:44:55:66", portNo: 1, isLocal: true},
		{mac: "11:22:33:44:55:66", portNo: 1, isLocal: true}, // 重复
	}
	out := dedupeFDBEntries("br-lan", in)
	if len(out) != 3 {
		t.Fatalf("want 3 unique, got %d: %#v", len(out), out)
	}
}
