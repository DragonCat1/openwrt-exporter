package collector

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/DragonCat1/openwrt-exporter/internal/config"
)

const namespace = "openwrt"

// New 按模块开关注册 stub collector（后续实现真实 Collect）。
func New(m config.Modules) []prometheus.Collector {
	var cols []prometheus.Collector
	if m.System {
		cols = append(cols, NewSystem())
	}
	if m.Netdev {
		cols = append(cols, NewNetdev())
	}
	if m.DHCP {
		cols = append(cols, NewDHCP())
	}
	if m.Wireless {
		cols = append(cols, NewWireless())
	}
	if m.BridgeFDB {
		cols = append(cols, NewBridgeFDB())
	}
	return cols
}
