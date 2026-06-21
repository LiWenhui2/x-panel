# 添加入站节点功能测试

## 浏览器入口

开发 VM：`http://192.168.191.128:8080/`

该地址仅用于当前 VMware 私有网络测试。系统尚未实现登录认证，不应映射到公网。

## 已支持字段

- 协议：VLESS、VMess。
- 监听：IP、端口、启用状态。
- 客户端：UUID、email 标识、VMess Alter ID。
- 传输：TCP、WebSocket 与 WS path。
- 安全：None、TLS；TLS 模式要求 VM 中真实存在的证书与私钥绝对路径。
- 策略：sniffing、总流量限制、到期时间。

## 浏览器测试

1. 打开入站列表，确认已有 VLESS 与 VMess 两条测试数据。
2. 点击“添加入站”。
3. 输入不重复的端口、UUID 和客户端标识。
4. 选择协议和传输；WebSocket 模式填写以 `/` 开头的路径。
5. 点击“创建入站”，确认弹窗关闭且列表立即刷新。
6. 点击“配置预览”，确认出现 SHA-256 和完整 Xray JSON。
7. 使用重复端口或 UUID 再次创建，确认页面显示冲突错误。

## VM 验证命令

```bash
systemctl is-active xpanel
sudo systemctl show xpanel -p Environment --no-pager
curl http://127.0.0.1:8080/api/v1/health
curl http://127.0.0.1:8080/api/v1/inbounds | python3 -m json.tool
curl -X POST http://127.0.0.1:8080/api/v1/config/preview | python3 -m json.tool
```

`config/preview` 成功表示配置已经通过 VM 中 `/opt/xpanel/bin/xray run -test` 的真实校验。

## 本次部署结果

- Ubuntu：22.04.4 LTS x86_64。
- Xray：26.3.27。
- XPanel SHA-256：`4683b7c8ba75b5cce7d8098965ba378bf4a0292f86bc853d795c057db4b04ec8`。
- 服务监听：`0.0.0.0:8080`，仅用于 VMware 私有网。
- 主机访问：首页 HTTP 200。
- 数据库自动迁移：通过，原 VLESS 数据保留。
- VMess + WebSocket 入库：通过。
- 真实 Xray 校验：通过，生成 3 个 inbound（API、VLESS、VMess）。

