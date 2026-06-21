# XPanel

XPanel 是一个从零实现的 Xray Web 管理面板 Demo/原型工程。它参考了常见 Xray 面板的功能形态，但不复用 x-ui 的代码。

当前版本已经支持：

- Web 面板创建 VLESS / VMess 入站节点；
- TCP / WebSocket 传输；
- TLS 字段预留；
- SQLite 持久化；
- 生成并预览 Xray JSON 配置；
- 使用真实 Xray 执行 `run -test` 配置校验；
- 将配置写入 `/var/lib/xpanel/xray/config.json`；
- 通过 systemd 托管并重启 `xpanel-xray.service`；
- 每个节点导出 `vless://` 或 `vmess://` 客户端导入链接；
- VPS 一键安装脚本。

> 安全提醒：当前版本还没有登录鉴权、HTTPS、审计和权限系统。默认安装会让面板只监听 `127.0.0.1:8080`，建议通过 SSH 隧道访问，不要直接把 8080 暴露到公网。

## 快速一键安装

适用于 Ubuntu / Debian 系服务器，使用 root 执行。

如果仓库是公开仓库：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/LiWenhui2/x-panel/main/packaging/install.sh)
```

如果仓库是私有仓库，请先在 VPS 上配置好 GitHub SSH key，然后执行：

```bash
XPANEL_REPO_URL=git@github.com:LiWenhui2/x-panel.git \
bash <(curl -fsSL https://raw.githubusercontent.com/LiWenhui2/x-panel/main/packaging/install.sh)
```

可选环境变量：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `XPANEL_REPO_URL` | `https://github.com/LiWenhui2/x-panel.git` | 源码仓库地址 |
| `XPANEL_BRANCH` | `main` | 安装分支 |
| `XPANEL_INSTALL_DIR` | `/opt/xpanel/src` | 源码目录 |
| `XPANEL_DATA_DIR` | `/var/lib/xpanel` | 数据库与 Xray 配置目录 |
| `XPANEL_XRAY_DIR` | `/opt/xpanel/bin` | Xray 二进制目录 |
| `XPANEL_LISTEN` | `127.0.0.1:8080` | 面板监听地址 |
| `XPANEL_GO_VERSION` | `1.26.4` | 安装脚本使用的 Go 版本 |
| `XPANEL_NODE_MAJOR` | `24` | 安装脚本使用的 Node.js 主版本 |

安装完成后检查：

```bash
systemctl status xpanel --no-pager
curl http://127.0.0.1:8080/api/v1/health
```

## 安全面板访问方式

由于面板默认只监听服务器本机地址，请在你的电脑上建立 SSH 隧道：

```powershell
ssh -L 18080:127.0.0.1:8080 root@你的服务器IP
```

保持这个 SSH 窗口打开，然后浏览器访问：

```text
http://127.0.0.1:18080/
```

如果你使用 `8080` 本地端口出现：

```text
bind [127.0.0.1]:8080: Permission denied
```

说明本机 8080 已被占用，换成 `18080`、`28080` 等端口即可。

## 添加并启用节点

1. 打开面板。
2. 点击“添加入站”。
3. 建议初次测试使用：
   - 协议：`vless`
   - 传输：`tcp`
   - TLS：关闭
   - listen：填写 VPS 公网 IP，例如 `139.180.205.138`
   - port：选择未占用端口，例如 `24443`
4. 保存后点击“应用配置”。
5. 在服务器上放行节点端口：

```bash
ufw allow 24443/tcp comment xpanel-vless
ufw status verbose
```

6. 回到面板，点击该节点“导出”，复制链接到 v2rayN / v2rayNG / Shadowrocket 测试。

## 服务说明

安装后有两个 systemd 服务：

| 服务 | 作用 |
|---|---|
| `xpanel.service` | Web 面板与 API 服务 |
| `xpanel-xray.service` | 面板托管的 Xray 服务端进程 |

关键路径：

| 路径 | 说明 |
|---|---|
| `/usr/local/bin/xpanel` | 面板二进制 |
| `/opt/xpanel/bin/xray` | Xray 二进制 |
| `/var/lib/xpanel/xpanel.db` | SQLite 数据库 |
| `/var/lib/xpanel/xray/config.json` | 生成的 Xray 配置 |
| `/etc/systemd/system/xpanel.service` | 面板服务 |
| `/etc/systemd/system/xpanel-xray.service` | Xray 服务 |
| `/etc/sudoers.d/xpanel` | 允许面板重启 Xray 的最小 sudoers 授权 |

## 常用运维命令

查看面板状态：

```bash
systemctl status xpanel --no-pager
journalctl -u xpanel -n 100 --no-pager
```

查看 Xray 状态：

```bash
systemctl status xpanel-xray --no-pager
journalctl -u xpanel-xray -n 100 --no-pager
```

查看监听端口：

```bash
ss -lntp
```

验证 Xray 配置：

```bash
/opt/xpanel/bin/xray run -test -config /var/lib/xpanel/xray/config.json
```

重启服务：

```bash
systemctl restart xpanel
systemctl restart xpanel-xray
```

升级到 GitHub 最新版本：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/LiWenhui2/x-panel/main/packaging/install.sh)
```

## 本地开发

后端：

```powershell
$env:XPANEL_DATA_DIR='var'
$env:XPANEL_LISTEN='127.0.0.1:8080'
.\.tools\go\bin\go.exe run .\cmd\xpanel
```

前端：

```powershell
cd web
npm.cmd install
npm.cmd run dev
```

测试：

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
cd web
npm.cmd run test
npm.cmd run build
```

生产构建 Linux 二进制：

```powershell
$env:GOCACHE = Join-Path (Resolve-Path .).Path '.gocache'
$env:GOOS='linux'
$env:GOARCH='amd64'
$env:CGO_ENABLED='0'
.\.tools\go\bin\go.exe build -a -buildvcs=false -trimpath -ldflags='-s -w' -o .\dist\xpanel-linux-amd64 .\cmd\xpanel
```

## 项目结构

```text
cmd/xpanel/              Go 服务入口
internal/api/            HTTP API
internal/inbound/        入站领域模型、校验和服务
internal/configcompiler/ Xray 配置编译器
internal/runtime/        Xray 校验与配置应用
internal/storage/sqlite/ SQLite 存储
web/                     Vue 3 + TypeScript 前端
packaging/               安装脚本、systemd、sudoers
docs/                    测试、部署和开发记录
api/openapi.yaml         OpenAPI 草稿
```

## 当前限制

- 尚未实现登录鉴权，请不要公网暴露面板端口。
- 尚未实现节点编辑/删除。
- 尚未实现流量统计和用户维度限额。
- TLS / REALITY 仍需继续完善 UI 与配置矩阵。
- 一键安装脚本目前面向 Ubuntu / Debian。

## 更多文档

- [VPS 手动部署复现文档](./docs/VPS_DEPLOYMENT.md)
- [测试说明](./docs/TESTING.md)
- [开发记录](./docs/DEVELOPMENT_LOG.md)
- [技术开发计划](./TECHNICAL_DEVELOPMENT_PLAN.md)

