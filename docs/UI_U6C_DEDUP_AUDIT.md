# U6c：只读媒体展示与全站功能去重

基线：U6b `e049b29`。逐页检查当前 `web/src/views`、所属面板、请求调用、普通链接、确认流程和公共插件运行时；不修改后端、公共 UI API、插件运行时或依赖。

## 主页面逐页审计

| 页面 | 主归属与审计结果 | 其他位置的处理 |
| --- | --- | --- |
| 首页 `/dashboard` | 只汇总就绪、待办、计数、搜索和插件仪表盘；告警确认归运维 | 删除快捷确认及其 POST，只保留待办普通文字链接。重新读取首页各组快照不是执行连接测试或后台任务。 |
| 搜索 `/search` | 搜索、只读搜索历史、发现推荐、资源结果与下载投递 | 插件发现/推荐从媒体库迁入 `discover`；带入作品身份但不自动开始搜索。历史清理仍只链接运维。 |
| 订阅 `/subscriptions` | 订阅规则、预演、启停、排队与运行记录 | 订阅运行不是手动搜索或通用队列重放；没有跨页订阅修改入口。 |
| 下载 `/downloads` | 下载器、任务控制、上传与手动投递 | 删除已有上传的“入库恢复”对话框挂载，改为带 `transferId` 的普通链接到整理。下载器配置仅此处拥有，不在连接页重复挂载。 |
| 媒体库 `/library` | 内置媒体与 Emby 的只读海报墙、详情及季集浏览 | 不再有插件来源、管理跳转、播放方式或主刷新按钮；仅工具栏图标重读。 |
| 入库整理 `/organize` | 扫描、识别确认、资产身份、输出恢复、归档 | 上传入库恢复统一在 recovery。物理批量整理移交文件页，携带已选候选/资产 ID 的普通文字链接替代执行按钮。 |
| 文件 `/files` | 目录操作、物理整理、受管 STRM 原文、修复与文件级链路诊断 | 独占 `FileBatchDialog` 挂载；接收整理页已选 ID 后读取原选择预览，仍需 PreviewApprove 批准，不自动执行。 |
| 播放服务 `/playback` | 自研服务配置、媒体用户、准入、会话与预处理 | Emby 连接管理只保留普通文字链接；媒体用户不在连接或设置页重复编辑。撤销媒体用户全部登录仅在在线会话执行，用户卡片仅链接该标签。管理员会话与媒体用户会话是不同账号域。 |
| 连接 `/connections` | 115/CD2、Emby 连接与网关、通知、TMDB/代理 | 其他页只有指向其归属标签的普通文字链接，不重复执行配置保存。连接测试、全局网关诊断与文件诊断对象和证据不同，不删唯一接口。 |
| 站点 `/sites` | 模板、账号凭据、指标与采集计划 | 搜索/连接页只链接账号页；没有第二处站点账号编辑。 |
| 插件 `/plugins` | 安装、配置、权限审核和实例工具 | 发现推荐展示不安装/启停插件；首页仪表盘及实例运行时按安全要求保持原样。实例事件/运行记录与系统日志不是同一查询。 |
| 运维 `/ops` | 队列、备份、告警确认、日志、只读数据库、缓存查看清理 | 删除缓存 TTL 表单及 GET/PUT 设置调用，普通文字链接到设置。系统日志查看只归此处，插件内部实例日志按原隔离边界保留。 |
| 设置 `/settings` | 通用访问、命名分类、搜索订阅、外部 API、移动端、管理员安全 | 缓存有效期唯一配置入口；清理只链接运维，TMDB/代理只链接连接页。批次流程只读命名设置，不保存系统模板。 |

单个输出日志的中断恢复（整理）与已有 STRM 内容的全局/批量修复（文件）是不同操作；保留各自唯一接口。通用队列的 system.check 重排与下载/预处理等业务核对重试也不混为一类，不能以“去重”为由重放未知业务。

以上“重读当前结果/错误重试”是按页面读取不同业务快照，不会测试连接、同步 Emby、创建任务或重复执行写操作。导航链接不是第二个执行入口。

## 实际删除的重复执行入口（接口只迁移，无退出）

| 操作 | 唯一执行归属 | 迁移方式 |
| --- | --- | --- |
| 确认同一告警 | 运维 · 备份与告警 | 首页删 `acknowledge` 与按钮，告警原有目标文字链接保留。POST `/ops/alerts/{code}/ack` 仍在 MaintenancePanel。 |
| 撤销媒体用户全部登录 | 播放服务 · 在线会话 | 媒体用户卡片移除重复 POST 与确认入口，改成带 userId 的普通链接；按用户突出会话；没有正在播放的会话也可读取该用户登录概况并进入原确认流程，不丢失空闲登录的撤销能力。单设备撤销作用域不同仍保留。 |
| 缓存有效期配置 | 设置 · 搜索订阅 | 运维移除 settings/application 读写和 TTL 草稿，改为普通链接；两个 TTL 字段与冲突保存能力由 U6b ConfigPanel/revision 保留。 |
| 已有上传的入库恢复 | 入库整理 · 恢复与归档 | 下载页删 OutputRecoveryDialog，只传 transferId 到整理。GET `/transfers/{id}/output-preview` 与 POST `/transfers/{id}/resume-import` 仍由同一共享组件调用。 |
| 候选/资产的物理批量整理 | 文件 · 文件浏览 | 整理页移除 FileBatchDialog 挂载，普通链接携带已选 ID。文件页接手原 `open(candidates/assets)`，保留 organize-selection 与预览、审批、核验全部接口。识别确认与物理整理明确分工。 |
| 插件发现/推荐 | 搜索 · 发现推荐 | 从媒体库移出，原 GET `/media/library/sources`、kind、分页、来源归属、hasMore/truncated 和 metadata/details 保留。 |
| Emby 设置入口 / STRM 修复 / 媒体用户 / 系统日志 | 连接 / 文件 / 播放服务 / 运维 | 媒体库不再有管理链接；其他已有普通文字链接保留，没有复制执行表单。 |

## 媒体库与播放链接

- `/media/library?source=managed` 是默认来源；Emby 首页、全部媒体、`parentId` 子列表与 `/media/emby/items/{id}` 详情都保留。
- 列表仅标题、类型和分页；375px 海报墙两列。详情展示真实海报、简介、年份、类型、季集；评分、大小、清晰度没有返回值时显示“—”。
- `/integrations` 只读取是否有启用的 Emby；未配置显示“未连接”，不放管理按钮或链接。读取连接状态失败不冒充未连接或空列表。
- 旧 `/library?source=plugins` 迁移 `/search?tab=discover`，保留其他参数和 hash；快速前往可直接找到发现推荐。
- `originalSTRM` 指向受保护的 GET `/media/assets/{id}/strm`，接口返回已验证文件原文，不是网页流媒体地址。本页仅以“复制播放链接”读取原文并复制其中的稳定链接，不生成 CDN 地址、不打开播放器、不做中转/转码。复制失败仅在当前详情展示可手动选择的字段，切换/关闭后清空。文件与整理页的管理能力不受影响。
- `GET /metadata/details` 在推荐详情及目录条目有真实 TMDB ID 时继续使用，不靠标题猜测身份。

## 后端契约限制（不伪造、不越界改后端）

1. catalog.Item 当前只有标题、来源、年份、类型、简介、海报、路径、状态、季和集编号，不保证评分/大小/清晰度/TMDB ID。现有详情回执未提供的字段不能捏造。
2. managed 的 ListParent 当前忽略 parentId，没有作品级/季级实体和全库季集查询。本页只把**当前分页真实文件**按标题与年份分组，再按真实季/集编号浏览，明确告知不代表整部已收齐，绝不编造 ID 或伪造跨页完整性。
3. Emby 目录当前为带 parentId 的递归分页，没有单独季对象。本页用真实剧集 ID 请求子项，再按返回的季编号分层；季 0 保留。季集页只声明当前返回范围，并保留后续分页。
4. 没有网页播放接口，因此页面不提供播放、中转、HLS、转码或假播放链接。

## 证据与验证

- API 目录只更新调用点，没有把任何原有前端接口改成“未使用”等分类。
- 运维 AST 守卫仍保留全部 required API，扫描范围扩展到迁入的 settings，并识别实际 `useRevision` 的 GET/PUT；原 required 集合不减。通用修订写入另受 U6b 守卫校验。
- 新单测覆盖只读边界、Emby 未连接、来源/详情回执失效、季 0 与分层、分页归属、tab/kind 同步、原始链接清空及去重唯一挂载。
- E2E 保留原业务断言，执行入口变更的用例改为跟随普通链接，再在主归属页面确认；不删除预览/核对断言。
- 截图：`/tmp/media-tool-u6c-screenshots/library-{375,1440}-{light,dark}.png`、`library-detail-1440-dark.png`、`discover-1440-dark.png`。海报为隔离 mock，未向仓库存入示例媒体。

## 变更文件清单

- `docs/FRONTEND_API_ROUTES.md`
- `docs/UI_U6C_DEDUP_AUDIT.md`
- `web/e2e/framework.spec.ts`
- `web/e2e/u1-features.spec.ts`
- `web/e2e/u2-workspace.spec.ts`
- `web/e2e/u3-downloads.spec.ts`
- `web/e2e/u4-fixture.ts`
- `web/e2e/u4-library.spec.ts`
- `web/e2e/u6c-library.spec.ts`
- `web/src/acquisition/DiscoveryPanel.vue`
- `web/src/acquisition/discovery-view.test.ts`
- `web/src/acquisition/useDiscovery.test.ts`
- `web/src/acquisition/useDiscovery.ts`
- `web/src/dashboard/useDashboard.ts`
- `web/src/files/BrowserPanel.vue`
- `web/src/guards/u6-api.test.ts`
- `web/src/guards/u6c-readonly.test.ts`
- `web/src/media/PosterWall.vue`
- `web/src/media/catalog.css`
- `web/src/media/model.ts`
- `web/src/media/useCatalog.test.ts`
- `web/src/media/useCatalog.ts`
- `web/src/navigation/pages.ts`
- `web/src/operations/CachePanel.vue`
- `web/src/operations/operations.test.ts`
- `web/src/playback/SessionsPanel.vue`
- `web/src/playback/UsersPanel.vue`
- `web/src/playback/playback.test.ts`
- `web/src/router.test.ts`
- `web/src/router/redirects.ts`
- `web/src/views/DashboardWorkspaceView.vue`
- `web/src/views/DownloadsWorkspaceView.vue`
- `web/src/views/MediaWorkspaceView.vue`
- `web/src/views/OrganizeWorkspaceView.vue`
- `web/src/views/SearchWorkspaceView.vue`

## 最终验证结果

- `npm run typecheck` 通过；`npm test` 79 个文件、463 个单测通过；`npm run build` 通过。
- `npx playwright test` 210 通过，1 个 Linux 专用用例按原条件跳过，未增加重试。
- F2 插件表单旧用例补上“初始 JSON 与动态表单均已读取”的同步屏障，再编辑并验证双向绑定；没有修改插件运行时。
- 六张指定 mock 截图已生成并检查，375px 两列与无横向溢出断言通过；发现推荐占满工作区。
- 所有 API 分类保留，新增非测试代码 748 行，低于 1200 行；未改后端、依赖、公共 UI API 或插件运行时。
- 隔离测试串行执行，限 media_tool_test 与 Redis DB15；任务专用 Go 缓存已清理。仅本地提交，不推送、不部署。
