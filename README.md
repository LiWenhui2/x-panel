# XPanel

![platform](https://img.shields.io/badge/platform-Linux%20x64-2563eb)
![runtime](https://img.shields.io/badge/runtime-Go%201.26%20%7C%20Node.js%2024-7c3aed)
![core](https://img.shields.io/badge/core-Xray-0f766e)
![installer](https://img.shields.io/badge/installer-One--click-334155)

XPanel is a lightweight web panel for creating, applying and exporting Xray inbound nodes.

## One-click installation

Run as `root` on a fresh Ubuntu/Debian VPS:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/LiWenhui2/x-panel/main/packaging/install.sh)
```

During installation, the script asks for:

- panel port;
- administrator username;
- administrator password.

The installer downloads a prebuilt Linux binary from the latest GitHub Release by default, so small VPS instances do not need to run `npm ci` or compile Go code. If no release asset is available, it falls back to building from source with a temporary swap file on low-memory servers.

Useful installer overrides:

```bash
# Force source build
XPANEL_INSTALL_MODE=source bash <(curl -fsSL https://raw.githubusercontent.com/LiWenhui2/x-panel/main/packaging/install.sh)

# Install a specific release tag
XPANEL_RELEASE_TAG=v0.1.0 bash <(curl -fsSL https://raw.githubusercontent.com/LiWenhui2/x-panel/main/packaging/install.sh)
```

## Open the panel

After installation, open the address printed by the installer:

```text
http://YOUR_SERVER_IP:PANEL_PORT/
```

For a production deployment, place the panel behind an HTTPS reverse proxy and restrict access with your cloud firewall whenever possible.

## Command line control menu

After installation, run:

```bash
x-panel
```

Menu options:

```text
1) Show service status
2) Change panel port
3) Change administrator username/password
4) Restart panel
5) Restart Xray
6) Show access hint
0) Exit
```

Common direct commands:

```bash
x-panel status
x-panel port
x-panel user
```

## Service management

Panel service:

```bash
systemctl status xpanel --no-pager
journalctl -u xpanel -n 100 --no-pager
```

Managed Xray service:

```bash
systemctl status xpanel-xray --no-pager
journalctl -u xpanel-xray -n 100 --no-pager
```

Generated Xray config:

```bash
/var/lib/xpanel/xray/config.json
```

Panel environment file:

```bash
/etc/xpanel/xpanel.env
```

## Upgrade

Run the installer again:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/LiWenhui2/x-panel/main/packaging/install.sh)
```

Existing data is kept under:

```text
/var/lib/xpanel
```
