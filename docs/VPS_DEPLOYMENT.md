# XPanel VPS 手动部署复现文档

本文记录将 XPanel 部署到真实 Linux VPS 的步骤。示例服务器为 Ubuntu 24.04，目标用户为 `root`，请把 IP、端口、UUID 按你的实际情况替换。

## 1. 本地准备

在项目根目录构建前端和 Linux 二进制：

```powershell
cd C:\Users\Administrator\Desktop\proxy\XPanel

cd web
npm.cmd run build
cd ..

$env:GOCACHE = Join-Path (Resolve-Path .).Path '.gocache'
$env:GOOS='linux'
$env:GOARCH='amd64'
$env:CGO_ENABLED='0'
.\.tools\go\bin\go.exe build -a -buildvcs=false -trimpath -ldflags='-s -w' -o .\dist\xpanel-linux-amd64 .\cmd\xpanel
```

准备 Xray 官方 Linux 压缩包，例如：

```text
C:\Users\Administrator\Downloads\Xray-linux-64.zip
```

## 2. 上传文件到 VPS

```powershell
ssh root@你的服务器IP "mkdir -p /tmp/xpanel-deploy"

scp .\dist\xpanel-linux-amd64 `
    .\packaging\xpanel.service `
    .\packaging\xpanel-xray.service `
    .\packaging\xpanel-sudoers `
    C:\Users\Administrator\Downloads\Xray-linux-64.zip `
    root@你的服务器IP:/tmp/xpanel-deploy/
```

## 3. 在 VPS 上安装服务

SSH 登录服务器：

```bash
ssh root@你的服务器IP
```

执行安装：

```bash
set -e
cd /tmp/xpanel-deploy

id xpanel >/dev/null 2>&1 || useradd --system --home-dir /var/lib/xpanel --shell /usr/sbin/nologin xpanel

mkdir -p /var/lib/xpanel/xray /opt/xpanel/bin

install -m 0755 ./xpanel-linux-amd64 /usr/local/bin/xpanel

if command -v unzip >/dev/null 2>&1; then
  unzip -o Xray-linux-64.zip -d /opt/xpanel/bin >/dev/null
else
  python3 -m zipfile -e Xray-linux-64.zip /opt/xpanel/bin
fi

chmod 0755 /opt/xpanel/bin/xray
chown -R xpanel:xpanel /var/lib/xpanel

install -m 0644 ./xpanel.service /etc/systemd/system/xpanel.service
install -m 0644 ./xpanel-xray.service /etc/systemd/system/xpanel-xray.service
install -m 0440 ./xpanel-sudoers /etc/sudoers.d/xpanel
visudo -cf /etc/sudoers.d/xpanel

systemctl daemon-reload
systemctl enable xpanel.service xpanel-xray.service
systemctl restart xpanel

systemctl is-active xpanel
/opt/xpanel/bin/xray version | head -n 1
```

说明：

- `xpanel.service` 是面板服务。
- `xpanel-xray.service` 是真正运行节点端口的 Xray 服务。
- 面板默认只监听 `127.0.0.1:8080`，不要直接暴露到公网，因为当前 Demo 还没有登录鉴权。

## 4. 本机安全访问面板

在 Windows PowerShell 打开 SSH 隧道：

```powershell
ssh -L 8080:127.0.0.1:8080 root@你的服务器IP
```

保持这个窗口不要关闭，然后浏览器打开：

```text
http://127.0.0.1:8080/
```

## 5. 添加节点并应用配置

在面板中添加节点时，建议：

- `listen` 填你的服务器公网 IP，例如 `139.180.205.138`。
- 协议先选 `vless`。
- 传输先选 `tcp`。
- TLS 先不开启，等基础链路跑通后再加证书。
- 端口选择一个未占用端口，例如 `24443`。

添加后点击：

```text
应用配置
```

应用配置会执行：

1. 从数据库生成 Xray 配置；
2. 使用 `/opt/xpanel/bin/xray run -test -config ...` 校验；
3. 写入 `/var/lib/xpanel/xray/config.json`；
4. 重启 `xpanel-xray.service`。

## 6. 放行节点端口

如果服务器启用了 UFW，需要放行节点端口。示例：

```bash
ufw allow 24443/tcp comment xpanel-vless
ufw status verbose
```

注意：不要放行 `8080`，除非你已经实现登录鉴权、HTTPS 和访问控制。

## 7. 验证部署

在 VPS 上检查面板：

```bash
systemctl status xpanel --no-pager
ss -lntp | grep 8080
```

预期：

```text
127.0.0.1:8080  users:(("xpanel",...))
```

检查 Xray：

```bash
systemctl status xpanel-xray --no-pager
ss -lntp | grep 24443
```

预期：

```text
你的服务器IP:24443  users:(("xray",...))
```

从本机测试 TCP 端口：

```powershell
Test-NetConnection 你的服务器IP -Port 24443
```

预期：

```text
TcpTestSucceeded : True
```

## 8. 导入客户端测试

在面板入站列表中点击节点的“导出”按钮，复制链接到 v2rayN。

如果你通过 SSH 隧道访问面板，并且节点 `listen` 填的是公网 IP，导出的链接会类似：

```text
vless://UUID@你的服务器IP:24443?type=tcp&security=none&path=%2Fxpanel#VPS%20VLESS%20TCP
```

导入后在 v2rayN 测延迟。

## 9. 常用运维命令

查看面板日志：

```bash
journalctl -u xpanel -n 100 --no-pager
```

查看 Xray 日志：

```bash
journalctl -u xpanel-xray -n 100 --no-pager
```

重启面板：

```bash
systemctl restart xpanel
```

重启 Xray：

```bash
systemctl restart xpanel-xray
```

查看生成的配置：

```bash
cat /var/lib/xpanel/xray/config.json
```

测试生成配置是否有效：

```bash
/opt/xpanel/bin/xray run -test -config /var/lib/xpanel/xray/config.json
```

## 10. 本次 VPS 验证记录

本次已在真实 VPS `139.180.205.138` 上验证：

- 面板服务：`xpanel.service` active。
- Xray 服务：`xpanel-xray.service` active。
- 面板监听：`127.0.0.1:8080`。
- 测试节点：VLESS TCP，监听 `139.180.205.138:24443`。
- UFW 已放行：`24443/tcp`。
- 本机 TCP 测试：`139.180.205.138:24443` reachable。

