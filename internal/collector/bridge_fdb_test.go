package collector

import (
	"os"
	"path/filepath"
	"testing"
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
