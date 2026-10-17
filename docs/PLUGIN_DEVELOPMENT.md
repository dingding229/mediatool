# Go 内置插件开发（V2）

## 仓库与构建

SDK、Host DTO、事件枚举、transport、mock host和golden工具均在独立Go module
`github.com/dingding229/mediatool-plugin/sdk`，插件在 `plugins/<id>/`，registry显式登记
元数据与工厂。不导入主程序internal，避免循环依赖。主程序已删除plugin-sdk/go和
examples/plugins。hello-go是F1真实worker示例，不是WASM或脚本插件。

1. 在插件仓库修改manifest和Go接口实现；依赖编译期锁定（可vendor），禁止包初始化I/O、
   init函数及非纯顶级求值，不能在import时创建实例。
2. 使用SDK测试工具跑单元、mock Host、真实Go二进制stdio golden；插件仓库CI vet/test/race。
3. 推送 `dingding229/mediatool-plugin`；主程序精确bump伪版本（不使用replace作为发布证据）。
4. `go run ./cmd/plugin-check` 审计registry全部非标准库传递依赖的初始化。
   `go run ./cmd/plugin-assets` 从固定pin导出本地只读资源清单；`--check`用于CI。
5. 主程序按标准Actions→Hub不可变镜像流程发布。运行时无编译器、包下载器、外部市场。

所有Go构建使用CGO_ENABLED=0的linux/amd64；同一app以plugin-worker子命令执行。
主程序和前端消费同一个 `library.json`，插件ESM位于其仓库`plugins/<id>/ui/dist/`，
构建期复制到`/plugins/<id>/ui/dist/`，不从网络加载。F1只提供资源清单，执行/共享Vue
加载器是F2范围；没有相关插件时资源数组为空。

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

同UID65532，rlimit和整树RSS/CPU看门狗，崩溃退避重启。进程组停止、Pdeathsig及subreaper
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
