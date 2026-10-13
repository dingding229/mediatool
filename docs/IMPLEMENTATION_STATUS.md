> 当前管理员改用首次注册的账号密码登录，旧启动Key不再生效。操作与恢复见ADMIN_ACCOUNTS.md。

# 当前实现与验证状态

更新：2026-10-13。源码版本0.16.0-dev，数据库迁移37；本轮插件/核心功能调整见PLUGIN_DEVELOPMENT.md及plans/core-sandbox-sites-20261012.md，前轮说明见WORKBENCH_EXPANSION.md。发布以GitHub Actions与服务器实际镜像/报告为准，历史段落不代替当前证据。

主要业务已实现，本轮继续完成不依赖真实账号的可靠性与管理功能。**测试服务通过不等于真实115/PT/Emby客户端通过；整体项目未宣布验收完成。** 历史过程保留在 WORK_LOG.md，本文件只记录当前状态。

## 本轮0.16 资源获取工作台

实际SSD副标题、CHDBITS/UBITS默认查询分页、已知优惠/账号级屏蔽、明确空做种统计与TTG活动标记适配；多选四类筛选、模板网格/列表、独立TMDB/站点缓存、媒体信息卡片和资源获取统一UI。说明ACQUIRE_WORKSPACE.md，计划plans/pt-acquire-20261013.md。旧快照依解析指纹失效需重新搜索。真实非零做种完整性、站点挑战和播放器解码不因合成样本而宣称全部可用。

## 本轮0.15 搜索/订阅交互

表头远程排序、来源规范显示、站点动作/历史状态对齐；订阅TMDB查询布局和缓存提示；删除体积/做种人数限制、系统统一间隔、正则关键词。说明SUBSCRIPTION_SEARCH_UI.md，计划plans/search-layout-20261013.md。schema37只授权转换订阅限制/间隔，保护其余业务字段、账号和STRM。

## 本轮0.14.1 图标/全选/定时刷新

- 六站真实公开ICO内置Go模块并由同源接口返回，避免浏览器跨域失败；v1.1.1。
- 模板全选/取消全选/半选仅作用于尚未加载项，仍显式提交加载。
- 持久化站点数据定时刷新，默认60分钟、15分钟～7天/可关闭；复用单app调度器/Worker，每账号独立持久任务。
- 手动/定时同站并发锁、15秒网络超时、revision/模板指纹保护、失败保留历史快照及日志；前端仅30秒读快照。
- 当前说明见SITE_METRIC_REFRESH.md；发布以Actions/服务器回执为准。

## 本轮0.14 模板选择与资源搜索

- 模板目录与加载状态分离，schema35仅保留已有账号依赖的模板，后续管理员勾选加载；重启不会全量加载。旧定义/账号不删除。
- 六模板v1.1.0、favicon/官网链接、多行Go共享规则；模板快照摘要对比上一版未改变解析语义。
- 搜索仅来源/分辨率/编码/站点筛选，关键字与完整快照做种人数/文件大小升降序排序；下载身份不变。
- 已添加站点数按全部PT账号计数；Passkey独立字段完整换行与复制、移除原始JSON区块。
- 计划见plans/pt-select-search-20261013.md；发布和实际站点验收仍依据回执，不以fixture代替生产。

## 历史0.13 Go站点与管理页（后续已改为选择加载）

- 六站改为真正Go模板构造函数和类型，固定Go模块版本构建，启动全部自动登记，不动态执行远程代码。
- 独立模板页只展示英文名，隐藏来源/提交/导入选择/已导入列表；PT站点页去掉重复连接块。
- 模板下拉且内置名称/域名、必需字段、UA获取、RSS实际地址发现和XML校验、原值编辑回显；重复站点前后端禁止。
- 种站点数按权威正做种站点计数，不累加种子数量；schema34唯一索引不自动删除旧重复账号。
- 新功能和验证边界见PT_GO_TEMPLATES.md，正式发布以本轮Actions/服务器回执为准。

## 本轮0.12原生模板仓库切换

- 六个模板已上传mediatool-site；主程序内置唯一仓库目录、固定快照/摘要校验/原子同步。
- 原生templateVersion=1、catalogVersion=1，删除原有配置继承与旧仓库导入；旧模板拒绝执行，不提供回退兼容。
- schema33记录source_repository，后台显示旧来源不支持并禁用旧来源恢复；新增site-template-sync CLI。
- 原模板文件从主仓库移除，仅保留公共冻结testdata用于回归；当前使用说明见NATIVE_SITE_TEMPLATES.md。
- 本轮发布后在明确用户授权下导入原生6模板，不删账号或订阅；验证状态以Actions/服务器回执为准，真实五站点账号仍未验收。

## 本轮0.11.2站点模板更新

- 新增CHDBITS（ptchdbits）、SSD、OURBITS、UBITS、TTG自包含精简模板及索引；只保留4个核心请求，说明见examples/sites/README.md。
- SSD/UBITS明确汇总型做种统计，校验唯一容器、用户/路径、完整数量/体积及异常/零语义；不强套列表或伪填0。
- 搜索声明结果容器以拒绝登录/挑战伪空结果；TTG使用独立search_field/c=M及实际下载路径，纠正统计XPath残留html selection。
- 促销缺失保持未知，H&R不假填固定天数，不截断两位数字；解析契约指纹更新为v4，不复用旧证据。
- 新站点本轮只有合成页面回归，无真实账号实测；不自动替换生产定义或账号，不声称后台已有任意JSON上传。数据库仍32，无新迁移。发布以本轮Actions/服务器回执为准。

## 本轮0.11.1更新

- 0day识别、电影/剧集命名模板保留为主程序基本功能，从插件列表移除。
- 插件仅接受ZIP中实际可执行的WASM代码；删除Webhook/rules/手填Manifest，不提供旧插件迁移或兼容层。
- WASI入口、导入、内存、超时及输出限制补测；保留命令/API、页面/仪表盘、事件、Cron、实例配置/状态/分身等执行能力。
- HDSKY独立精简模板与合成页面回归，仅保留4个业务请求；不自动覆盖生产账号/快照，不声称支持后台任意JSON上传。
- schema32增加WASM-only写入约束；恢复验证fixture改为真实WASM代码。发布/服务器验证状态以本轮回执为准。

## 本轮0.11更新

- HDSKY实测明确空做种响应与有界分页；站点搜索按已声明/经响应确认的下一页链接获取，不推测页码。
- 订阅结构化分辨率/来源/编码，与搜索共用解析，未知不匹配；预演保存逐项解释，旧订阅默认不限。
- 功能型Go/WASM插件框架：市场仓库、ZIP升级、设置、启停生命周期、命令/API、页面/沙箱UI、仪表盘、定时服务、事件、分身、加密数据与日志；不兼容Python包、不复制GPL实现。
- 最新用户要求：管理员原始日志/数据库展示不脱敏；完整请求和302 URL可查看，公网匿名拒绝。仅公开源码/无秘密验证回执发布，原始生产日志不外传。
- 增加备份快照导出、临时恢复库防护、逐表摘要对比、密文可读与dry-run及二次迁移幂等核验。真实回执以本轮发布报告为准。

## 工作台0.10更新

- 日志“简约模式”；PostgreSQL/Redis同款只读脱敏控制台。
- TMDB自动电影/剧集搜索、0day查询线索、整个资源快照筛选后分页。
- PT上传/下载/分享率/等级/魔力/做种统计及最近成功快照，不完整数据明确标记。
- ZIP声明式/HTTPS插件包：安装/升级、加密设置、授权启停、测试、历史、卸载；不执行任意代码。
- PWA公开静态资源缓存、离线说明及安装/更新提示；公网安装需要有效HTTPS入口。
- 本轮本地32项前端单元、49项隔离E2E、全Go race、vet、18项Python检查通过。
  TMDB/站点展示的fixture测试不是所有真实站点或客户端完整验收；不宣称1.0完成。

## 已实现

| 领域 | 当前能力 |
|---|---|
| 基础 | Go/Vue、PostgreSQL任务租约与Outbox、Redis可重建缓存、三个Worker、Docker部署 |
| 服务配置 | AES-GCM凭据加密、修订保护、脱敏、审计、数据库配置优先、停用不偷偷回退旧Emby环境配置 |
| PT/下载 | 固定配置快照、NexusPHP/M-Team核心规则、多站搜索、TMDB确认、qBittorrent、v1/v2/hybrid解析、共享文件读取及做种保留 |
| 订阅 | 电影/剧集、预演、定时搜索/RSS、首批基线、种子清单季集覆盖、重复下载抑制 |
| CloudDrive2 | 官方gRPC、远程上传、持久化会话、哈希与远端校验、暂停/恢复/取消/核对、显式会话绑定 |
| 上传可靠性 | RPC与操作员共用PG锁，Worker控制修订与租约检查；确认字节区间去重，哈希进度独立；恢复前再次校验本地源内容 |
| 云盘索引 | 独立流式扫描、两次完整扫描确认缺失、移动跟踪、分页、最多20项明确身份批量确认、手工身份/季集修正 |
| 扫描调度 | 持久化计划，默认停用，指定子目录周期对账；不把部分/失败扫描当成完整缺失证据 |
| 媒体输出 | STRM与NFO意图先加密写入数据库，再落盘；摘要/归属保护、重启恢复与冲突重试入口；共享tvshow.nfo以剧集身份归属 |
| 官方115 | 自己AppID的PKCE扫码授权、加密Token原子刷新/终态停重试、实际播放器UA取链、direct-only/1KiB同UA验证；CD2扫描上传不变 |
| 授权恢复 | 长轮询无数据库锁，兑换前持久claim/清verifier、多进程不重复兑换；显式取消/编辑/凭据版本变化拒绝旧结果 |
| 播放 | 固定assetId+文件Key，按实际UA动态取链；指定部署严格302、禁止媒体中转/转码，Range/HEAD、缓存隔离、凭据撤销/轮换 |
| Emby | 独立根网关、用户权限校验、托管origin/资产判断、指定部署元数据返回STRM原链接和对应文件Key，禁止转码/封装；固定EMBY_PUBLIC_URL避免信任伪造转发头 |
| 通知 | 站内/Telegram、明确拒绝/限流/未知结果、修订冻结、人工风险确认重发 |
| 运维 | 离线主密钥九类加密表原子轮换/默认dry-run（包含115授权及search-run命名空间AAD）、连接旧版本优雅退休、备份临时库恢复验证、验收脚本读取有效数据库配置 |

## 验证边界

- Go、真实隔离PostgreSQL/Redis、自有gRPC/HTTP提供方fixture、Vue与Chrome测试分别记录。
- 上传绑定还需真实CloudDrive2版本确认；列表key并非在所有版本中都可当作upload_id。没有远端控制确认时拒绝绑定。
- 周期子目录对账不是CloudDrive2 PushMessage实时增量消费。
- 身份修正保持既有输出位置；涉及不同剧集的共享NFO归属冲突时拒绝，不替换其他集的身份。
- readiness并发检查不等于视频代理吞吐、跨网络CDN播放或生产容量承诺。
- amd64在当前arm64宿主机上的运行属于模拟运行，不等同物理x86 NAS验证。
- 真实服务账号、播放器解码、电视/iOS等客户端兼容验收仍未完成。

本轮测试命令与结果见 OFFLINE_HARDENING.md、WORK_LOG.md。

## 2026-10-09：实际客户端播放兼容状态

已完成：stream/original授权分流、相对DirectStream/Transcoding URL、Sen原生UA提示、UA/IP隔离缓存保持、固定deliveryReason诊断和安全日志。已对照用户提供的embyExternalUrl源码；详见REFERENCE_EMBY_EXTERNAL_URL.md及ADR0005。本节为2026-10-09的CD2阶段快照；2026-10-10已启用自己的115官方授权取链，不复制AList/私有115预览分支。

已验证：兼容UA native原始流302/CDN206、服务器外网络正确UA206/默认Sen UA403、代理非零Range、真实Chrome首播/快进/暂停和一次恢复。尚未完成：Sen实际采用新header后零服务器媒体流量，以及约8分30秒位置的反复冷恢复（出现HLS分片404/FFmpeg0帧）。不能把现有有限字节/一次出画面报告当成全部播放验收。

## 115官方取链路线

已新增open115播放适配器、自己的AppID的PKCE扫码授权、加密Token原子刷新/并发串行和终态停重试。CD2继续扫描上传，官方取链读取实际播放器UA；官方启用时禁止静默服务器代理，1KiB CDN验证后302。迁移16及Vue入口已部署，现有配置/账号不变。

Go/race/vet、真实隔离PG协议/凭据测试、前端构建及12项隔离浏览器E2E通过（扫码E2E为明确的fixture，不冒充真实115授权）。2026-10-09部署时官方连接启用数为0；该历史前置条件已被2026-10-10的用户真实授权完成。配置步骤见OFFICIAL_115_PLAYBACK.md。

## 2026-10-10：真实授权与可靠性补齐

- 用户已完成自己的115授权，官方连接启用且ready。item11/source mediasource_11在默认SenPlayer/6.2.1 UA及普通测试UA下均网关302到cdnfhnfile.115cdn.net，同UA CDN206/1024字节；1MiB/256MiB偏移通过，错误Emby Token401。操作者Mac网络直接CDN206样本亦通过。没有重建STRM或替换账号/密钥。
- 这些证据证明官方取链/授权网关/客户端网络字节，不证明真实Sen解码、全部拖动/音轨/字幕。网页约8分30秒的重复HLS冷恢复仍有404/FFmpeg0帧，未宣布修复。
- 新增源码迁移17：一次性兑换先持久化exchanging并清verifier；扫码轮询与兑换均不持有播放锁，完成时重新验证会话、连接修订、管理员凭据版本；中断/未知结果不会自动重放旧device code。取消和新二维码不会被旧成功结果覆盖。
- rekey补齐open115_authorizations及open115-auth命名空间AAD；终态/兑换中的空payload保留，任一坏密文导致全表回滚。未对业务库轮换主密钥。
- 测试入口统一为scripts/test-go.sh，显式选择media_tool_test、Redis DB15、test-only迁移、-p 1。避免.env.test只设置TEST_DATABASE_URL而普通迁移仍沿用开发库DATABASE_URL。

当前全部需求与未通过门槛见FIRST_ROUND_ACCEPTANCE.md，不能仅凭115官方302成功把整个项目标为完成。

部署补充：2026-10-10远端已迁移17、server/三Worker就绪；默认Sen UA真实用户网关original/Static302、CDN206/1024及非零Range复测通过，保留原官方会话和媒体，其他23容器ID摘要未变。最新迁移17备份在新建临时库逻辑恢复/事实一致并清理通过；恢复副本密文解密未验证。详情见REMOTE_TEST_DEPLOYMENT.md。

## 自有文件上传与网页复测（2026-10-10）

49,982字节自有5秒视频经真实CD2远程上传，会话持久保存、云大小/SHA-1一致，受控Range206读取整文件SHA-256一致，原文件保留；不创建虚假PT/下载/影视身份记录，也不宣称已测常规上传吞吐。维护工具cmd/cloud-upload-smoke限制自有小文件/配置根/固定UUID目录，持久journal、未知启动不重建、恢复必须显式确认。

实际Chrome仍出现HLS network/空缓冲；相同参数服务端master/main/首TS200且FFmpeg有帧，故网页仍未通过，但不能把所有新失败都归为0帧。增加保留流式/WS writer接口的安全访问metrics，记录status/responseBytes/duration，不记录查询/凭据/请求体。

发布补充：上述维护工具/安全访问指标及标准流中止修复均已部署，仍迁移17。真实自有文件通过云SHA-1/整文件SHA-256验证及同一session再核验；最终默认Sen UA的网关302/CDN206、错误Token401通过，资产/授权/其他23容器保持。网页初始502/404仍待定位，之后的DOM缓冲和时间前进不在Mac锁屏状态下被冒认为完整画面验收。


## HLS冷启动与取消语义（2026-10-10）

已部署固定SHA已知Emby4.10.1.0网页模块的HLS首字节60秒策略，片段GET/HEAD上游75秒、普通API30秒、共享池；不改能力/重试/总预算/许可或未知版本。实际JS（application/x-javascript）GET22,264字节/no-store/policy头已通过。已取消请求按request_cancelled中止，不误记为已送达502；首TS10秒断开后同会话约0.938秒200，45秒请求约10.573秒200。

正确510秒timeline片段85.ts仍上游500、FFmpeg实际-ss但无帧，旧Chrome本轮未观察到重新获取该JS且仍10秒networkError；不得标冷恢复或实际网页新策略已验收。详情见HLS_STARTUP_RELIABILITY.md。全量Go race/vet、12项E2E和9项Python/Node测试通过，PG仍17/账号媒体保持。


## 2026-10-10：最新302复核与HLS诊断范围纠偏

Sen原生UA的original/Static再次验证302→cdnfhnfile.115cdn.net、CDN206/1KiB，错误Token401无Location；真实客户端解码仍未验收。此次明确使用管理员登录会话，不能代表非管理员权限。旧HLS工具仅检查播放许可，误将管理员会话标作普通用户；已改为默认拒绝管理员、显式管理员诊断且role准确标注，不补建账号或使用其他项目凭据。历史报告不覆盖；此前ordinaryUserScopedSourceVerified=true及“同URI就是同编码”的说法不再用于验收。

独立FFmpeg seek/null读取约15.402秒成功；直接Emby也出现一次85.ts 200与一次500，网关仍有500，网页冷恢复保持未通过，不能假定只是代理超时、固定15秒上限或只需创建目录。工具增加限定loopback的直接上游对照、总会话查找预算、禁止5xx立即重试与连接关闭回归。本轮仅诊断脚本/测试/文档变更，15项Python/Node回归通过，没有新的运行镜像/迁移。具体证据与下一步见HLS_STARTUP_RELIABILITY.md。


## 2026-10-10：PT配置兼容本地推进

新增请求体/数组/跨字段依赖/分组资源、有序条件的jsonb规范化、整批事务导入、明确同提交恢复与Vue恢复操作；未知促销和H&R不推断为免费/无约束。固定上游193站点静态加载，159核心规则支持/34部分支持，所有accountVerified=false；时间偏移和其他限制如实报告。此阶段未改远端运行版本，真实PT/qBittorrent/通知与网页客户端门槛仍开放。见PT_CONFIG_COMPATIBILITY.md及WORK_LOG本轮收尾结果。


## 2026-10-10：PT兼容发布与实际镜像验证

上节“本地推进”已完成发布：最终镜像在真实amd64服务器和本地原生arm64的只读/无网络/非root审计通过；本地amd64模拟异常未修复、未当成运行通过。Go/race/vet、Vue7项单测/构建、最终13项E2E与15项脚本测试通过。指定项目仅更新Media Tool与三个Worker，无新迁移/自动PT配置；用户媒体、115授权、Emby/CD2及23个其他容器保留。官方302/CDN206/错误Token401再验证通过（管理员会话字节证据，非客户端解码）。回退与证据见PT_CONFIG_COMPATIBILITY.md；真实PT、订阅和网页冷续播仍未验收。


## 2026-10-10：网页启动缓存修复已发布，播放仍有限通过

固定SHA的index/apploader新增独立缓存键，不伪造Emby4.10.1.0版本；真实Chrome已加载改写的3100/22264字节脚本。实际HLS构造配置离线核验首字节60秒/总加载120秒/原全局默认不变。网页时间轴约8分30秒有一次真实出画面，随后恢复仍有网络/暂停/黑屏/重新加载，**不是稳定冷恢复或全部客户端通过**。独立无重试的510秒仍15.124秒上游500，异常为FfRunException/StartFfMpeg；不误推目录/固定15秒预算根因。

Go/race/vet、13E2E和17脚本回归通过；真实非root只读无网络amd64镜像验证后仅更新本程序和三个Worker，无迁移/账号/授权/原媒体修改，其他23容器保持。原生302→115CDN206/错误Token401再次保持；真实PT/订阅/通知与持续网页/客户端矩阵门槛继续开放。完整证据/回退见HLS_STARTUP_RELIABILITY.md本轮追加。


## 2026-10-10 V2启动保护已发布：首播有限通过，seek保持未通过

严格原版本hash/唯一锚点的90秒启动guard仅让HLS已明确选择、仍有次数的非fatal瞬时重试继续；权限/fatal/取消/耗尽立即终止，不手动重试或改换session。35状态场景、原回调函数离线、真实SDK字段/自建XHR、全量Go/race/vet、18脚本和13E2E通过。实际镜像验证后更新本程序/3Worker，无迁移或账号/授权/媒体修改，其他23容器保留。

Chrome按标准入口加载revision2；首播真实经历两次500/guard等待后实际影片画面与currentTime35→71→158秒连续播放，UI暂停/恢复有效。拖动510秒失败，退出后恢复四次有限等待仍约90秒终止，**不是seek或全部客户端矩阵通过**。PT业务/订阅/通知、非管理员、音轨/字幕/正常设备矩阵继续开放。精确证据与回退见HLS_STARTUP_RELIABILITY.md V2节。

## 2026-10-10 当前用户策略：严格直链，不再自动转码

用户明确不需要转码，指定项目已启用Emby连接directPlayOnly。旧HLS启动/缓存修复仅在兼容模式保留，不作为当前验收目标。严格模式移除托管来源的转码/封装地址，媒体请求默认拒绝除已鉴权原始/Static以外的上游输出，STRM亦拒绝代理；不伪造客户端能力。item17的磁盘STRM与上游Path一致，网关显示不同入口为了Emby鉴权/隐藏Key，实际三个入口同302临时目标/115 CDN206验证通过；HLS/转码请求409。详见DIRECT_PLAY_ONLY.md，播放器仍需支持原始格式，非管理员与真实设备解码矩阵没有凭采样自动通过。

## 2026-10-10 后续明确需求：媒体信息直接使用STRM链接

用户要求取消25015/Videos/.../stream替代地址。当前严格模式因此直接返回STRM中固定的25014解析地址，不把115临时URL写入元数据，不预绑定客户端UA，也不追加Emby Token。文件级Key向有权读取该媒体元数据的用户显示（用户明确选择），不再将“Key不出现”作为本模式验收；115凭据、Cookie与管理密钥仍不暴露。转码/重新封装与媒体中转拒绝策略继续。发布和item11/item17路径一致性以original-strm-*记录为准。


## 2026-10-10 变化触发扫描候选

源码迁移18补PushMessage适配、10秒合并的PG目录dirty、原子扫描/Job/dirty消费、已有扫描后的补扫、连接修订/范围变更和关闭保护、只读能力诊断CLI及Vue默认关闭的变化开关。当前粒度为已配置子目录，仍保留周期扫描及两次完整缺失投票。不自动改用户扫描计划，不改STRM原链接/禁止转码策略。发布与真实事件能力以cloud-events-*记录为准，真实PT业务/通知和客户端门槛保留。


## 2026-10-10 输出迁移候选

迁移19扩展现有journal为write/archive，明确预览与planDigest确认，Movie↔Series及跨剧集/季集变更可生成新输出并归档旧托管文件，不变asset/播放Key/云端/做种文件；实际新文件校验与原子no-replace、rename-before-commit恢复，共享NFO/目录不自动清理。字幕完整清单新增idx/sub配对/PGS等，整批事务提交。隔离测试通过不等于已有用户媒体已迁移；默认不触发，更多单集provider metadata/多集NFO与实际设备仍待验。详见OUTPUT_MIGRATION.md及output-migration-*。


## 2026-10-10 单集metadata候选

迁移19不变，TMDB显式季集验证与单集标题/简介/播出日/时长/ID，未知值不捏造，写资产/NFO并保持共享整剧metadata隔离。新输出或明确迁移使用连续范围/稀疏E链而非只写首集；不自动变已有文件。管理GET只读预览与旧响应隔离补齐。最终真实provider/发布/原STRM策略证据见episode-metadata-*，不外推非连续文件在所有Emby设备识别。


## 2026-10-10 输出归档管理与明确清理

源码migration20新增归档清单、默认30天/修订保护保留期、1–20项只读预览、持久化明确批准、隔离阶段/幂等查询/人工重试与保留审计；无自动删除Worker。原媒体/做种源/当前输出/共享NFO不删除，待恢复或当前输出缺失时拒绝清理。详见ARCHIVE_CLEANUP.md；真实发布以本阶段部署/验收报告为准，不在生产自动创建或清理示例归档。STRM原链接/115实际UA取链/不转码策略保持不变。


## 2026-10-11 固定STRM取链诊断与预算

已新增连接复用、可配置30秒默认总取链预算（5秒–1分钟）、缓存排队/取消边界、504超时与程序生成requestId关联的固定阶段/数字日志；固定入口写期限有限而不是被管理30秒写期限夹断。无新迁移、不改变STRM原链接/播放器UA/不转码不relay；本阶段已在指定项目发布，八组原STRM冷/热HTTP请求及额外两次复读通过302/CDN206，分阶段日志和正常Token刷新/缓存失效已实测。长期稳定性与真实设备矩阵未全验，见PLAYBACK_RESOLUTION.md。


## 2026-10-11 非连续合集完整Emby投影

实际同镜像Emby 4.10.1.0消费矩阵发现E01E03误扩成1–3，新输出改为每个明确集号独立索引、同asset/原固定URI，NFO明确整体文件播放。单集/连续范围不变，不用代表首集漏掉声明集。确认/下载入库/明确迁移复用投影，别名字幕整组边界、URI一致合并、旧链无迁移拒绝、Emby逐路径确认与UI提示已实现；不自动改现有文件，不转码/relay。源码保持20，本阶段是否已发布以emby-projection-*报告为准。


## 2026-10-11 跨版本原输出核对恢复

既有输出不匹配时先暂停，不自动叠加新布局；新GET预览/POST证明批准/专用import-only Job完成原布局恢复，原定资产/URI/做种源保留。源字幕与NFO/固定URL/文件级授权共同proof，旧批准/修订不能执行；后台候选恢复不误确认旧稀疏链。无需新迁移，是否已发布以output-recovery-*为准，详见OUTPUT_IMPORT_RECOVERY.md。


## 2026-10-11 RSS核心兼容发布状态

已实现RSS错误源/基线保护、已核对的路径种子识别与加密下载提示、Atom URI/文本和RSS-only过期资源元数据隔离。全量本地回归、原生amd64的32项顶层测试及独立备份恢复通过后，已发布rss-core；不把这些受控样本写成用户真实PT账号验收。没有新迁移或自动接受pre-dessert许可证/创建站点账号。实际PT账号和完整业务/设备矩阵仍需验收。范围、证据和下一步见PT_CONFIG_COMPATIBILITY.md、REMAINING_PLAN.md。


### 历史RSS候选构建收尾（2026-10-11，发布前记录）

已完成linux/arm64与linux/amd64候选Docker构建；原生aarch64 Docker运行配置审计通过（UID65532、只读根/只读参考卷、无网络，193/159/34/0、accountVerified=false）。首次SSD bind挂载被Colima拒绝，保留失败日志；改为复制到本轮独立参考卷并只读挂载，未改原参考文件权限，临时容器/卷已删除。此处不是原生amd64运行、RSS实际账号或NAS设备验收。

准备了linux/amd64的sites/search/subscriptions三份独立测试二进制及校验摘要，下一步在指定x86_64服务器的临时网络/独立PostgreSQL库运行，再进行受保护发布。源码/镜像/测试manifest见data/remote-deploy/rss-core-candidate-manifest.json；诊断见data/diagnostics/rss-round-audit-20261011.json。镜像构建后的补充只涉及文档状态和一个负例XML转义修正（避免跨域URL负例先在XML语法处失败），运行代码未变；修正后的RSS回归单独通过并重新交叉构建sites测试二进制。初始源码包/构建证据保留，最终当前源码与构建源码差异由manifest明确列出，不谎称两者全部文件逐字相同。当前未创建SSH身份、未连接或更改正式部署，原STRM/不转码策略保持。


## RSS发布收尾：2026-10-11

media-tool:rss-core-20261011已发布至指定项目，仅更新server/worker/transfer-worker/library-worker，无生产迁移、PG20不变。原生x86_64测试使用全新内部网络与独立PostgreSQL容器/卷，sites/search/subscriptions共32项顶层测试（28/1/3）全部通过，无跳过；未访问生产库或使用真实PT身份。镜像配置加载审计193/159/34/0通过，静态accountVerified仍false。32项跨编译测试不带race，本地全量race证据另行记录，不混为同一次验证。

发布前备份在另一全新PostgreSQL容器恢复，并对8类密文表执行离线dry-run/回滚检查；无生产改密或主密钥轮换。所有临时容器/卷/网络和含密钥的临时目录已清理。发布后12项原STRM媒体详情及GET/POST PlaybackInfo逐字一致，四组原链接/UA请求302→cdnfhnfile.115cdn.net→206，HLS/转码仍拒绝；不声称实际设备解码或非管理员矩阵通过。密码记录/.env/compose、资产/绑定/输出/播放授权/扫描计划、5份磁盘输出和Emby/CD2/其他23容器由前后摘要核对保持。

证据：data/remote-deploy/rss-{native-pg-check,backup-restore-check,deployment,preservation-check,original-strm-check}.json与三份已校验native日志。回退镜像media-tool:before-rss-core-20261011；数据库备份/root/docker/mediatool/private/database-before-rss-core-20261011.sql.gz；旧源码/root/docker/mediatool/private/source-before-rss-core-20261011。仅回退镜像/代码不自动恢复或覆盖生产数据库。

本次发布后文档使用独立docs-supplement同步，保留原源码包/镜像/manifest，只更新文档、不再重建服务。本项目PT/qBittorrent/Telegram连接、站点定义与订阅当前仍0，没有借用其他项目账号。下一步继续PT请求契约和RSS重试/多站点一致性，同时待用户真实账号完成合法资源搜索→下载→保留做种→CD2→STRM/NFO→Emby→通知、缺集/促销及设备权限/HTTPS/持续稳定性验收。整体目标保持active/未完成。


## 2026-10-11 当前发布：source-reconfirm / 数据库22

每run搜索快照、来源revision/定义指纹与独立加密提示，多站点RSS收集/决策冻结，专用下载来源预览/明确确认、重复任务媒体/Hash/季集门槛现已发布。正式部署image为media-tool:source-reconfirm-20261011，PG从20升级22；21/22不再只是本地候选。历史“仍rss-core/PG20/候选”段落描述此前阶段，以本节和source-reconfirm发布证据为当前状态。保留原STRM固定链接、实际请求UA取链和302/no-transcode策略。


## 来源恢复发布收尾：2026-10-11

已发布media-tool:source-reconfirm-20261011，schema20→22，仅停止/更新server、worker、transfer-worker、library-worker；账号/主密钥/115授权/媒体身份不改，无PT账户或规则自动导入。全量本地race/vet、Vue9项/构建、18脚本、23E2E；原生x86_64五包30项顶层测试无跳过，使用新内部网络/独立PostgreSQL，非生产库/非真实PT资源。原生跨编译fixture不带race，本地race证据另列。ARM64原生只读无网络配置审计通过，193/159/34/0仍只是静态支持，不提高accountVerified。

补验证qB接受但返回503的未知结果按已落库Hash核对、无第二次Add；旧receipt不恢复新暂停代，来源revision/同run提示在网络读取中变化被拒绝；读取无效task详情后不以空结构覆盖原数据。初始候选不包含最后这一读取失败保护，故保留初始包并生成validated独立源码/镜像/fixture清单；正式切换的是validated版本，不替换或冒充历史证据。

发布前备份已在另一个独立PG恢复，升级22并执行9类密文dry-run回滚，再结构降20/升22验证。第一次SSH克隆派发失败后读取证实无marker/process/目录/容器/网络/卷；第二次执行输出丢失，后续发现服务器已完成的passed回执，采用回执而没有第三次执行。发布亦以持久化receipt核对，不盲目重放。临时资源、含密钥目录与SSH材料在收尾清理。

发布后12项原STRM媒体详情/GET/POST PlaybackInfo逐字一致，四组代表性播放器UA头请求302→cdnfhnfile.115cdn.net→206，HLS/转码拒绝，验证窗口无新增FFmpeg日志；不是实际设备解码或非管理员权限验收。原5份磁盘输出、密码记录/.env/compose、资产绑定/播放授权/扫描计划与Emby/CD2/其他23容器由摘要核对保持。API仍要求管理员登录，首次注册已关闭，密码新设置6–20位。

回退镜像media-tool:before-source-reconfirm-20261011；备份/root/docker/mediatool/private/database-before-source-reconfirm-20261011.sql.gz；旧源码/root/docker/mediatool/private/source-before-source-reconfirm-20261011。发布/原生/克隆/保护证据为data/remote-deploy/source-reconfirm-*报告与validated-release-manifest。文档收尾另用docs-supplement，不修改运行代码或再次重启。

真实PT/qB/外部通知连接与订阅当前仍0；合法资源搜索→下载→做种保留→CD2→STRM/NFO→Emby→通知、真实缺集/促销与设备/权限/HTTPS/容量稳定性仍未完成。下一步继续pre-dessert请求体/分页/框架等更宽契约，以及上述实际验收，整体目标active。


### 2026-10-11 混合来源证明候选状态

已补本地实现：按记录的成功采集证明（非仅搜索结果状态）校验空RSS；拒绝同run不同来源修订/定义合并，保留最早观察时间；冻结与基线提交事务再校验并保留来源锁；对旧摘要缺失任务和已知qB身份恢复补回归。全量隔离race/vet、Vue9、脚本18、构建及一次完整24E2E通过；扫码测试最初超时证据仍保留，未以最终一次通过宣称根因或真实业务验收。

此候选没有新增迁移，当前正式已发布证据仍source-reconfirm/PG22。候选双架构/原生/生产备份保护切换未完成前不得标为已部署；全链路和真实设备权限验收也未完成。STRM原链接、客户端UA取链、无转码/中转的已发布用户要求不变。


### 2026-10-11 已发布来源一致性加固

请求体/分页及混合空RSS证明、提交事务校验、旧来源摘要任务的显式恢复已部署；schema仍22。真实x86_64六包83项无跳过、本地race/vet、Vue9/脚本18/完整24E2E、备份克隆/九类密文dry-run及发布后原STRM/302/拒绝转码回归证据分别保留。原密码/主密钥配置、115授权、5份输出、Emby/CD2/其他23容器保持。当前正式PT/qBittorrent/外部通知与订阅均0，真实全链路、设备/非管理员/HTTPS/持续容量门槛未完成；整体项目未完成验收。


### 2026-10-11 PT附加鉴权头入口候选

新增操作员显式输入的加密requestHeaders JSON，仅用于当前规则required_headers，不填入公开settings、不自动刷新或猜测授权，不转发到其他origin。普通编辑留空保留、{}明确清除，账号修订/换endpoint保护、九类密文rekey不变。后端严格重复键/类型/边界、Manager到HTTP/PG证据与Vue掩码/局部错误/长表单有界滚动和375px可视控件已补回归。静态站点审计158/35仍不代表账号验证。此为未发布新候选，生产基线仍pt-mixed-proof/22，原STRM播放规则保持。


### 2026-10-11 16:21 PT附加鉴权头及origin修复已发布

pt-request-headers正式版本、schema22无迁移。加密JSON凭据入口、明确定义required头、literal/字节/重复键校验、origin/重定向/Body/TLS保护、未启用账号显式只读验证已部署。真实AMD64七包115顶层无Skip，之前ARM64启动/静态审计及本地full race/vet、14前端单位/18脚本/25E2E分别记录。生产备份独立恢复/九类密文dry-run保持，原账号/主密钥/授权、5输出及受保护容器保持。发布后12原STRM元数据、四组代表性UA302/CDN206与拒绝转码检查通过。

静态193/158/35/0和accountVerified=false不变，未自动创建站点/账号/批准或借用凭据。当前正式PT/qB/外部通知/订阅仍0，真实完整资源链路、设备/权限/HTTPS/容量门槛未完成；整体项目未完成验收。历史E2E等待/网络失败根因未确认，最终一次完整通过不等于长期稳定性。


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
