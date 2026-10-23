package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

type mockUbus struct {
	list []string
	resp map[string]json.RawMessage
	err  error
}

func (m *mockUbus) List(context.Context, string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.list, nil
}

func (m *mockUbus) Call(_ context.Context, path, method string, dest any) error {
	if m.err != nil {
		return m.err
	}
	raw, ok := m.resp[path+"|"+method]
	if !ok {
		raw = []byte(`{}`)
	}
	return json.Unmarshal(raw, dest)
}

func metricLabels(m *dto.Metric) map[string]string {
	out := make(map[string]string, len(m.Label))
	for _, lp := range m.Label {
		out[lp.GetName()] = lp.GetValue()
	}
	return out
}

func TestWirelessCollectorMockUbus(t *testing.T) {
	payload := []byte(`{
		"clients": {
			"aa:bb:cc:dd:ee:ff": {
				"signal": -55,
				"noise": -95,
				"bytes": {"rx": 100, "tx": 200},
				"rate": {"rx": 144400, "tx": 866700}
			}
		}
	}`)
	client := &mockUbus{
		list: []string{"hostapd.wlan0"},
		resp: map[string]json.RawMessage{
			"hostapd.wlan0|get_clients": payload,
			"hostapd.wlan0|get_status":  []byte(`{"ssid":"HomeWiFi"}`),
		},
	}
	c := NewWirelessWith(client, []string{filepath.Join(t.TempDir(), "missing.leases")})
	mfs := collectAll(t, c)
	assertGauge(t, mfs, "openwrt_wireless_stations", 1)
	assertGauge(t, mfs, "openwrt_wireless_scrape_errors_total", 0)

	rx := mfs["openwrt_wireless_station_receive_bytes_total"]
	if rx == nil || len(rx.Metric) != 1 || rx.Metric[0].Counter.GetValue() != 100 {
		t.Fatalf("unexpected rx: %#v", rx)
	}

	info := mfs["openwrt_wireless_station_info"]
	if info == nil || len(info.Metric) != 1 {
		t.Fatalf("want 1 station_info, got %#v", info)
	}
	labels := metricLabels(info.Metric[0])
	if labels["hostname"] != "-" || labels["ip"] != "-" || labels["ssid"] != "HomeWiFi" {
		t.Fatalf("missing lease should be '-', ssid filled: %#v", labels)
	}

	assertGauge(t, mfs, "openwrt_wireless_station_receive_rate_mbps", 144.4)
	assertGauge(t, mfs, "openwrt_wireless_station_transmit_rate_mbps", 866.7)
	assertGauge(t, mfs, "openwrt_wireless_station_noise_dbm", -95)
}

func TestWirelessJoinDHCPLease(t *testing.T) {
	dir := t.TempDir()
	exp := time.Now().Add(time.Hour).Unix()
	mustWrite(t, filepath.Join(dir, "dhcp.leases"),
		fmt.Sprintf("%d b2:14:f5:a6:5b:ca 192.168.1.104 TAS-AN00 *\n", exp)+
			fmt.Sprintf("%d 52:72:24:f4:62:4e 192.168.1.128 Xiaomi-14-Pro *\n", exp),
	)

	payload := []byte(`{
		"clients": {
			"B2:14:F5:A6:5B:CA": {"signal": -71, "bytes": {"rx": 1, "tx": 2}},
			"52:72:24:F4:62:4E": {"signal": -64, "bytes": {"rx": 3, "tx": 4}}
		}
	}`)
	client := &mockUbus{
		list: []string{"hostapd.phy1-ap0"},
		resp: map[string]json.RawMessage{
			"hostapd.phy1-ap0|get_clients": payload,
			"hostapd.phy1-ap0|get_status":  []byte(`{"ssid":"MyAP"}`),
		},
	}
	c := NewWirelessWith(client, LeasePathsForTest(dir))
	mfs := collectAll(t, c)
	assertGauge(t, mfs, "openwrt_wireless_stations", 2)

	info := mfs["openwrt_wireless_station_info"]
	if info == nil || len(info.Metric) != 2 {
		t.Fatalf("want 2 station_info, got %#v", info)
	}
	byMAC := map[string]map[string]string{}
	for _, m := range info.Metric {
		lm := metricLabels(m)
		byMAC[lm["mac"]] = lm
	}
	a := byMAC["b2:14:f5:a6:5b:ca"]
	if a["hostname"] != "TAS-AN00" || a["ip"] != "192.168.1.104" || a["ssid"] != "MyAP" || a["iface"] != "phy1-ap0" {
		t.Fatalf("TAS-AN00 labels: %#v", a)
	}
	b := byMAC["52:72:24:f4:62:4e"]
	if b["hostname"] != "Xiaomi-14-Pro" || b["ip"] != "192.168.1.128" {
		t.Fatalf("Xiaomi labels: %#v", b)
	}

	// 字节 Counter 只有 iface+mac，无 hostname（控制基数）
	rx := mfs["openwrt_wireless_station_receive_bytes_total"]
	for _, m := range rx.Metric {
		lm := metricLabels(m)
		if _, ok := lm["hostname"]; ok {
			t.Fatalf("bytes should not carry hostname: %#v", lm)
		}
	}
}

func TestWirelessMissingLeaseStable(t *testing.T) {
	client := &mockUbus{
		list: []string{"hostapd.wlan0"},
		resp: map[string]json.RawMessage{
			"hostapd.wlan0|get_clients": []byte(`{"clients":{"aa:bb:cc:dd:ee:01":{"signal":-50}}}`),
			"hostapd.wlan0|get_status":  []byte(`{}`),
		},
	}
	c := NewWirelessWith(client, []string{"/no/such/leases"})
	mfs := collectAll(t, c)
	info := mfs["openwrt_wireless_station_info"]
	lm := metricLabels(info.Metric[0])
	if lm["hostname"] != "-" || lm["ip"] != "-" || lm["ssid"] != "-" {
		t.Fatalf("want stable '-', got %#v", lm)
	}
}

func TestParseHostapdClientsTopLevel(t *testing.T) {
	raw := json.RawMessage(`{"11:22:33:44:55:66":{"bytes":{"rx":1,"tx":2},"signal":-70,"noise":-90,"rate":{"rx":6000,"tx":12000}}}`)
	got, err := parseHostapdClients(raw)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := got["11:22:33:44:55:66"]
	if !ok || st.rxBytes != 1 || st.txBytes != 2 || !st.signalSet || st.signal != -70 {
		t.Fatalf("unexpected: %#v", got)
	}
	if !st.noiseSet || st.noise != -90 || !st.rxRateSet || st.rxRateMbps != 6 || st.txRateMbps != 12 {
		t.Fatalf("rate/noise: %#v", st)
	}
}

func TestHostapdRateToMbps(t *testing.T) {
	// 正常 kbit/s（LuCI ÷1000）
	if g := hostapdRateToMbps(468000); g != 468 {
		t.Fatalf("kbps path: got %v want 468", g)
	}
	if g := hostapdRateToMbps(866700); g != 866.7 {
		t.Fatalf("kbps path hi: got %v", g)
	}
	// 驱动已给 kbps 再 *100 → ubus 大 100 倍（用户看到的 46800 Mbps）
	if g := hostapdRateToMbps(46800000); g != 468 {
		t.Fatalf("inflated path: got %v want 468", g)
	}
	if g := hostapdRateToMbps(58500000); g != 585 {
		t.Fatalf("inflated tx: got %v want 585", g)
	}
}
