#!/usr/bin/env bash
set -Eeuo pipefail

REPO_URL=${XPANEL_REPO_URL:-"https://github.com/LiWenhui2/x-panel.git"}
BRANCH=${XPANEL_BRANCH:-"main"}
RELEASE_REPO=${XPANEL_RELEASE_REPO:-"LiWenhui2/x-panel"}
RELEASE_TAG=${XPANEL_RELEASE_TAG:-"latest"}
INSTALL_MODE=${XPANEL_INSTALL_MODE:-"auto"}
INSTALL_DIR=${XPANEL_INSTALL_DIR:-"/opt/xpanel/src"}
DATA_DIR=${XPANEL_DATA_DIR:-"/var/lib/xpanel"}
XRAY_DIR=${XPANEL_XRAY_DIR:-"/opt/xpanel/bin"}
GO_VERSION=${XPANEL_GO_VERSION:-"1.26.4"}
NODE_MAJOR=${XPANEL_NODE_MAJOR:-"24"}
PANEL_PORT=${XPANEL_PANEL_PORT:-"8080"}
ADMIN_USERNAME=${XPANEL_ADMIN_USERNAME:-"admin"}
ADMIN_PASSWORD=${XPANEL_ADMIN_PASSWORD:-"admin123456"}
MIN_BUILD_MEMORY_MB=${XPANEL_MIN_BUILD_MEMORY_MB:-"1800"}
TEMP_SWAP_SIZE_MB=${XPANEL_TEMP_SWAP_SIZE_MB:-"2048"}
TEMP_SWAP_FILE=${XPANEL_TEMP_SWAP_FILE:-"/tmp/xpanel-install.swap"}
BINARY_PATH=""
TEMP_SWAP_CREATED=0

log() { printf '\033[1;32m[XPANEL]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[XPANEL]\033[0m %s\n' "$*" >&2; exit 1; }

progress_bar() {
  local current=$1 width=32 filled empty
  filled=$(( current * width / 100 ))
  empty=$(( width - filled ))
  printf '['
  printf '%*s' "${filled}" '' | tr ' ' '#'
  printf '%*s' "${empty}" '' | tr ' ' '-'
  printf '] %3d%%' "${current}"
}

run_with_progress() {
  local label=$1
  shift
  local progress=2 status=0 pid
  log "${label}"
  "$@" &
  pid=$!
  while kill -0 "${pid}" 2>/dev/null; do
    printf '\r\033[1;32m[XPANEL]\033[0m '
    progress_bar "${progress}"
    printf ' %s' "${label}"
    if (( progress < 95 )); then
      progress=$(( progress + 2 ))
    fi
    sleep 1
  done
  wait "${pid}" || status=$?
  if (( status == 0 )); then
    printf '\r\033[1;32m[XPANEL]\033[0m '
    progress_bar 100
    printf ' %s\n' "${label}"
  else
    printf '\r\033[1;31m[XPANEL]\033[0m '
    progress_bar "${progress}"
    printf ' %s failed\n' "${label}" >&2
  fi
  return "${status}"
}

cleanup_temp_swap() {
  if (( TEMP_SWAP_CREATED == 1 )); then
    log "Removing temporary build swap"
    swapoff "${TEMP_SWAP_FILE}" 2>/dev/null || true
    rm -f "${TEMP_SWAP_FILE}" 2>/dev/null || true
  fi
}

build_xpanel_binary() {
  local arch
  arch="$(detect_arch)"
  (cd "${INSTALL_DIR}" && GOMAXPROCS="${XPANEL_BUILD_PROCS:-1}" CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" go build -buildvcs=false -trimpath -ldflags='-s -w' -o "dist/xpanel-linux-${arch}" ./cmd/xpanel)
}

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
  PANEL_PORT=$(prompt_value "Panel port" "${PANEL_PORT}")
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

detect_server_ip() {
  local address
  address=${XPANEL_PUBLIC_IP:-}
  if [[ -z "${address}" ]]; then
    address=$(curl -4fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)
  fi
  if ! [[ "${address}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
    address=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "src") {print $(i+1); exit}}')
  fi
  if ! [[ "${address}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
    address=$(hostname -I 2>/dev/null | awk '{print $1}')
  fi
  printf '%s' "${address:-127.0.0.1}"
}

install_base_deps() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y --no-install-recommends ca-certificates curl git unzip tar sudo ufw
}

install_build_deps() {
  export DEBIAN_FRONTEND=noninteractive
  if ! command -v node >/dev/null 2>&1 || ! node -v | grep -Eq "^v${NODE_MAJOR}\."; then
    log "Installing Node.js ${NODE_MAJOR}.x"
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
    apt-get install -y --no-install-recommends nodejs
  fi
}

ensure_build_memory() {
  local mem_mb swap_mb total_mb
  mem_mb=$(awk '/MemTotal:/ {print int($2 / 1024)}' /proc/meminfo 2>/dev/null || echo 0)
  swap_mb=$(awk '/SwapTotal:/ {print int($2 / 1024)}' /proc/meminfo 2>/dev/null || echo 0)
  total_mb=$(( mem_mb + swap_mb ))

  if (( total_mb >= MIN_BUILD_MEMORY_MB || TEMP_SWAP_SIZE_MB <= 0 )); then
    return
  fi
  if [[ -e "${TEMP_SWAP_FILE}" ]]; then
    log "Low memory detected, but ${TEMP_SWAP_FILE} already exists; skipping temporary swap creation"
    return
  fi

  log "Low memory detected (${mem_mb}MB RAM, ${swap_mb}MB swap); creating ${TEMP_SWAP_SIZE_MB}MB temporary build swap"
  if command -v fallocate >/dev/null 2>&1; then
    fallocate -l "${TEMP_SWAP_SIZE_MB}M" "${TEMP_SWAP_FILE}" || dd if=/dev/zero of="${TEMP_SWAP_FILE}" bs=1M count="${TEMP_SWAP_SIZE_MB}" status=progress
  else
    dd if=/dev/zero of="${TEMP_SWAP_FILE}" bs=1M count="${TEMP_SWAP_SIZE_MB}" status=progress
  fi
  chmod 600 "${TEMP_SWAP_FILE}"
  mkswap "${TEMP_SWAP_FILE}" >/dev/null
  swapon "${TEMP_SWAP_FILE}"
  TEMP_SWAP_CREATED=1
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
    log "Existing source tree found; resetting it to origin/${BRANCH}"
    git -C "${INSTALL_DIR}" remote set-url origin "${REPO_URL}"
    git -C "${INSTALL_DIR}" fetch origin "${BRANCH}" --prune
    git -C "${INSTALL_DIR}" reset --hard
    git -C "${INSTALL_DIR}" clean -fd
    git -C "${INSTALL_DIR}" checkout -B "${BRANCH}" "origin/${BRANCH}"
    git -C "${INSTALL_DIR}" reset --hard "origin/${BRANCH}"
    git -C "${INSTALL_DIR}" clean -fd
  else
    rm -rf "${INSTALL_DIR}"
    git clone --branch "${BRANCH}" --depth 1 "${REPO_URL}" "${INSTALL_DIR}"
  fi
}

release_download_url() {
  local asset=$1
  if [[ "${RELEASE_TAG}" == "latest" ]]; then
    printf 'https://github.com/%s/releases/latest/download/%s' "${RELEASE_REPO}" "${asset}"
  else
    printf 'https://github.com/%s/releases/download/%s/%s' "${RELEASE_REPO}" "${RELEASE_TAG}" "${asset}"
  fi
}

install_prebuilt_binary() {
  local arch asset url target
  case "${INSTALL_MODE}" in
    auto|release|source) ;;
    *) die "Invalid XPANEL_INSTALL_MODE: ${INSTALL_MODE}. Use auto, release, or source." ;;
  esac
  if [[ "${INSTALL_MODE}" == "source" ]]; then
    return 1
  fi

  arch="$(detect_arch)"
  asset="xpanel-linux-${arch}"
  url="$(release_download_url "${asset}")"
  target="/tmp/${asset}"
  log "Downloading prebuilt xpanel binary: ${url}"
  if curl -fL --retry 3 --retry-delay 2 "${url}" -o "${target}"; then
    chmod 0755 "${target}"
    BINARY_PATH="${target}"
    return 0
  fi

  if [[ "${INSTALL_MODE}" == "release" ]]; then
    die "Failed to download release binary. Set XPANEL_INSTALL_MODE=source to build on this server."
  fi
  log "Prebuilt binary unavailable; falling back to source build"
  return 1
}

build_project() {
  local arch
  arch="$(detect_arch)"
  install_build_deps
  install_go
  ensure_build_memory
  log "Building web assets"
  npm --prefix "${INSTALL_DIR}/web" ci --no-audit --no-fund --loglevel=error
  npm --prefix "${INSTALL_DIR}/web" run build -- --mode production
  mkdir -p "${INSTALL_DIR}/dist"
  run_with_progress "Building xpanel binary" build_xpanel_binary
  BINARY_PATH="${INSTALL_DIR}/dist/xpanel-linux-${arch}"
}

install_services() {
  log "Installing services"
  id -u xpanel >/dev/null 2>&1 || useradd --system --home-dir "${DATA_DIR}" --shell /usr/sbin/nologin xpanel
  install -d -o xpanel -g xpanel -m 0750 "${DATA_DIR}" "${DATA_DIR}/xray"
  install -d -m 0755 /etc/xpanel

  [[ -n "${BINARY_PATH}" && -f "${BINARY_PATH}" ]] || die "Built xpanel binary not found."
  install -m 0755 "${BINARY_PATH}" /usr/local/bin/xpanel
  install -m 0755 "${INSTALL_DIR}/packaging/x-panel" /usr/local/bin/x-panel
  install -d -m 0755 /usr/local/libexec
  install -m 0755 "${INSTALL_DIR}/packaging/xpanel-allow-port" /usr/local/libexec/xpanel-allow-port
  install -m 0755 "${INSTALL_DIR}/packaging/xpanel-control" /usr/local/libexec/xpanel-control
  install -m 0644 "${INSTALL_DIR}/packaging/xpanel.service" /etc/systemd/system/xpanel.service
  install -m 0644 "${INSTALL_DIR}/packaging/xpanel-xray.service" /etc/systemd/system/xpanel-xray.service
  install -m 0440 "${INSTALL_DIR}/packaging/xpanel-sudoers" /etc/sudoers.d/xpanel
  visudo -cf /etc/sudoers.d/xpanel >/dev/null

  cat >/etc/xpanel/xpanel.env <<EOF
XPANEL_DATA_DIR=${DATA_DIR}
XPANEL_LISTEN=0.0.0.0:${PANEL_PORT}
XPANEL_XRAY_BINARY=${XRAY_DIR}/xray
XPANEL_XRAY_CONFIG=${DATA_DIR}/xray/config.json
XPANEL_RELOAD_COMMAND="/usr/bin/sudo /usr/bin/systemctl restart xpanel-xray.service"
XPANEL_FIREWALL_COMMAND="/usr/bin/sudo /usr/local/libexec/xpanel-allow-port"
XPANEL_CONTROL_COMMAND="/usr/bin/sudo /usr/local/libexec/xpanel-control"
EOF

  chown -R xpanel:xpanel "${DATA_DIR}"
  XPANEL_DATA_DIR="${DATA_DIR}" /usr/local/bin/xpanel user set --username "${ADMIN_USERNAME}" --password "${ADMIN_PASSWORD}"
  systemctl daemon-reload
  systemctl enable xpanel.service xpanel-xray.service >/dev/null
  systemctl restart xpanel.service
  if ufw status | grep -q '^Status: active'; then
    ufw allow "${PANEL_PORT}/tcp" comment 'XPanel web panel' >/dev/null
  fi
}

post_check() {
  local server_ip
  log "Checking panel health"
  for attempt in $(seq 1 30); do
    if curl -fsS "http://127.0.0.1:${PANEL_PORT}/api/v1/health" >/dev/null 2>&1; then
      server_ip=$(detect_server_ip)
      log "Installation completed"
      cat <<EOF

Panel listener:
  0.0.0.0:${PANEL_PORT}

Open in your browser:
  http://${server_ip}:${PANEL_PORT}/

Control menu:
  x-panel
EOF
      return
    fi
    if ! systemctl is-active --quiet xpanel.service; then
      systemctl --no-pager --full status xpanel.service || true
      journalctl -u xpanel.service -n 80 --no-pager || true
      die "Panel service failed to start."
    fi
    sleep 1
  done
  systemctl --no-pager --full status xpanel.service || true
  journalctl -u xpanel.service -n 80 --no-pager || true
  ss -lntp || true
  die "Panel health check failed after waiting 30 seconds."
}

main() {
  trap cleanup_temp_swap EXIT
  require_root
  collect_setup
  install_base_deps
  install_xray
  checkout_source
  install_prebuilt_binary || build_project
  install_services
  post_check
}

main "$@"
