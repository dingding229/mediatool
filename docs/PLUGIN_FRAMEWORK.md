# 内置插件框架 V2（F1）

## 来源、发布与管理员选择

唯一代码来源是 `github.com/dingding229/mediatool-plugin`，具体 pin 以 `go.mod` 和下述清单为准。
主程序和前端资源清单使用同一个 pin；
`internal/modules/plugins/v2/library.json` 是构建期生成、CI 校验的只读清单。
插件与 Go SDK 编译进 app，镜像不增加 Python 或 Go 工具链。

新增/修改插件：插件仓库 vet/test/race/golden → 推送插件仓库 → 主程序 bump pin →
初始化副作用及资源清单门禁 → Actions → Docker Hub 不可变镜像 → 标准备份部署。
**F1 仅发布构建，不部署；框架部署留 R-F。** 不存在上传、远程市场、外部仓库 URL、
单插件运行期升级、运行期源码编译或下载，不在 UI 暴露源码/提交/版本选择器。

目录查询不调用工厂，不创建实例或目录。管理员安装前确认当前 fingerprint 的所有权限；
安装默认停用。支持启停、分身、加密 JSON 配置、按实例查看 KV 与日志、保留数据卸载。
卸载是 tombstone，不删除私有数据；再次安装新建实例。分身重新加密配置/KV，不共享目录。
管理 API 仅管理端口、管理员会话+CSRF，写操作带 operationKey、CAS revision 和审计。

## 生命周期、升级与授权

`plugin_v2_definitions/instances/configs/kv/grants/operations` 是基础存储。
旧六表归档为 `plugin_legacy_wasm_*`，不再执行或分发；归档密文加入主密钥轮换，
保留原 `plugin:<id>` / `plugin-data:<id>` AAD。迁移62不修改非插件业务表。
有 V2 实例时 Down 明确拒绝有损降级，应恢复备份。

启动 SelfCheck 检查 SDK contractVersion=2、pin 和清单一致。Refresh 只刷新已选定义：
无变化不写；元数据更新换 fingerprint，旧 grant_revision 失效；执行面/资源/权限变化
保守地停用待管理员重新确认。移除的定义标记 missing，配置/KV/data 保留。
审批未完成不会因重复同步被复位。锁与 revision CAS 保证并发刷新/操作的一致性。
Host 每次从数据库读取最新授权，不跨调用缓存。读用 KEY SHARE，不互相串行或阻塞续租；
写的配额/CAS由实例专用 advisory lock串行，提交前条件 UPDATE重新校验generation、
grant_revision、lease和指纹，撤权竞争失败则回滚副作用。已提交完成的调用不追溯撤销。

## 执行边界与残余风险

每个启用实例同 binary `plugin-worker`，配置/日志/数据库初始化之前早分流，
JSON-RPC 2.0 长度帧双向stdio。固定白名单env、CLOEXEC、Pdeathsig、独立进程组；
整树RSS/CPU监测、rlimit、owner lease/fence、退避重启、整组kill/reap。
worker与app同UID65532；父进程最早期dumpable=0阻止同UID读父environ/mem/ptrace。
pprof CPU采样保持可用；core dump被有意关闭。不引入setuid、UID池、helper或seccomp，
不要求放宽生产cap_drop ALL/no-new-privileges。

Landlock用已有x/sys尽力自限制，ABI不支持发warning继续。CGO/race不支持
AllThreadsSyscall时仅为降级测试；CI另用CGO_ENABLED=0、原生Linux AMD64、
非root、cap-drop ALL/no-new-privileges执行内核及真实同binary worker门禁。

这是**受信代码缺陷隔离，不是恶意代码沙箱**：同UID仍可能读取app可读的文件
（如文件形式主密钥）；Landlock不可用时不能保证实例间文件隔离，由维护者代码审核兜底。
同页ESM亦是trusted UI，不是iframe沙箱。密钥/连接/115授权不进入worker环境。
管理员原始业务日志按既有政策可见，但不能公开提交到Git/Actions工件。

## F1 管理 API

| 方法/路径（均以 `/api/v1` 开头） | 行为 |
|---|---|
| GET /plugins/catalog | 只读编译目录；pageSize≤200 |
| GET /plugins/resources | 同pin的只读资源/摘要清单 |
| GET /plugins | 分页实例，pageSize≤200 |
| GET /plugins/{instance}?after=key | 配置及KV（每页≤50，单值≤128KiB） |
| POST /plugins/operations | install/enable/disable/clone/configure/review/remove；幂等+CAS+审计 |
| GET /logs/query?module=plugins&instance=UUID | 有界日志窗口，instance精确过滤 |

F2 已实现函数绑定、plugin.invoke、API/服务/命令/事件、JSON树和本地ESM执行，详见下文 β 记录。
F3业务Host、F4事件全覆盖、F5库管理完善及F6验收插件未实现，不能宣称MP功能已对等。
详见 `plans/20261017-plugin-system-v2.md`。

## F2-α：已接线的扩展基础设施（主程序待 F2-β 统一提交）

SDK 以 Go 函数绑定 API/Service/Command/Action/Module。运行时在 init 后冻结，
`plugin.describe` 返回 descriptorVersion=2 快照；handler ID 来自 kind+逻辑 ID，
不是函数地址或作者填写的 handler 字符串。宿主再次校验 manifest、实时授权、
API path/cmd/service 冲突并按 priority/handler 稳定排序。Module 执行及链式事件由下文 F2-β 接通。

管理监听器动态路由 `/api/v1/plugin/{instance}/{path...}` 支持精确静态 path，不接受
通配或参数模板。admin 的 GET 也须管理员会话，写方法另须 CSRF。apikey 使用
X-API-Key；bearer 使用 Authorization: Bearer。**采用实例专属 key，不扩权既有
external_api_keys**：256bit 随机值仅创建时返回，库中只存摘要，绑定实例、route:METHOD、
定义 fingerprint、grant revision 与到期时间；配置换代/启停后旧 key 失效。
public 须声明并确认 api.public（写另需 api.public.write）；每实例每分钟最多60次。
凭据只能在头中传递，Cookie/Authorization/X-API-Key 均不进入 worker。

API 请求/响应体分别≤64KiB，头≤16KiB，查询≤4KiB/32键，调用总预算5秒；
读写网络 deadline 有界。结果只允许明确状态码与 Content-Type/Retry-After 头，拒绝
Set-Cookie、Location、CORS、HTML、连接控制等。响应不经普通管理 JSON envelope 二次包裹。

迁移63增加插件专属版本快照、服务调度、事件 journal/subscription、加密调用信箱、
手动幂等请求、key及限流表；仅为 files.* 在共享 outbox 增加一个局部索引。
调用参数与结果沿用主密钥/AAD加密，并加入 rekey；不会新增原文凭据存储。

服务支持 cron（Asia/Shanghai）、interval、date、once；scheduleSetting 可绑定配置
顶级字段：false/缺失关闭自动调度，true沿用声明值，cron字符串/interval秒数可覆盖。
手动触发独立于自动调度开关，仍需实例启用与管理员确认。同服务待执行/在途只有一个，
misfire coalesce 到最近一次，不补跑历史积压。jobs 只负责幂等唤醒，插件错误不启动
第二套业务重试。进程内取得 RPC 执行额度后才领调用 lease（35秒，执行最多30秒）。

files.succeeded/failed 在原业务事务内 EmitTx，bridge 独立消费，不修改通知消费者游标。
插件只收到 operationId/state 的白名单 DTO，不收到原始错误/路径。journal cursor 在
桥接事务锁下分配，不以共享 outbox 的时间高水位跳过迟提交事务。订阅保存独立生效
cursor/source_since；配置/授权换代重建边界，普通 worker 重启保留边界。
每实例最多16个在途/待执行调用，journal最多10万条，达到上限明确报忙并保留源事实。

调用断连、超时、失租记 unknown，不能盲目重放。事件按实例/授权修订/事件/handler
去重，回放按 cursor、最多200条；只有明确确认 unknown 才能把未知记录重置 pending。
运行历史分页≤200。内嵌 UI 已增加服务/命令/动作确认提交和状态历史，不把202当成功。
Telegram 入站仍留 F4；JSON/ESM、多key与侧栏见下文 F2-β。

服务 due/once 记录、调用信箱和 jobs 唤醒在同一事务提交，避免进程恰在两者之间崩溃丢失
一次性任务。服务 pump 只在对应 jobs 唤醒成功后才领调用 lease，遵循已有队列暂停策略。
unknown 会阻止同授权修订的该服务继续自动调度，避免非幂等副作用被盲目重做；管理员可
明确手动执行，或重新启停/保存配置建立新授权修订。API 未声明 Content-Type 时固定为
application/json，不允许浏览器自动嗅探为HTML。插件查询参数里的凭据名会在私有访问日志
脱敏；其他业务原始URL日志政策不变。

API执行时，宿主在该worker单lane执行窗内安装descriptor.RequiredScopes上限，所有反向
Host调用再与实时实例授权相交；不能从参数伪造或提升该上限。hello为每条路由显式声明
最小Host读/写scope。原生Go主动延迟到调用窗外、改用背景context或直接执行文件/网络等
副作用仍属于已注明的受信代码审核边界，不能将此机制称作恶意代码沙箱。


## F2-β：链式/模块与真实接线、界面（2026-10-17）

- `NameRecognize` 在工作台识别和 library.Recognize 的事务前调用，输入只含 basename 与
  `NameHint{title,year,mediaType,season,episodes}`，不含路径、凭据或媒体身份。
  实时读取当前 generation/grant 的注册表；priority 降序，instance/handler 稳定排序。
  最多8个 handler，单 handler 200ms（含排队），整条链与module合计≤2秒。
  严格 schema 检查后才采纳补充/改写；Continue=false 截断并使用合法提示。
  `events.chain.name.recognize`、`module.metadata.name_recognize` 必须已确认。
  模块优先级取第一个成功的覆盖；失败/超时/非法结果跳过并回到内置识别。无权限缓存。
  识别只提供提示，既有索引证据、TMDB身份确认与写文件规则不变；有上下文递归保护。
- `DownloadAdded` 接实际 acquisition.Add → transition(download.started) 的业务事务。
  事件的一次身份是 outbox UUID，而不是可复用的 taskId。状态UPDATE fence防止同一重试重复
  发射，同任务未来发生新事实会有新UUID。EmitTx 的第二个参数是一次事实ID：files.*用
  operationId正确；不得改为库/目录这类长期聚合ID。插件只收到 taskId/state，不能取得磁链、
  下载地址、路径或原始错误。TransferComplete/Failed 与通知消费者保持独立游标。
- 命令具备全局冲突检查、内部 ExecuteCommand 入口与管理页面二次确认手动执行；内部调用方
  负责认证/审计。Telegram入站没有伪造接通，仍属F4。
- 管理端 `GET /plugins/{id}/ui/{form|page|dashboard|meta}?key=...` 与
  `GET /plugins/ui-surfaces` 需要管理员会话；媒体端口不注册。只允许声明的 UI provider，
  调用仍走 generation/fence 的加密信箱；树≤256节点/12层、单prop≤8KiB。
  JSON白名单：Text、VRow/VCol/VCard/VCardTitle/VCardText、VTextField/VTextarea、
  VSwitch/VCheckbox/VSelect、VAlert/VChip/VDivider/VBtn。VBtn不会执行原始事件字符串，
  动作使用已授权手动入口。平面model、when{model,equals}不用eval，不透传HTML/URL/on*；
  未知组件安全占位。表单最终通过已有 configure CAS/CSRF/审计写配置。
- page、form、dashboard三类provider可运行。DashboardMeta 的key必须声明，≤16项，
  refresh=15..86400秒、cols=1..12，border/title生效；timer和组件卸载时清理。
  侧栏链接是实例UUID+声明page，不是插件源码路径；只展示启用且运行的实例。
- ESM从同一个Go pin在构建期导出到插件ID命名空间，Vite将其与宿主作为一个共享chunk图
  构建。固定 `@mediatool/plugin-ui` facade暴露同一Vue、白名单组件与API client。
  不做运行时字符串重写/eval，不加载第二份Vue，不接受远程UI。构建门禁拒绝远程/任意bare
  import、跨插件目录导入、动态import表达式。原始资源摘要清单与前后端pin保持一致。
  **同页ESM是管理员确认的受信代码，不是沙箱，可能访问当前管理员页面能力**；不可声称
  具备恶意插件隔离。Go worker环境/资源边界与D8规则不放宽。
- API继续只支持精确静态路径；`{param}` 单段模板（无通配）明确留后续，不影响F2验收。
- 未增加依赖/扩展；Schema63尚未发布，在该迁移补齐chain/module/ui CHECK及download.started
  outbox局部索引，依旧执行Up→Down→Up与非插件数据不变检查。

F2仅完成框架和三条真实触发点，F3业务Host、F4全面事件/Telegram入站、F5完善与F6验收
插件仍待执行。F2发布镜像但不部署，R-F才访问自有服务器。


Linux race 集成测试由test-go.sh先构建CGO_ENABLED=0的真实app worker，宿主仍开-race；
不把race测试执行器充当生产worker（它与生产AS/RSS画像并非同一种程序）。独立的非root、
cap-drop/no-new-privileges静态内核门禁仍完整保留，不扩大任何生产资源限额。启动失败仅记
stage/常量错误类、RSS超限记数值，不记录参数、载荷或原始stderr。


原生 CI 进一步确认：RLIMIT_NPROC=64按真实UID累计线程，共享runner UID测试前已有51线程，
叠加race宿主会触发worker创建线程失败。CI只对v2/httpapi/acquisition三个插件集成测试执行器
使用专用UID65532、cap-drop ALL/no-new-privileges（宿主仍开race）；其余包与独立静态内核门禁
不变。此adapter只存在于测试脚本，不进入镜像，不是生产setuid launcher，不新增UID池或helper。
R-F2审核后撤销此旧NPROC方案：共享真实UID限制不具备实例语义，且生产宿主UID65532
已有约90线程（app约12），不能依赖CI专用UID掩盖。worker不再设置RLIMIT_NPROC，
保留其他rlimit/隔离；RSS watchdog同次200ms快照累计整树num_threads（包含leader），
每worker默认256，PLUGIN_MAX_TASKS可配置32–4096，超限立即SIGKILL进程组，
收割后沿用崩溃退避。日志reason=tasks_exceeded，DB使用既有resource_limit类别。
采样无法完全阻止瞬时fork风暴；生产先测app稳态tasks，再只为app增加pids_limit: 2048
作为容器级硬兜底。除最终image digest和该一行，不改私有Compose其他设置。

## R-F：无识别 hook 的热路径

NameRecognize 的 chain/module 注册表合并为一次有界查询。仅缓存“当前没有识别 hook”的
负结论（2秒TTL、每进程最多32个DB pool槽）；不缓存描述符、权限或调用结果。实例操作、
refresh、descriptor发布、Host配置换代、进入running/续租时清除负结论。epoch防止在途旧查询
在失效之后回填；查询失败不记作“无hook”。同进程启停/授权变更立即生效，其他进程的变更
最多2秒后重新查询；实际调用仍逐次执行实时授权和generation/fence检查。零插件整库识别
不再逐条执行两次SQL；正结果也只执行一次注册表查询。200ms/handler、2秒总预算不变。


## F3-A 获取链 Host API

公开 typed 客户端：插件仓库 `sdk/host/downloads`、`host/search`、`host/sites`、`host/subscriptions`；
共享 DTO、scope/级别与 unsupported 列表在 `host/business.Methods`，核心/UI从同一个pin校验。
安装确认不会隐式获得未声明权限，危险权限醒目标记。完整方法表、业务入口和限制见
`docs/plans/20261017-plugin-system-v2.md` 的 F3-A 实施记录。

每次调用查当前 grant/generation；业务写在原事务提交前 fence。网络动作不持有授权锁，
operationKey 收据加密保留七天，未知结果不自动重放。`.host/` KV为宿主保留、不交给worker或clone。
站点只返回受控状态/marker，不回传认证header、Set-Cookie、HTML或带passkey RSS；
foreign种子只读；deleteFiles=false 仅退休已导入且已清除HR义务的本程序种子，true 显式进入原HR/所有权/云端文件校验审批，不开放任意删除。
下载 retry/recheck、原始站点HTML、未知手动提交的重复风险覆盖、追剧完成明确unsupported。
F3-A不部署；F3-B/C/D尚未开始。


## F3-B：媒体库和文件链

typed Host 客户端新增 library/files/storage/strm/metadata/classification；完整 RPC/scope/级别/原生
入口见设计文档 F3-B 实施表，权限页和 capabilities 继续从同一 pin 的共享方法表生成。
文件写入必须预览→检查相对目标/digest/revision→显式确认；storage mkdir/relocate 只生成预览。
不支持裸复制/上传/Resolve/STRM正文改写，安全文本读取尚为 unsupported。
rootIds/storageIds 是文件/存储 scope 的交集选择器；库历史/资产的未实现 selector fail closed，
不会放大为全库。保持原生更小批次上限与外部结果 unknown/reconcile 语义。

## F3-C：媒体服务器与播放链

新增 typed mediaserver/catalog/mediausers/playback/mediaprobe 客户端及原生详情扩展，仍为
contractVersion=2。完整方法/scope/级别见设计文档 F3-C 表。危险标记从同 pin 的方法表生成。

Host 由实时 grants/generation 创建 PluginPrincipal；原生/Emby 只提供安全管理目录，不接受
客户端 userId/visibleLibraries 作为权限。无真实成员主体的 Home/Discover/NextUp/UserData
明确 unsupported。Emby 无法证明归属的 scoped lookup 也不放大到管理员全局读取。

插件不能获取或替换 Resolve、Playback/StreamURLs、播放/115 URL、Session 激活、原始
Image/Playback HTTP 客户端、Emby handler、凭据或用户认证流程。用户写入仅 patch 既有记录，
撤销设备保留同事务用户→设备锁、原生 revision/归属校验与 live fence。播放/访问只给安全摘要，
不提供 IP/UA/原始日志或策略写入口。探测仅入队，GC 复用既有有界服务，无新增 ffmpeg 路径。

## F3-D：平台域

Typed cache/integrations/jobs/maintenance/network/notifications/settings/system 已接入；详细方法、
scope、危险级别和限制见设计文档 F3-D 表。settings 仅4个 allowlist 键；jobs 仅本实例有证据的
未执行手动服务 wake；缓存仅明确 tmdb/search ID；通知复用 outbox 且只能查/重试本实例。
HTTP 固定 host 代理 profile，精确 public origin，network 与 network.proxy.use 都实时要求，
不返回正文/响应头/代理秘密、不跟随跳转。maintenance 无备份恢复/发布能力，system 无进程/环境。
hello-go 1.3 的手动 F3 只读验收把 boolean 结果写入自身 KV；扩权须管理员重新确认。


### F4-A：获取链策略与提交事实

六种账号/指标/订阅事实只在事务提交后进入独立游标的事件 journal；投递前与执行时均实时检查
`events.subscribe.<type>` 授权。SDK typed payload 位于 `sdk/contracts/acquisition.go`。
`events.chain.resource.selection` 只重排/筛选已过滤的 ≤200 个 ID 窗口；
`events.chain.resource.download` 只能拒绝或关闭 post-import 清理，不能提供 URL、路径或绕过 HR/源快照。
两者均 prepare→200ms handler / 2s 整链→schema 校验→提交 fence，不在写事务中等 RPC。
降级仅用于纯策略结果，绝不执行插件外部下载后再重放内置写操作。hello-go 1.4.0 默认 no-op。
本组新增 scope 需重新确认；没有变更 STRM/302、媒体认证、下载来源/所有权保护或生产资源限制。


### F4-B：整理、字幕和删除的事实

整理失败只取终态；重试/未知/待核对不广播失败。字幕以 owned journal 的实际复制和摘要证据为准，
没有字幕不发事件。隐藏历史不等于删文件；下载删除只有 provider/文件读回确认后才与业务状态原子入 outbox，
false 删除绝不发文件删除事件。typed DTO 位于 SDK `contracts/organizing.go`，数据均经白名单投影。

五条 `events.chain.*` 策略只运行在 prepare：transfer.rename/build、intercept、overwrite.check、storage.operation。
重命名只改变相对输出路径，不能改正文/源身份/播放授权；root、归属、布局、碰撞、季集和摘要由宿主最终检查。
既有输出必须走明确迁移，不能通过 hook 追加新布局。intercept/overwrite 只能拒绝/跳过；storage 只选已批准 handle，
当前入口不扩大原 connection/root 的授权集合。hello-go 1.5.0 默认全部 no-op，新 scope 必须重新确认。

## F4-C：元数据、覆盖观测与附加来源

- `metadata.scraped`：`library.saveScrapeReport`（含公开的
  `PersistScrapeReport`）和 `nativetmdb.Engine.Process` ready 更新在同一短事务写
  versioned outbox。Enrich/TMDB HTTP 阶段不发成功事件；ready 更新仍带 started-at fence。
  事件只含对象 ID、类别、TMDB identity 和事实版本，不含刮削报告、图片/播放 URL/token。
- `CoverageObserver.ExpectedEpisodes/Check/Observe`：按订阅配置的 season 观测**整个季**，
  不以 rules.episodes 子集或 completeCollection 缩小期待范围；复用 TMDB NativeDetails
  的完整 Episodes（保留未播/无日期条目，不能用下载用的已播集过滤结果）。可暴露证据
  复用 ExposableAssetsSQL：ready binding、present 云文件以及已有 .strm 输出缺一不可。
  无权威清单、非剧集、空/超 200 集、TMDB 不可用均为 unknown；部分覆盖不发完成事件。
  这是当前清单的覆盖事实，不声称整部剧已完结，不修改 enabled/last_state/coverage 等业务列。
- `subscriptions.completion`（`subscriptions.read`）现在返回 typed CoverageObservation，
  读调用重新推导并做授权 fence，不写 outbox、不伪造 MP 状态。后台每轮最多 10 个订阅，
  Redis 专属 `plugins:v2:coverage:*` 观测 TTL 10 分钟、游标 1 小时、lease 50 秒；
  事务内复核 subscription revision 和资产证据后才写完成事实，回滚不发。
- `subscribe.episodes.refresh` 只能在完整期待清单上补充，不得删集/添加 owned；
  `subscribe.completion.check` 只能否决，不能越过宿主实物证据。两者使用独立精确 scope。
- `media.recognize.convert` / `module.metadata.identify` 接 Recognize/Suggest：
  只可选原始 TMDB 搜索结果中存在的 (mediaType, id)，不传文件证据或图片，不自动确认。
  chain + module 共用 2 秒预算，各最多 8 handlers、单 handler 200ms；读返回前复核 fence。
- `SourceRegistry.Refresh` 在后台获取 `discover.source` / `recommend.source` typed 候选，
  共用 2 秒刷新预算；每种最多 8 providers，每 provider 本轮最多 200 条，TTL 2 分钟。
  宿主目前没有内置推荐引擎，绝不把插件结果说成内置推荐。仅返回 identity/title/year，
  来源显式标记为 `plugin:<instanceId>`，不返回图片/媒体/播放 URL。
- Home 仅附加已缓存的 `pluginSources`（非空才输出）；另有管理员只读
  `GET /api/v1/media/library/sources?kind=discover.source|recommend.source&page=1&limit=20`。
  size≤200，page≤1000，逐页读取缓存，绝不同步 invoke、fetch 或懒刷新。`truncated=true`
  表示 provider 超出本轮 200 条快照，`hasMore` 只描述当前缓存可翻页的剩余条目。
  Home 两种来源合计只给 100ms 缓存读取预算（超时丢弃附加来源、不影响内置首页），
  单独分页读取预算 200ms。零插件转换链使用 2 秒 TTL 的负向存在提示，
  启停/发布/授权/换代使 epoch 失效；从不缓存正向授权。
  缓存绑定 exact snapshot；读取前后均检查实时 descriptor/grants/lease，撤权/换代立即隐藏。
- 精确映射与测试索引：`testdata/contracts/plugin-v2-f4c-event-map.json`。
  不新增迁移/依赖、不部署；Telegram/其他 F4-D 能力不属于本组。

## F4-D：系统事实、通知与 Telegram 入站

- `notification.queued` 在 Dispatcher.Prepare 的业务事务中记录，仅表示消息进入现有通知队列，
  不表示送达。payload 仅 notificationId/origin；插件来源的 queued 事实保留在 journal，
  不再次投递给插件通知订阅，避免 post_message 自激或跨插件通知循环。原出站 Telegram
  token/chat/重试/未知结果人工确认流程不变。本组不声称提供 delivered 广播。
- `system.operation.failed` 来自终态 failed（非 retry/paused）的 jobs 完成/租约耗尽，以及
  safeerrors.Bus 的显式安全报告。总线只接 source/code 枚举，不采集日志、异常或堆栈；
  plugin.* 任务、plugins 来源及带插件 actor 的报告被排除。按分钟聚合，窗口关闭后最多
  每批 200 条写 versioned outbox；封闭桶防迟到写丢失，保留一天后分批清理。
- `settings.changed` 只给 area/revision/changed-key diff，无任何值。应用 allowlist 为
  cleanupAfterImport/searchCacheSeconds/tmdbCacheSeconds/subscriptionCron/scraping；
  native 只给 serverName/enabled 的变化键。access、可信代理、域名安全字段、账号、模板路径
  不导出。插件配置使用独立 `plugin.config.changed`，只含 instanceId/revision，不含配置。
- 精确 scope：events.subscribe.notification.queued / system.operation.failed / settings.changed /
  plugin.config.changed / messaging.inbound / messaging.action.received（均带 events.subscribe. 前缀）。
  入站事件只投递给实际命令目标实例，Deliver 与 Replay 都实时检查 scope/代际。

### Telegram CommandIngress

选择 **long-poll**：不引入公网匿名回调、反向代理或 webhook secret 部署变更；不自动删除
已有 Telegram webhook。使用现有 Telegram 集成；管理 API 保存 settings 时携带原 revision，
保留原 settings 并合并 `inboundEnabled: true`、`allowedChatIds: "10,-100123"`、
`allowedUserIds: "20,21"`。默认 false；两个 allowlist 任一为空或没有启用的集成时，
不发 getUpdates/任何 Telegram 请求。仅管理员可通过原有集成设置接口配置，不能通过插件配置。

- HTTPS 主机固定 api.telegram.org、不跟随重定向；15s long-poll、20s HTTP 上限，单批 10 个
  update，响应最多 1MiB；失败有退避，日志只固定原因，不输出 token URL/provider body。
- chat 与 user 必须同时匹配；忽略 bot、过旧消息、未注册命令及超过 200 字符的消息。
  当前只接受明确的 `/hello` 形式，不猜测 `/cmd@other_bot` 或执行任意消息/Shell。
  参数去 URL/凭据样式及已知 bot token；插件只收到 telegram/opaque actor/受限参数。
- 入站按连接全局 60/min、actor 5/min、actor 状态最多 200。bot 凭据指纹隔离偏移；
  同一短事务消费 update_id、一次性 nonce、速率计数，排入当前 descriptor 的命令 mailbox，
  写 targeted messaging.inbound/action.received，再提交偏移。重启不会重放已提交命令；
  回滚不前移偏移、不消费 nonce、不发事件。事件含收到/排队语义，不冒充命令已执行。
- poll offset/lease 由 PG 持久化，40s lease，RPC/long-poll 不跨写事务。超过六天无更新允许
  从 offset=0 重新取仅服务器仍保留的更新，处理 Telegram 长空闲后的 update_id 重置。
- `POST /api/v1/messaging/command-actions` 仅管理监听器，管理员会话+CSRF；输入
  chatId/userId/command/arguments，返回 5 分钟有效、不超过 64 bytes 的 callbackData。
  可供宿主 inline keyboard 使用，不是插件 Host API，也不会自动发送消息。
  签名绑定连接与 nonce/期限，记录再绑定 opaque actor、配置 revision、实例及 snapshot。
  nonce 一次消费，过期/篡改/换配置/换代/跨发送者均拒绝；全局最多 200 个有效 nonce。
  callback ACK 失败不回滚或重新执行已提交命令。
- 不新增迁移：只使用 app_settings 的 plugins.ingress.* 插件框架专属键；不修改其他设置。
  状态不保存 bot token、raw chat/user、原始消息。旧 nonce 分批最多 200 清理。

### 通知与媒体服务器副作用模块

`module.notifications.channel`（后台总预算 10s）和 `module.mediaserver.refresh`（30s）接入
Dispatcher.Deliver、library.Refresh、acquisition.RefreshEmby；只给 requestId/objectId/kind，
不传宿主 token、chat、云盘路径、图片/播放 URL。按优先级选择当前已授权 provider。

使用 **纯 offer → execute** 两阶段协议：只有 offer 阶段的明确 not_started 才可使用内置
发送方；offer 返回 ready 后持久化 started，再发 execute。execute 只接受 sent/requested/
unknown，not_started 在 execute 阶段非法；任何派发错误/超时/非法回执均 unknown，绝不
再调用第二个发送方。offer 是受信插件的纯协商契约，不能执行网络/文件副作用；与其他
受信代码约束一样由 SDK/维护者审核保障，不把同 UID Go 进程伪称恶意代码沙箱。

plugins.effects.* 持久收据绑定 request hash；相同 requestId 重试不重发，换参数拒绝。
内置媒体刷新回落也先记 builtin_started，再请求远端：若远端成功而本地后续写入失败，
重试只继续确认，不再次扫库；未知进入现有 operator-required/通知 outcome_unknown 流程，
需读回核对，不能自动重试或把 requested 当 visible/已送达。无 provider 时内置路径保持。

### F4-0 最小宿主

管理监听器的`POST /api/v1/plugins/workflows`提供save/approve/run/status/cancel（均admin+CSRF+
confirm），沿用持久jobs与实例动作收据。批准绑定DAG/动作权限快照，步骤≤32，深度≤16，串行，
取消与unknown不自动重放。`workflow.started`与受理事务同提交，BeforeStep在事务外等待后fence。
不新增迁移；框架状态只在plugins.* namespace。现有插件页运行历史可查看动作步骤。

身份broker仅框架，真实provider UI/会话签发明确unsupported。本地密码登录原样保留。
预绑定管理员账号、精确origin、一次性5min state/nonce、issuer/audience/过期和实时授权都由宿主检查；
插件isAdmin没有效力。禁用/撤权/换代后的BrokerIdentity立即无效，不能用于绕过内置认证。

### F5-α 内置目录与权限确认

目录只展示当前镜像定义的元数据、版本记录、声明权限、安装数量；图标从同pin的资源摘要清单加载，
无分发或版本选择入口。实例显示可用状态，缺失定义仍保留名称、配置、KV与数据。
管理员通过`GET /api/v1/plugins/{id}/review`读取一致的确认预览：已批准版本、scope新增/移除及
资源范围变化。危险权限以文字和危险标签提示；历史基线未知时明确说明，不推测旧版本。
确认使用原operations的CSRF/CAS/operationKey/审计；成功后保持停用，必须显式启用。
本组无迁移；非秘密批准摘要使用既有plugins.*框架KV命名空间，真正的数据快照恢复留F5-β。
来源后台刷新尊重120秒新鲜度：空结果退避封顶900秒，失败封顶300秒；配置、状态或授权变化立即
令旧snapshot不可见，新generation忽略旧负缓存。首页不等待worker，200ms/2s生产预算不变。

### F5-β 实例数据快照

管理员在实例「数据快照」中创建、列出、删除或恢复，写需二次确认和CSRF/CAS。
创建同样要求完全停用，以避免运行中目录不一致；恢复只允许本实例且当前定义可用，配置/数据schema
必须精确相等。未知schema返回不支持，不执行迁移脚本或旧代码，不回退权限或业务副作用。
配置/KV密文副本引用存PG、纳入rekey；data仅受控相对路径，拒绝链接、执行文件和代码包。
单份data64MiB、单文件16MiB、4096项、深度16；单实例5份/256MiB，全局64份/1GiB，
包含恢复阶段预留，超限拒绝而非擅自删旧快照。卸载仍保留快照并占用额度。
恢复先fsync临时树再atomic exchange；lease/日志记录目录身份。重启先核对完成状态，必要时交换回旧目录，
不确定时保持实例fence。清理不能确认的目录不会放行启用。unsupported文件系统不降级为非原子改名。
操作总预算2分钟、lease3分钟；已清理操作收据保留7天，晚到请求仍受revision保护。备份/恢复需包含
插件表和/data/plugins，快照不是独立的主密钥备份。不会导出KV明文或提供上传/下载入口。
