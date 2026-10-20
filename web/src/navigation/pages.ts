export interface PageTab { id: string; label: string; keywords?: string }
export interface PrimaryPage { id: string; name: string; group: string; description: string; tabs: PageTab[] }
export const primaryPages: PrimaryPage[] = [
  { id: 'dashboard', name: '首页', group: '', description: '查看运行概况与服务状态。', tabs: [{ id: '', label: '首页' }] },
  { id: 'search', name: '搜索', group: '获取', description: '发现资源，或从搜索历史继续查看结果。', tabs: [{ id: 'query', label: '搜索', keywords: 'TMDB 站点 资源' }, { id: 'history', label: '搜索历史', keywords: '站点缓存 历史记录' }] },
  { id: 'subscriptions', name: '订阅', group: '获取', description: '管理订阅规则与自动跟进。', tabs: [{ id: '', label: '订阅' }] },
  { id: 'downloads', name: '下载', group: '获取', description: '跟进下载、做种、上传与手动入库。', tabs: [{ id: 'tasks', label: '下载任务', keywords: '下载器 qBittorrent 做种' }, {id:'uploads',label:'上传入库',keywords:'上传 核对 批次 115'}, {id:'manual',label:'手动添加',keywords:'种子 磁力 手动下载'}] },
  { id: 'library', name: '媒体库', group: '媒体', description: '浏览媒体内容与 Emby 服务。', tabs: [{ id: 'browse', label: '媒体浏览', keywords: 'Emby 海报 电影 剧集' }] },
  { id: 'organize', name: '入库整理', group: '媒体', description: '先核对识别，再扫描、确认、整理和恢复。', tabs: [{id:'candidates',label:'待确认',keywords:'候选 识别 核对'},{id:'scans',label:'扫描',keywords:'目录 定时扫描 覆盖'},{id:'assets',label:'已入库',keywords:'资产 STRM 身份 季集'},{id:'history',label:'整理历史',keywords:'批次 对账 重命名 记录'},{id:'recovery',label:'恢复与归档',keywords:'输出恢复 归档 保留期 清理'}] },
  { id: 'files', name: '文件', group: '媒体', description: '管理文件、固定播放入口与 STRM 修复。', tabs: [{ id: 'browser', label: '文件浏览', keywords: '本地 云盘 目录 重命名 移动' }, { id: 'strm', label: 'STRM 目录', keywords: '固定链接 输出' }, { id: 'tools', label: 'STRM 工具', keywords: '批量修复 302 诊断' }] },
  { id: 'playback', name: '播放服务', group: '媒体', description: '配置播放服务、媒体用户和播放器准入。', tabs: [{ id: 'settings', label: '服务设置', keywords: '自研媒体库 播放设置' }, { id: 'users', label: '媒体用户', keywords: '账号 权限 登录' }, { id: 'players', label: '播放器准入', keywords: 'UA User-Agent 白名单' },{id:'sessions',label:'在线会话',keywords:'正在播放 设备'},{id:'processing',label:'预处理',keywords:'媒体探测 单集截图'}] },
  { id: 'connections', name: '连接', group: '系统', description: '检查依赖状态，管理云盘与通知渠道。', tabs: [{ id: 'overview', label: '总览', keywords: '服务连接 就绪 readiness' }, { id: 'storage', label: '115/CD2', keywords: '云盘连接 115 授权 CloudDrive2 配额' }, { id: 'emby', label: 'Emby', keywords: '播放 网关 诊断 API Key' }, { id: 'notifications', label: '通知渠道', keywords: '通知中心 Telegram Bark 投递' }, { id: 'aux', label: 'TMDB/代理', keywords: 'TMDB 元数据 网络代理' }] },
  { id: 'sites', name: '站点', group: '系统', description: '维护 PT 账号与选用的站点模板。', tabs: [{ id: 'accounts', label: '站点账号', keywords: 'PT HDSKY Cookie 分享率' }, { id: 'templates', label: '站点模板', keywords: '规则 内置 加载' }] },
  { id: 'plugins', name: '插件', group: '系统', description: '管理内置插件、实例配置与授权。', tabs: [{ id: '', label: '插件' }] },
  { id: 'ops', name: '运维', group: '系统', description: '查看后台任务、备份、日志和系统数据。', tabs: [{ id: 'jobs', label: '任务队列', keywords: 'Worker Cron 重试' }, { id: 'maintenance', label: '备份与告警', keywords: '维护 恢复 告警 趋势' }, { id: 'logs', label: '日志', keywords: '程序日志 debug info warn error' }, { id: 'data', label: '数据浏览', keywords: '数据库 PostgreSQL Redis 只读' }, { id: 'cache', label: '缓存', keywords: 'TMDB 缓存 TTL 清理' }] },
  { id: 'settings', name: '设置', group: '系统', description: '管理系统配置、目录分类和外部访问。', tabs: [{ id: 'general', label: '系统配置', keywords: '' }, { id: 'classification', label: '目录分类', keywords: '动漫 国家 题材 分类规则' }, { id: 'api', label: '外部 API', keywords: '开放接口 凭据 权限' }] },
]
export const settingsSections = [
  { id: 'access', label: '访问与域名', keywords: '管理入口 公开地址 端口' },
  { id: 'library', label: '命名与刮削', keywords: '命名模板 电影 剧集' },
  { id: 'automation', label: '搜索与订阅', keywords: 'RSS Cron 调度 搜索缓存' },
  { id: 'account', label: '管理员账号', keywords: '密码 安全' },
  { id: 'mobile', label: '移动端应用', keywords: 'PWA 安装' },
]
export const pageById = (id: string) => primaryPages.find(page => page.id === id)!
