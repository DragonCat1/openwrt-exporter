package collector

import (
	"context"
	"encoding/json"
	"testing"
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

func TestWirelessCollectorMockUbus(t *testing.T) {
	payload := []byte(`{
		"clients": {
			"aa:bb:cc:dd:ee:ff": {
				"signal": -55,
				"bytes": {"rx": 100, "tx": 200}
			}
		}
	}`)
	client := &mockUbus{
		list: []string{"hostapd.wlan0"},
		resp: map[string]json.RawMessage{
			"hostapd.wlan0|get_clients": payload,
		},
	}
	c := NewWirelessWithClient(client)
	mfs := collectAll(t, c)
	assertGauge(t, mfs, "openwrt_wireless_stations", 1)
	assertGauge(t, mfs, "openwrt_wireless_scrape_errors_total", 0)

	rx := mfs["openwrt_wireless_station_receive_bytes_total"]
	if rx == nil || len(rx.Metric) != 1 || rx.Metric[0].Counter.GetValue() != 100 {
		t.Fatalf("unexpected rx: %#v", rx)
	}
}

func TestParseHostapdClientsTopLevel(t *testing.T) {
	raw := json.RawMessage(`{"11:22:33:44:55:66":{"bytes":{"rx":1,"tx":2},"signal":-70}}`)
	got, err := parseHostapdClients(raw)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := got["11:22:33:44:55:66"]
	if !ok || st.rxBytes != 1 || st.txBytes != 2 || !st.signalSet || st.signal != -70 {
		t.Fatalf("unexpected: %#v", got)
	}
}
