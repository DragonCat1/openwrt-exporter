#!/usr/bin/env bash
# 交叉编译 Release 产物（arm64 = Filogic；amd64 = 本机测）
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${ROOT}/dist"
VERSION="${VERSION:-dev}"
mkdir -p "$OUT"
rm -f "$OUT"/openwrt-exporter-linux-* "$OUT"/SHA256SUMS

build() {
  local goos=$1 goarch=$2
  local name="openwrt-exporter-${goos}-${goarch}"
  echo "building ${name} ..."
  # 静态链接，便于 OpenWrt musl/glibc 环境直接跑
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o "${OUT}/${name}" "${ROOT}/cmd/openwrt-exporter"
}

build linux arm64
build linux amd64

(
  cd "$OUT"
  sha256sum openwrt-exporter-linux-arm64 openwrt-exporter-linux-amd64 > SHA256SUMS
  ls -la
  cat SHA256SUMS
)
