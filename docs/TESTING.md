# XPanel Demo 测试手册

## 1. 测试范围

当前测试覆盖：

- inbound 领域输入校验。
- Xray 配置输出的确定性和 JSON 有效性。
- Stats API 所需基础配置的生成。
- Xray API 保留端口冲突检测。
- SQLite migration、写入和读取。
- HTTP 创建 inbound 到配置预览的完整 Demo 流程。
- Vue TypeScript 类型检查和生产构建。

## 2. 环境要求

- Go 1.26 或更高版本。
- Node.js 24 或当前维护中的 LTS 版本。
- npm 11 或兼容版本。
- 可选：目标版本的 Xray 二进制。

Windows 仓库内的便携 Go 位于 `.tools/go/bin/go.exe`，该目录不会提交 Git。

## 3. 后端测试

在仓库根目录运行：

```powershell
.\.tools\go\bin\go.exe test .\... -count=1
```

需要竞态检测时：

```powershell
.\.tools\go\bin\go.exe test -race .\... -count=1
```

预期所有 package 返回 `ok`，没有 `FAIL`。SQLite 测试只写入 `t.TempDir()`，不会修改开发数据库。

## 4. 前端测试与构建

首次运行先安装锁定依赖：

```powershell
cd web
npm.cmd install
npm.cmd run test
npm.cmd run build
```

预期：

- Vitest 正常退出。
- `vue-tsc` 不报告类型错误。
- Vite 在 `web/dist` 生成生产资源。

当前页面逻辑较薄，前端测试脚本主要验证测试运行器和工程配置；下一阶段应增加组件测试与 Playwright E2E。

## 5. 手工 API 测试

### 5.1 启动空数据库

```powershell
$env:XPANEL_DATA_DIR="$PWD\var-manual"
$env:XPANEL_SEED_DEMO='false'
.\.tools\go\bin\go.exe run .\cmd\xpanel
```

另一个终端执行：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/api/v1/health
Invoke-RestMethod http://127.0.0.1:8080/api/v1/inbounds
```

预期健康接口返回 `status=ok`，inbound 列表为空。

### 5.2 创建 VLESS inbound

```powershell
$body = @{
  remark='Manual VLESS'
  listen='0.0.0.0'
  port=10443
  protocol='vless'
  network='tcp'
  security='none'
  clientId='11111111-1111-4111-8111-111111111111'
  email='manual@example.com'
  enabled=$true
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/inbounds -ContentType application/json -Body $body
```

预期返回 HTTP 201，`tag` 为 `inbound-1`。

### 5.3 编译并校验配置

```powershell
$result = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/config/preview
$result.sha256
$result.config | ConvertTo-Json -Depth 20
```

预期：

- 返回 64 位十六进制 SHA-256。
- 配置包含 `api`、`stats`、`policy`、两个 inbound 和 routing rule。
- 重复请求得到相同 SHA-256。

### 5.4 验证错误输入

把端口改为 `70000` 再提交，预期 HTTP 422 和 `validation_failed`。重复使用相同端口或 UUID时，当前 SQLite 层会返回 HTTP 500；下一阶段将把唯一约束错误映射为明确的 HTTP 409。

## 6. 使用真实 Xray 校验

将目标 Xray 二进制放到本机，然后启动前设置：

```powershell
$env:XPANEL_XRAY_BINARY='C:\absolute\path\to\xray.exe'
.\.tools\go\bin\go.exe run .\cmd\xpanel
```

调用 `/api/v1/config/preview` 时，后端会：

1. 生成临时 JSON 文件。
2. 执行 `xray run -test -config <temp-file>`。
3. 最多等待 10 秒。
4. 删除临时文件。
5. 校验失败时返回 HTTP 422 和 `xray_validation_failed`。

该步骤只验证配置，不启动代理监听端口。

## 7. 前后端联调

终端 A：

```powershell
$env:XPANEL_SEED_DEMO='true'
.\.tools\go\bin\go.exe run .\cmd\xpanel
```

终端 B：

```powershell
cd web
npm.cmd run dev
```

浏览器访问 `http://127.0.0.1:5173`，确认：

1. 页面能读取演示 inbound。
2. 点击“编译并校验 Xray 配置”后出现 SHA-256 和 JSON。
3. 修改端口与 UUID 后可创建第二条记录。
4. 非法端口或 UUID会显示后端错误。

## 8. Linux systemd 验证

在一次性 Linux VM 中执行：

```bash
go test ./...
go build -buildvcs=false -trimpath -o dist/xpanel-linux-amd64 ./cmd/xpanel
sudo bash packaging/install-local.sh dist/xpanel-linux-amd64
systemctl is-active xpanel
curl --fail http://127.0.0.1:8080/api/v1/health
sudo systemctl restart xpanel
curl --fail http://127.0.0.1:8080/api/v1/health
```

检查 `/var/lib/xpanel/xpanel.db` 的 owner 为 `xpanel:xpanel`，服务进程不以 root 身份运行。

## 9. 已知限制

- Demo 没有认证，禁止公网暴露。
- 尚未把前端构建产物嵌入 Go 二进制。
- migration 暂为内嵌 SQL，后续迁移到版本化 migration 文件和 sqlc。
- 只实现一个 VLESS 客户端以及 TCP/WS 的最小字段。
- TLS 仅生成模式字段，尚未接入证书路径，因此真实 Xray 校验时应先使用 `security=none`。
- 尚未实现配置 revision、正式发布、进程托管和自动回滚。
- `install-local.sh` 不是联网生产安装器。

## 10. 阶段 0 验收结果记录模板

```text
日期：
提交：
操作系统：
Go / Node / Xray 版本：
go test ./...：PASS / FAIL
npm test：PASS / FAIL
npm run build：PASS / FAIL
API Demo：PASS / FAIL
真实 Xray config test：PASS / FAIL / NOT RUN
systemd VM：PASS / FAIL / NOT RUN
备注：
```
