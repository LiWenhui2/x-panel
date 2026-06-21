#!/usr/bin/env bash
set -Eeuo pipefail

REPO_URL=${XPANEL_REPO_URL:-"https://github.com/LiWenhui2/x-panel.git"}
BRANCH=${XPANEL_BRANCH:-"main"}
INSTALL_DIR=${XPANEL_INSTALL_DIR:-"/opt/xpanel/src"}
DATA_DIR=${XPANEL_DATA_DIR:-"/var/lib/xpanel"}
XRAY_DIR=${XPANEL_XRAY_DIR:-"/opt/xpanel/bin"}
PANEL_LISTEN=${XPANEL_LISTEN:-"127.0.0.1:8080"}
GO_VERSION=${XPANEL_GO_VERSION:-"1.26.4"}
NODE_MAJOR=${XPANEL_NODE_MAJOR:-"24"}

log() { printf '\033[1;32m[XPANEL]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[XPANEL]\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31m[XPANEL]\033[0m %s\n' "$*" >&2; exit 1; }

require_root() {
  if [[ ${EUID} -ne 0 ]]; then
    die "请使用 root 运行：sudo bash packaging/install.sh"
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) die "暂不支持当前架构：$(uname -m)" ;;
  esac
}

install_apt_deps() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y ca-certificates curl git unzip tar sudo
  if ! command -v node >/dev/null 2>&1 || ! node -v | grep -Eq "^v${NODE_MAJOR}\."; then
    log "安装 Node.js ${NODE_MAJOR}.x"
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
    apt-get install -y nodejs
  fi
}

install_go() {
  local arch archive url
  arch="$(detect_arch)"
  if command -v go >/dev/null 2>&1 && go version | grep -q "go${GO_VERSION}"; then
    return
  fi
  archive="go${GO_VERSION}.linux-${arch}.tar.gz"
  url="https://go.dev/dl/${archive}"
  log "安装 Go ${GO_VERSION}"
  curl -fL "${url}" -o "/tmp/${archive}"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "/tmp/${archive}"
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
}

install_xray() {
  local arch zip url
  arch="$(detect_arch)"
  case "${arch}" in
    amd64) zip="Xray-linux-64.zip" ;;
    arm64) zip="Xray-linux-arm64-v8a.zip" ;;
  esac
  url="https://github.com/XTLS/Xray-core/releases/latest/download/${zip}"
  log "安装 Xray core"
  install -d -m 0755 "${XRAY_DIR}"
  curl -fL "${url}" -o "/tmp/${zip}"
  unzip -o "/tmp/${zip}" -d "${XRAY_DIR}" >/dev/null
  chmod 0755 "${XRAY_DIR}/xray"
}

checkout_source() {
  log "拉取源码：${REPO_URL} (${BRANCH})"
  install -d -m 0755 "$(dirname "${INSTALL_DIR}")"
  if [[ -d "${INSTALL_DIR}/.git" ]]; then
    git -C "${INSTALL_DIR}" fetch --all --prune
    git -C "${INSTALL_DIR}" checkout "${BRANCH}"
    git -C "${INSTALL_DIR}" pull --ff-only origin "${BRANCH}"
  else
    rm -rf "${INSTALL_DIR}"
    git clone --branch "${BRANCH}" --depth 1 "${REPO_URL}" "${INSTALL_DIR}"
  fi
}

build_project() {
  log "构建前端"
  npm --prefix "${INSTALL_DIR}/web" ci
  npm --prefix "${INSTALL_DIR}/web" run build

  log "构建后端"
  mkdir -p "${INSTALL_DIR}/dist"
  (cd "${INSTALL_DIR}" && CGO_ENABLED=0 go build -a -buildvcs=false -trimpath -ldflags='-s -w' -o dist/xpanel-linux-amd64 ./cmd/xpanel)
}

install_services() {
  log "安装 systemd 服务"
  id -u xpanel >/dev/null 2>&1 || useradd --system --home-dir "${DATA_DIR}" --shell /usr/sbin/nologin xpanel
  install -d -o xpanel -g xpanel -m 0750 "${DATA_DIR}" "${DATA_DIR}/xray"

  install -m 0755 "${INSTALL_DIR}/dist/xpanel-linux-amd64" /usr/local/bin/xpanel
  install -m 0644 "${INSTALL_DIR}/packaging/xpanel.service" /etc/systemd/system/xpanel.service
  install -m 0644 "${INSTALL_DIR}/packaging/xpanel-xray.service" /etc/systemd/system/xpanel-xray.service
  install -m 0440 "${INSTALL_DIR}/packaging/xpanel-sudoers" /etc/sudoers.d/xpanel
  visudo -cf /etc/sudoers.d/xpanel >/dev/null

  mkdir -p /etc/systemd/system/xpanel.service.d
  cat >/etc/systemd/system/xpanel.service.d/override.conf <<EOF
[Service]
Environment=XPANEL_DATA_DIR=${DATA_DIR}
Environment=XPANEL_LISTEN=${PANEL_LISTEN}
Environment=XPANEL_XRAY_BINARY=${XRAY_DIR}/xray
Environment=XPANEL_XRAY_CONFIG=${DATA_DIR}/xray/config.json
Environment="XPANEL_RELOAD_COMMAND=/usr/bin/sudo /usr/bin/systemctl restart xpanel-xray.service"
ReadWritePaths=${DATA_DIR}
EOF

  chown -R xpanel:xpanel "${DATA_DIR}"
  systemctl daemon-reload
  systemctl enable xpanel.service xpanel-xray.service >/dev/null
  systemctl restart xpanel.service
}

post_check() {
  log "检查服务状态"
  systemctl --no-pager --full status xpanel.service || true
  curl -fsS "http://127.0.0.1:${PANEL_LISTEN##*:}/api/v1/health" >/dev/null || die "面板健康检查失败"
  log "安装完成"
  cat <<EOF

面板默认只监听：${PANEL_LISTEN}

如果 ${PANEL_LISTEN} 是 127.0.0.1:8080，请在本机使用 SSH 隧道访问：
  ssh -L 18080:127.0.0.1:8080 root@你的服务器IP

然后浏览器打开：
  http://127.0.0.1:18080/

添加节点后请：
  1. 在面板点击“应用配置”
  2. 放行节点端口，例如：ufw allow 24443/tcp
  3. 导出链接并导入客户端测试
EOF
}

main() {
  require_root
  install_apt_deps
  install_go
  install_xray
  checkout_source
  build_project
  install_services
  post_check
}

main "$@"
