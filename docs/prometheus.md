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
| `openwrt_wireless_station_info` | 站点标识（labels：`iface`,`mac`,`hostname`,`ip`,`ssid`；无租约 → `-`） |
| `openwrt_wireless_station_signal_dbm` / `noise_dbm` | 信号 / 噪声（labels：`iface+mac`） |
| `openwrt_wireless_station_receive_rate_mbps` / `transmit_rate_mbps` | PHY 链路速率 Mbps（见下方换算） |
| `openwrt_wireless_station_receive_bytes_total` / `transmit_bytes_total` | 每站字节（`iface+mac`；吞吐用 `rate()`） |

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

无线每站吞吐（带 hostname join）：

```promql
rate(openwrt_wireless_station_receive_bytes_total[5m]) * 8
  * on(instance, mac, iface) group_left(hostname, ip)
  openwrt_wireless_station_info

rate(openwrt_wireless_station_transmit_bytes_total[5m]) * 8
  * on(instance, mac, iface) group_left(hostname, ip)
  openwrt_wireless_station_info
```

站点设备表（MAC / 主机名 / IP）：

```promql
openwrt_wireless_station_info
```

**PHY 速率换算**：OpenWrt `hostapd` ubus 将驱动 `current_*_rate`（100kbps 单位）`×100` 后写入 `rate.rx/tx`，名义为 **kbit/s**，正常 **÷1000 → Mbps**（与 LuCI 一致）。若驱动已给 kbps 仍被 ×100，会出现约 100 倍偏大（如 468Mbps 显示成 46800）；exporter 在结果 >10Gbps 时再 ÷100 纠正。

**Grafana 表为何必须 join**：`station_info` 带 `hostname/ip/ssid`，而 signal/noise/phy/字节只有 `iface+mac`。Table 里若直接并多个未对齐的 query，Prometheus 各系列各占一行，会出现「空 hostname」壳行。正确做法：以 `station_info` 为底，其它指标一律

`* on(instance, mac, iface) group_left(hostname, ip, ssid) openwrt_wireless_station_info`

再 merge，保证**一行一设备**（见仪表盘「无线站点」面板）。

**在线状态按 mac 聚合且保留主机名**：`station_info` 含 hostname/ip/ssid，直接画 timeline 会因 ip/ssid 或 `-`→具名 拆成多行。面板先 `max by (instance, mac) (...)`（**不**把 ip/ssid 放进分组），再 `* on (instance, mac) group_left (hostname)` 挂上当前主机名（`topk` 优先 `hostname!="-"`），图例 `{{hostname}} | {{mac}}`。exporter 指标仍保留全部 label。同 hostname 不同 MAC 仍为两行（正确）。

连接跟踪数量（仪表盘主曲线用 entries；可选参考 max）：

```promql
openwrt_conntrack_entries
openwrt_conntrack_max
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

仪表盘覆盖：CPU%/内存%/负载/温度/运行时间、连接跟踪、接口吞吐、**有线接口协商速率**、DHCP、无线站点表/在线状态/吞吐等。若某面板无数据（如无 thermal / 无 ubus），对应图为空属正常。

**有线协商速率 vs LuCI 端口状态**：面板查询默认 `device=~"^(lan[0-9]+|eth1|wan.*)$"` 且 `> 0`，对齐 LuCI 可见交换口/WAN。Filogic DSA 上 **eth0** 常为接交换机的 CPU/SoC 口（sysfs 可报 2500），LuCI 端口页通常不展示，故默认不画；**br-*** 桥设备 speed 易误导，亦排除。若 WAN 实际是 `eth0`，请改 PromQL 或依赖 dashboard 的 `device` 变量。
