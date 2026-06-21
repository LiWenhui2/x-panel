#!/usr/bin/env bash
set -Eeuo pipefail

if [[ ${EUID} -ne 0 ]]; then
  echo "请使用 sudo 运行此脚本" >&2
  exit 1
fi

SOURCE_BINARY=${1:-./dist/xpanel-linux-amd64}
if [[ ! -f ${SOURCE_BINARY} ]]; then
  echo "找不到构建产物: ${SOURCE_BINARY}" >&2
  echo "先运行: GOOS=linux GOARCH=amd64 go build -o dist/xpanel-linux-amd64 ./cmd/xpanel" >&2
  exit 1
fi

id -u xpanel >/dev/null 2>&1 || useradd --system --home-dir /var/lib/xpanel --shell /usr/sbin/nologin xpanel
install -d -o xpanel -g xpanel -m 0750 /var/lib/xpanel
install -m 0755 "${SOURCE_BINARY}" /usr/local/bin/xpanel
install -m 0644 packaging/xpanel.service /etc/systemd/system/xpanel.service
systemctl daemon-reload
systemctl enable --now xpanel
systemctl --no-pager status xpanel

echo "XPanel Demo 已安装，默认监听 127.0.0.1:8080"

