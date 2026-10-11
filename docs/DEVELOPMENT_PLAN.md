# Media Tool 开发计划

- 文档日期：2026-10-08（Asia/Shanghai）
- 状态：需求已确认；已交付 M1 框架和 M0 协议/最小播放代码。M0 真实验收仍需本地连接配置与测试媒体，不表示真实业务闭环已经完成。
- 计划基线：Go + Vue 3 + PostgreSQL + Redis + CloudDrive2 + Emby + Docker。

## 1. 产品目标与范围

构建模块化媒体自动化管理程序：通过 PT 订阅、搜索、下载资源，在本地保留做种文件，经 CloudDrive2 上传/索引 115，生成链接型 STRM，以 Emby 为媒体库和播放器，通过符合条件的 HTTP 302 将播放请求导向当前有效的云端地址。

用户明确要求：

1. 后端 Go，PostgreSQL 主数据库，Redis 缓存。
2. 模块化，适配器可扩展。
3. PT 订阅/搜索/下载，站点配置兼容 pre-dessert-sites。
4. 115 统一经 CloudDrive2 接入。
5. STRM 使用固定 HTTP 链接，播放时获取临时地址。
6. 通知功能。
7. 主要模块为 PT、STRM、Emby 302、通知。
8. Docker 部署。
9. 前端 Vue。

首期不重复开发视频播放器，不直接维护 115 私有登录 API，不默认把私有 PT 种子交给云盘离线下载。

## 2. 总体技术方案

| 层级 | 选择 | 边界 |
| --- | --- | --- |
| 后端 | Go，标准库 HTTP / ReverseProxy | 模块化单体，不先拆微服务 |
| 前端 | Vue 3 + TypeScript + Vite + Pinia + Vue Router + Element Plus | 管理控制台，移动端基本可用 |
| 主库 | PostgreSQL + pgx + SQL 迁移 | 唯一业务事实与持久化任务 |
| 缓存 | Redis + go-redis | 可重建，不充当唯一任务存储 |
| 任务 | Go Worker + PostgreSQL 行锁/租约 | 至少一次处理，业务操作幂等 |
| 下载器 | 首期 qBittorrent | 后续 Transmission 等 |
| 存储 | CloudDrive2 gRPC 适配器 | 115 登录在 CloudDrive2 中完成 |
| 播放 | 固定 STRM 解析端点 + Emby 反代 | 302 优先，保留受控代理/转码 |
| 通知 | Outbox + 站内/Webhook/Telegram | 不阻断主业务 |
| 部署 | 多阶段 Docker 镜像 + Compose | HTTP、Worker、迁移独立角色 |

Go 模块和 npm 依赖固定版本，提交 lock 文件。Docker 基础镜像固定 manifest digest。升级前检查兼容性、漏洞与许可证。

## 3. 核心架构

```text
Vue 管理控制台
  └─ Go HTTP
       ├─ 管理 API / 登录 / 审计
       ├─ 任务 SSE（状态刷新提示）
       ├─ /api/v1/cloudStorage/strm302
       └─ Emby 网关

Go Worker
  ├─ 配置同步 / PT 搜索 / 订阅
  ├─ 下载状态与做种管理
  ├─ 上传 / 文件扫描 / 媒体识别
  ├─ STRM 生成 / Emby 刷新
  └─ Outbox / 通知投递

PostgreSQL：业务状态、任务、文件映射、Outbox
Redis：搜索、直链、热点映射、限流等可重建数据
外部：PT / qBittorrent / CloudDrive2 → 115 / Emby
```

HTTP 和 Worker 共用领域模块，隔离运行，避免扫描上传阻塞播放。后续可将高流量播放网关拆成独立进程；M1 不做动态插件加载。

## 4. 已确认的固定 STRM 方案

### 4.1 格式

```text
https://<部署域名>/api/v1/cloudStorage/strm302?apiKey=<播放专用Key>&pickCode=<可选真实PickCode>&path=<URL编码后的完整云盘路径>
```

路径示例（不是播放 URL）：

```text
/115/MS_Media/媒体/电影/动画电影/幽灵公主 (1997)/幽灵公主 (1997) - 1080p.mkv
```

不得复制用户曾提供的真实 API Key。域名从 APP_PUBLIC_URL 读取，查询参数统一编码。STRM 是单行 UTF-8 文本，不存临时 CDN URL、不存 CloudDrive2 管理令牌。

CloudDrive2 公开 GetDownloadUrlPath 请求以 path 查询，get_direct_url=true 请求可用直链；当前协议没有 pickCode 参数。首期按 CloudDrive2 路径解析，pickCode 只在有真实值时保存为兼容字段。不得把 CloudDrive2 文件 ID 当 PickCode。

### 4.2 流程

```text
Emby 读取 STRM
  → 固定解析入口
  → 校验播放专用 Key 与允许目录
  → 定位 CloudDrive2 文件
  → 查询短期缓存或申请当前直链
  → 判断有效期 / 请求头 / 客户端限制
  → 302 Location: 临时直链
  → 播放端访问 115/CDN
```

- 过期时重新解析同一固定地址，不因直链过期重建 STRM。
- `Cache-Control: no-store` 防止浏览器长期缓存重定向。
- 云端路径移动/重命名与直链过期是不同问题：路径型地址可能需重建；后续稳定 assetId 可保持路径变动下的地址。
- 多存储阶段增加 storageId/assetId，不在 M1 接口中默默接受未实现参数。
- HEAD/GET/Range、拖动、续播、长时间播放都要实测。

### 4.3 权限与边界

ADMIN_API_KEY 和 STRM_API_KEY 分离。STRM Key 目前是允许目录范围内的 bearer 能力；持有者能请求允许目录的文件，不代表 Emby 用户已经被授权。先保留内网边界，M5 增加用户权限联动、文件级短期授权及可撤销凭据。

302 无法替播放器设置下一跳任意请求头。CloudDrive2 直链若附带 User-Agent / additionalHeaders 或潜在 IP 条件，必须探测和建立客户端兼容策略；无法满足时不强制直连。网关在返回 302 后通常无法观测播放器访问 CDN 的错误，不能声称所有错误都可透明刷新重试。

Emby 可能由服务端读取 STRM 后再向客户端传输；单独建立 strm302 端点不保证绕过 Emby 的视频流量。必须在真实 Emby / 客户端中验证 PlaybackInfo、媒体源地址和播放路由，再实现网关联动，防止回退循环。

## 5. M0 技术验证（优先于大范围业务开发）

### CloudDrive2

- 固定官方 proto 版本并生成 Go 客户端。
- 认证、令牌更新、目录扫描与权限/能力检测。
- FindFileByPath / GetDownloadUrlPath 获取及字段语义。
- 实际 115 文件身份、PickCode 是否可获、重命名移动后的关联。
- 远程上传接口；不适合时再评估 WebDAV/挂载写入。
- 上传完成事件与云端可读校验，不能以本地写入结束作为完成。
- PushMessage 变化覆盖、断线恢复和扫描对账。

### Emby 播放

选小体积授权测试视频，验证 STRM 扫描、服务端探测、直接播放、302、Range、续播、字幕、多音轨、转码和外网地址可达性。记录客户端与服务器版本；不宣称所有客户端都兼容。

### PT 配置

固定 pre-dessert-sites commit，选择 3～5 个代表站点：NexusPHP、API 请求站点、特殊规则站点。解析兼容、脱敏 fixture 测试、真实授权账号联调分开验收。

### 授权

上游 LICENSE 自述为 AGPL-3.0 WITH NON-COMMERCIAL CLAUSE，包含额外非商业条款。使用/修改/分发或商业化前核对授权。不能认为“格式兼容”自动赋予配置、图标分发权，也不能假设运行时下载必然规避义务。M1 不复制上游配置或图标。

## 6. 模块开发范围

### 6.1 PT 配置与站点

- 导入/同步公共配置、站点配置和图标，保留来源 commit 与回滚快照。
- 本地覆盖与上游隔离，不破坏原文件。
- 凭据管理、连接测试、健康与兼容状态。
- schema/reuse_schema、disabled_extends、各级字段合并和过滤语义。
- XPath / CSS / JsonPath / XML / 无解析；编码、分页、时区、参数/请求体/头和占位符。
- fields.any、条件分支、属性、数组、删除节点和过滤器链。
- 正则引擎差异与超时防护，不能静默改变语义。
- 特殊下载地址、token 申请与浏览器环境单独适配。
- 对未支持项明确报错，未知原始字段保留用于未来扩展，不执行任意远程代码。

### 6.2 搜索

- 多站并发、逐站结果、限流、超时与故障隔离。
- 标准化资源标题、促销、体积、做种人数、分类、发布时间。
- 筛选/排序/手动下载/加入订阅。
- 站点+种子 ID、InfoHash、媒体身份/季集/版本分层去重。
- 不仅按标题去重，也不将站点请求失败伪装成无结果。

### 6.3 订阅

- 电影、剧集、整季、指定季集、缺集/补集。
- 轮询搜索和 RSS；RSS 能力需检测或由用户填写，不假设所有配置都有。
- 初次历史补抓策略、暂停/恢复、手动触发。
- 硬性条件：站点、类型、分辨率、编码、体积、关键词、促销、做种人数、H&R。
- 优先级：站点/来源/发布组/音轨/字幕/升级策略。
- 缺失信息不按满足条件处理，保存每次决策原因。
- 合集覆盖和多版本升级去重，下载决策采用数据库唯一约束防并发重复。

### 6.4 下载与上传

- qBittorrent 认证、任务添加、分类标签、文件列表/进度与完成状态。
- .torrent 校验，不把登录页/错误页当种子。
- 下载完成文件校验、复制入库，不移动或重命名正在做种的源文件。
- 上传与做种为独立状态机；上传需恢复、去重、完成对账。
- 保留时长/分享率/站点规则与 H&R，达到条件前不清理做种文件。
- 默认不提交私有 PT 种子至网盘离线下载；可选离线能力需另行验证规则与支持。

```text
入库：待下载 → 下载中 → 本地完成 → 上传中 → 云端校验 → STRM → 入库完成
做种：待开始 → 做种中 → 满足保留条件 → 可清理
```

### 6.5 云盘与媒体索引

- 全量/增量/事件触发/周期对账。
- 云端身份与内部 asset UUID 解耦。
- 文件缺失先标记复核，接口故障不直接造成删除。
- 路径移动/重命名/账号变化失效映射及缓存。
- 电影名称年份、季集/合集、样片排除、多版本与人工确认。
- 首期 MKV/MP4 等常规视频；复杂原盘/多段/压缩包不作为 MVP 必达项。

### 6.6 STRM

- 稳定解析地址、路径编码、目录范围、凭据撤销轮换。
- 标准目录/命名模板、增量/全量、冲突/缺失/重建。
- 原子写入与幂等，只有本程序管理的输出可清理。
- 外挂字幕、可选 NFO、批量去抖 Emby 刷新。
- 固定链接有效性、文件路径变化修复与播放诊断。

### 6.7 Emby 网关

- 认证/API/图片/字幕/进度/WebSocket 基础反代。
- 保留调用者身份验证权限，不以管理员 API Key 替代。
- ItemId + MediaSourceId → asset/file 映射。
- PlaybackInfo/客户端能力/直接播放/直接流/转码决策。
- 内网 STRM 地址不得直接下发外网客户端。
- 302 优先，特殊头/不可直连时受控代理或 Emby 转码；不把 HLS 分片统一改为直链。
- 直链缓存：账号+文件身份+必要访问上下文，按有效期减余量；并发合并、更新失效。
- 不全局缓存带用户权限的 PlaybackInfo 响应。
- 受控流代理正确处理 Range/取消/背压，不放大下载流量；回退不能形成递归。

### 6.8 通知

站内、Webhook、Telegram 首期；邮件/企业微信/钉钉后续。订阅命中、下载/上传/入库完成与失败、认证失效、外部服务不可用等事件进入事务 Outbox。投递幂等、去重、指数退避/抖动、死信和人工重发。失败不回滚媒体入库；保护 URL、Passkey 和敏感载荷。

## 7. 数据与可靠任务

实体：系统设置、用户/角色/审计、加密凭据、站点快照/账号、订阅/规则/决策、种子/下载/上传、存储/云端文件/媒体/季集/资产、STRM、Emby 映射、任务/运行/租约、Outbox、通知渠道/投递。

M1 迁移只是上述领域的基础表，不表示每张表业务已实现。扩展时逐个版本新增表/约束，禁止破坏式“修改已有迁移”。

关键唯一约束：站点账号+种子 ID、订阅下载决策、存储+文件身份、媒体+季集/版本、STRM 输出路径、事件+渠道。数据更改与 Outbox 同事务。

队列：SKIP LOCKED、租约令牌、心跳、过期恢复、失败重试、死信，所有完成写入校验当前租约。跨数据库/云盘/下载器不承诺全局恰好一次。外部副作用需执行前后对账，不只依赖任务状态。

Redis 清空可重建；缓存故障降级时限制上游并发，避免请求风暴。Redis 可加短期锁但不决定唯一业务事实。

## 8. 前端计划

| 页面 | 范围 |
| --- | --- |
| 初始化向导 | 管理员、数据库、下载器、CloudDrive2、Emby 检查 |
| 总览 | 实际健康、任务、缺集和异常 |
| 站点 | 凭据、兼容报告、更新/回滚 |
| 搜索 | 聚合筛选、下载、创建订阅 |
| 订阅 | 规则、缺集、决策原因 |
| 下载与传输 | 下载/做种/上传分别展示 |
| 云盘索引 | 目录、扫描、文件/媒体映射 |
| STRM | 预览、生成、冲突、重建 |
| Emby | 策略、连接与播放诊断 |
| 通知 | 渠道、事件、测试/重发 |
| 系统 | 日志、审计、备份与设置 |

按领域划分路由/接口，OpenAPI 生成 TypeScript 客户端（M1 先建立契约与手写最小类型，不能声称已生成）。SSE 只发刷新提示，重连通过数据库查询恢复。长列表分页/虚拟化，加载/空态/错误分别显示，密钥遮罩、键盘焦点、中文、响应式、深浅主题。首期默认浅色；深色主题与完整可访问性测试在后续实施。

## 9. 工程组织

```text
cmd/media-tool/         HTTP / Worker / healthcheck
cmd/migrate/            迁移
internal/modules/      领域模块与接口
internal/adapters/     CloudDrive2 / qBittorrent / PT / 通知
internal/platform/     配置 / 认证 / DB / Redis / HTTP / Web
migrations/            版本化 SQL
web/                   Vue
api/                   OpenAPI
scripts/               初始化 / 本地开发 / 构建
deploy/                Dockerfile / Compose
docs/                  计划 / 架构 / ADR / 状态
tests/                 后续共享 fixture 和兼容矩阵
```

接口：SiteAdapter、Downloader、StorageProvider、PlaybackResolver、NotificationChannel。调用方向：HTTP/Worker → 领域 → 适配器；域模型不依赖前端与 HTTP 框架。M1 在 main 完成显式装配。

## 10. 安全、部署与观测

- 管理认证、会话/CSRF、速率限制，后续角色与用户审计。
- 管理与播放 Key 分离，STRM bearer 需可撤销与限范围；凭据加密主密钥与备份分离。
- 日志不记录查询参数、Cookie、Authorization、Passkey/播放 Key/请求体。
- 防路径穿越、开放代理、SSRF、重定向到内网；外部 URL 和 DNS/重定向链应按适配器校验。
- 只信任明确前置代理，不信任客户端 X-Forwarded-*。
- Docker 默认非 root，DB/Redis 不暴露端口，服务仅绑定宿主机回环。
- 生产 HTTPS、持久化、单独迁移、就绪探测、优雅退出、多架构构建验证。
- CloudDrive2 如需挂载，仅相应服务获得必要权限；本应用不默认 privileged。
- 两种部署：连接既有服务优先；完整外部服务 profile 后续提供。
- 备份/恢复/升级回退实测，不只写文档。
- 结构化日志和 requestId 已建立；后续指标涵盖积压、延迟、缓存、302/回退及上游故障。

## 11. 实施阶段（相对周期，非交付日期承诺）

按一名后端、一名前端估算 8～10 周；单人开发或外部权限受限需重新估算。

| 阶段 | 估算 | 通过条件 |
| --- | --- | --- |
| M0 技术验证 | 3～5 工作日 | 授权视频播放闭环、CloudDrive2 上传/直链、代表站点样本 |
| M1 工程基础 | 第 1～2 周 | Go/Vue、配置/认证、主库/缓存、迁移、任务、Compose |
| M2 PT 与搜索 | 第 2～3 周 | 配置引擎、凭据、3～5 站点实测、多站搜索 |
| M3 订阅下载 | 第 4 周 | 电影/季集订阅、RSS/搜索、qBittorrent、做种状态 |
| M4 云盘 STRM | 第 5～6 周 | 上传对账、索引/识别、STRM/字幕、Emby 扫描 |
| M5 播放通知 | 第 7 周 | 用户权限/多媒体源、302/回退、缓存、通知 |
| M6 稳定发布 | 第 8～9 周 | 兼容矩阵、故障恢复、安全、双架构、备份恢复 |
| 缓冲 | 第 10 周 | 外部接口差异和文档 |

前端与测试持续进行。M0 先消除核心风险；当前用户要求先搭框架，M1 可以先建立接口，但真实播放和业务就绪仍以 M0/M4/M5 为门槛。

## 12. 验收清单

### 业务

- [ ] 导入固定配置快照并报告解析兼容程度。
- [ ] 授权账号完成代表站点搜索和种子获取。
- [ ] 电影订阅、剧集缺集及防重复覆盖。
- [ ] 本地下载/做种保留、云端上传完成校验。
- [ ] 中文/特殊字符固定 STRM，Emby 识别。
- [ ] 直链过期无需重建 STRM；路径移动明确修复。
- [ ] 支持的客户端实测 302，其余明确代理/转码/不支持。
- [ ] 通知可查询、去重、重试，失败不回滚主流程。

### 兼容矩阵

Emby Web、桌面、Android/电视、iOS（有测试设备时），记录媒体格式/客户端/服务端版本，以及播放、Range、拖动、续播、字幕/音轨、转码结果。标注“302 通过 / 需代理 / 需转码 / 暂不支持”。

### 恢复与安全

- [ ] 重复任务不重复下载/上传/STRM。
- [ ] Worker 重启、租约失效、上传完成但提交丢失可对账恢复。
- [ ] Redis 清空不丢业务；CD2 故障不误删。
- [ ] Docker 重建持久化，实际备份可恢复。
- [ ] 未授权用户不能绕过 Emby 权限。
- [ ] 凭据和临时链接不出现在公开日志/配置/测试产物。

M1 已通过的代码级测试和未执行的集成项见 IMPLEMENTATION_STATUS.md，不把框架单元测试等同真实服务验收。

## 13. 后续扩展

更多 PT/浏览器适配、Transmission、更多存储、稳定 assetId、多版本升级、离线下载、复杂原盘、Jellyfin、通知渠道、多用户权限。按照适配接口扩展，不提前为未验证功能引入庞大动态插件机制。

## 14. 原始资料（实施时重新核对并固定快照）

- PT 配置：[仓库](https://github.com/mantou568/pre-dessert-sites)、[LICENSE](https://github.com/mantou568/pre-dessert-sites/blob/main/LICENSE)。
- CloudDrive2：[开发者指南](https://www.clouddrive2.com/api/CloudDrive2_gRPC_API_Guide.html)、[proto](https://www.clouddrive2.com/api/clouddrive.proto)。
- Emby：[STRM](https://emby.media/support/articles/Strm-Files.html)、[PlaybackInfo](https://dev.emby.media/reference/RestAPI/MediaInfoService/postItemsByIdPlaybackinfo.html)。
- qBittorrent：[Web API](https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-%28qBittorrent-5.0%29)。
- Vue：[TypeScript](https://vuejs.org/guide/typescript/overview.html)。
- PostgreSQL：[SELECT / SKIP LOCKED](https://www.postgresql.org/docs/18/sql-select.html)。
- Go：[ReverseProxy](https://pkg.go.dev/net/http/httputil)。

## 15. M0 实施进展（2026-10-08）

已实现官方 gRPC 认证/读取/直链、固定 STRM 实时解析、UA/头受控代理、最小 Emby 静态流权限映射，以及只读诊断和真实环境验收脚本。本地协议服务的串联测试通过。上传仍属于 M4，不把读取接口成功当作上传验证。

真实服务地址/凭据为空，报告为 needs_configuration；M0 的人工客户端和真实 115/CDN 验收仍未完成。详见 M0_PLAYBACK_VALIDATION.md，不据代码测试自动勾选前述真实验收清单。
