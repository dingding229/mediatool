package system

type Module struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Summary   string   `json:"summary"`
	Stage     string   `json:"stage"`
	Milestone string   `json:"milestone"`
	Features  []string `json:"features"`
}

func Modules() []Module {
	return []Module{
		{"sites", "PT 站点", "配置兼容与账号连接", "partial", "M2", []string{"配置快照与本地覆盖", "公共规则继承", "站点连接测试"}},
		{"search", "资源搜索", "多站聚合与资源筛选", "partial", "M2", []string{"并发搜索与站点限流", "资源去重", "手动下载"}},
		{"subscriptions", "资源订阅", "按电影与季集追踪资源", "partial", "M3", []string{"已实现：RSS/周期搜索与规则预演", "已实现：种子季集覆盖与去重", "待验证：真实站点和客户端"}},
		{"downloads", "下载与传输", "本地下载、做种，再上传网盘", "partial", "M3/M4", []string{"qBittorrent 接入", "做种文件保留", "上传完成对账"}},
		{"cloudStorage", "云盘索引", "通过 CloudDrive2 连接 115", "partial", "M0/M4", []string{"已实现：官方 gRPC 认证与文件读取", "已实现：当前直链与连接诊断", "已实现：上传、流式扫描、人工身份确认"}},
		{"strm", "STRM 管理", "固定链接，播放时解析临时地址", "partial", "M4", []string{"已实现：链接编码与范围校验", "已实现：实时 302 与受控代理", "已实现：下载/候选入库、NFO和本地字幕"}},
		{"emby", "Emby 网关", "保留原有认证与转码链路", "partial", "M5", []string{"已实现：静态流权限校验与 STRM 映射", "已实现：302/代理，保留转码路径", "待验证：真实客户端与完整兼容矩阵"}},
		{"notifications", "通知中心", "按业务事件投递通知", "partial", "M5", []string{"事务 Outbox 表结构", "Webhook 与 Telegram", "重试与投递去重"}},
		{"jobs", "任务队列", "PostgreSQL 持久化任务与租约", "foundation", "M1", []string{"SKIP LOCKED 领取", "心跳与过期恢复", "重试与租约令牌隔离"}},
	}
}
