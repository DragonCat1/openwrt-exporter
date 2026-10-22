package collector

import (
	"path/filepath"
	"testing"
)

func TestNetdevCollectorFixtures(t *testing.T) {
	root := t.TempDir()
	proc := filepath.Join(root, "proc")
	sys := filepath.Join(root, "sys")
	mustWrite(t, filepath.Join(proc, "net", "dev"), `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 1000 10 1 2 0 0 0 0 2000 20 3 4 0 0 0 0
    lo: 100 1 0 0 0 0 0 0 100 1 0 0 0 0 0 0
`)
	mustWrite(t, filepath.Join(sys, "class", "net", "eth0", "operstate"), "up\n")
	mustWrite(t, filepath.Join(sys, "class", "net", "eth0", "carrier"), "1\n")
	mustWrite(t, filepath.Join(sys, "class", "net", "eth0", "speed"), "1000\n")
	mustWrite(t, filepath.Join(sys, "class", "net", "lo", "operstate"), "unknown\n")

	c := NewNetdevWithPaths(Paths{Proc: proc, Sys: sys})
	mfs := collectAll(t, c)

	mf := mfs["openwrt_network_receive_bytes_total"]
	if mf == nil {
		t.Fatal("missing receive_bytes")
	}
	found := false
	for _, m := range mf.Metric {
		labels := map[string]string{}
		for _, lp := range m.Label {
			labels[lp.GetName()] = lp.GetValue()
		}
		if labels["device"] == "eth0" && m.Counter.GetValue() == 1000 {
			found = true
		}
	}
	if !found {
		t.Fatal("eth0 rx bytes not found")
	}

	upMF := mfs["openwrt_network_up"]
	for _, m := range upMF.Metric {
		for _, lp := range m.Label {
			if lp.GetName() == "device" && lp.GetValue() == "eth0" {
				if m.Gauge.GetValue() != 1 {
					t.Fatalf("eth0 up want 1 got %v", m.Gauge.GetValue())
				}
			}
		}
	}
}
