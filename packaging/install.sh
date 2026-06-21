#!/usr/bin/env bash
set -Eeuo pipefail

REPO_URL=${XPANEL_REPO_URL:-"https://github.com/LiWenhui2/x-panel.git"}
BRANCH=${XPANEL_BRANCH:-"main"}
INSTALL_DIR=${XPANEL_INSTALL_DIR:-"/opt/xpanel/src"}
DATA_DIR=${XPANEL_DATA_DIR:-"/var/lib/xpanel"}
XRAY_DIR=${XPANEL_XRAY_DIR:-"/opt/xpanel/bin"}
GO_VERSION=${XPANEL_GO_VERSION:-"1.26.4"}
NODE_MAJOR=${XPANEL_NODE_MAJOR:-"24"}
PANEL_PORT=${XPANEL_PANEL_PORT:-"8080"}
ADMIN_USERNAME=${XPANEL_ADMIN_USERNAME:-"admin"}
ADMIN_PASSWORD=${XPANEL_ADMIN_PASSWORD:-"admin123456"}

log() { printf '\033[1;32m[XPANEL]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[XPANEL]\033[0m %s\n' "$*" >&2; exit 1; }

require_root() {
  [[ ${EUID} -eq 0 ]] || die "Please run as root."
}

prompt_value() {
  local label=$1 default=$2 value
  if [[ -t 0 ]]; then
    read -r -p "${label} [${default}]: " value || true
    printf '%s' "${value:-$default}"
  else
    printf '%s' "${default}"
  fi
}

prompt_password() {
  local label=$1 default=$2 value
  if [[ -t 0 ]]; then
    read -r -s -p "${label} [${default}]: " value || true
    printf '\n' >&2
    printf '%s' "${value:-$default}"
  else
    printf '%s' "${default}"
  fi
}

collect_setup() {
  log "Initial setup"
  PANEL_PORT=$(prompt_value "Panel local port" "${PANEL_PORT}")
  ADMIN_USERNAME=$(prompt_value "Administrator username" "${ADMIN_USERNAME}")
  ADMIN_PASSWORD=$(prompt_password "Administrator password" "${ADMIN_PASSWORD}")
  if ! [[ "${PANEL_PORT}" =~ ^[0-9]+$ ]] || (( PANEL_PORT < 1 || PANEL_PORT > 65535 )); then
    die "Invalid panel port: ${PANEL_PORT}"
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) die "Unsupported architecture: $(uname -m)" ;;
  esac
}

install_apt_deps() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y ca-certificates curl git unzip tar sudo
  if ! command -v node >/dev/null 2>&1 || ! node -v | grep -Eq "^v${NODE_MAJOR}\."; then
    log "Installing Node.js ${NODE_MAJOR}.x"
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
    apt-get install -y nodejs
  fi
}

install_go() {
  local arch archive
  arch="$(detect_arch)"
  if command -v go >/dev/null 2>&1 && go version | grep -q "go${GO_VERSION}"; then
    return
  fi
  archive="go${GO_VERSION}.linux-${arch}.tar.gz"
  log "Installing Go ${GO_VERSION}"
  curl -fL "https://go.dev/dl/${archive}" -o "/tmp/${archive}"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "/tmp/${archive}"
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
}

install_xray() {
  local arch zip
  arch="$(detect_arch)"
  case "${arch}" in
    amd64) zip="Xray-linux-64.zip" ;;
    arm64) zip="Xray-linux-arm64-v8a.zip" ;;
  esac
  log "Installing Xray core"
  install -d -m 0755 "${XRAY_DIR}"
  curl -fL "https://github.com/XTLS/Xray-core/releases/latest/download/${zip}" -o "/tmp/${zip}"
  unzip -o "/tmp/${zip}" -d "${XRAY_DIR}" >/dev/null
  chmod 0755 "${XRAY_DIR}/xray"
}

checkout_source() {
  log "Checking out source: ${REPO_URL} (${BRANCH})"
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
  log "Building web assets"
  npm --prefix "${INSTALL_DIR}/web" ci
  npm --prefix "${INSTALL_DIR}/web" run build
  log "Building xpanel binary"
  mkdir -p "${INSTALL_DIR}/dist"
  (cd "${INSTALL_DIR}" && CGO_ENABLED=0 go build -a -buildvcs=false -trimpath -ldflags='-s -w' -o dist/xpanel-linux-amd64 ./cmd/xpanel)
}

install_services() {
  log "Installing services"
  id -u xpanel >/dev/null 2>&1 || useradd --system --home-dir "${DATA_DIR}" --shell /usr/sbin/nologin xpanel
  install -d -o xpanel -g xpanel -m 0750 "${DATA_DIR}" "${DATA_DIR}/xray"
  install -d -m 0755 /etc/xpanel

  install -m 0755 "${INSTALL_DIR}/dist/xpanel-linux-amd64" /usr/local/bin/xpanel
  install -m 0755 "${INSTALL_DIR}/packaging/x-panel" /usr/local/bin/x-panel
  install -m 0644 "${INSTALL_DIR}/packaging/xpanel.service" /etc/systemd/system/xpanel.service
  install -m 0644 "${INSTALL_DIR}/packaging/xpanel-xray.service" /etc/systemd/system/xpanel-xray.service
  install -m 0440 "${INSTALL_DIR}/packaging/xpanel-sudoers" /etc/sudoers.d/xpanel
  visudo -cf /etc/sudoers.d/xpanel >/dev/null

  cat >/etc/xpanel/xpanel.env <<EOF
XPANEL_DATA_DIR=${DATA_DIR}
XPANEL_LISTEN=127.0.0.1:${PANEL_PORT}
XPANEL_XRAY_BINARY=${XRAY_DIR}/xray
XPANEL_XRAY_CONFIG=${DATA_DIR}/xray/config.json
XPANEL_RELOAD_COMMAND=/usr/bin/sudo /usr/bin/systemctl restart xpanel-xray.service
EOF

  chown -R xpanel:xpanel "${DATA_DIR}"
  XPANEL_DATA_DIR="${DATA_DIR}" /usr/local/bin/xpanel user set --username "${ADMIN_USERNAME}" --password "${ADMIN_PASSWORD}"
  systemctl daemon-reload
  systemctl enable xpanel.service xpanel-xray.service >/dev/null
  systemctl restart xpanel.service
}

post_check() {
  log "Checking panel health"
  curl -fsS "http://127.0.0.1:${PANEL_PORT}/api/v1/health" >/dev/null || die "Panel health check failed."
  log "Installation completed"
  cat <<EOF

Panel local listener:
  127.0.0.1:${PANEL_PORT}

Open an SSH tunnel from your computer:
  ssh -L 18080:127.0.0.1:${PANEL_PORT} root@your-server

Then open:
  http://127.0.0.1:18080/

Control menu:
  x-panel

Default values are used when you press Enter during setup:
  username: admin
  password: admin123456
EOF
}

main() {
  require_root
  collect_setup
  install_apt_deps
  install_go
  install_xray
  checkout_source
  build_project
  install_services
  post_check
}

main "$@"
