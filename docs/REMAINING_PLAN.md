## 2026-10-13 当前五站点模板更新

CHDBITS（ptchdbits）、SSD、OURBITS、UBITS、TTG自包含精简模板已开发；索引与差异见examples/sites/README.md，执行计划见plans/site-templates-20261013.md。仅保留4个核心请求，不含账号秘密/签到/验证码等。SSD/UBITS使用明确的汇总统计，TTG保留独立搜索/下载/做种契约，缺失促销/H&R及异常页面保持未知。搜索结果容器与解析契约指纹v4已补齐。schema仍32。

本地Go race/vet、Vue32/构建、E2E52、Python18通过。正式发布以本轮Actions/Hub/服务器回执为准。新模板没有真实账号实测，不自动替换已有定义，不声称后台支持任意JSON上传；真实PT下载/订阅/入库/通知链路和设备播放稳定性仍需单独验收。

---

## 2026-10-12 当前插件/核心功能调整

0day解析和电影/剧集命名模板改为基本功能，不显示为插件。插件统一为可执行WASM ZIP；旧Webhook/rules/手填Manifest及旧示例已删除，不实现旧版数据兼容。已核对用户指定MoviePilot-Plugins V3开发文档及加载/生命周期源码，开发指南为PLUGIN_DEVELOPMENT.md。HDSKY独立精简模板仅保留4个业务请求，不自动替换已配置站点。

本地Go race/vet、Vue32项/构建、52E2E、18Python通过；正式发布以本轮Actions、不可变Docker Hub镜像和服务器回执为准。新模板目前为可独立加载的JSON，不声称后台已有任意本地JSON上传；真实PT下载、完整订阅/媒体导入/通知链路、设备解码及持续稳定性仍不是本轮自动测试覆盖的验收。

---

# 当前剩余工作与完成门槛

更新：2026-10-12。当前源码0.11.0-dev/迁移31；部署和验收以当次Actions及服务器回执为准。

## 本轮实施

HDSKY做种统计/站点分页、订阅结构化筛选和预演、插件功能框架、原始运维日志、最新备份恢复工具
已接入源码。具体实施/边界见plans/pt-subscriptions-20261012.md与PLUGIN_FRAMEWORK.md。
不据此宣称全部真实PT链路、所有站点、PWA公网安装或所有客户端都通过。

## 仍可继续开发

1. CloudDrive2更细变化父目录合并队列（当前为配置范围对账）。
2. 大规模媒体库/搜索快照/任务队列的容量与长期运行验证。
3. 订阅优选与版本升级策略（本轮只加硬性结构化条件，未自动替换现有媒体）。
4. 更多安全的插件宿主能力与通知渠道；所有新增权限仍须显式授权，不提供任意SQL/宿主命令。
5. 更多站点解析语义及下载器/CD2实际版本矩阵，未知和不支持不能当成功。

## 需要真实资源/设备/部署条件

- 有效HTTPS域名及Android/iOS的PWA安装/更新验证。当前HTTP保持用户指定测试入口。
- 合法小PT资源完整下载→保留做种→CD2上传校验→STRM/NFO→Emby→通知闭环。
- 一个真实订阅有限启用后的缺集、去重、RSS与促销变化观察；不自动开启现有订阅。
- 实际播放器解码、音轨/字幕、长拖动/续播与普通Emby用户权限；302/Range采样不等同全部通过。
- 选定测试文件并确认的实际移动/输出迁移，不能自动改用户现有影视。

## 不再作为本部署目标

恢复码后台找回、自动媒体中转/HLS/转码、ARM64镜像发布均不在当前范围。
持续保持一个Compose、app/PostgreSQL/Redis三个主服务、AMD64 Hub发布、原STRM严格302。

## 历史记录（保留发生时状态，不能直接作为当前待办）

## 本轮不依赖账号的开发

| 项目 | 当前状态 |
|---|---|
| 数据库配置驱动的播放验收工具 | 已实现，支持文件级assetId、独立网关和当前路径 |
| Emby托管链接严格识别 | 已实现，固定origin/资产归属/legacy开关；严格模式按用户要求返回STRM原链接并显示文件Key，115凭据/管理密钥不暴露 |
| 文件与数据库中断恢复 | 加密恢复日志、STRM/NFO事务内stage、摘要保护、后台恢复、人工冲突重试已实现 |
| 上传恢复与并发 | RPC互斥、控制revision、Worker租约、明确暂停状态、人工绑定门槛、去重字节进度已实现 |
| 媒体管理与扫描 | 候选/资产分页、明确身份批量确认、身份/季集修正、默认停用的持久化子目录扫描计划已实现 |
| 主密钥轮换与连接回收 | 离线全表原子rekey/default dry-run、活动RPC排空后回收已实现 |
| 115官方播放 | 自己AppID的PKCE授权/实际UA取链已实现；真实授权后默认Sen UA的302、CDN206与非零Range样本通过，真实客户端解码仍待验 |
| 授权可靠性 | 扫码长轮询不持有播放数据库锁；兑换前持久化一次性claim并清verifier，取消/连接编辑/账号改密优先，未知结果不自动重放 |
| 发布验证 | 双架构镜像、并发/IPv6/固定反代origin/备份恢复按证据记录，不夸大验证范围 |

## 仍可继续开发/完善

1. CloudDrive2 PushMessage事件触发已补本地实现（默认关闭、范围dirty合并、重连对账/unsupported回退）；当前粒度为配置子目录。仍需发布/真实受控事件验收及更细变化目录增量队列，见CLOUD_CHANGE_SCANNING.md。
2. 跨媒体类型/剧集/季集输出迁移与完整托管字幕清单已补候选实现：明确预览/证明确认、新输出依赖完成后归档旧托管文件、共享NFO保留。仍需发布/实际用户明确迁移、多集命名/更丰富单集provider metadata及归档保留期/人工清理验收，见OUTPUT_MIGRATION.md。
3. pre-dessert-sites宽泛语义与真实版本兼容继续验收；有序case/JSON请求体/数组/跨字段/分组展开/同提交恢复已完成本地实现。算术时间偏移、PCRE差异、浏览器挑战与专用框架仍有明确限制，该兼容版本已发布，仍需更宽语义和真实账号验收。
4. 大规模库/长时间代理吞吐、缓存egress context、更复杂多反代部署与生产容量验证。
5. 真实CloudDrive2会话发现字段、qBittorrent v2哈希别名和客户端能力的版本矩阵。

## 需要真实账号/设备的验收顺序

1. 已有115文件 → 扫描确认 → 固定STRM → Emby网关：已完成授权302/CDN206的有限验证；仍需真实Sen解码/音轨/字幕/长拖动、直链过期重取和文件移动；当前用户明确禁止转码，不再以网页HLS冷恢复为本部署门槛。
2. 一个合法小资源 → PT搜索 → qBittorrent → CloudDrive2上传 → STRM/NFO → Emby可见 → 通知，保留做种源文件。
3. 先预演，再启用一个订阅；验证缺集、重复、RSS基线和促销变化。
4. 原格式直链的桌面/Android-TV/iOS矩阵、音轨/字幕，以及实际NAS双架构环境；网页不兼容时明确失败，不自动转码。

## 完成门槛

测试提供方与真实业务验收分开记录；真实播放配置缺失、未知上传结果、通知超时和单纯100%进度都不能作为通过证据。整体需求只有真实链路与必要异常恢复逐项验收后才完成。

## 首轮推进顺序

1. 保持当前用户授权与已有STRM，发布并回归授权/rekey可靠性修复。
2. 检查本项目PT、qBittorrent、通知配置是否完整；缺配置只列缺项，不借用其他项目账号。
3. 自有49,982字节视频的真实CD2上传/云端SHA-1/全文件SHA-256已通过（不冒充PT业务）；接着用合法PT小资源走下载→保留做种→上传→STRM/NFO→Emby→通知，记录真实证据。
4. 再启用一个订阅进行缺集/重复/RSS验证，同时补真实客户端与网页冷恢复矩阵。
5. HTTPS、安全恢复、双架构实机/容量门槛逐项验收。详细矩阵见FIRST_ROUND_ACCEPTANCE.md；原计划第12节不凭fixture自动勾选。

2026-10-10补充：真实自有文件上传已通过，详见OWNED_UPLOAD_VALIDATION.md。实际Chrome在官方授权后仍出现HLS network/空缓冲，服务端相同参数的master/main/首TS能200且FFmpeg有帧；不能继续只把问题归为原先0帧。已补状态码/响应字节/耗时的安全访问日志用于定位初始请求，保持转码/鉴权语义不变。


2026-10-10追加：首播冷启动已复现10秒客户端断开、同会话随后可读；固定版本网页HLS首字节60秒/片段反代75秒有限预算和取消分类已部署，实际脚本GET已确认。旧Chrome缓存模块是否刷新仍未证明；正确510秒片段85.ts上游500/实际FFmpeg seek无帧，**冷恢复并未解决**。首要继续定位该seek编码生命周期，并在正常设备验证新策略加载。见HLS_STARTUP_RELIABILITY.md。


2026-10-10诊断推进：原生UA官方302/CDN206再次通过，但明确是管理员登录会话的字节证据；旧HLS工具误标普通用户，现默认非管理员/显式管理员并区分报告。尚未选到合格活动非管理员会话，不自动创建账号。直接Emby续播也有200/500交替，独立FFmpeg seek读取成功约15.4秒；不将代理或固定15秒超时当成已确认根因。接着优先获取失败时的准确编码/输入异常与网页新策略实际加载证据，再让正常设备验证Sen网关25015的实际解码与非管理员权限；其他PT/qBittorrent/通知门槛保留。见HLS_STARTUP_RELIABILITY.md。


2026-10-10 PT兼容推进：固定37bf5c5064ae437f54e26b57406ca84d480daab5，193站点配置均静态加载（159核心规则支持/34部分支持，不是账号验收）。修复请求体、字段数组/依赖/分组、条件顺序的jsonb保存和未知促销/H&R；新增原子批次、指定同提交恢复及Vue操作入口。该阶段随后已完成最终回归、原生双架构实际镜像验证及指定项目发布；下一步做用户合法PT小资源真实业务链路。详见PT_CONFIG_COMPATIBILITY.md。


2026-10-10发布收尾：PT配置语义/同提交恢复已发布，PG17/三个Worker/两个资产/一份官方授权保持，23个其他容器不变；原生实际Docker审计通过，本地amd64模拟运行异常仍开放。下一步不是继续堆静态成功数：优先网页seek失败/实际加载新HLS策略与客户端矩阵，同时用本项目用户自己的PT、qBittorrent与通知凭据推进合法资源全链路；不能借别的项目凭据或用自有上传样本替代PT链路。详见PT_CONFIG_COMPATIBILITY.md发布收尾。


2026-10-10网页缓存推进：已发布固定版本index/apploader缓存命名空间，真实Chrome模块src/服务器200字节数和离线HLS构造配置证明新策略实际加载。时间轴510秒曾取得影片画面，但首播/恢复仍有网络错误、暂停和重新加载；不能标稳定播放。下一步针对首次非fatal网络事件拒绝promise与原HLS有限重试/编码慢输入的关系，必须带fatal/取消/有限总预算，不用强制直放或无限重试。独立510秒500/FFmpeg启动错误与真实PT链路仍待验。见HLS_STARTUP_RELIABILITY.md。


2026-10-10 V2启动推进：90秒总启动预算、只等待HLS本身已配置的nonfatal瞬时重试已发布。真实Chrome首播两次500后连续到158秒/有画面/暂停恢复；510秒seek及新的冷恢复仍失败，90秒终止。下一步从已播放seek边界与上游输入/FFmpeg启动失败继续，不把启动修复扩大成全播放通过；保持原始PT/qBittorrent/CD2/通知/订阅验收及未配置用户凭据门槛。详见HLS_STARTUP_RELIABILITY.md V2证据。

2026-10-10用户目标修正：当前明确不需要转码。指定部署改为严格直链；不再继续以网页HLS转码、重试或兼容模式首播作为用户当前目标。后续客户端验证按原格式直链（Sen等原生播放器）与无转码/中转否定门槛执行；若网页不支持TS/MKV/音频格式则明确不兼容，不能悄悄开启转码。item17入口映射/三个302入口及拒绝HLS已实测，详见DIRECT_PLAY_ONLY.md。


## 2026-10-10 当前下一步

1. 变化推送已发布并真实只读收到RPC响应帧；默认关闭，真实受控文件变更/更细目录队列仍待验。
2. 继续不等账号的输出迁移/归属冲突处理、单集元数据/字幕清单和用户界面状态修正。
3. 指定项目PT/qBittorrent/外部通知配置目前均0、订阅0；请在服务连接用自己的账号完成配置后，才能验收真实搜索→取种→做种下载→CD2上传→STRM/NFO→Emby→通知。不得借用其他23容器的配置或把fixture替代此业务门槛。
4. STRM原链接/不转码保持已发布要求；HTTPS、真实设备权限/解码与受控恢复矩阵继续保留。


## 2026-10-10 单集元数据推进

候选代码完成明确季集TMDB验证、单集NFO、资产/整剧隔离、多集连续/稀疏命名、只读管理预览；实际已入库文件不自动变化。单集元数据已真实只读查询并发布（migration19），Emby消费者命名矩阵仍待验。归档查看/保留期/明确批准的人工清理已实现（源码migration20），本阶段发布/验收状态见ARCHIVE_CLEANUP.md与archive-cleanup-*报告；完整PT/下载/订阅/通知链路仍需本项目自己的配置。见EPISODE_METADATA.md。


## 2026-10-11 当前推进状态

归档管理已发布migration20，默认30天且无自动清理；真实归档/当前输出/源媒体未删除。上阶段原条目11/17及原生/浏览器UA四组302→115CDN206复核通过，但实际取链有10秒间歇超时。连接池复用、统一有限取链预算、504/取消分类和无敏感分阶段证据已发布；实际八组原STRM冷/热302/CDN206和两次刷新后缓存复读通过，不把短期成功当成长期无故障。持续性结论以resolve-budget-*报告为准，见PLAYBACK_RESOLUTION.md。后续仍需真实设备长seek/权限/解码和自己配置PT/qBittorrent/外部通知后的完整业务链路，不能以增加默认等待时间声称网络故障已全部消失。


## 2026-10-11 Emby消费者覆盖推进

已使用同版本实际独立Emby复核，发现并修复非连续E01E03误扩为1–3的问题；新非连续合集采用每个明确集号的完整文件索引，共享STRM原链接，不假称单集切片。实际Season0/普通稀疏集合与连续范围矩阵见EMBY_EPISODE_PROJECTION.md。别名展开/收敛、字幕放大边界、逐输出路径Emby确认和Vue明确预览已补实现；本阶段发布/验证以emby-projection-*报告为准。真实用户媒体未自动迁移，真实设备/普通用户、PT下载/做种/订阅/外部通知仍未全验。


## 2026-10-11 跨版本未完成任务恢复

已补旧布局全集合双重核对、operator-required暂停、明确迁移后的专用入库恢复（不重新上传、不重写STRM/NFO/已确认身份）、当前文件/Key/源字幕证明和候选后台完成门槛。下载页可按assetId聚焦云盘索引；原Job不盲重试，批准/执行/再批准受控制修订保护。源码保持20，实际发布/保护与验收见OUTPUT_IMPORT_RECOVERY.md和output-recovery-*报告。仍需真实账号的PT/下载/订阅/通知与完整设备/普通用户/持续性门槛，不把fixture标作业务完成。


## 2026-10-11 RSS核心兼容补充（已发布，真实账号待验收）

已补RSS/Atom结构验证、已核对的UNIT3D数字GUID/键路径下载、相对URL与xml:base、错误源不建立/清空订阅基线、私有下载路径仅加密保存，以及RSS-only不继承上一次搜索的促销/H&R/做种/体积信息。同run搜索+RSS元数据保留，定义明确声明的RSS体积/促销过滤器继续执行。具体范围和限制见PT_CONFIG_COMPATIBILITY.md；这是本地隔离实现，不把合法空源、受控HTTP/PG样本当真实站点验收。

双架构Docker构建及原生amd64隔离PG测试、独立备份恢复已通过；已仅更新指定项目server及三个Worker，发布后原STRM/UA/CDN302、5份输出、账号/授权和23个其他容器保护检查通过。下一步继续核对PT请求配置与RSS重试/多站点基线一致性，再完成用户真实账号链路。真实PT→下载→做种保留→CD2→STRM/NFO→Emby→通知仍需用户自己的账号和合法资源；timezone消费者语义和专用框架验证仍未解决，193站点审计数不作提升。完整目标保持未完成。


### 历史RSS候选构建收尾（2026-10-11，发布前记录）

已完成linux/arm64与linux/amd64候选Docker构建；原生aarch64 Docker运行配置审计通过（UID65532、只读根/只读参考卷、无网络，193/159/34/0、accountVerified=false）。首次SSD bind挂载被Colima拒绝，保留失败日志；改为复制到本轮独立参考卷并只读挂载，未改原参考文件权限，临时容器/卷已删除。此处不是原生amd64运行、RSS实际账号或NAS设备验收。

准备了linux/amd64的sites/search/subscriptions三份独立测试二进制及校验摘要，下一步在指定x86_64服务器的临时网络/独立PostgreSQL库运行，再进行受保护发布。源码/镜像/测试manifest见data/remote-deploy/rss-core-candidate-manifest.json；诊断见data/diagnostics/rss-round-audit-20261011.json。镜像构建后的补充只涉及文档状态和一个负例XML转义修正（避免跨域URL负例先在XML语法处失败），运行代码未变；修正后的RSS回归单独通过并重新交叉构建sites测试二进制。初始源码包/构建证据保留，最终当前源码与构建源码差异由manifest明确列出，不谎称两者全部文件逐字相同。当前未创建SSH身份、未连接或更改正式部署，原STRM/不转码策略保持。


## RSS发布收尾：2026-10-11

media-tool:rss-core-20261011已发布至指定项目，仅更新server/worker/transfer-worker/library-worker，无生产迁移、PG20不变。原生x86_64测试使用全新内部网络与独立PostgreSQL容器/卷，sites/search/subscriptions共32项顶层测试（28/1/3）全部通过，无跳过；未访问生产库或使用真实PT身份。镜像配置加载审计193/159/34/0通过，静态accountVerified仍false。32项跨编译测试不带race，本地全量race证据另行记录，不混为同一次验证。

发布前备份在另一全新PostgreSQL容器恢复，并对8类密文表执行离线dry-run/回滚检查；无生产改密或主密钥轮换。所有临时容器/卷/网络和含密钥的临时目录已清理。发布后12项原STRM媒体详情及GET/POST PlaybackInfo逐字一致，四组原链接/UA请求302→cdnfhnfile.115cdn.net→206，HLS/转码仍拒绝；不声称实际设备解码或非管理员矩阵通过。密码记录/.env/compose、资产/绑定/输出/播放授权/扫描计划、5份磁盘输出和Emby/CD2/其他23容器由前后摘要核对保持。

证据：data/remote-deploy/rss-{native-pg-check,backup-restore-check,deployment,preservation-check,original-strm-check}.json与三份已校验native日志。回退镜像media-tool:before-rss-core-20261011；数据库备份/root/docker/mediatool/private/database-before-rss-core-20261011.sql.gz；旧源码/root/docker/mediatool/private/source-before-rss-core-20261011。仅回退镜像/代码不自动恢复或覆盖生产数据库。

本次发布后文档使用独立docs-supplement同步，保留原源码包/镜像/manifest，只更新文档、不再重建服务。本项目PT/qBittorrent/Telegram连接、站点定义与订阅当前仍0，没有借用其他项目账号。下一步继续PT请求契约和RSS重试/多站点一致性，同时待用户真实账号完成合法资源搜索→下载→保留做种→CD2→STRM/NFO→Emby→通知、缺集/促销及设备权限/HTTPS/持续稳定性验收。整体目标保持active/未完成。


## 2026-10-11 每轮搜索/订阅快照候选（未发布，迁移21仅用于隔离测试库）

已在真实隔离PostgreSQL复现旧结果串用其他运行的标题/做种/免费/H&R数据，以及多站点RSS失败后重试合并旧条目、重复累加数量的问题。候选实现新增每run公共结果快照及独立加密下载提示，捕获实际HTTP客户端的账号revision/配置指纹；旧记录不以当前数据自动回填。RSS收集失败且尚无决策/下载副作用时清空该run临时集合后重新收集，完整收集后冻结集合；后续重试不再取另一份RSS响应。成功空源也保留来源证明，配置编辑或超过10分钟的快照需新评估。当前10分钟是程序证据窗口，不等于站点促销必然维持10分钟。

下载创建与Worker读取已绑定searchRunId；HTTP/前端明确提交打开弹窗时的快照ID，旧客户端省略时仅找最近的有效快照，不能回退到任意全局资源行。订阅排队在事务内复核revision/启用状态，已有任务不因新搜索自动改来源。新增第9类密文表纳入离线rekey，保留AAD身份。

正式部署仍rss-core/数据库20，本候选尚未发布。发布前必须继续完成已排队任务因过期/站点变化而暂停时的“重新确认来源”专用恢复路径、同种子不同媒体/Hash的重复任务语义、更多并发/中断场景及原生amd64/迁移备份验证。不能将当前暂停机制当作完整恢复功能，也不能使用旧8类rekey工具轮换已升级到21的数据库。真实PT链路/客户端权限/HTTPS/容量门槛不变，整体目标active。


## 2026-10-11 下载来源专用恢复候选（未发布，候选迁移22）

已补待提交下载来源恢复：标记source_recovery_required，公开控制revision；GET来源预览读取本轮PT种子InfoHash和下载器按Hash状态，不向qB提交、不改绑定，不公开签名URL。确认须带预览digest、控制revision及明确许可，保持原媒体/保存路径/下载器；新Hash不同、下载器/路径变动、活动lease、过期快照、停用/修改的订阅不允许恢复。确无原Hash的未提交旧任务采用单独“首次身份确认”，不谎称已证明旧、新种子相同。自动任务复用原订阅规则库，免费/H&R/做种限制不被恢复绕过。

同确认串行/并发重放返回receipt，不重置较新job；同任务advisory锁覆盖Add与来源提交，仅网络预览不持数据库写锁。复用原download.add任务，不新建另一份下载/上传/做种源。通用重试不绕过来源门槛。新增手动任务在创建时已pin种子Hash；重复创建在resource锁内校验媒体/季集/Hash，不再对不同媒体无条件返回已有任务。qB返回错Hash或保存位置不一致不会被当正常成功。

正式部署仍rss-core/数据库20；21/22仅在本地media_tool_test应用，本轮没有SSH或服务器修改。全量race/vet及串行23E2E、Vue9项/构建与18脚本已验证，下一步仍需更宽未知提交/并发/回退矩阵、原生amd64及新schema的备份/密钥轮换验证，然后保护发布。真实PT/qB/通知链路与设备矩阵不因这些fixture通过而完成。整体目标active。


## 2026-10-11 来源恢复发布前验证进展（仍候选）

已验证qB接受种子却返回503后的实际HTTP样本：InfoHash在提交前落库，重试按已知Hash核对已存在任务，无需过期快照取链、无第二次Add。旧accepted确认在再次暂停后只返回receipt，不恢复新一代任务。来源账号revision或同run下载提示在qB读取期间变化的竞争已复现，预览结束重新校验来源且比较取链前后snapshot digest，防止混用旧种子内容/新证明。

隔离库20→22→20→22迁移往返通过：旧搜索链接保留但不伪回填快照，旧任务不自动暂停/重绑，旧resources密文/原task身份保持。仍需双架构镜像/原生amd64测试、新schema克隆恢复+9类rekey+真实部署维护窗迁移与回退核对，完成前正式服务器保持rss-core/20。原STRM/CDN302无转码策略及整体真实业务验收门槛不变。


## 来源恢复发布收尾：2026-10-11

已发布media-tool:source-reconfirm-20261011，schema20→22，仅停止/更新server、worker、transfer-worker、library-worker；账号/主密钥/115授权/媒体身份不改，无PT账户或规则自动导入。全量本地race/vet、Vue9项/构建、18脚本、23E2E；原生x86_64五包30项顶层测试无跳过，使用新内部网络/独立PostgreSQL，非生产库/非真实PT资源。原生跨编译fixture不带race，本地race证据另列。ARM64原生只读无网络配置审计通过，193/159/34/0仍只是静态支持，不提高accountVerified。

补验证qB接受但返回503的未知结果按已落库Hash核对、无第二次Add；旧receipt不恢复新暂停代，来源revision/同run提示在网络读取中变化被拒绝；读取无效task详情后不以空结构覆盖原数据。初始候选不包含最后这一读取失败保护，故保留初始包并生成validated独立源码/镜像/fixture清单；正式切换的是validated版本，不替换或冒充历史证据。

发布前备份已在另一个独立PG恢复，升级22并执行9类密文dry-run回滚，再结构降20/升22验证。第一次SSH克隆派发失败后读取证实无marker/process/目录/容器/网络/卷；第二次执行输出丢失，后续发现服务器已完成的passed回执，采用回执而没有第三次执行。发布亦以持久化receipt核对，不盲目重放。临时资源、含密钥目录与SSH材料在收尾清理。

发布后12项原STRM媒体详情/GET/POST PlaybackInfo逐字一致，四组代表性播放器UA头请求302→cdnfhnfile.115cdn.net→206，HLS/转码拒绝，验证窗口无新增FFmpeg日志；不是实际设备解码或非管理员权限验收。原5份磁盘输出、密码记录/.env/compose、资产绑定/播放授权/扫描计划与Emby/CD2/其他23容器由摘要核对保持。API仍要求管理员登录，首次注册已关闭，密码新设置6–20位。

回退镜像media-tool:before-source-reconfirm-20261011；备份/root/docker/mediatool/private/database-before-source-reconfirm-20261011.sql.gz；旧源码/root/docker/mediatool/private/source-before-source-reconfirm-20261011。发布/原生/克隆/保护证据为data/remote-deploy/source-reconfirm-*报告与validated-release-manifest。文档收尾另用docs-supplement，不修改运行代码或再次重启。

真实PT/qB/外部通知连接与订阅当前仍0；合法资源搜索→下载→做种保留→CD2→STRM/NFO→Emby→通知、真实缺集/促销与设备/权限/HTTPS/容量稳定性仍未完成。下一步继续pre-dessert请求体/分页/框架等更宽契约，以及上述实际验收，整体目标active。


## 2026-10-11 PT请求/分页候选补充（未发布）

已完成定义优先的body/form/default JSON、明确UA、结果数组/文档完整性、声明页码的5页/500项有界跟进、重复去重/partial结果与提示。同run重试不联合旧页，已消费/订阅冻结快照不重写；任务精确证明和9类rekey语义稳定补测。24E2E与375px/桌面提示已核对。本轮无SSH或正式部署修改，无新数据库迁移，正式发布记录仍source-reconfirm/22。

下一步先补更宽分页/混合RSS来源、旧摘要任务兼容与并发/备份原生矩阵，再构建候选并保护发布；不能把当前HTTP/PG样本当真实站点验收。无声明分页的站点仍只取一页、明确partial，零/一基起点/next-link/cursor/专用鉴权与框架待验证。用户自己的PT/qB/外部通知完整业务链路、设备权限/HTTPS/持续容量仍是整体完成门槛。配置库静态158/35/0（193加载）仅更准确暴露此前的required-header限制，不冒称193站点实测。


## 2026-10-11 当前来源一致性推进（待保护发布）

混合搜索失败+成功空RSS证明的旧回归已确认并修复；进一步复现并修复了“决策阶段改空账号/定义仍提交旧RSS基线”的窗口。候选按真实HTTP client证明拒绝同run跨修订合并、保留最早采集时间，并在冻结与最终提交事务中持有来源锁。不完整/异常未来证明明确拒绝，不改变partial/failed搜索警告。迁移仍22，无新schema。

旧任务缺少精确sourceSnapshotDigest的专用人工重确认与未知qB提交已知Hash路径补齐隔离PG回归：新来源不会自动替换；明确批准复用原Task/Job，重复恢复不重复Add；已确认远端存在时不回取过期种子。全量隔离race/vet、Vue9/脚本18/构建及完整24E2E已通过一次。首次E2E扫码等待失败保留并加入fixture诊断，运行授权逻辑未修改，尚未证明最初失败根因或外推真实扫码稳定性。

下一步：完成本候选的双架构镜像/六包原生AMD64验证；重新读取正式账号/任务状态，基于当前source-reconfirm/22做项目专属备份克隆、9类rekey dry-run、保护发布；回归STRM原链接、302到115 CDN且无转码/中转。继续用户自己PT/qBittorrent/外部通知的合法资源全链路、真实订阅/做种保留及真实设备/普通用户/HTTPS/容量门槛。既有115授权和Emby/CD2配置不得重置，参考站点库不自动导入或替代许可确认。整体目标仍未完成。


## 2026-10-11 13:28 当前发布与下一步

PT请求/分页及混合来源证明候选已完成保护发布（`media-tool:pt-mixed-proof-20261011`，schema22无新迁移）。真实x86_64、默认CPU设置、六包83个顶层测试无跳过；隔离备份恢复/九类密文dry-run保持原字节和事实。本地AMD64仿真SIGSEGV在本次实际主机未复现，未据此宣称其根因已确认。

发布后12项Emby元数据保留STRM原链接、四组代表性UA302→115CDN206通过，HLS/转码拒绝，原5份输出/密码与主密钥配置/授权身份/资产绑定、Emby/CD2及其他23容器保持。仅替换server及三个Worker，未自动导入站点、创建账号或来源批准。文档补充与原源码包/镜像manifest分离保留，不再次重启。

当前下一步：
1. 继续不依赖账号的站点规则宽语义、专用鉴权声明/不支持项与容量/部署验证；不能把158/35静态审计变成真实账号兼容承诺。
2. 本项目13:28复核PT账号、qBittorrent、外部通知连接与订阅均0；需要用户用自己的账号配置，才可验收合法小资源搜索→下载→保留做种→CD2上传→STRM/NFO→Emby可见→通知。不得使用其他项目凭据。
3. 随后验证一个预演再启用的实际订阅（缺集/促销/去重/RSS基线），并完成真实设备解码/音轨/字幕/长seek、普通用户权限、HTTPS/NAS双架构/容量稳定性门槛。

既有115授权/Emby/CD2无需重新设置。整体目标仍未完成；fixture、有限Range字节与短期检查不替代上述业务/设备验收。发布、备份/回退和保护记录见data/remote-deploy/pt-mixed-proof-*。


## 2026-10-11 14:xx 新候选：声明请求头与来源边界

已补未发布的PT请求头/重定向协议修复：明确required头可执行，缺值/无效声明在HTTP前失败；origin含scheme/有效端口，跨origin清凭据且不重发私密Body、不降级HTTPS，禁止CookieJar绕过显式账号边界；同origin正常取链保持。全量隔离race/vet、四包PG及24E2E通过，固定库158/35数量不虚增。

下一步继续生产者明确的账号鉴权与更宽语义，不猜sunny Authorization、zhuque CSRF、timezone或专用浏览器规则；完成新候选原生与保护发布。当前正式基线仍pt-mixed-proof/schema22，115授权/STRM不变，真实PT/qB/外部通知及设备/权限/HTTPS/容量门槛未完成，整体目标active。


## 2026-10-11 附加鉴权头候选推进

除上一轮声明头/origin协议修复，本轮补账号到请求的完整配置入口：PT连接的附加鉴权头JSON加密保存，只用于required_headers，留空保留/{}明确清除；源revision和重定向限制保持，绝不猜测自动授权。新增后端严格JSON/HTTP/PG/rekey与Vue校验/遮罩/小屏入口。正式基线仍pt-mixed-proof/schema22，候选在新原生/备份/保护发布通过前不标为已部署。真实PT账号链路、设备权限/HTTPS/容量等门槛不因手工头入口而完成。


本候选补齐“先验证后启用”PT流程：明确的管理连接检查可只读验证未启用账号，但Site业务入口仍拒绝未启用账号，不改变Enabled或账号revision，不创建搜索/下载/订阅。隔离PG正例先复现旧检查失败，再按私有只读构造路径修复；测试验证检查前后业务拒绝和编辑开启后的凭据/来源保护。最终代码再跑全量隔离race/vet，E2E/新原生与候选发布状态以本轮记录为准。之前一次系统检查UI等待超时后读取隔离库，job实际在约2秒内succeeded；这不是证明UI定时/SSE根因已解决。追加只在失败时输出无secret状态/页面可见性诊断，未延长等待或改变Worker行为，之后完整25项通过。


## 2026-10-11 16:21 当前发布与下一步

声明required头/origin保护与PT附加鉴权头入口已完成保护发布（`media-tool:pt-request-headers-20261011`，schema22无新迁移）。真实x86_64默认CPU条件、七包115顶层测试无跳过；七份原始fixture日志单独保存，不借用上一版本83项证明。新备份在独立PG恢复，九类密文表dry-run保持原字节与事实，不轮换生产主密钥/授权。

已发布入口：服务连接→PT站点账号→附加鉴权头JSON，加密保存且只用于该请求required_headers；留空保留/{}清除。可以明确只读验证未启用账号，但不会自动启用或加入搜索/下载/订阅。程序不获取/刷新未知CSRF，不补Bearer，不改静态158/35范围或accountVerified。应经HTTPS管理地址填写；数据库加密不是HTTP传输加密，本次未悄悄改变原用户批准的公网HTTP部署。

发布后12媒体信息原STRM一致、四组代表性UA原URL302→115CDN206通过，HLS/转码拒绝。原5输出、密码与主密钥配置、115授权/资产/绑定/播放Key、Emby/CD2/PG/Redis和其他23容器保持，仅切server及三个Worker。服务器登录/主bundle/服务连接模块资源200且含新入口；这不冒称登录了真实管理员并完成浏览器业务操作。

下一步：
1. 用户用自己的PT账号、qBittorrent与外部通知配置完成合法小资源→下载→保留做种→CD2上传→STRM/NFO→Emby→通知实际验收。16:19正式复核这三类连接、站点定义及订阅仍0；禁止借用其他项目凭据。
2. 预演再启用一个真实订阅，验证缺集/促销/去重/RSS基线和异常来源恢复；手工鉴权头期限/站点版本与自动producer仍需实际账号验证。
3. 完成真实设备解码/音轨/字幕/长seek、非管理员权限、HTTPS/NAS双架构与持续容量矩阵；继续不依赖账号的确定配置语义与部署检验，不制造“193站实测支持”。

整体目标仍未完成。历史候选/未发布段落保留发生时状态，当前以pt-request-headers-*发布/native/备份/保护证据和文档补充SHA为准。已有115/Emby/CD2无需重设。


## 2026-10-11 PT执行契约候选（本地修复，尚未部署）

固定配置的35个partial原因已逐项核对：21个站点含未验证的timezone算术语义、13个专用框架需真实验证、2个缺少明确鉴权生产者、1个声明浏览器执行；分类存在重叠，不相加冒充站点总数。无配置/账号依据时不实现猜测时区、PCRE替代、浏览器挑战或自动Bearer/CSRF。静态范围仍193加载/158解析支持/35部分支持/0无效，accountVerified=false。

本轮先复现并修复另一项确定问题：chrome/disabled及parser声明曾被普通HTTP/HTML-CSS默认路径忽略。新增独立execution_contract模块，chrome=true、disabled=true、非布尔执行标记、未知/非字符串解析器和缺失请求规则在HTTP前拒绝；搜索预检位于账号验证之前。None只用于原始取链/文件字节，不用于搜索/登录/令牌文档字段提取。保留未声明/空字符串的既有CSS缺省和受支持解析器大小写别名。显式RSS URL也不能绕过已声明浏览器/禁用规则；没有rss定义的手工RSS仍可使用。管理员明确检查未启用账号的旧能力不变，这与定义中的disabled请求标记不同。

兼容判定从Repository拆为独立模块，新增排序去重的固定issueCodes；离线审计增加issueDefinitionCounts，按涉及站点数而不是同一站点重复字段数统计。审计不输出原始解析器名、URL、字段名、选择器、请求体、头名/值；筛选器只扫描fields/flat_map_fields及any分支，不再把请求JSON/参数/constant/case/default中的同名filters数据当执行指令。旧jsonb分支顺序缺失的限制仍保留，新增安全原因码不会恢复顺序或自动重导入。

回归包括HTTP前零请求、缺失令牌不能请求站点根路径、浏览器登录不能假阳性、RSS声明保护、字段/普通数据边界、原因统计脱敏。隔离PG搜索回归确认上述失败为failed且不产生空集合的source_revision/definition_fingerprint/observed_at证明，不创建可用资源。schema保持22，无新增迁移；播放器/Emby/115/CD2代码未修改，原STRMURL→按实际UA取115临时链接→302原则不变。

当前本地全量隔离race与vet、Vue14单位测试/构建、Python18脚本测试通过；完整E2E及新的双架构/原生发布证据另行记录，不能复用上一版本的原生115项证明。本轮未建立SSH会话或变更指定生产服务器。当前正式发布仍pt-request-headers-20261011/41ac.../schema22。实际PT/qB/做种/CD2/STRM-NFO/Emby/通知、真实订阅/设备/权限/HTTPS/容量门槛仍待完成。


本候选补充验证：当前完整E2E25项通过，未修改现有E2E用例、超时、重试或期望；未增加UI改动，也不将fixture页面等同真实管理员浏览器登录。全部测试在本地隔离media_tool_test/Redis15顺序执行。第一轮完整Go测试日志各包通过，但外层zsh使用只读status变量导致结果收集命令退出；已保留该日志并原样重新执行完整race及vet，第二轮显式exit=0。没有据包装命令问题改变业务逻辑或放宽测试。


本候选构建/收尾：ARM64与AMD64 Docker镜像已生成，当前源码与打包内容391文件在构建结束逐项一致；源包、两个镜像归档及8个AMD64 CGO0测试二进制校验均通过。ARM64新镜像在UID65532/只读根/无权限提升下启动，loopback随机端口health与login资源200；断网静态审计原因计数与193/158/35/0一致，参考配置不在runtime镜像，任务临时容器/volume已移除。这是缺依赖启动/静态资源检查，不是管理员浏览器登录或依赖全链路。AMD64仅完成构建/交叉编译，物理x86_64的8包原生执行尚待进行。

候选manifest：data/remote-deploy/pt-execution-contract-build-manifest.json；ARM64运行证据：pt-execution-contract-local-image-check.json；本轮综合审计：data/diagnostics/pt-execution-contract-round-audit-20261011.json。没有SSH或生产更改，正式版本仍pt-request-headers/schema22。本段为本地构建后的文档补充，不重写源包/镜像/测试二进制；文档差异单独保存在pt-execution-contract-local-docs-supplement.json，运行代码与打包版本不变。

下一步先执行新的物理AMD64原生8包测试（含离线审计新回归），再重读指定服务器状态、恢复验证新备份及九类密文dry-run、保护发布并回归原STRM链接302到115 CDN与无转码/媒体中转。不得复用旧镜像115项原生证明；真实账号链路、订阅/设备/权限/HTTPS/容量仍保留为整体验收门槛。


## 2026-10-11 18:31 PT执行契约候选已完成指定服务器发布

本轮按用户“部署到服务器测试”执行，正式镜像`media-tool:pt-execution-contract-20261011`，实际ID`sha256:3305cc9054775d72f4c001aea93bafad502ceb0f708984de7fa181caaf90c0e2`。schema仍22，无新迁移；只更新server与三个Worker，既有管理员密码/主密钥配置、115授权身份、2资产/绑定/播放Key、原5个输出文件与Emby/CD2/PG/Redis/migrate保持。新状态/保护证据均为pt-execution-contract-*，不能用上一版本的原生115项代替本次验证。

新源码包391文件与两个架构归档校验通过。指定物理x86_64、默认CPU条件下，fresh内部PG/network/volume、应用只读UID65532，8包128个顶层测试全部通过且无Skip：sites66/integrations15/acquisition17/search14/subscriptions8/secrets4/协议迁移2/离线审计2。8份原始fixture日志独立取回并逐项核验测试名/摘要；CGO0交叉编译后二进制原生执行不是race，本地完整race/vet证据单列。固定站点参考194文件逐字摘要匹配；静态193/158/35/0及accountVerified=false不虚增，不自动导入站点或接受许可。

新生产备份在独立PG恢复成功，候选migrate幂等保持22；九类现有密文默认dry-run可读、committed=false，全部原密文字节及原事实保持，不轮换或导出生产主密钥。回退镜像`media-tool:before-pt-execution-contract-20261011`；旧源码`/root/docker/mediatool/private/source-before-pt-execution-contract-20261011`；备份`/root/docker/mediatool/private/database-before-pt-execution-contract-20261011.sql.gz`，SHA256 `a42ff260d9b21e8e0599e7d3332a9b9fbcf90e19e63c693a754a1398757ec963`。本次原生、克隆与切换都单次派发并记录PID出生时间/持久回执，遇到未知输出只读回查，不盲目重复切换。

切换后12项媒体信息/PlaybackInfo逐项返回原始STRM固定URL，不改为25015/Videos/.../stream；4组代表UA分别原URL302→cdnfhnfile.115cdn.net→206/1024字节Range通过。坏EmbyToken401、HLS/转码409且无新FFmpeg日志。仅限直链/元数据及小范围字节验证，不等于实际播放器解码、全部音轨字幕/长seek或非管理员验收。公网管理ready/login及Emby网关公开信息200；管理接口仍需登录，注册关闭，新密码6–20位。服务资源含原加密附加头入口，未绕过登录或读取已存头值；没有改UI或宣称实际管理员浏览器操作。

初始上传窗口其他容器ID摘要发生变化，部署保护先暂停：只读元数据发现其他项目duanju/juku在18:05:46重建，早于本次18:15原生Docker测试开始，任务此前仅上传文件，未操作该项目。触发方/原因未确定，历史Docker事件不足以恢复全过程，原观察及差异保留。MediaTool全部保护项前后一直相同；随后两次切换前清单稳定，切换后24个非本项目容器ID与切换前一致。不能写成“整个上传窗口所有其他容器ID从未变化”。相关证据为unrelated-change/created-observation、unrelated-drift-reconciliation及precutover/after。

18:30正式读取PT账号/qBittorrent/外部Telegram/站点定义/订阅仍0，3Worker在线、1个已授权115身份。真实合法PT→qB→保留做种→CD2→STRM/NFO→Emby→外部通知、真实订阅、设备/非管理员权限/HTTPS/NAS容量门槛仍未完成。现有115/Emby/CD2配置无需重设。当前批准的公网HTTP不是传输加密；未悄悄改访问范围或开启中转/转码。整体项目未宣布验收完成。


## 2026-10-11 文件/媒体管理六项更新已发布

指定服务器当前正式镜像`media-tool:files-management-20261011`，ID`sha256:1391e89d6c90a6b83fb900e2e0da9083d42fb8429042fa9f62fbfc6f81ff6ce2`，数据库22→23。六项用户更新均已接通：115/CD2影视自定义命名/模板/分类移动及侧车、独立服务配置路由、115与明确本地根目录管理/回收恢复、程序与Emby媒体库浏览、115无Token保存待授权连接、显式会话/CSRF/no-store密钥回显。具体入口、界限和操作方式见FILE_MANAGEMENT.md。程序不会自动重命名用户现有影视，必须选择文件预览并确认；无永久删除、无视频中转或转码。

本轮最终全量隔离race/vet、Vue17项、Python18项及完整28E2E通过；桌面/375px截图已实际核对，手机文件操作改为卡片，预览取消没有文件副作用。新物理x86_64默认CPU、只读UID65532、fresh内部PG+独立Redis15、无测试端口发布，13包207个顶层测试全部通过无Skip：sites66/integrations16/acquisition17/search14/subscriptions8/secrets4/协议迁移2/离线审计2/filemanager8/catalog1/library25/clouddrive2 16/httpapi28。library的既有显式fixture exporter只在该测试容器的空/owned-fixtures tmpfs运行，未排除用例；13份原始日志独立核验。初始两次隔离环境在migrate连接检查失败、未开始测试包，已清理且失败记录保留；第三次使用TCP和认证SQL就绪检测后通过，未改变应用镜像/二进制或放宽测试，不将旧Unix就绪报告当成真实网络连接证明。

新生产备份在fresh PG恢复，22→23/idempotent23及九类密文dry-run通过，迁移及dry-run全部原字节保持；旧镜像的九类密文只读工具也能读取23，未自动降库/恢复生产库。回退镜像`media-tool:before-files-management-20261011`，旧源码`/root/docker/mediatool/private/source-before-files-management-20261011`，备份`/root/docker/mediatool/private/database-before-files-management-20261011.sql.gz`（SHA256 `5bca7248a7d2d634a4b6e9b7aa3d59488ccc17143a691791491cce2f816fec71`）。新目录/文件操作记录有下迁移门槛，不在仍有本地连接或未核对操作时破坏性回退。

仅切server与三个Worker；管理账号密码、主密钥/配置、115授权身份、资产/绑定/播放Key/计划与原5输出字节保持。24个其他容器的ID从本轮最初清单到切换后一致。Emby/CD2/PG/Redis/Emby-loopback保持，migrate一次性容器本轮前后均不存在，不能凭旧发布记录假定存在。12项媒体信息/PlaybackInfo为STRM原链接，四组代表UA原URL302→cdnfhnfile.115cdn.net→206/1KiB通过，坏Token401、HLS/转码409且无新FFmpeg日志。

公网新页面与对应服务bundle200且含实际功能入口；匿名文件/媒体库/回显API401，注册保持关闭、密码6–20位。服务静态探测初版错误依赖会被压缩的JS变量名，改为稳定功能文案/路径后通过，未修改运行代码；同样未绕过真实管理员登录或声称真实影视批量整理。目录权限、客户端解码/全部字幕音轨、完整PT/qB/通知业务与HTTPS/NAS容量验收仍是整体门槛，不凭207个fixture宣布全项目完成。
