# 内置插件框架 V2（F1）

## 来源、发布与管理员选择

唯一代码来源是 `github.com/dingding229/mediatool-plugin`，当前 pin 为
`v0.0.0-20261017081253-7db725679179`。主程序和前端资源清单使用同一个 pin；
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
