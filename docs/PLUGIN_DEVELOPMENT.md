# Go 内置插件开发（V2）

## 仓库与构建

SDK、Host DTO、事件枚举、transport、mock host和golden工具均在独立Go module
`github.com/dingding229/mediatool-plugin/sdk`，插件在 `plugins/<id>/`，registry显式登记
元数据与工厂。不导入主程序internal，避免循环依赖。主程序已删除plugin-sdk/go和
examples/plugins。hello-go 是持续演进的真实 worker/框架验收插件，不是 WASM 或脚本插件。

1. 在插件仓库修改manifest和Go接口实现；依赖编译期锁定（可vendor），禁止包初始化I/O、
   init函数及非纯顶级求值，不能在import时创建实例。
2. 使用SDK测试工具跑单元、mock Host、真实Go二进制stdio golden；插件仓库CI vet/test/race。
3. 推送 `dingding229/mediatool-plugin`；主程序精确bump伪版本（不使用replace作为发布证据）。
4. `go run ./cmd/plugin-check` 审计registry全部非标准库传递依赖的初始化。
   `go run ./cmd/plugin-assets` 从固定pin导出本地只读资源清单；`--check`用于CI。
5. 主程序按标准Actions→Hub不可变镜像流程发布。运行时无编译器、包下载器、外部市场。

所有Go构建使用CGO_ENABLED=0的linux/amd64；同一app以plugin-worker子命令执行。
主程序和前端消费同一个 `library.json`，插件ESM位于其仓库`plugins/<id>/ui/dist/`，
构建期复制到`/plugins/<id>/ui/dist/`，不从网络加载。JSON 与构建期 ESM/共享 Vue 加载器均已接通；资源仍只能来自同一 pin，绝不加载远程 UI。

## 接口与能力

必选 `InitPlugin/GetState/StopService` 与可选接口表达
GetForm/GetPage/GetAPI/GetService/GetCommand/GetDashboard(+Meta)/GetModule/GetActions。
辅助客户端提供Config、SaveData/GetData/DelData、DataPath、PostMessage等。
每次Host请求携带本实例身份/generation，Host强制声明scope与最新授权；不暴露SQL、
数据库连接、Redis连接或115令牌。配置/KV返回revision，写操作传expectedRevision。

F1 hello-go只使用当前已实现的config.read、data.read/write、state.read、ui.json。
业务Host覆盖在F3；F2-A将API/Service/Command/Action的Go函数直接绑定为稳定handler id，
并补充线上descriptor元数据。不要把G1现有字符串descriptor当作最终的作者绑定API。

数据在PG加密，AAD绑定实例/行；私有文件目录默认`/data/plugins/<UUID>/{data,tmp}`。
开发/隔离测试可用`PLUGIN_DATA_ROOT`设置干净绝对plugins目录，不影响生产默认。
子进程env不包含主程序秘密，不能通过环境获取数据库/主密钥/115授权。
调用超时不代表副作用未发生；保留operationKey并查询/重试同一操作，不能创建新幂等键
盲目重放。写入有CAS/fence；撤权/升级后旧generation拒绝。

## 资源与调试

同UID65532，保留AS/NOFILE等rlimit（不设置按共享真实UID统计的NPROC）；整树RSS/CPU及task看门狗，崩溃退避重启。task默认256，可由PLUGIN_MAX_TASKS设置32–4096，200ms采样超限整组kill，原因tasks_exceeded；瞬时fork突刺由容器pids_limit兜底。进程组停止、Pdeathsig及subreaper
处理孤儿。Landlock只做best effort；同UID可读共享文件的残余风险由受信代码审核兜底。
不要在stdout输出普通日志（stdout专供RPC）；stderr有截断/限速，宿主module=plugins、
instance字段用于过滤。不得把私密日志、配置或令牌提交到公共仓库或CI工件。

本机测试用media_tool_test/Redis DB15且串行。真实隔离不能以macOS或CGO/race降级测试
冒充；主CI `scripts/test-plugin-kernel.sh`在原生AMD64验证静态worker、父proc拒读、
实例目录、Landlock warning、FD/rlimit、崩溃收敛与double-fork收割。

## F2-α 绑定与执行

Go 作者直接返回带 Handler 的 sdk.API/Service/Command/Action/Module；handler ID 由
runtime生成，wire descriptor只包含有界元数据。API方法列表、auth/path与manifest精确
一致，RequiredScopes只能是已声明且已授权的子集。当前 Cmd 用 manifest 的逻辑命令 ID
（如 hello），未来消息入口去掉 `/` 后匹配；不必手填 handler 字符串。服务 Handler 接收
JSON Kwargs，Action 接收/返回 ActionContext 与继续标记，module只注册，执行留β。

hello-go 1.1.2 提供四种auth API（admin可GET/POST），scheduled=true时一秒计数服务、
hello命令/动作、typed TransferComplete；后者用稳定event ID作为自己的KV幂等键。
这是合成计数示例，不发真实下载或通知。插件仓库真实stdio descriptor/lifecycle golden
与race三轮覆盖绑定、权限和调用。任意插件新增/修改仍先推插件仓库，再bump主程序pin。

保存配置（包括 Host.UpdateConfig）会推进授权修订并结束旧 generation；应在处理器末尾
调用，不能继续以旧身份发 Host 写入。普通计数/运行状态使用 KV，不反复改配置。新 worker
重新绑定 scheduleSetting；旧待执行调用被取消，已发出但未确认的调用保留 unknown。


## F2-β：Go 绑定与本地界面

使用 `services.Events.OnChainPriority(contracts.ChainNameRecognize,20,handler)` 注册
同步链，handler接收ChainEnvelope并返回NameRecognition JSON。Continue=false表示拦截；
必须返回完整合法hint，不能附加文件路径或身份ID。GetModule返回
`sdk.Module{ID:"metadata.name_recognize",Priority:10,Handler:fn}`。每次最多200ms，
必须尊重ctx取消；不要在识别链中做长I/O。识别链/模块的Host调用受宿主scope ceiling限制，
没有声明业务写权限；把后台副作用放service/event，不能靠链超时盲目重试。

`GetForm`返回Content+Defaults，`Component{Component:"VTextField",Model:"greeting"}`
绑定顶级配置，`When:&Condition{Model:"enabled",Equals:json.RawMessage("true")}`作条件；
只有白名单props生效。配置保存仍由宿主CAS处理。GetDashboard与GetDashboardMeta声明
多个key，meta refresh>=15秒，cols 1..12；Sidebar声明对应Pages key。

ESM由插件库提交 `plugins/<id>/ui/dist/index.js`，manifest.ui.mode=vue、entry固定为该目录内
文件，声明ui.trusted/ui.json/ui.sidebar。只能静态import同插件的相对文件或
`@mediatool/plugin-ui`，后者共享defineComponent/h/ref/组件库/apiClient。宿主不从网络加载UI，
不用远程CDN模块。组件接收instanceId、pageKey、api，清理timer/订阅用onUnmounted。
Go库升级→主程序bump pin→构建期检查/打包→标准Actions/Hub；运行期不编译/下载。

hello-go 1.2.0 已包含API/service/command/action、TransferComplete与DownloadAdded、
NameRecognize/模块、JSON表单与两个仪表盘、共享Vue的交互页面；普通识别默认不改写，
示例用recognizeTitle/moduleTitle/intercept配置显式启用。API模板路径留后续。

### F4-C typed 事件与策略

SDK `contracts/discovery.go` 提供 MetadataScraped、CoverageObservation、MediaConversion、
EpisodeExpectation/CompletionCheck 和 SourceRequest/SourceResult。hello-go 1.6.0 默认
只接收安全事实并原样返回候选/期待清单，来源默认为空，不改业务结果。新增权限需由管理员
重新确认。媒体转换不能造新的 TMDB identity；期待刷新只能补充；完成判断只能否决。
发现/推荐处理函数在后台收到 page=1,size=200；返回 HasMore 表明快照截断，不支持任意 URL。
客户端要翻页应读取宿主缓存接口，不能在浏览请求中驱动 worker。季覆盖观测不是剧集完结状态，
也不是下载集合收集结束；Host Subscriptions.Completion 只读且 unknown 不代表完成。

### F4-D 系统事件与入站命令

SDK `contracts/platform_events.go` 提供 NotificationQueued、OperationFailed、SettingsChanged、
PluginConfigChanged、InboundMessage、CommandInput，以及 EffectRequest/EffectReceipt。
hello-go 1.7.1 演示六项事件安全收据、已注册 `/hello` 命令与不改变结果的模块 offer 拒绝。

通知/刷新模块必须先处理 phase=offer（纯协商：not_started 或 ready）；仅在 host 随后发出
phase=execute 时才允许外部副作用。execute 后只能返回 sent/requested/unknown，不能用
not_started 或报错诱导 host 重发。共享 Telegram bot token 永不进入 SDK/插件；若提供
其他渠道，使用插件自身经管理员批准的配置，不向 Host 索取共享凭据。

命令参数必须当作不可信用户输入，禁止拼成 Shell；只有管理员允许的 Telegram chat/user
才进入注册表。InboundMessage 只含目标实例、命令、opaque actor、kind；不含原始 chat/user、
token 或回调签名。系统错误是聚合摘要，不是原始异常/日志订阅。

## F4 工作流与外部身份（最小宿主）

SDK现有Action函数绑定可作为已批准DAG的步骤。DAG≤32步/深度16、首期串行、总30min，
修改后必须重新确认。仅`workflows.run`加精确`workflowIds`可调用run/status/cancel（全部高危），
不能递归自行授权其他实例。unknown/失败不自动重试；步骤记录在实例运行历史。
`workflow.execution`只能收窄批准的执行，不能凭hook提升动作scope。

可选`GetAuthProviders`只提供subject assertion，不发cookie，不决定管理员角色。
须显式确认`auth.provider`与`events.chain.auth.verification`，subject需管理员预绑定。
首期真实外部登录入口返回unsupported；hello的fake只作golden/真实worker验收。
内置本地密码登录不经插件；broker身份每次验证代际/授权/绑定/账号版本，撤权立即失效。

## F5-G 工具与管理面板

工具在插件仓库实现 `sdk.ToolProvider.GetAgentTools(ctx) []sdk.Tool`，字段为Name、Description、
InputSchema、RequiredScopes、EffectKind、Handler；manifest扩展AgentTools与agent.tools均要声明。
可参考hello-go的hello.echo（纯回显）和hello.remember（只写本实例KV），不是远程下载安装示例。
效果分类read/write/dangerous由共享Host方法表校验，只能高报，不能低报。所需scope必须是已声明且
实时授权的子集；Host每次调用继续检查，运行期只获得本工具scope上限。受信代码的直接OS行为
仍依赖维护者审核，不把effect声明当成恶意代码沙箱。write/dangerous需要管理员二次确认。

schema使用有界JSON Schema子集：type、properties/required/additionalProperties、items、enum、
minimum/maximum、min/maxLength、min/maxItems及文案。未知关键词/$ref/regex不忽略而是拒绝；
不自动注入默认值，不作类型转换或网络解析。整数及数字enum精确比较，数字词法和指数有界。
输入16KiB、输出32KiB、schema8KiB/8层128节点；人工调用5秒。失败/unknown通过运行历史核对，
不能改变operationKey盲重放。没有LLM聊天、自动Agent、token计费，AgentLLMProvider仍N/A。

开发发布严格双仓顺序：mediatool-plugin本地vet/test/race/golden→提交推送→该仓Actions成功→
mediatool go.mod精确pin该提交（无replace）→生成同pin静态清单→主仓测试/Actions→Hub不可变digest→
按审核授权备份部署。插件库不能在运行期选择源码、提交、版本或加载远程UI。

## F6-α 插件开发参考

`plugins/site-checkin` 示例使用 cron、sites.request/checkin 和 notifications.publish，认证始终由宿主附加。
`plugins/library-refresh-notify` 示例使用 typed TransferCompleted、精确广播订阅、持久合并 service 和稳定操作 key。
两者有 manifest/history、JSON表单/状态页、mock Host 单测和 descriptor/manifest golden，无额外二进制或远程 UI。

开发顺序不变：插件仓库开发测试推送 → CI 绿 → 本仓精确 pin → 原生 AMD64 同 app worker E2E → 标准发布。
`plugintest.Host.Client()` 提供直接 typed mock；`plugintest.Start` 测 stdio/RPC。非只读 mock 操作也检查 key、
请求参数与实时 scope，保留成功/unknown 收据，不因测试后端更换而重写旧收据；新逻辑操作必须使用新 key。

后台服务或广播 handler 遇到需要人工处理的外部依赖，可在保存安全状态及通知请求后返回 `contracts.NeedsAction`。
宿主完成收据再 CAS 停用自身实例，needs_action 不自动重启；这不是绕过权限或跨实例状态修改能力。
错误和通知只用固定 reason code，不把 HTML、认证 URL 或原始异常当作状态。普通链错误仍回落内置逻辑。

## R 冻结与协作顺序

公共 DTO / scope / effectKind / typed events / golden 以插件仓库 SDK 为唯一来源；
主仓 `PLUGIN_SUPPORT_MATRIX.md` 从精确 pin 生成并在 CI 校验，registered 不代表越过环境/actor/adapter 前提。
新增/改变插件行为先更新该插件 manifest.version/history，在插件仓库执行 build/vet/test/race/golden、提交推送、等 CI，
然后主仓 bump 精确伪版本并生成资源清单，最后主仓门禁/Actions/Hub；部署仍需明确授权。
仅修文档不改插件行为时，不虚增六插件 manifest 版本；当前版本及历史以冻结清单和各 manifest 为准。
目录监控当前是默认3层、上限5层的有界递归，最内层文件计算 size/mtime 静默；旧 F6-β 单层描述已由 F6-γ 替代。

所有演示默认 active=false。`torrent-assistant` write=false/recheck=false 并不意味着 active=true 时不会通知；
`wishlist-subscribe` create=false 并不意味着 active=true 时零外连，resolve 也不能当只读测试。
停用 UI 只读元数据/加密配置，不调用工厂。需要动态表单应由管理员明确启用，或留待单独设计静态 schema。
真实同 app worker 测试统一使用 testworker.Ready 等待完整 handler 注册后开始计时；测试不得放宽200ms/2s预算。
F4 生产已观测首调23–86ms、回落0%；R 只报告测试环境真实 receipt/log 样本，无样本写“无样本”，不推算生产回落率。
