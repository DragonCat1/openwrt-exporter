package collector

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/DragonCat1/openwrt-exporter/internal/ubus"
)

// WirelessCollector 无线关联站：优先 ubus hostapd.get_clients；失败时 scrape_errors+1 且站点数为 0。
type WirelessCollector struct {
	client ubus.Client

	stations      *prometheus.Desc
	stationInfo   *prometheus.Desc
	stationSignal *prometheus.Desc
	rxBytes       *prometheus.Desc
	txBytes       *prometheus.Desc
	scrapeErrors  *prometheus.Desc
}

func NewWireless() *WirelessCollector {
	return NewWirelessWithClient(ubus.NewExecClient())
}

func NewWirelessWithClient(client ubus.Client) *WirelessCollector {
	labels := []string{"iface", "mac"}
	return &WirelessCollector{
		client: client,
		stations: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "stations"),
			"Number of associated wireless stations across hostapd interfaces.",
			nil, nil,
		),
		stationInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_info"),
			"Associated station present (value 1).",
			labels, nil,
		),
		stationSignal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_signal_dbm"),
			"Station signal strength in dBm (if reported by hostapd).",
			labels, nil,
		),
		rxBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_receive_bytes_total"),
			"Per-station RX bytes from hostapd (Counter; use rate() for throughput).",
			labels, nil,
		),
		txBytes: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "wireless", "station_transmit_bytes_total"),
			"Per-station TX bytes from hostapd (Counter; use rate() for throughput).",
			labels, nil,
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
	ch <- c.rxBytes
	ch <- c.txBytes
	ch <- c.scrapeErrors
}

func (c *WirelessCollector) Collect(ch chan<- prometheus.Metric) {
	var errs float64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
		stations, err := c.fetchClients(ctx, path)
		if err != nil {
			errs++
			continue
		}
		for mac, st := range stations {
			mac = strings.ToLower(mac)
			total++
			ch <- prometheus.MustNewConstMetric(c.stationInfo, prometheus.GaugeValue, 1, iface, mac)
			ch <- prometheus.MustNewConstMetric(c.rxBytes, prometheus.CounterValue, st.rxBytes, iface, mac)
			ch <- prometheus.MustNewConstMetric(c.txBytes, prometheus.CounterValue, st.txBytes, iface, mac)
			if st.signalSet {
				ch <- prometheus.MustNewConstMetric(c.stationSignal, prometheus.GaugeValue, st.signal, iface, mac)
			}
		}
	}

	ch <- prometheus.MustNewConstMetric(c.stations, prometheus.GaugeValue, float64(total))
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.GaugeValue, errs)
}

type wifiStation struct {
	rxBytes, txBytes float64
	signal           float64
	signalSet        bool
}

func (c *WirelessCollector) fetchClients(ctx context.Context, path string) (map[string]wifiStation, error) {
	var raw json.RawMessage
	if err := c.client.Call(ctx, path, "get_clients", &raw); err != nil {
		return nil, err
	}
	return parseHostapdClients(raw)
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
		out[mac] = st
	}
	return out, nil
}
