package main

import (
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/DragonCat1/openwrt-exporter/internal/collector"
	"github.com/DragonCat1/openwrt-exporter/internal/config"
)

// version 可由 -ldflags "-X main.version=v0.1.0" 注入。
var version = "dev"

func main() {
	cfg := config.Parse(os.Args[1:])

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector.New(cfg.Modules)...)

	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><h1>openwrt-exporter</h1><p>version ` + version + `</p><p><a href="` + cfg.MetricsPath + `">metrics</a></p></body></html>`))
	})

	addr := cfg.ListenAddress
	log.Printf("openwrt-exporter %s listening on %s%s (modules=%v)", version, addr, cfg.MetricsPath, cfg.EnabledModuleNames())
	// 前台 Listen；OpenWrt 上由 procd 托管，不自行 daemonize。
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
