# 开发记录

## 2026-06-21：阶段 0 最小 Demo

### 已完成

- 建立 Go 模块化单体目录。
- 建立 Vue 3 + TypeScript + Vite 前端目录。
- 增加 inbound 领域模型、输入校验和 repository 接口。
- 增加 SQLite WAL repository 与初始 schema。
- 增加确定性 Xray ConfigCompiler 和 SHA-256。
- 增加 JSON Validator 与真实 Xray CommandValidator。
- 增加 health、inbound 和 config preview API。
- 增加最小管理页面、OpenAPI 骨架、systemd unit 与本地安装脚本。
- 增加 Go 单元/集成测试和测试手册。

### 关键决策

- Demo 保持 Xray 二进制可选，CI 不依赖外部下载即可测试核心逻辑。
- 配置预览只测试，不写入生效配置、不启动 Xray，避免把 POC 误当生产发布器。
- 后端默认监听 `127.0.0.1`，即使误启动也不会直接暴露到公网。
- SQLite 单连接配合 WAL，先保证写入行为明确；并发策略在压测后调整。
- 前端暂不引入完整 UI 组件库，先验证契约和核心工作流。

### 下一阶段

1. 引入正式 migration 文件、sqlc 和数据库错误映射。
2. 实现初始化管理员、Argon2id、Session、CSRF 和登录限速。
3. 实现配置 revision、原子文件发布、Xray supervisor 与自动回滚。
4. 完善 VLESS TCP/WS/TLS/REALITY schema 和兼容矩阵。
5. 将前端生产资源嵌入 Go 二进制。
6. 建立 GitHub Actions 与 Linux VM 安装测试。

### 本次验证结果

- Go 版本：1.26.4 windows/amd64（官方 SHA-256 校验通过）。
- `go test ./... -count=1`：PASS，5 个含测试 package 全部通过。
- `go vet ./...`：PASS。
- `npm run test`：PASS，1 个测试文件、2 个测试用例。
- `npm run build`：PASS，Vite 生产构建成功。
- `npm audit --audit-level=low`：PASS，0 vulnerabilities。
- API 冒烟测试：PASS；health=`ok`，种子 inbound=1，生成 Xray inbound=2。
- Windows 构建：PASS，`dist/xpanel.exe`。
- Linux amd64 交叉构建：PASS，`dist/xpanel-linux-amd64`，CGO disabled。
- 真实 Xray `run -test`：NOT RUN；本机未提供 Xray 二进制。
- Linux systemd VM：NOT RUN；当前执行环境为 Windows。
## 2026-06-22：入站节点导出链接

### 已完成

- 在入站列表每一行新增“导出”按钮。
- 支持生成 `vless://` 分享链接，包含 UUID、地址、端口、传输方式、TLS 状态、路径和备注。
- 支持生成 `vmess://` 分享链接，按 v2rayN 常见 JSON 结构 Base64 编码。
- 导出地址规则：如果监听地址是 `0.0.0.0`、`::` 或 `127.0.0.1`，使用当前浏览器访问面板的 hostname，例如 `192.168.191.128`。
- 新增导出弹窗，展示完整链接，并提供复制按钮；普通 HTTP 私有 IP 场景下会使用 textarea 兼容复制。

### 验证结果

- `npm.cmd run test`：PASS。
- `npm.cmd run build`：PASS，生成 `index-BHTwhYJM.js`。
- `go test ./... -count=1`：PASS。
- Linux amd64 二进制构建：PASS，SHA-256 `9b6d946c00760b431a01c6b2ef56077b41445f947a28e54425e6965242804541`。
- VM 部署：PASS，`xpanel` systemd 服务状态 `active`。
- Windows 主机访问 `http://192.168.191.128:8080/`：HTTP 200，页面资源为 `index-BHTwhYJM.js`。

## 2026-06-22：应用配置并托管 Xray 服务端

### 已完成

- 新增 `POST /api/v1/config/apply`。
- 后端应用流程：编译数据库入站配置 → 使用 Xray `run -test` 校验 → 原子写入 `/var/lib/xpanel/xray/config.json` → 重启 `xpanel-xray.service`。
- 新增 `internal/runtime/FileApplier`，避免通过 shell 拼接命令执行 reload。
- 前端新增“应用配置”按钮。
- 新增 `xpanel-xray.service`，由 systemd 独立托管服务端 Xray。
- 新增最小 sudoers 授权：`xpanel` 用户仅允许免密 restart/start `xpanel-xray.service`。
- 修正 `xpanel.service`：关闭 `NoNewPrivileges`，否则 sudoers 无法生效。

### VM 验证结果

- `npm.cmd run test`：PASS。
- `npm.cmd run build`：PASS，生成 `index-DfFlzUog.js`。
- `go test ./... -count=1`：PASS。
- Linux amd64 二进制构建：PASS，SHA-256 `521b5bf4dfc8bf15957c03729f9a2d31f1ae635416330e242f7f196379f46705`。
- `POST /api/v1/config/apply`：PASS，生成配置 SHA-256 `fa2ee3b75b0974582a45a4a839aa33deaf349c4acf80a617fe8abfb30bcad280`。
- `xpanel-xray.service`：active。
- VM 监听端口：`10443`、`20999`、`21828`、`20926`、`23525` 均由 `/opt/xpanel/bin/xray` 监听。
- Windows 主机 TCP 连通性：上述业务端口均 reachable。
- Windows 主机访问 `http://192.168.191.128:8080/`：HTTP 200，页面资源为 `index-DfFlzUog.js`。
