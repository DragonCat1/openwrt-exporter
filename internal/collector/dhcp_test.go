package collector

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestDHCPLeasesParse(t *testing.T) {
	dir := t.TempDir()
	exp := time.Now().Add(time.Hour).Unix()
	mustWrite(t, filepath.Join(dir, "dhcp.leases"),
		"# comment\n"+
			"1000000000 aa:bb:cc:dd:ee:ff 192.168.1.10 oldhost *\n"+ // 过期
			fmt.Sprintf("%d 11:22:33:44:55:66 192.168.1.20 phone *\n", exp),
	)

	c := NewDHCPWithPaths(LeasePathsForTest(dir))
	mfs := collectAll(t, c)
	assertGauge(t, mfs, "openwrt_dhcp_leases", 1)

	info := mfs["openwrt_dhcp_lease_info"]
	if info == nil || len(info.Metric) != 1 {
		t.Fatalf("want 1 lease_info, got %#v", info)
	}
}
