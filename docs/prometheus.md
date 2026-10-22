# Prometheus 与 Grafana

## scrape 配置示例

在 `prometheus.yml` 中增加 job（将地址换成路由 LAN IP）：

```yaml
scrape_configs:
  - job_name: openwrt
    scrape_interval: 15s
    scrape_timeout: 10s
    static_configs:
      - targets: ['192.168.1.1:9100']
        labels:
          instance: 'home-router'
```

建议 scrape **15s～30s**；家用路由也可 30s～60s。不要亚秒级 scrape。

## 关键指标

| 指标 | 含义 |
|------|------|
| `openwrt_cpu_usage_ratio` | CPU 忙闲比（0–1），由 `/proc/stat` 差分 |
| `openwrt_memory_usage_ratio` | 内存使用率（0–1） |
| `openwrt_load_1` / `_5` / `_15` | 负载 |
| `openwrt_thermal_celsius` | 温度（`zone`/`type`） |
| `openwrt_system_uptime_seconds` | 运行时间 |
| `openwrt_conntrack_entries` / `_max` | 连接跟踪表占用 |
| `openwrt_network_receive_bytes_total` / `transmit_bytes_total` | 接口字节 Counter |
| `openwrt_network_up` / `carrier` | 口是否 up / 有载波 |
| `openwrt_dhcp_leases` | 租约数 |
| `openwrt_wireless_stations` | 无线关联站数 |
| `openwrt_wireless_station_receive_bytes_total` / `transmit_bytes_total` | 每站字节 |

完整列表以 `/metrics` 为准。

## `rate()` 示例

接口吞吐（bps）：

```promql
rate(openwrt_network_receive_bytes_total[5m]) * 8
rate(openwrt_network_transmit_bytes_total[5m]) * 8
```

按设备过滤：

```promql
rate(openwrt_network_receive_bytes_total{device="br-lan"}[5m]) * 8
```

无线每站吞吐：

```promql
rate(openwrt_wireless_station_receive_bytes_total[5m]) * 8
rate(openwrt_wireless_station_transmit_bytes_total[5m]) * 8
```

连接表使用率：

```promql
openwrt_conntrack_entries / openwrt_conntrack_max
```

CPU / 内存（已是 ratio，可直接面板显示为百分比 `* 100`）：

```promql
openwrt_cpu_usage_ratio * 100
openwrt_memory_usage_ratio * 100
```

## Grafana 导入

1. 准备好 Prometheus 数据源，且已 scrape 到 `openwrt_*` 指标。
2. Grafana → **Dashboards** → **New** → **Import**。
3. 上传仓库中的 [`grafana/openwrt-exporter.json`](../grafana/openwrt-exporter.json)，或粘贴其内容。
4. 选择 Prometheus 数据源 → Import。

仪表盘覆盖：CPU%、内存%、负载、温度、uptime、conntrack、接口吞吐、DHCP 租约、无线站数等。若某面板无数据（如无 thermal / 无 ubus），对应图为空属正常。
