# openwrt-exporter

面向 OpenWrt 的 Prometheus exporter（Go）。在路由上暴露系统健康、接口吞吐、DHCP/有线接入与无线 station 等指标；有线吞吐为**接口级**，**不做**有线每设备速率。默认读 `/proc`、sysfs、ubus，不做抓包。

目标平台示例：MediaTek Filogic（aarch64，约 512MB RAM）。

## 功能

| 模块 | 内容 |
|------|------|
| system | 负载、uptime、CPU 温度、CPU 使用率、内存使用率、conntrack 连接数 |
| netdev | 口状态（operstate/carrier）、接口 RX/TX 字节与包等 Counter |
| dhcp | DHCP 租约 |
| bridge_fdb | 有线设备接入（桥 FDB） |
| wireless | 关联站：hostname/IP（DHCP 按 MAC join）、SSID、信号/噪声、PHY 速率、RX/TX bytes |

**无线标签策略**：`openwrt_wireless_station_info` 带齐 `iface/mac/hostname/ip/ssid`（无租约时为 `-`）；字节 Counter 与信号/速率 Gauge 仅 `iface+mac`，Grafana 用 `* on(mac)` / `group_left` join。详见 [docs/prometheus.md](docs/prometheus.md)。

速率请用 Prometheus `rate()`，见 [docs/prometheus.md](docs/prometheus.md)。

## 快速使用

```bash
# 本机
./openwrt-exporter-linux-amd64 -listen :9100
curl -s localhost:9100/metrics

# OpenWrt（Filogic / arm64）
./openwrt-exporter-linux-arm64 -listen :9100
```

常用参数：`-listen`、`-web.telemetry-path`、`-uci`、`-collector.<模块名>`。

预编译包见 [Releases](https://github.com/DragonCat1/openwrt-exporter/releases)。OpenWrt 安装、交叉编译与常见坑见 **[docs/deploy-openwrt.md](docs/deploy-openwrt.md)**。

## 文档

| 文档 | 说明 |
|------|------|
| [docs/deploy-openwrt.md](docs/deploy-openwrt.md) | 交叉编译、procd/UCI、安装路径、常见坑 |
| [docs/prometheus.md](docs/prometheus.md) | scrape 配置、关键指标与 `rate()` 示例 |
| [grafana/openwrt-exporter.json](grafana/openwrt-exporter.json) | Grafana 仪表盘（导入步骤见该文件旁说明 / prometheus 文档） |

## 本地构建

```bash
go test ./...
go build -o bin/openwrt-exporter ./cmd/openwrt-exporter
```

交叉编译（Filogic）：

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags='-s -w' -o bin/openwrt-exporter-linux-arm64 ./cmd/openwrt-exporter
```
