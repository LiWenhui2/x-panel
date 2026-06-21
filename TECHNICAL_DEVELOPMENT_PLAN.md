# XPanel 技术开发与一键部署方案

> 文档状态：Draft v1.0  
> 目标平台：Linux 单机服务器（首期支持 Debian 12/13、Ubuntu 24.04/26.04）  
> 产品定位：独立实现的 Xray Web 管理面板，不复用 x-ui 源代码  
> 临时代号：`xpanel`（开发时可替换为正式产品名）

## 1. 文档目的

本文定义 XPanel 的产品边界、总体架构、技术栈、核心数据模型、安全要求、开发阶段、测试策略、发布流程和 Linux 一键部署方案。它既是开发蓝图，也是架构评审、任务拆分和验收的基准。

XPanel 不实现代理协议和数据转发。VLESS、VMess、Trojan、Shadowsocks、SOCKS、HTTP 等协议由独立的 Xray-core 进程处理；XPanel 负责管理用户、监听入口、配置、运行状态、流量、证书、备份和升级。

## 2. 建设目标与边界

### 2.1 核心目标

1. 提供直观、安全的 Web 管理界面。
2. 支持多 inbound、多客户端、多协议和多种传输方式。
3. 将数据库中的领域模型稳定编译为 Xray JSON 配置。
4. 在配置发布前进行语义校验和 Xray 原生校验。
5. 支持 Xray 启动、停止、重启、健康检查和失败回滚。
6. 采集系统状态、实时流量、累计流量和客户端配额。
7. 支持到期时间、流量上限、启停状态和定时禁用。
8. 支持 HTTPS、登录保护、二步验证、审计日志和备份恢复。
9. 提供可重复、可校验、可升级、可卸载的一键 Linux 部署体验。
10. 使用单一代码库完成首期交付，并为未来多节点控制预留边界。

### 2.2 首期非目标

- 不修改或重新实现 Xray-core。
- 不在首期建设 Kubernetes、微服务或服务网格。
- 不把面板设计为多租户商业计费系统。
- 不在首期支持 Windows/macOS 服务端部署。
- 不在首期实现跨地域多节点一致性。
- 不允许在管理页面执行任意 Shell 命令。

## 3. 技术栈决策

### 3.1 推荐技术栈

| 层次 | 选择 | 说明 |
|---|---|---|
| 后端语言 | Go 当前稳定版，`go.mod` 固定最低版本 | 单二进制、低资源占用、并发与进程管理成熟、适合 Linux 多架构发行 |
| HTTP 路由 | `go-chi/chi` | 轻量、稳定、贴近标准库，便于保持清晰边界 |
| API 契约 | OpenAPI 3.1 + `oapi-codegen` | 契约先行，生成 Go 类型和 TypeScript 客户端，降低前后端漂移 |
| 数据访问 | `sqlc` + SQL migration | SQL 显式可审计，避免复杂 ORM 隐式行为 |
| 默认数据库 | SQLite，WAL 模式 | 单机面板部署简单、备份容易、运维成本低 |
| 可选数据库 | PostgreSQL | 未来多节点、较高写入量或外置数据库场景使用 |
| SQLite 驱动 | 纯 Go SQLite 驱动 | 降低 CGO 和多架构交叉编译复杂度；选型前做并发与备份压测 |
| 配置迁移 | `golang-migrate/migrate` 或内嵌顺序 migration | 每个发布包携带数据库升级脚本，升级前自动备份 |
| 日志 | 标准库 `log/slog`，JSON 输出 | journald 采集友好，可添加敏感字段脱敏 Handler |
| 定时任务 | 内部调度器 + 数据库租约 | 单机先使用 `robfig/cron/v3`，任务必须幂等 |
| 密码哈希 | Argon2id | 参数随服务器能力校准，并记录算法版本 |
| 前端 | Vue 3 + TypeScript + Vite | 生态成熟、表单和管理后台开发效率高 |
| UI 组件 | Naive UI 或 Ant Design Vue 4 | 二选一并固定，不同时引入两套组件库 |
| 前端状态 | Pinia + TanStack Query for Vue | Pinia 管本地状态，Query 管服务器缓存与请求状态 |
| 表单校验 | Zod | 与 TypeScript 模型配合，复杂协议表单可组合 |
| 图表 | Apache ECharts | 流量、资源使用率和趋势图 |
| 测试 | Go testing、Testcontainers、Vitest、Playwright | 单元、集成、端到端分层测试 |
| 反向代理 | Caddy（推荐）或 Nginx | 生产环境终止 TLS；面板自身仅监听本机端口 |
| 服务管理 | systemd | 自启动、资源限制、日志、重启策略与安全沙箱 |
| 发布 | GitHub Actions + GitHub Releases | 多架构构建、SBOM、校验和、签名和可重复发布 |

### 3.2 为什么选择模块化单体

首期所有管理能力运行在一个 Go 进程中，但按领域拆分 package。模块之间通过接口交互，禁止 controller 直接操作数据库、文件或 Xray 进程。

这种结构具备以下优势：

- 一个面板二进制即可部署和升级。
- SQLite 与本地 Xray 适合单机事务边界。
- 调试、备份和故障定位简单。
- 未来可把 `agent`、通知或统计模块拆出，而不提前承担分布式复杂度。

### 3.3 不推荐的选择

- 不建议以微服务作为起点。
- 不建议用 JWT 保存后台登录状态；同源后台使用服务端 Session 更易撤销。
- 不建议让前端直接编辑完整 Xray JSON 作为主要操作模式。
- 不建议让面板常驻 root 身份。
- 不建议运行时自动拉取“latest”版本；所有版本必须固定并验证。
- 不建议把业务数据仅存放在生成后的 `config.json` 中。

## 4. 总体架构

```text
Browser
  │ HTTPS
  ▼
Caddy / Nginx
  │ HTTP 127.0.0.1:2053
  ▼
XPanel Go Process
  ├─ REST API / Session / RBAC
  ├─ Inbound & Client Domain
  ├─ Config Compiler & Validator
  ├─ Xray Runtime Supervisor
  ├─ Traffic Collector
  ├─ Scheduler / Notification
  ├─ Audit / Backup / Update
  └─ SQLite
        │
        ├─ atomic config publish
        ▼
  Xray Child Process
        └─ local-only gRPC Stats API
```

### 4.1 运行时边界

- XPanel 与 Xray 使用不同二进制、不同职责和独立版本。
- XPanel 以专用系统用户 `xpanel` 运行。
- Xray 由 Runtime Supervisor 启动为子进程，但配置校验也通过 Xray 二进制完成。
- Xray Stats API 只监听 `127.0.0.1` 的随机或保留端口。
- 面板默认只监听 `127.0.0.1`，公网 HTTPS 交给 Caddy/Nginx。
- 如需监听 1024 以下端口，仅对 Xray 二进制赋予 `cap_net_bind_service`，不提升面板权限。

### 4.2 后端模块

| 模块 | 职责 |
|---|---|
| `auth` | 登录、登出、Session、密码、TOTP、恢复码、限速 |
| `rbac` | 管理员、角色、权限策略 |
| `inbound` | inbound 生命周期、端口冲突、启停与排序 |
| `client` | 客户端身份、UUID/密码、配额、到期与状态 |
| `protocol` | 协议、传输、安全参数的领域类型和校验规则 |
| `configcompiler` | 把领域对象编译为确定性的 Xray 配置 |
| `runtime` | Xray 校验、启动、停止、重启、健康检查、日志和回滚 |
| `traffic` | 查询 Stats API、累计流量、速率和聚合 |
| `systeminfo` | CPU、内存、磁盘、网络、运行时间和版本信息 |
| `certificate` | 证书元数据、到期检查；首期不自行实现 ACME |
| `notification` | Webhook、Telegram、告警策略和发送记录 |
| `scheduler` | 到期检查、流量采集、备份、告警和清理任务 |
| `audit` | 记录管理员、操作对象、来源 IP、结果和变更摘要 |
| `backup` | 一致性备份、恢复预检和恢复 |
| `update` | Release 清单、签名校验、升级前备份和版本回退 |

## 5. 代码仓库结构

```text
xpanel/
├─ cmd/
│  ├─ xpanel/                 # server 与管理 CLI
│  └─ configcheck/            # 可选的离线配置诊断工具
├─ internal/
│  ├─ app/                    # 依赖装配与生命周期
│  ├─ auth/
│  ├─ rbac/
│  ├─ inbound/
│  ├─ client/
│  ├─ protocol/
│  ├─ configcompiler/
│  ├─ runtime/
│  ├─ traffic/
│  ├─ systeminfo/
│  ├─ notification/
│  ├─ scheduler/
│  ├─ audit/
│  ├─ backup/
│  ├─ update/
│  ├─ api/                    # OpenAPI handlers 与中间件
│  └─ storage/                # sqlc、transaction、migration
├─ api/openapi.yaml
├─ migrations/sqlite/
├─ migrations/postgres/
├─ web/                       # Vue 工程
├─ packaging/
│  ├─ systemd/
│  ├─ install.sh
│  ├─ uninstall.sh
│  └─ caddy/
├─ scripts/
├─ tests/
│  ├─ integration/
│  └─ e2e/
├─ Dockerfile
├─ Makefile
└─ go.mod
```

依赖方向必须是：`api -> application/domain -> repository interface`。基础设施实现依赖领域接口，领域层不能引用 HTTP、SQLite 或 `os/exec`。

## 6. 数据模型

### 6.1 主要表

| 表 | 关键字段 |
|---|---|
| `admins` | id、username、password_hash、status、last_login_at、created_at |
| `roles` / `admin_roles` | 角色与关联 |
| `sessions` | id_hash、admin_id、ip、user_agent、expires_at、revoked_at |
| `inbounds` | id、tag、remark、listen、port、protocol、enabled、revision |
| `inbound_clients` | id、inbound_id、email、credential、enabled、expiry_at、traffic_limit |
| `transport_settings` | inbound_id、network、schema_version、config_json |
| `security_settings` | inbound_id、mode、schema_version、config_encrypted |
| `traffic_counters` | subject_type、subject_id、uplink、downlink、updated_at |
| `traffic_samples` | subject_id、bucket_at、uplink_delta、downlink_delta |
| `config_revisions` | version、sha256、content、status、created_by、created_at |
| `runtime_events` | action、old_pid、new_pid、result、error、created_at |
| `settings` | namespace、key、value、secret、version |
| `audit_logs` | actor_id、action、resource、resource_id、ip、result、diff、created_at |
| `notification_rules` | event、channel、threshold、enabled |
| `notification_deliveries` | rule_id、event_id、status、attempts、last_error |

### 6.2 数据建模规则

- UUID、密码、私钥等凭据不得出现在普通日志和审计 diff 中。
- 可查询、可约束的核心字段必须结构化存储。
- 协议差异较大的部分允许 JSON 存储，但必须包含 `schema_version` 并经过后端类型校验。
- 使用数据库唯一索引保证端口、tag、管理员用户名等约束。
- 所有流量字段使用 64 位整数并明确单位为 byte。
- 时间统一保存 UTC，API 使用 RFC 3339，前端负责本地化展示。
- 每个可修改资源包含 revision，用于乐观锁，防止浏览器并发覆盖。

## 7. 协议与配置编译器

### 7.1 首期支持矩阵

建议按价值分批实现，而不是一次铺开所有组合：

1. 第一批：VLESS、VMess、Trojan；TCP、WebSocket、gRPC；TLS、REALITY。
2. 第二批：Shadowsocks、SOCKS、HTTP、Dokodemo-door。
3. 第三批：XHTTP、HTTPUpgrade、SplitHTTP 等随 Xray 稳定演进的传输方式。

每个组合建立明确的兼容矩阵。UI 只能选择后端声明为兼容的组合；后端永远进行最终校验。

### 7.2 ConfigCompiler 设计

编译过程必须是纯函数风格：相同输入得到字节级稳定的输出。建议流程：

1. 从数据库读取一个一致性快照。
2. 校验端口、tag、客户端身份和协议字段。
3. 组装 inbound、routing、policy、stats、API 和日志配置。
4. 自动注入仅本机可访问的 Stats API inbound。
5. 规范化字段顺序与默认值。
6. 生成格式化 JSON。
7. 计算 SHA-256，并与当前生效版本比较。
8. 保存候选 revision。

禁止把不受约束的前端 JSON 原样拼入最终配置。高级 JSON 编辑器只能作为明确标注的专家功能，并经过 JSON Schema、字段白名单和 Xray 原生校验。

## 8. 配置发布事务与 Xray 生命周期

### 8.1 发布状态机

```text
DRAFT -> COMPILED -> VALIDATED -> ACTIVATING -> ACTIVE
                     │               │
                     └-> REJECTED    └-> ROLLED_BACK / FAILED
```

### 8.2 安全发布步骤

1. 获取全局发布互斥锁，避免并发重启。
2. 在数据库事务中生成候选配置记录。
3. 写入数据目录中的临时文件，并执行 `fsync`。
4. 执行 Xray 原生配置测试命令，限制执行时间和输出大小。
5. 测试失败：标记候选版本 `REJECTED`，不影响运行中实例。
6. 测试成功：保存当前版本备份，以原子 rename 替换配置文件。
7. 启动新 Xray 进程，等待端口、进程和 Stats API 健康检查通过。
8. 健康检查成功后停止旧进程，标记新版本 `ACTIVE`。
9. 若无法并行启动，则执行短暂停机重启；失败时立即恢复旧文件并启动旧版本。
10. 写入 runtime event 和脱敏后的审计日志。

### 8.3 Runtime Supervisor 要求

- 使用固定的 Xray 绝对路径，不允许 UI 输入可执行文件路径。
- 参数由代码生成，不执行 Shell，不使用 `sh -c`。
- 限制 stdout/stderr 缓冲区大小，避免内存耗尽。
- 记录 PID、启动时间、退出码和最近错误。
- 对崩溃使用指数退避，避免高频重启循环。
- 连续失败达到阈值后进入 degraded 状态并告警。
- 面板退出时向子进程发送优雅终止信号，超时后再强制结束。

## 9. 流量统计、配额和到期控制

### 9.1 采集策略

- 每 10 秒查询一次本地 Xray Stats API。
- 内存中计算瞬时速率，数据库保存累计值。
- 每分钟写入聚合样本；历史明细按保留策略清理。
- 每次采集记录 Xray 实例标识，防止重启后计数器归零导致负增量。
- 数据库写入失败时保留有限内存队列，恢复后补写。
- 面板重启后以持久累计值为准，不依赖 Xray 单次进程累计量。

### 9.2 配额执行

当客户端达到到期时间或流量上限：

1. 使用事务把客户端状态更新为 disabled，并记录原因。
2. 合并同一个调度周期内的变更，仅发布一次配置。
3. 发布成功后发送通知。
4. 发布失败则保持错误状态并告警，不能假装已经禁用。

应设计明确的重置周期：不重置、每日、每周、每月或管理员手动重置。重置必须写审计日志。

## 10. API 与前端设计

### 10.1 API 风格

- 基础路径：`/api/v1`。
- JSON 字段使用 camelCase。
- 错误结构统一为 `code`、`message`、`details`、`requestId`。
- 修改请求支持 revision/ETag 乐观锁。
- 列表接口统一分页、排序和过滤规则。
- 导入、恢复、升级等危险操作使用二次确认 token。
- OpenAPI 是唯一契约来源，CI 检查生成代码是否最新。

建议资源：

```text
POST   /auth/login
POST   /auth/logout
GET    /me
GET    /system/status
GET    /inbounds
POST   /inbounds
PUT    /inbounds/{id}
DELETE /inbounds/{id}
POST   /inbounds/{id}/clients
POST   /config/validate
POST   /config/publish
GET    /config/revisions
POST   /config/revisions/{id}/rollback
GET    /runtime/status
POST   /runtime/restart
GET    /traffic/summary
GET    /audit-logs
POST   /backups
POST   /restore/preflight
```

### 10.2 管理页面

1. 首次初始化向导。
2. 登录、TOTP 设置与恢复码。
3. 仪表盘：系统、Xray、流量、告警。
4. Inbound 列表与协议表单。
5. 客户端、配额、到期和连接信息。
6. 配置预览、校验错误与版本回滚。
7. Xray 日志和运行状态。
8. 证书、通知、备份和系统设置。
9. 管理员与审计日志。

协议表单采用“基础设置 + 高级设置”的渐进结构。危险字段必须说明影响，密钥默认遮挡且不通过普通详情接口返回。

## 11. 身份认证与安全基线

### 11.1 登录与 Session

- 密码使用 Argon2id；保存参数、salt 和 hash。
- 首次启动生成一次性初始化 token，不设置通用默认密码。
- 使用 256 bit 随机 Session ID，数据库仅保存其 hash。
- Cookie 设置 `HttpOnly`、`Secure`、`SameSite=Strict` 和限定 Path。
- 登录、TOTP、恢复码和重置密码接口分别限速。
- 修改密码、关闭 2FA、恢复和升级要求重新验证密码。
- 支持撤销当前 Session 和全部 Session。

### 11.2 Web 安全

- 所有状态变更请求启用 CSRF 防护。
- 配置 CSP、HSTS、X-Content-Type-Options 和 frame 限制。
- 不使用内联脚本；前端依赖打入构建产物，不依赖公共 CDN。
- 严格校验 Host、Origin、代理头和可信反向代理地址。
- API 错误不泄露数据库路径、命令行、密钥或堆栈。
- 上传文件限制大小、格式和解压后总量。

### 11.3 操作系统安全

- 创建无登录 Shell 的 `xpanel` 系统用户。
- 配置目录 `/etc/xpanel` 权限 `0750`；敏感文件 `0640` 或更严格。
- 数据目录 `/var/lib/xpanel` 仅服务用户可写。
- systemd 启用 `NoNewPrivileges`、`PrivateTmp`、`ProtectHome`、`ProtectSystem=strict` 等沙箱项。
- 只为明确需要的路径配置 `ReadWritePaths`。
- Xray 下载包必须验证 SHA-256；发布清单应使用 Sigstore/Cosign 或 minisign 签名。
- 生成 SBOM，并在 CI 中运行依赖与容器漏洞扫描。

## 12. Linux 文件布局

```text
/usr/local/bin/xpanel              # 管理 CLI/服务二进制
/opt/xpanel/bin/xray               # 固定版本 Xray 二进制
/opt/xpanel/releases/<version>/    # 可回退的 XPanel 发布版本
/etc/xpanel/config.yaml            # 非敏感服务配置
/etc/xpanel/env                    # 可选敏感环境变量，0600
/var/lib/xpanel/xpanel.db          # SQLite 数据库
/var/lib/xpanel/runtime/config.json
/var/lib/xpanel/backups/
/var/cache/xpanel/downloads/
/etc/systemd/system/xpanel.service
/usr/local/lib/xpanel/xpanel-cli.sh # 可选的 Shell 包装器
```

业务日志默认写 stdout/stderr，由 journald 管理；不要同时维护一个无限增长的日志文件。

## 13. 一键安装与管理命令

### 13.1 用户体验

正式发布后可提供：

```bash
curl -fsSL https://example.com/install.sh | sudo bash
```

对安全要求更高的用户提供两步模式：

```bash
curl -fLO https://example.com/install.sh
less install.sh
sudo bash install.sh --version 1.0.0
```

安装完成后统一使用：

```bash
xpanel status
xpanel start
xpanel stop
xpanel restart
xpanel logs
xpanel update
xpanel update --version 1.2.0
xpanel backup
xpanel restore /path/to/backup.tar.zst
xpanel reset-password
xpanel doctor
xpanel uninstall
```

这里的 `xpanel` 是同一个 Go 二进制的 CLI 子命令。服务管理子命令可调用固定参数的 `systemctl`，不得接受任意服务名或任意 Shell 片段。

### 13.2 安装脚本步骤

`install.sh` 必须启用严格模式，并按以下顺序执行：

1. 检查 root 权限、systemd、磁盘空间和受支持发行版。
2. 检测 `amd64`、`arm64` 等架构并映射到发布包名称。
3. 从固定 HTTPS 地址获取版本化 `manifest.json`。
4. 下载指定版本包、SHA-256 文件和签名。
5. 先验签，再校验摘要；任一步失败立即退出。
6. 创建系统用户和目录，设置 owner、mode。
7. 解压到临时目录，防止路径穿越；完成后原子移动到 release 目录。
8. 安装或切换 `/usr/local/bin/xpanel` 软链接。
9. 安装固定版本 Xray，验证其版本和摘要。
10. 写入 systemd unit；已存在配置和数据库绝不覆盖。
11. 执行数据库 migration preflight。
12. `systemctl daemon-reload`，enable 并启动服务。
13. 运行 `xpanel doctor --post-install`。
14. 输出本机访问地址、初始化 token 的获取方式和后续 TLS 指南。

禁止使用未经校验的 `latest.tar.gz`，禁止安装脚本静默修改防火墙、SSH 配置或 SELinux 策略。

### 13.3 systemd 单元建议

```ini
[Unit]
Description=XPanel Xray Management Panel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=xpanel
Group=xpanel
ExecStart=/usr/local/bin/xpanel serve --config /etc/xpanel/config.yaml
WorkingDirectory=/var/lib/xpanel
Restart=on-failure
RestartSec=5s
TimeoutStopSec=30s
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
ReadWritePaths=/var/lib/xpanel /var/cache/xpanel
UMask=0027
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
```

最终 unit 需在真实发行版上测试。部分 Xray 能力可能需要根据功能矩阵增加最小化的 capability 或 systemd 权限，不能为省事关闭全部沙箱。

## 14. 升级、回滚、备份与卸载

### 14.1 应用升级

1. 查询签名后的版本清单，不默认自动安装。
2. 检查目标版本、数据库 migration 和 Xray 兼容范围。
3. 创建数据库、配置和当前二进制的一致性备份。
4. 下载并校验新包。
5. 执行 migration dry-run/preflight。
6. 停止服务，切换 release 软链接并运行 migration。
7. 启动服务并执行健康检查。
8. 失败时切回旧 release；若 migration 不可逆，则恢复升级前数据库。

数据库 migration 原则上保持向前兼容至少一个版本，使二进制回滚成为可能。

### 14.2 Xray 升级

- Xray 与面板分别版本化。
- 维护支持矩阵，例如面板版本声明最低与已测试的 Xray 范围。
- 下载、摘要验证、配置测试和实际启动检查全部通过后才切换。
- 保留最近两个 Xray 二进制，以便一键回滚。
- 不从管理页面接受任意下载 URL。

### 14.3 备份格式

备份包包含：

- SQLite 在线一致性备份，而不是简单复制正在写入的数据库文件。
- 面板配置与当前生效的 Xray 配置 revision。
- 证书可选择包含；包含时备份必须加密。
- `metadata.json`：产品版本、schema 版本、Xray 版本、摘要和创建时间。

恢复前必须验证包签名/摘要、schema 兼容性、可用磁盘和路径安全。恢复操作要求二次认证并自动保存恢复前备份。

### 14.4 卸载

默认卸载只移除服务和程序，保留 `/etc/xpanel`、`/var/lib/xpanel`。只有显式执行 `xpanel uninstall --purge-data` 并再次确认时才删除数据。

## 15. CI/CD 与发布物

### 15.1 CI 阶段

1. Go 格式化、静态检查、`go vet`、race test。
2. 前端 lint、类型检查、单元测试和生产构建。
3. OpenAPI lint 与生成代码一致性检查。
4. SQLite/PostgreSQL migration 往返与升级测试。
5. ConfigCompiler golden tests。
6. 使用真实 Xray 二进制执行配置兼容测试。
7. API 集成测试和 Playwright E2E。
8. `govulncheck`、依赖审计、secret scan。
9. 构建多架构发布包、生成 SBOM、校验和并签名。

### 15.2 Release 内容

```text
xpanel_1.0.0_linux_amd64.tar.zst
xpanel_1.0.0_linux_arm64.tar.zst
checksums.txt
checksums.txt.sig
manifest.json
manifest.json.sig
sbom.spdx.json
install.sh
```

安装脚本自身也必须版本化。主域名上的安装入口只负责选择稳定脚本版本，实际下载地址不可被用户输入覆盖，除非使用明确的开发模式参数。

## 16. 测试策略

### 16.1 单元测试

- 每一种协议和传输组合的字段校验。
- ConfigCompiler 输出 golden files。
- 流量计数器重启、归零、溢出和补写。
- 到期、时区、月末与重置周期。
- 密码、Session、TOTP、CSRF 和权限策略。
- 路径、压缩包和下载清单安全校验。

### 16.2 集成测试

- 使用临时 SQLite/PostgreSQL 实例运行真实 migration。
- 启动真实 Xray 子进程，验证配置、Stats API 和异常退出。
- 模拟端口占用、配置错误、磁盘写满、数据库锁和进程崩溃。
- 验证配置发布失败后的文件、数据库和进程状态一致。
- 在 Debian/Ubuntu VM 中测试安装、升级、回滚、备份和卸载。

### 16.3 E2E 测试

- 初始化、登录、2FA、退出。
- 创建 inbound/client、发布配置并观察运行状态。
- 修改为非法配置并确认线上实例不受影响。
- 达到流量/到期限制后的自动禁用。
- 备份、修改数据、恢复和一致性检查。

### 16.4 发布验收门槛

- 所有支持组合均通过当前目标 Xray 版本的原生配置测试。
- 安装脚本在全新 Debian/Ubuntu VM 幂等执行两次均成功。
- 从上一稳定版本升级和回滚成功。
- 配置发布故障注入不会丢失最后可用配置。
- 高危安全扫描无未处置问题。
- 备份可在另一台同架构或不同架构服务器恢复。

## 17. 分阶段实施计划

### 阶段 0：需求与技术验证（1–2 周）

- 确认协议/传输支持矩阵。
- 验证目标 Xray 版本的配置测试命令和 Stats API。
- 完成 OpenAPI、数据库和 ConfigCompiler 原型。
- 验证 Go 多架构构建、SQLite 驱动和 systemd 子进程行为。
- 输出 ADR：数据库、UI 库、SQLite 驱动、签名方案。

完成标准：能够从内存模型生成一个有效 VLESS 配置、启动 Xray 并读取流量。

### 阶段 1：可运行 MVP（3–5 周）

- 项目骨架、migration、日志和配置管理。
- 首次初始化、登录、Session 和基础安全头。
- Inbound/client CRUD。
- VLESS、VMess、Trojan 与第一批传输组合。
- ConfigCompiler、原生校验、发布和回滚。
- Xray 状态、重启和有限日志查看。
- Vue 管理页面和基础仪表盘。

完成标准：单台 Linux 服务器可稳定创建配置并运行，不因错误编辑破坏最后可用实例。

### 阶段 2：完整管理能力（3–5 周）

- 流量采集、趋势、配额、到期和重置周期。
- Shadowsocks、SOCKS、HTTP、Dokodemo-door。
- 配置历史、差异预览和手动回滚。
- TOTP、恢复码、管理员和审计日志。
- 通知渠道、系统监控和告警。
- 备份与恢复。

完成标准：覆盖传统 x-ui 的主要日常管理功能，并具备安全审计能力。

### 阶段 3：生产部署与供应链（2–4 周）

- systemd hardening、目录权限和非 root 运行。
- 一键安装、doctor、升级、回滚和卸载 CLI。
- 多架构 Release、签名、SBOM 和安全扫描。
- Debian/Ubuntu 安装矩阵与故障注入测试。
- 运维手册、灾难恢复手册和版本兼容策略。

完成标准：全新服务器一条命令安装，升级失败自动恢复，备份可跨服务器验证恢复。

### 阶段 4：后续演进

- 多节点 Agent/Controller 架构。
- PostgreSQL 正式支持。
- WebAuthn、OIDC 和更细粒度 RBAC。
- 更多 Xray 新协议/传输能力。
- HA 控制面与集中监控。

## 18. 关键风险与应对

| 风险 | 应对措施 |
|---|---|
| Xray 配置随版本变化 | 建立版本支持矩阵、fixture 和真实二进制兼容测试 |
| 错误配置导致节点中断 | 候选配置校验、原子发布、健康检查、自动回滚 |
| 流量计数在重启后不一致 | 实例 epoch、持久累计、归零检测和幂等增量 |
| 面板权限过大 | 非 root 运行、固定命令参数、systemd 沙箱、最小 capability |
| 安装脚本供应链攻击 | HTTPS、固定版本、摘要、签名、SBOM、禁止任意源 |
| SQLite 并发或损坏 | WAL、短事务、busy timeout、在线备份和定期完整性检查 |
| 升级破坏数据 | 升级前备份、兼容 migration、release 软链接和恢复演练 |
| 协议表单复杂失控 | 支持矩阵、schema 驱动、渐进式 UI 和领域校验 |

## 19. 首批工程任务清单

按以下顺序启动工程最稳妥：

1. 建立 ADR 和协议支持矩阵。
2. 创建 Go/Vue monorepo、Makefile 和 CI 基线。
3. 定义 OpenAPI 错误模型、认证接口和 inbound 资源。
4. 定义数据库 schema 与 migration 流程。
5. 实现 VLESS 领域模型和纯函数 ConfigCompiler。
6. 使用真实 Xray 完成 validate/start/stop/health POC。
7. 实现 config revision 和发布状态机。
8. 实现初始化、Argon2id 登录、Session、CSRF 和限速。
9. 实现 inbound/client API 与 Vue 页面。
10. 实现 Stats API 流量采集和持久累计。
11. 添加审计、备份、doctor 和故障注入测试。
12. 最后实现安装、升级和卸载脚本；脚本只编排已经稳定的 CLI 能力。

## 20. 决策摘要

推荐最终方案是：

- Go 模块化单体负责控制面。
- Vue 3 + TypeScript 构建管理后台，并嵌入 Go 发布物。
- SQLite 作为单机默认存储，保留 PostgreSQL repository 接口。
- Xray-core 保持独立二进制和进程边界。
- 所有配置通过“编译、校验、快照、发布、健康检查、回滚”事务流生效。
- 面板非 root 运行，TLS 默认交给 Caddy/Nginx。
- 一个版本化、验签的一键安装器配合 systemd 完成 Linux 部署。
- 先完成稳定的单机产品，再考虑多节点和微服务。

这套架构能够覆盖 x-ui 类型产品的完整功能，同时避免继承旧项目的技术债，并把长期最容易出事故的配置发布、权限、升级和恢复能力放在第一等位置。
