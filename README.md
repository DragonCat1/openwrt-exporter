# openwrt-exporter

面向 OpenWrt 的 Prometheus exporter（Go）。目标平台示例：MediaTek Filogic（约 512MB RAM）。

默认以读内核/宿主已有计数为主（`/proc`、sysfs、ubus hostapd），**不做抓包**；有线吞吐仅接口级，**不做有线每设备速率**。速率用 Prometheus `rate()`。

## 指标概览

| 模块 | 主要指标 | 数据源 |
|------|----------|--------|
| system | 负载、uptime、thermal、**CPU 使用率**、**内存使用率**、**conntrack 连接数** | `/proc/loadavg`、`/proc/uptime`、`/sys/class/thermal`、`/proc/stat` 差分、`/proc/meminfo`、`nf_conntrack_{count,max}` |
| netdev | operstate/carrier/speed、RX/TX bytes/packets/errs/drop | `/proc/net/dev` + sysfs |
| dhcp | 租约数与 lease_info | `/tmp/dhcp.leases`（后备 `/var/dhcp.leases`） |
| bridge_fdb | 桥 FDB 接入（无 per-MAC 速率） | `/sys/class/net/*/brforward` |
| wireless | 关联站、signal、每站 RX/TX bytes | `ubus call hostapd.<if> get_clients` |

### CPU / 内存 / 连接数

- `openwrt_cpu_usage_ratio`：相邻两次 scrape 对 `/proc/stat` 做差分；**首次 scrape 为 0**。
- `openwrt_memory_usage_ratio`：`1 - MemAvailable/MemTotal`。
- `openwrt_conntrack_entries` / `openwrt_conntrack_max`：优先 `nf_conntrack_*`；模块未加载时 `openwrt_system_scrape_errors_total` 会记一笔（无 `/proc/net/nf_conntrack` 全表扫描后备，避免连接风暴热点）。

### 无线（无 OpenWrt / 无 ubus 的环境）

桌面与 CI 通常没有 `ubus`/`hostapd`：进程仍可编译运行，`openwrt_wireless_stations` 为 0，`openwrt_wireless_scrape_errors_total` ≥ 1。

在路由器上验证：

```sh
ubus list 'hostapd.*'
ubus call hostapd.wlan0 get_clients
curl -s localhost:9100/metrics | grep openwrt_wireless
```

## 本地构建与测试

```bash
go test ./...
go build -o bin/openwrt-exporter ./cmd/openwrt-exporter
./bin/openwrt-exporter -listen :9100
curl -s localhost:9100/metrics
```

常用 flag：`-listen`、`-web.telemetry-path`、`-uci`、`-collector.<name>`。

## OpenWrt 部署

- [`openwrt/files/etc/init.d/openwrt-exporter`](openwrt/files/etc/init.d/openwrt-exporter) — procd，`respawn`，前台 Listen
- [`openwrt/files/etc/config/openwrt-exporter`](openwrt/files/etc/config/openwrt-exporter) — UCI

```sh
/etc/init.d/openwrt-exporter enable
/etc/init.d/openwrt-exporter start
```

## 范围外

- 有线 per-MAC 速率（nft/eBPF/抓包）
- 忙轮询 / 亚秒 ticker / 热路径频繁 fork CLI（无线 ubus 仅在 scrape 时调用）
