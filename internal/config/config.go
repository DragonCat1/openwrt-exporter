package config

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

// Modules 控制五个采集模块的开关（flag 与 UCI 共用）。
type Modules struct {
	System    bool
	Netdev    bool
	DHCP      bool
	Wireless  bool
	BridgeFDB bool
}

// Config 进程运行时配置：监听地址、指标路径、模块开关。
type Config struct {
	ListenAddress string
	MetricsPath   string
	UCIConfigPath string
	Modules       Modules
}

// Parse 先读可选 UCI 文件，再用命令行 flag 覆盖（便于本地调试）。
func Parse(args []string) Config {
	cfg := Config{
		ListenAddress: ":9100",
		MetricsPath:   "/metrics",
		UCIConfigPath: "/etc/config/openwrt-exporter",
		Modules: Modules{
			System:    true,
			Netdev:    true,
			DHCP:      true,
			Wireless:  true,
			BridgeFDB: true,
		},
	}

	fs := flag.NewFlagSet("openwrt-exporter", flag.ExitOnError)
	uciPath := fs.String("uci", cfg.UCIConfigPath, "UCI config file path (optional; missing file is OK)")
	listen := fs.String("listen", cfg.ListenAddress, "HTTP listen address")
	metricsPath := fs.String("web.telemetry-path", cfg.MetricsPath, "Path under which to expose metrics")
	system := fs.Bool("collector.system", cfg.Modules.System, "Enable system collector")
	netdev := fs.Bool("collector.netdev", cfg.Modules.Netdev, "Enable netdev collector")
	dhcp := fs.Bool("collector.dhcp", cfg.Modules.DHCP, "Enable DHCP leases collector")
	wireless := fs.Bool("collector.wireless", cfg.Modules.Wireless, "Enable wireless station collector")
	bridgeFDB := fs.Bool("collector.bridge_fdb", cfg.Modules.BridgeFDB, "Enable bridge FDB collector")

	// 先扫一遍找出 -uci，再加载 UCI，最后正式 Parse（flag 覆盖 UCI）。
	_ = fs.Parse(peekUCIArgs(args))
	cfg.UCIConfigPath = *uciPath
	if err := loadUCI(&cfg, cfg.UCIConfigPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: reading UCI %s: %v\n", cfg.UCIConfigPath, err)
	}

	// 用 UCI 结果作为 flag 默认值后重新解析完整 args。
	fs = flag.NewFlagSet("openwrt-exporter", flag.ExitOnError)
	uciPath = fs.String("uci", cfg.UCIConfigPath, "UCI config file path (optional; missing file is OK)")
	listen = fs.String("listen", cfg.ListenAddress, "HTTP listen address")
	metricsPath = fs.String("web.telemetry-path", cfg.MetricsPath, "Path under which to expose metrics")
	system = fs.Bool("collector.system", cfg.Modules.System, "Enable system collector")
	netdev = fs.Bool("collector.netdev", cfg.Modules.Netdev, "Enable netdev collector")
	dhcp = fs.Bool("collector.dhcp", cfg.Modules.DHCP, "Enable DHCP leases collector")
	wireless = fs.Bool("collector.wireless", cfg.Modules.Wireless, "Enable wireless station collector")
	bridgeFDB = fs.Bool("collector.bridge_fdb", cfg.Modules.BridgeFDB, "Enable bridge FDB collector")
	_ = fs.Parse(args)

	cfg.UCIConfigPath = *uciPath
	cfg.ListenAddress = *listen
	cfg.MetricsPath = *metricsPath
	cfg.Modules = Modules{
		System:    *system,
		Netdev:    *netdev,
		DHCP:      *dhcp,
		Wireless:  *wireless,
		BridgeFDB: *bridgeFDB,
	}
	return cfg
}

func peekUCIArgs(args []string) []string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-uci" || a == "--uci" {
			if i+1 < len(args) {
				return []string{"-uci", args[i+1]}
			}
			return nil
		}
		if strings.HasPrefix(a, "-uci=") {
			return []string{a}
		}
		if strings.HasPrefix(a, "--uci=") {
			return []string{"-uci=" + strings.TrimPrefix(a, "--uci=")}
		}
	}
	return nil
}

// loadUCI 解析极简 UCI 文本（option key 'value'），无 libuci 依赖。
func loadUCI(cfg *Config, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "option" {
			continue
		}
		key := fields[1]
		val := strings.Trim(strings.Join(fields[2:], " "), "'\"")
		switch key {
		case "listen":
			cfg.ListenAddress = val
		case "metrics_path":
			cfg.MetricsPath = val
		case "collector_system":
			cfg.Modules.System = uciBool(val, cfg.Modules.System)
		case "collector_netdev":
			cfg.Modules.Netdev = uciBool(val, cfg.Modules.Netdev)
		case "collector_dhcp":
			cfg.Modules.DHCP = uciBool(val, cfg.Modules.DHCP)
		case "collector_wireless":
			cfg.Modules.Wireless = uciBool(val, cfg.Modules.Wireless)
		case "collector_bridge_fdb":
			cfg.Modules.BridgeFDB = uciBool(val, cfg.Modules.BridgeFDB)
		}
	}
	return sc.Err()
}

func uciBool(v string, def bool) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// EnabledModuleNames 返回当前启用的模块名列表（日志用）。
func (c Config) EnabledModuleNames() []string {
	var out []string
	if c.Modules.System {
		out = append(out, "system")
	}
	if c.Modules.Netdev {
		out = append(out, "netdev")
	}
	if c.Modules.DHCP {
		out = append(out, "dhcp")
	}
	if c.Modules.Wireless {
		out = append(out, "wireless")
	}
	if c.Modules.BridgeFDB {
		out = append(out, "bridge_fdb")
	}
	return out
}
