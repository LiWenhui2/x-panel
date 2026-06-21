# XPanel

![platform](https://img.shields.io/badge/platform-Linux%20x64-2563eb)
![runtime](https://img.shields.io/badge/runtime-Go%201.26%20%7C%20Node.js%2024-7c3aed)
![core](https://img.shields.io/badge/core-Xray-0f766e)
![installer](https://img.shields.io/badge/installer-One--click-334155)

XPanel is a lightweight web panel for creating, applying and exporting Xray inbound nodes.

The panel is installed as a local-only service by default and should be accessed through an SSH tunnel. This keeps the management UI away from the public internet.

## One-click installation

Run as `root` on a fresh Ubuntu/Debian VPS:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/LiWenhui2/x-panel/main/packaging/install.sh)
```

During installation, the script asks for:

- panel local port;
- administrator username;
- administrator password.

Press `Enter` to use the defaults:

| Item | Default |
|---|---|
| Panel port | `8080` |
| Username | `admin` |
| Password | `admin123456` |

## Open the panel

Create an SSH tunnel from your computer:

```bash
ssh -L 18080:127.0.0.1:8080 root@YOUR_SERVER_IP
```

Then open:

```text
http://127.0.0.1:18080/
```

If local port `18080` is already used, change only the first port:

```bash
ssh -L 28080:127.0.0.1:8080 root@YOUR_SERVER_IP
```

Then open:

```text
http://127.0.0.1:28080/
```

## Add and apply a node

1. Sign in to the panel.
2. Click `Add Inbound`.
3. For the first test, use:
   - protocol: `VLESS`;
   - transport: `TCP`;
   - security: `none`;
   - listen IP: your VPS public IP;
   - port: any free port, for example `24443`.
4. Save the inbound.
5. Click `Apply Config`.
6. Allow the node port in the server firewall:

```bash
ufw allow 24443/tcp comment xpanel-node
```

7. Click `Export` in the panel and import the generated link into your client.

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
