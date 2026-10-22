# openwrt-exporter

面向 OpenWrt 的 Prometheus exporter（Go）。目标平台示例：MediaTek Filogic（约 512MB RAM）。

默认以读内核/宿主已有计数为主（`/proc`、sysfs、ubus/nl80211），**不做抓包**；有线吞吐仅接口级，**不做有线每设备速率**。

## 状态

脚手架阶段：HTTP `/metrics`、五个 collector stub、flag + 极简 UCI、procd/UCI 示例。真实采集尚未实现。

## 模块

| 模块 | flag | 计划数据源 |
|------|------|------------|
| system | `-collector.system` | `/proc/loadavg`、`/proc/uptime`、thermal sysfs |
| netdev | `-collector.netdev` | `/proc/net/dev`、sysfs operstate/carrier |
| dhcp | `-collector.dhcp` | `/tmp/dhcp.leases` / odhcpd / ubus |
| wireless | `-collector.wireless` | hostapd via ubus 或 nl80211 stations |
| bridge_fdb | `-collector.bridge_fdb` | 桥 FDB（有线接入，无 per-MAC 速率） |

## 本地构建

```bash
go build -o bin/openwrt-exporter ./cmd/openwrt-exporter
./bin/openwrt-exporter -listen :9100
curl -s localhost:9100/metrics
```

常用 flag：

- `-listen` 监听地址（默认 `:9100`）
- `-web.telemetry-path` 指标路径（默认 `/metrics`）
- `-uci` UCI 文件路径（默认 `/etc/config/openwrt-exporter`；文件不存在则忽略）
- `-collector.<name>` 模块开关

## OpenWrt 部署（示例）

仓库内示例文件：

- [`openwrt/files/etc/init.d/openwrt-exporter`](openwrt/files/etc/init.d/openwrt-exporter) — procd，`respawn`，前台运行
- [`openwrt/files/etc/config/openwrt-exporter`](openwrt/files/etc/config/openwrt-exporter) — UCI 默认配置

进程由 procd 托管，**不**自行 daemonize；日志走 stdout → `logread`。

```sh
/etc/init.d/openwrt-exporter enable
/etc/init.d/openwrt-exporter start
```

## 范围外

- 有线 per-MAC 速率（nft/eBPF/抓包）
- 忙轮询 / 亚秒 ticker / 热路径 fork CLI（`iw`、`iptables -L` 等）
