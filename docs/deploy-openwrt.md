# OpenWrt 部署指南

适用于 MediaTek Filogic 等 **aarch64（arm64）** 路由；也可按同样步骤部署到其他 `linux/arm64` 或本机 `amd64` 测试。

## 1. 获取二进制

### 方式 A：下载 Release（推荐）

1. 打开 [Releases](https://github.com/DragonCat1/openwrt-exporter/releases)
2. Filogic / 多数 arm64 OpenWrt：下载 `openwrt-exporter-linux-arm64`
3. 本机 Linux 测试：下载 `openwrt-exporter-linux-amd64`
4. （可选）对照同目录 `SHA256SUMS` 校验：

```sh
sha256sum -c SHA256SUMS --ignore-missing
```

### 方式 B：交叉编译

在开发机（需 Go 1.22+）：

```bash
git clone https://github.com/DragonCat1/openwrt-exporter.git
cd openwrt-exporter

# Filogic / OpenWrt aarch64
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags='-s -w' \
  -o openwrt-exporter-linux-arm64 ./cmd/openwrt-exporter

# 本机 amd64（可选）
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags='-s -w' \
  -o openwrt-exporter-linux-amd64 ./cmd/openwrt-exporter
```

说明：

- **必须** `CGO_ENABLED=0`，得到静态/少依赖二进制，避免 OpenWrt 上缺 glibc/musl 符号。
- Filogic 一般为 **arm64**；老款 MT7621 等为 **mipsle**，需另设 `GOARCH`（本仓库默认 Release 以 arm64/amd64 为主）。

## 2. 拷贝到路由器

假设路由 IP 为 `192.168.1.1`，SSH 可用：

```sh
scp openwrt-exporter-linux-arm64 root@192.168.1.1:/usr/bin/openwrt-exporter
scp openwrt/files/etc/init.d/openwrt-exporter root@192.168.1.1:/etc/init.d/openwrt-exporter
scp openwrt/files/etc/config/openwrt-exporter root@192.168.1.1:/etc/config/openwrt-exporter
```

在路由上：

```sh
chmod +x /usr/bin/openwrt-exporter /etc/init.d/openwrt-exporter
```

### 推荐路径

| 文件 | 路径 |
|------|------|
| 二进制 | `/usr/bin/openwrt-exporter` |
| procd init | `/etc/init.d/openwrt-exporter` |
| UCI 配置 | `/etc/config/openwrt-exporter` |

仓库内示例：

- [`openwrt/files/etc/init.d/openwrt-exporter`](../openwrt/files/etc/init.d/openwrt-exporter)
- [`openwrt/files/etc/config/openwrt-exporter`](../openwrt/files/etc/config/openwrt-exporter)

## 3. UCI 与 procd

默认 UCI（可按需改监听地址与模块开关）：

```
config openwrt-exporter 'main'
	option enabled '1'
	option listen ':9100'
	option metrics_path '/metrics'
	option collector_system '1'
	option collector_netdev '1'
	option collector_dhcp '1'
	option collector_wireless '1'
	option collector_bridge_fdb '1'
```

启用并开机自启：

```sh
/etc/init.d/openwrt-exporter enable
/etc/init.d/openwrt-exporter start
/etc/init.d/openwrt-exporter status   # 若固件支持
logread -e openwrt-exporter          # 看日志
```

进程由 **procd** 托管，**前台 Listen**，不自行 daemonize。修改 UCI 后：

```sh
/etc/init.d/openwrt-exporter reload
# 或
/etc/init.d/openwrt-exporter restart
```

手工前台调试（不经 init）：

```sh
/usr/bin/openwrt-exporter -listen :9100 -uci /etc/config/openwrt-exporter
```

## 4. 验证

```sh
curl -s http://127.0.0.1:9100/metrics | head
# 无线（需 hostapd + ubus）
ubus list 'hostapd.*'
ubus call hostapd.wlan0 get_clients
```

从局域网 Prometheus 主机应能访问 `http://<路由LAN_IP>:9100/metrics`。若不能，检查防火墙（见下）。

## 5. 常见坑

| 现象 | 处理 |
|------|------|
| `not found` / 无法执行 | 确认架构是 arm64；用 `file openwrt-exporter` 看是否 ELF aarch64；勿下成 amd64 |
| 缺动态库 | 用 `CGO_ENABLED=0` 重编；勿开 CGO |
| 端口不通 | OpenWrt 防火墙放行 9100/tcp（`lan` 区），或仅让 Prometheus 主机访问 |
| 无线指标为空 | 确认 `ubus list hostapd.*` 有对象；iface 名可能是 `wlan0`/`phy0-ap0` 等 |
| DHCP 租约为 0 | 检查 `/tmp/dhcp.leases` 是否存在（dnsmasq）；odhcpd 路径可能不同 |
| conntrack 报错 | 需加载 `nf_conntrack`；`ls /proc/sys/net/netfilter/nf_conntrack_count` |
| CPU% 首次为 0 | 正常：需至少两次 scrape 才能差分 |
| 闪存空间紧 | 二进制约数 MB；可放 `/usr/bin` 或外置存储后改 init 中的 `PROG` |

## 6. 与 opkg

当前以 **手工拷贝 + procd/UCI** 为主。若自行打 ipk，需打包：

- `/usr/bin/openwrt-exporter`
- `/etc/init.d/openwrt-exporter`
- `/etc/config/openwrt-exporter`（conffiles）

无官方 opkg 源时，不要依赖 `opkg install openwrt-exporter`，直接用 Release 产物即可。
