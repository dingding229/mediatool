> **账号登录更新（2026-10-09）：** 首次运行在管理界面注册唯一管理员；注册成功后公开注册关闭。之后使用账号密码登录，旧ADMIN_API_KEY不再生效，旧会话已通过迁移撤销。密码6–20个字符，不生成默认账号。不在聊天提供密码。详见 [管理员账号说明](docs/ADMIN_ACCOUNTS.md)。

# Media Tool

Go + Vue 3 的模块化媒体自动化程序。PostgreSQL保存业务事实/任务/恢复日志，Redis保存可重建缓存；CloudDrive2负责115扫描/上传；播放可使用已授权的115官方开放平台按播放器UA动态取链。STRM只保存固定解析地址，不保存临时CDN链接。

**当前0.27.0-dev，源码迁移50。主要业务已实现；真实115授权、默认Sen UA的网关302/CDN206与非零Range样本已通过，真实PT入库全链路和客户端完整验收仍未完成，不宣称完整1.0。** 不注入模拟账号、示例下载或假直链。

本轮新增：HDSKY做种/分页证据、订阅结构化筛选、功能型Go/WASM插件、原始业务日志及隔离备份验证。
插件功能和权限见 [PLUGIN_FRAMEWORK.md](docs/PLUGIN_FRAMEWORK.md)，日志/备份见 [RAW_LOGS_AND_BACKUP.md](docs/RAW_LOGS_AND_BACKUP.md)。

本轮新增：内置媒体库 Provider v1、独立媒体用户管理和独立媒体端口根路径 `/` 的授权只读目录。播放票据及完整 Emby 替代尚未实现；现有播放不切换。详见 [内置媒体库与媒体用户](docs/NATIVE_MEDIA_SERVER.md)。

新增播放器UA访问记录与白名单：默认仅记录，可从真实UA记录加入，明确启用后限制播放入口。说明见 [PLAYER_UA_ACCESS.md](docs/PLAYER_UA_ACCESS.md)。

本轮更新：访问IP/核验Emby用户、促销排序、115官方空间，以及独立STRM目录/工具和保留原文的缺失文件批量修复。说明见 [FILES_PLAYBACK_WORKBENCH.md](docs/FILES_PLAYBACK_WORKBENCH.md)。

## 能力

- 加密服务连接、修订保护和审计；管理员日志/数据库按用户要求显示原始值，普通会话可撤销。
- 内置 mediatool-site Go模板模块，构建固定依赖、按选择加载站点并保留既有账号所需模板；独立模板页和模板驱动账号表单，UA/RSS自动获取，多站搜索、真实内置ICO、模板全选、站点数据定时刷新。
- TMDB人工身份确认、qBittorrent下载及共享源文件校验，不移动做种源文件。
- CloudDrive2远程上传与校验，暂停/恢复/取消/核对/受约束人工绑定会话。
- 电影/剧集订阅、预演、周期搜索/RSS、首次基线、季集覆盖与去重。
- 云盘流式索引、缺失两次确认、移动跟踪、分页、批量确认、身份/季集修正、默认停用的子目录扫描计划和CloudDrive2变化推送对账；身份输出迁移需预览后明确确认。
- 固定assetId+文件级Key的STRM、NFO和本地字幕；加密恢复日志保护STRM/NFO落盘中断及手工文件冲突。
- 独立Emby根网关、用户权限校验、Range/HEAD、上下文隔离缓存；当前用户严格模式直接使用STRM原链接，禁止转码/媒体中转，旧认证流入口仅兼容模式保留。
- 自己AppID的115官方PKCE扫码授权、加密Token原子刷新、实际UA取链、同UA最多1KiB CDN验证后302；官方模式失败不偷偷代理。
- 站内与Telegram通知、明确失败/限流/未知结果、人工重复风险确认。
- 上传RPC与控制修订/Worker租约互斥；按确认区间去重字节，不把哈希进度当上传。
- 离线原子主密钥轮换、旧gRPC连接排空回收、备份临时库恢复检查、AMD64 GitHub Actions / Docker Hub 构建发布。

## 开始

已有私密docker-compose.yml不要重新初始化或替换主密钥；部署参数直接写在该文件。开发测试.env仅供本地隔离测试，不是服务器部署配置。

```bash
python3 scripts/init-compose.py  # 仅首次，用直接私密Compose；已有密钥不能替换
docker compose pull
docker compose up -d --wait
```

基础部署管理8080、Emby网关8091。初始化默认只绑定回环；直接编辑私密 docker-compose.yml 指定管理端口及网关端口；外部Emby/电视需明确配置共享目录与网络/HTTPS访问。提供方凭据在各自所属模块填写，“服务连接”仅查看，不发到聊天。

```bash
# 连接 NAS 目录：直接编辑同一私密 docker-compose.yml 的 volumes 和用户参数
# 不再使用额外 Compose 覆盖文件。
docker compose up -d --wait
```

先验证已有115文件播放，再验证一个合法小资源下载入库，最后先预演再启用一个订阅。

## 文档

- [当前状态](docs/IMPLEMENTATION_STATUS.md)
- [首轮需求与验收证据矩阵](docs/FIRST_ROUND_ACCEPTANCE.md)
- [115官方播放与授权](docs/OFFICIAL_115_PLAYBACK.md)
- [自有文件真实上传验证](docs/OWNED_UPLOAD_VALIDATION.md)
- [严格直链与STRM原链接](docs/DIRECT_PLAY_ONLY.md)
- [CloudDrive2变化触发扫描](docs/CLOUD_CHANGE_SCANNING.md)
- [安全输出迁移与字幕清单](docs/OUTPUT_MIGRATION.md)
- [单集元数据与多集覆盖](docs/EPISODE_METADATA.md)
- [兼容模式历史HLS限制](docs/HLS_STARTUP_RELIABILITY.md)
- [剩余范围与完成门槛](docs/REMAINING_PLAN.md)
- [配置与使用](docs/ALPHA_QUICKSTART.md)
- [本轮可靠性、测试与主密钥轮换](docs/OFFLINE_HARDENING.md)
- [原开发计划](docs/DEVELOPMENT_PLAN.md)
- [架构](docs/ARCHITECTURE.md)
- [执行记录](docs/WORK_LOG.md)
- [Docker部署](deploy/README.md)
- [OpenAPI](api/openapi.yaml)

## 验证

```bash
# 隔离media_tool_test，不可指向业务库；必须-p1避免各包TRUNCATE互相干扰
bash scripts/test-go.sh  # 显式覆盖为media_tool_test、迁移test-only、Redis DB15、-p 1
go vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
npm run test --prefix web
npm run build --prefix web
bash scripts/test-e2e.sh  # 仅隔离media_tool_test/18084，不在业务库注册测试管理员
```

报告不含真实密钥。APP_SECRET_KEY独立安全备份，不能只改环境变量而不轮换数据库ciphertext。临时CDN地址不写入STRM；索引可见和100%进度都不等于真实客户端播放成功。

- [PT配置兼容与固定快照恢复](docs/PT_CONFIG_COMPATIBILITY.md)

输出迁移归档可在“云盘索引 → 旧输出归档”查看。默认保留30天且**不自动删除**；物理清理须只读预览后明确批准。安全范围、请求结果核对与中断恢复见[归档管理说明](docs/ARCHIVE_CLEANUP.md)。

固定STRM冷取链默认总预算30秒（可配置5秒–1分钟），官方API连接池复用；超时明确返回504而不回退媒体中转。分阶段安全日志、取消/缓存边界及验收范围见[取链可靠性说明](docs/PLAYBACK_RESOLUTION.md)。

非连续剧集合集生成各个明确集号的STRM索引，所有索引共用同一固定链接、播放完整原文件，不补缺集或伪称单集切片。已有输出须明确预览迁移；实际Emby兼容证据见[多集投影说明](docs/EMBY_EPISODE_PROJECTION.md)。

跨版本未完成任务遇到旧布局时暂停等待核对，不混用新旧STRM。明确迁移后可批准“不重新上传”的专用入库恢复；说明与边界见[输出入库恢复](docs/OUTPUT_IMPORT_RECOVERY.md)。

## 单容器程序与 Docker Hub 发布

唯一官方部署文件：`docker-compose.yml`；仅 `app`、`postgres`、`redis` 三个服务。一个主程序进程承担管理端、302、三个队列和调度，启动自动迁移。
GitHub Actions 运行测试后仅发布 AMD64 到 `dingding229/mediatool`。服务器用 `scripts/update-server.sh` 拉取不可变 SHA / 摘要镜像，禁止直接上传本地构建镜像。
内置 slog JSON 日志可设置 `LOG_LEVEL=debug|info|warn|error`，Docker 有界轮转。
后台密码找回已移除：部署所有者创建空 password 文件并重启，在私密启动日志读取一次性临时密码，登录必须更换新密码。
详细部署和安全注意事项见 `deploy/README.md`，本轮计划见 `docs/UNIFIED_DEPLOYMENT_PLAN.md`。

本轮入口与部署详见 `docs/WORKBENCH_RELEASE_GUIDE.md`、恢复/数据保护详见 `docs/WORKBENCH_SECURITY.md`，插件合约详见 `docs/EXTENSIONS.md`。


## 工作台 UI（0.9.1）

界面按媒体浏览、资源获取、文件与入库、系统运维分组。顶部「快速前往」支持
Ctrl/⌘ K、方向键、Enter 和 Escape；此入口只搜索功能，不触发媒体或站点搜索。
总览展示最近读取的依赖/任务快照和查询时间，不显示模拟媒体或全量任务统计。
TMDB 搜索区分查询中、服务错误、无匹配结果、缓存结果与正常作品列表。

手机导航使用具有焦点约束的模态抽屉；提供跳到主要内容、键盘焦点、减少动态效果
和 375–1440px 的回归验证。具体设计约定见
`design-system/media-tool/MASTER.md`，本轮实施与验收范围见
`docs/plans/ui-refactor-20261012.md`。播放、文件确认、连接验证和账号恢复规则不变。

原生站点模板格式、来源切换和部署同步见 [docs/PT_GO_TEMPLATES.md](docs/PT_GO_TEMPLATES.md)。

## 文件与入库工作台

自动目录能力、扫描暂停/对账证据、0day+TMDB识别、持久化整理批次、托管输出和逐路径Emby确认、统一历史与恢复，详见 [使用说明](docs/FILES_LIBRARY_WORKBENCH.md)。真实媒体变更必须先预览并明确批准；隔离测试不会替代真实播放器完整解码验收。

云盘连接独立页、目录级命名模板、电影/剧集/季/集刮削选项及历史移除说明见 [云盘与刮削](docs/CLOUD_NAMING_SCRAPING.md)。

系统设置已按访问域名、命名刮削、搜索订阅、代理、TMDB、账号、移动端分组。支持种子文件/HTTP(S)种子链接/磁力链接手动下载，默认仅下载，不自动整理。详见 [域名与手动下载](docs/ACCESS_AND_MANUAL_DOWNLOADS.md)。
