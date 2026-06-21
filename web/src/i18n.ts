export type Language = 'en' | 'zh'

export const messages = {
  en: {
    firstRun: 'FIRST RUN SETUP', signInEyebrow: 'SECURE SIGN IN', createAdmin: 'Create administrator', welcome: 'Welcome back',
    setupCopy: 'Initialize your local XPanel administrator before managing nodes.', signInCopy: 'Sign in to manage Xray inbound nodes and service configuration.',
    username: 'Username', password: 'Password', createAccount: 'Create account', signIn: 'Sign in', inbounds: 'Inbounds', settings: 'Settings', signOut: 'Sign out',
    signedInAs: 'Signed in as {name}', nodeConsole: 'Node Console', panelOnline: 'Panel online', totalInbounds: 'Total inbounds', configuredListeners: 'Configured listeners',
    enabled: 'Enabled', disabled: 'Disabled', activeRecords: 'Active records', trafficQuota: 'Traffic quota', zeroUnlimited: '0 means unlimited', inboundNodes: 'Inbound nodes',
    inboundHelp: 'Create, apply and export client import links.', refresh: 'Refresh', preview: 'Preview', applyConfig: 'Apply Config', addInbound: 'Add Inbound',
    status: 'Status', remark: 'Remark', protocol: 'Protocol', listen: 'Listen', port: 'Port', transport: 'Transport', quota: 'Quota', expires: 'Expires', export: 'Export', config: 'Config',
    noNodes: 'No inbound nodes yet', noNodesHelp: 'Create the first inbound and apply configuration.', newInbound: 'NEW INBOUND', listenIp: 'Listen IP', random: 'Random',
    totalTraffic: 'Total traffic (GB)', expiry: 'Expiry', clientUuid: 'Client UUID', generate: 'Generate', clientEmail: 'Client email', alterId: 'Alter ID',
    wsPath: 'WebSocket path', tls: 'TLS', sniffing: 'Sniffing', certificateFile: 'Certificate file', keyFile: 'Key file', cancel: 'Cancel', saving: 'Saving…', createInbound: 'Create inbound',
    xrayConfig: 'XRAY CONFIGURATION', generatedConfig: 'Generated config', clientImportLink: 'CLIENT IMPORT LINK', exportName: 'Export {name}', pasteLink: 'Paste this link into a compatible client.',
    copyLink: 'Copy link', done: 'Done', unlimited: 'Unlimited', never: 'Never', adminCreated: 'Administrator account created.', signedIn: 'Signed in successfully.',
    inboundCreated: 'Inbound {name} created. Click Apply Config to restart Xray.', configApplied: 'Configuration applied: {path}', linkCopied: 'Import link copied.',
    exportTitle: 'Export import link', previewTitle: 'Preview generated config', english: 'English', chinese: '中文',
  },
  zh: {
    firstRun: '首次运行设置', signInEyebrow: '安全登录', createAdmin: '创建管理员', welcome: '欢迎回来',
    setupCopy: '请先创建 XPanel 管理员账户，然后再管理节点。', signInCopy: '登录后管理 Xray 入站节点和服务配置。',
    username: '用户名', password: '密码', createAccount: '创建账户', signIn: '登录', inbounds: '入站节点', settings: '设置', signOut: '退出登录',
    signedInAs: '当前用户：{name}', nodeConsole: '节点控制台', panelOnline: '面板在线', totalInbounds: '入站总数', configuredListeners: '已配置监听器',
    enabled: '已启用', disabled: '已禁用', activeRecords: '有效记录', trafficQuota: '流量配额', zeroUnlimited: '0 表示不限流量', inboundNodes: '入站节点',
    inboundHelp: '创建、应用并导出客户端导入链接。', refresh: '刷新', preview: '预览', applyConfig: '应用配置', addInbound: '添加入站',
    status: '状态', remark: '备注', protocol: '协议', listen: '监听地址', port: '端口', transport: '传输', quota: '配额', expires: '到期时间', export: '导出', config: '配置',
    noNodes: '暂无入站节点', noNodesHelp: '请创建第一个入站节点并应用配置。', newInbound: '新建入站', listenIp: '监听 IP', random: '随机',
    totalTraffic: '总流量（GB）', expiry: '到期时间', clientUuid: '客户端 UUID', generate: '生成', clientEmail: '客户端邮箱', alterId: '额外 ID',
    wsPath: 'WebSocket 路径', tls: 'TLS', sniffing: '流量探测', certificateFile: '证书文件', keyFile: '密钥文件', cancel: '取消', saving: '保存中…', createInbound: '创建入站',
    xrayConfig: 'XRAY 配置', generatedConfig: '生成的配置', clientImportLink: '客户端导入链接', exportName: '导出 {name}', pasteLink: '将此链接粘贴到兼容的客户端中。',
    copyLink: '复制链接', done: '完成', unlimited: '不限流量', never: '永不过期', adminCreated: '管理员账户已创建。', signedIn: '登录成功。',
    inboundCreated: '入站节点 {name} 已创建，请点击“应用配置”重启 Xray。', configApplied: '配置已应用：{path}', linkCopied: '导入链接已复制。',
    exportTitle: '导出导入链接', previewTitle: '预览生成的配置', english: 'English', chinese: '中文',
  },
} as const

export type MessageKey = keyof typeof messages.en
