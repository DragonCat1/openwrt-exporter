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

func main() {
	cfg := config.Parse(os.Args[1:])

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector.New(cfg.Modules)...)

	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><a href="` + cfg.MetricsPath + `">metrics</a></body></html>`))
	})

	addr := cfg.ListenAddress
	log.Printf("openwrt-exporter listening on %s%s (modules=%v)", addr, cfg.MetricsPath, cfg.EnabledModuleNames())
	// 前台 Listen；OpenWrt 上由 procd 托管，不自行 daemonize。
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
