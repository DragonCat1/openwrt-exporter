package collector

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/DragonCat1/openwrt-exporter/internal/ubus"
)

// WirelessCollector 无线关联站：ubus hostapd.get_clients + DHCP 租约按 MAC join。
// station_info 带 hostname/ip/ssid；字节 Counter 与速率 Gauge 仅 iface+mac，避免 label 基数膨胀。
type WirelessCollector struct {
	client     ubus.Client
	leasePaths []string

	stations      *prometheus.Desc
	stationInfo   *prometheus.Desc
	stationSignal *prometheus.Desc
	stationNoise  *prometheus.Desc
	rxRateMbps    *prometheus.Desc
	txRateMbps    *prometheus.Desc
	rxBytes       *prometheus.Desc
	txBytes       *prometheus.Desc
	scrapeErrors  *prometheus.Desc
}

func NewWireless() *WirelessCollector {
	return NewWirelessWith(ubus.NewExecClient(), defaultLeasePaths())
}

func NewWirelessWithClient(client ubus.Client) *WirelessCollector {
	return NewWirelessWith(client, defaultLeasePaths())
}

func NewWirelessWith(client ubus.Client, leasePaths []string) *WirelessCollector {
	idLabels := []string{"iface", "mac"}
	infoLabels := []string{"iface", "mac", "hostname", "ip", "ssid"}
	return &WirelessCollector{
		client:     client,
		leasePaths: leasePaths,
		stations: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "stations"),
			"Number of associated wireless stations across hostapd interfaces.",
			nil, nil,
		),
		stationInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_info"),
			"Associated station present (value 1). hostname/ip from DHCP lease join by MAC; missing → \"-\".",
			infoLabels, nil,
		),
		stationSignal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_signal_dbm"),
			"Station signal strength in dBm (hostapd). Labels: iface+mac (join info via mac).",
			idLabels, nil,
		),
		stationNoise: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_noise_dbm"),
			"Station noise floor in dBm when reported by hostapd.",
			idLabels, nil,
		),
		rxRateMbps: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_receive_rate_mbps"),
			"Station RX PHY rate in Mbps (from hostapd rate.rx, typically kbps/1000).",
			idLabels, nil,
		),
		txRateMbps: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_transmit_rate_mbps"),
			"Station TX PHY rate in Mbps (from hostapd rate.tx, typically kbps/1000).",
			idLabels, nil,
		),
		rxBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_receive_bytes_total"),
			"Per-station RX bytes from hostapd (Counter; use rate() for throughput). Labels: iface+mac.",
			idLabels, nil,
		),
		txBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_transmit_bytes_total"),
			"Per-station TX bytes from hostapd (Counter; use rate() for throughput). Labels: iface+mac.",
			idLabels, nil,
		),
		scrapeErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "scrape_errors_total"),
			"Partial scrape failures in wireless collector (this Collect). Non-zero on hosts without ubus/hostapd.",
			nil, nil,
		),
	}
}

func (c *WirelessCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.stations
	ch <- c.stationInfo
	ch <- c.stationSignal
	ch <- c.stationNoise
	ch <- c.rxRateMbps
	ch <- c.txRateMbps
	ch <- c.rxBytes
	ch <- c.txBytes
	ch <- c.scrapeErrors
}

func (c *WirelessCollector) Collect(ch chan<- prometheus.Metric) {
	var errs float64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	leases := leaseIndexByMAC(c.leasePaths)

	ifaces, err := c.client.List(ctx, "hostapd.*")
	if err != nil {
		// CI/桌面无 ubus：可编译运行，指标为空并记错误；路由上验证见 README
		errs++
		ch <- prometheus.MustNewConstMetric(c.stations, prometheus.GaugeValue, 0)
		ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
		return
	}

	total := 0
	for _, path := range ifaces {
		if !strings.HasPrefix(path, "hostapd.") {
			continue
		}
		iface := strings.TrimPrefix(path, "hostapd.")
		ssid := c.fetchSSID(ctx, path)
		stations, err := c.fetchClients(ctx, path)
		if err != nil {
			errs++
			continue
		}
		for mac, st := range stations {
			mac = strings.ToLower(mac)
			host, ip := "-", "-"
			if l, ok := leases[mac]; ok {
				host = stableLeaseLabel(l.hostname)
				ip = stableLeaseLabel(l.ip)
			}
			ssidLabel := stableLeaseLabel(ssid)

			total++
			ch <- prometheus.MustNewConstMetric(c.stationInfo, prometheus.GaugeValue, 1, iface, mac, host, ip, ssidLabel)
			ch <- prometheus.MustNewConstMetric(c.rxBytes, prometheus.CounterValue, st.rxBytes, iface, mac)
			ch <- prometheus.MustNewConstMetric(c.txBytes, prometheus.CounterValue, st.txBytes, iface, mac)
			if st.signalSet {
				ch <- prometheus.MustNewConstMetric(c.stationSignal, prometheus.GaugeValue, st.signal, iface, mac)
			}
			if st.noiseSet {
				ch <- prometheus.MustNewConstMetric(c.stationNoise, prometheus.GaugeValue, st.noise, iface, mac)
			}
			if st.rxRateSet {
				ch <- prometheus.MustNewConstMetric(c.rxRateMbps, prometheus.GaugeValue, st.rxRateMbps, iface, mac)
			}
			if st.txRateSet {
				ch <- prometheus.MustNewConstMetric(c.txRateMbps, prometheus.GaugeValue, st.txRateMbps, iface, mac)
			}
		}
	}

	ch <- prometheus.MustNewConstMetric(c.stations, prometheus.GaugeValue, float64(total))
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
}

type wifiStation struct {
	rxBytes, txBytes float64
	signal, noise    float64
	signalSet        bool
	noiseSet         bool
	rxRateMbps       float64
	txRateMbps       float64
	rxRateSet        bool
	txRateSet        bool
}

func (c *WirelessCollector) fetchClients(ctx context.Context, path string) (map[string]wifiStation, error) {
	var raw json.RawMessage
	if err := c.client.Call(ctx, path, "get_clients", &raw); err != nil {
		return nil, err
	}
	return parseHostapdClients(raw)
}

// fetchSSID 读 hostapd get_status 的 ssid；失败返回空（emit 时变为 "-"）。
func (c *WirelessCollector) fetchSSID(ctx context.Context, path string) string {
	var raw json.RawMessage
	if err := c.client.Call(ctx, path, "get_status", &raw); err != nil {
		return ""
	}
	var st struct {
		SSID string `json:"ssid"`
	}
	if json.Unmarshal(raw, &st) == nil && st.SSID != "" {
		return st.SSID
	}
	// 部分固件把 ssid 放在嵌套对象
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	if s, ok := m["ssid"]; ok {
		var ss string
		if json.Unmarshal(s, &ss) == nil {
			return ss
		}
	}
	return ""
}

// parseHostapdClients 兼容 `{ "clients": { "mac": {...} } }` 与顶层直接按 mac 索引。
func parseHostapdClients(raw json.RawMessage) (map[string]wifiStation, error) {
	var wrap struct {
		Clients map[string]json.RawMessage `json:"clients"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil && wrap.Clients != nil {
		return decodeStationMap(wrap.Clients)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	delete(top, "clients")
	return decodeStationMap(top)
}

func decodeStationMap(in map[string]json.RawMessage) (map[string]wifiStation, error) {
	out := make(map[string]wifiStation, len(in))
	for mac, raw := range in {
		if !looksLikeMAC(mac) {
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		st := wifiStation{}
		if b, ok := obj["bytes"]; ok {
			var bytes struct {
				RX float64 `json:"rx"`
				TX float64 `json:"tx"`
			}
			if json.Unmarshal(b, &bytes) == nil {
				st.rxBytes = bytes.RX
				st.txBytes = bytes.TX
			}
		}
		if s, ok := obj["signal"]; ok {
			var sig float64
			if json.Unmarshal(s, &sig) == nil {
				st.signal = sig
				st.signalSet = true
			}
		}
		if n, ok := obj["noise"]; ok {
			var noise float64
			if json.Unmarshal(n, &noise) == nil {
				st.noise = noise
				st.noiseSet = true
			}
		}
		// hostapd ubus：rate.rx/tx 多为 kbps（与 LuCI 一致需 /1000 → Mbps）
		if r, ok := obj["rate"]; ok {
			var rate struct {
				RX float64 `json:"rx"`
				TX float64 `json:"tx"`
			}
			if json.Unmarshal(r, &rate) == nil {
				if rate.RX > 0 {
					st.rxRateMbps = rate.RX / 1000.0
					st.rxRateSet = true
				}
				if rate.TX > 0 {
					st.txRateMbps = rate.TX / 1000.0
					st.txRateSet = true
				}
			}
		}
		out[strings.ToLower(mac)] = st
	}
	return out, nil
}
