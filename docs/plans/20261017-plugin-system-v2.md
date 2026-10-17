# 插件系统 V2：PS0 设计与实施计划

- 日期：2026-10-17（Asia/Shanghai）。状态：**PS0与F1-G1/G2已审核；F1-G3实施/门禁中，F2–F6待执行；不是全框架完成声明**。
- 最新决定覆盖原A2及修订1的分发设计：唯一插件来源为固定版本的 `github.com/dingding229/mediatool-plugin` Go module，SDK与插件源码同库；主程序pin提交，全部代码随app编译，只有管理员选择的实例才加载/启用。
- 每启用实例由supervisor启动**同一app二进制的 `plugin-worker` 子命令**，继续采用JSON-RPC 2.0 over stdio。没有独立插件可执行包、上传、外部仓库、远程安装或运行时编译入口。
- 历史审计基线：`7d8c67ca157d5c08137d311b154a814c0a2de731`（R3，Schema61）。G1已初始化插件库；G2精确pin其`7db7256`提交对应module版本，新增Schema62；G3统一提交并等待Actions，生产部署仍留R-F。
- PS0与G1已审核，G2基础实现和门禁已完成。G3完成管理UI、定义刷新、构建接线及正式AGENTS规则，门禁/发布记录见§21。生产Compose未改、不连接生产。
- 执行预算：每任务约1200行非测试代码；大阶段按子任务验收，超出内部再拆段连续完成。测试、golden、文档不计。
- 路径除具名前缀 `mediatool-plugin/` 指新独立仓库外，均相对本仓库根；“拟新增”不是当前已有实现。Host API名称是V2公共合同，不直接等同现有Go方法名。

## 0. 结论与审核重点

1. 接受修订2：采用与站点模板相同的**编译期固定来源＋运行期管理员选择**模式。只读目录能列出全部定义，但不自动创建实例、数据、服务、路由或worker；生产网络不可达也不影响内置目录安装。
2. SDK/DTO/事件/协议常量的唯一公共来源改为 `mediatool-plugin/sdk`，不是本仓库的独立SDK module；插件仓库绝不反向依赖主程序，主程序通过internal适配器实现Host端，避免循环依赖与双份协议漂移。
3. 同一二进制worker在app配置、日志存储、DB/Redis/Vault、迁移与业务服务初始化**之前**分流；启动参数不能允许插件选择核心server角色或任意可执行路径。环境/文件/进程隔离沿用A3。
4. **编译进app不等于业务自动加载，但Go包初始化发生在main前**。内置插件必须无副作用包初始化、无业务全局单例，registry只保存纯metadata/工厂引用；CI检查并人工审核依赖初始化。supervisor隔离的是实例worker运行，不为恶意包初始化提供隔离保证。详见§3/§12。
5. 主程序启动对已安装实例刷新定义；扩大权限/资源范围或危险执行面时立即撤销旧generation并停用待确认；定义被pin删除时保留配置/KV/data并标不可用。升级只经bump pin＋镜像发布，UI不选择源码/提交，也不能单独下载旧插件回滚。
6. A3采用同UID＋父进程dumpable限制、环境/FD白名单及资源收敛；同页ESM仍属网页级信任，`ui.trusted`不是浏览器沙箱；写副作用超时仍须unknown例外，不能重复fallback。
7. F1/F2后的R-F只发布可用框架；F3–F6未完成能力明确unsupported。不改变官方app/postgres/redis三服务、凭据/115/主密钥/STRM/媒体、真实UA与严格302，不增加中转/HLS/转码。

## 1. 参考证据与版本范围

### 1.1 固定的 MP 源码

2026-10-17 通过 GitHub 公共 API 核对 `jxxghp/MoviePilot` 的 `v2` 分支：提交 `1528176beacc0f9f8d8d9cbf1adf048b0da67b5c`，提交时间 `2026-09-26T01:28:11Z`。下面矩阵锁定此提交；未来上游更新不自动改本项目合同。

- `_PluginBase`：https://github.com/jxxghp/MoviePilot/blob/1528176beacc0f9f8d8d9cbf1adf048b0da67b5c/app/plugins/__init__.py
- 事件枚举： https://github.com/jxxghp/MoviePilot/blob/1528176beacc0f9f8d8d9cbf1adf048b0da67b5c/app/schemas/types.py
- 事件机制： https://github.com/jxxghp/MoviePilot/blob/1528176beacc0f9f8d8d9cbf1adf048b0da67b5c/app/core/event.py
- 业务模块调用： https://github.com/jxxghp/MoviePilot/blob/1528176beacc0f9f8d8d9cbf1adf048b0da67b5c/app/chain/__init__.py
- 用户指定 Wiki：https://wiki.movie-pilot.org/zh/plugindev 。实际读取的页面已指向 V3，页面标记更新时间 2026-08-18；作为入口和版本漂移提示，不覆盖上述 v2 基线。
- Wiki 当前链接的指南：https://github.com/jxxghp/MoviePilot-Plugins/blob/main/docs/Plugin_Development.md ，标题明确为 V3；本文不把其中 ORM、浏览器或 Agent 平台视为本程序现成功能。

固定基线 `_PluginBase` 含 `get_auth_providers`、`get_agent_tools`、`get_render_mode` 及异步 KV 变体；事件清单为 **31 个 EventType + 19 个 ChainEventType**，第 9 节逐项对应，保留上游 `CommandExcute` 拼写以方便核对。本项目实例隔离，**不支持** MP 任意传入另一 `plugin_id` 读写其配置/KV。

### 1.2 协议、Go初始化与隔离依据

- JSON-RPC 2.0：https://www.jsonrpc.org/specification
- Linux资源限制：https://man7.org/linux/man-pages/man2/getrlimit.2.html
- `/proc/<pid>/environ`：https://man7.org/linux/man-pages/man5/proc_pid_environ.5.html
- Landlock：https://www.kernel.org/doc/html/latest/userspace-api/landlock.html （实现检测ABI，不假定目标内核支持最新版能力）。
- Go模块与pin：https://go.dev/doc/modules/gomod-ref ；Go初始化顺序：https://go.dev/ref/spec#Package_initialization 。这里引用稳定语义，不据文档站新版升级仓库工具链。
- 固定构建标志：https://pkg.go.dev/cmd/go ；app发布产物静态校验：https://pkg.go.dev/debug/elf 与 https://pkg.go.dev/debug/buildinfo 。它们不再是插件上传校验器。
- 同源JS：https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Same-origin_policy

### 1.3 本地站点模式与插件仓库核对

- 本地主程序 `go.mod:10` 固定 `github.com/dingding229/mediatool-site v0.0.0-20261013180133-7dfba442a392`，`internal/modules/sites/go_templates.go` 导入该module的All/Get。
- `sites.Repository.RegisterGoTemplates` 只刷新 `site_template_loads` 和既有PT账户需要的定义；`LoadGoTemplates` 只记录/加载管理员选择的ID。`cmd/media-tool/main.go:run` 在迁移后调用注册。插件复用这种语义而不写站点表；不照搬旧 `native_repository.go` 的网络获取路径。
- PS0阶段（2026-10-17）对 `https://api.github.com/repos/dingding229/mediatool-plugin` 的匿名只读查询返回 `private=false,size=0,default_branch=main`；commits接口返回409/`Git Repository is empty.`。该条仅为PS0历史观察；G1随后验证push权限并初始化推送`7db7256`，G2已精确pin该提交。
- 插件仓库地址为用户固定的 `https://github.com/dingding229/mediatool-plugin`，不是可以配置的安装来源。F1使用本项目同套Git凭据，缺写权限就报告阻塞，不借用其他项目凭据、不改成二进制上传或外部仓库。

以上只读公开源码/元数据与本地仓库，未访问生产、数据库、凭据或媒体。

## 2. 现状审计与删除/替换范围

### 2.1 代码、入口、引用关系

| 位置（现有） | 当前职责/被谁引用 | V2 处理 |
|---|---|---|
| `internal/modules/plugins/plugins.go` | contractVersion=1、仅 wasm；`Registry`、识别覆盖；`cmd/media-tool/main.go:runWorker`、`httpapi.pluginRegistry`、`workbench.go:parseReleaseName` 调用 | 替换成 V2 门面；业务只依赖窄接口，不再散布 Registry 构造 |
| `plugins/framework.go` | wazero/WASI、单次 instantiate、stdout JSON、4 类 Host 权限、状态/KV/clone | 删除 WASM 执行器；拆 `protocol`、`runtime`、`repository`、`permissions`；可复用边界测试思想，不保留旧格式 |
| `plugins/package.go` | ZIP 白名单仅 manifest/settings/README/plugin.wasm/ui.html；摘要、安装、配置 | 删除运行期包安装器；V2只从编译registry选择定义，不保留ZIP/二进制上传或解包入口 |
| `plugins/services.go` | `extension_service_ticks` + `extension_jobs` + `jobs` 持久唤醒；cron、手动调用 | 替换调度数据模型，复用现有 jobs 与 `platform/scheduling`，不另开 APScheduler/后台自有计时器 |
| `plugins/events.go` | 轮询共享 outbox，旧 pending→unknown，不盲目重投 | 替换为 V2 广播 delivery；保留非幂等未知结果原则，补 lease/fence、版本化 DTO |
| `plugins/market.go` | HTTPS 有界拉取、仓库目录、摘要安装；内置仓库指向 examples index | 删除仓库获取/市场下载；替换为当前pin的只读内置目录，不请求GitHub或解析外部索引 |
| `plugins/*_test.go` | 旧 contract/WASM/沙箱用例 | 删除/改写专属 WASM 测试，迁移安全性质至 V2；不得以删测试逃避同级验收 |
| `internal/platform/pluginhost/host.go:New` | 当前只有 `library.list`、`downloads.list`、`tmdb.search`、`notifications.send`；前两者直接 SQL→动态 map | 替换成 typed Service 桥；业务 SQL 留在业务模块，不向插件暴露 SQL、pgx、Redis 或数据库形状 |
| `cmd/media-tool/main.go:runWorker` | 注册 `plugin.service`/`plugin.dispatch`；cron 调 `Schedule`/`DispatchEvents` | composition root 注入 supervisor、registry、events、bridge；老任务仅安全终结，不再执行 WASM |
| `httpapi/plugins.go`、`plugin_packages.go`、`plugin_framework.go` | 管理、ZIP、配置、旧 `/view/{kind}/{key}`、sandbox iframe | 移除包上传/外部来源路由；替换V2内置目录/实例管理API，新增单数 `/api/v1/plugin/{instance}/{path}` 业务路由；只在管理监听器注册 |
| `httpapi/workbench.go:parseReleaseName` | 直接调用 Registry.Recognize | 依赖纯 typed extensionpoint；F2 NameRecognize 接入并保留内置回落 |
| `plugin-sdk/go/sdk_wasm.go` | `wasip1` + `go:wasmimport` | 删除本仓库整个plugin-sdk/go；公共SDK迁至mediatool-plugin/sdk，不再保留平行module |
| `examples/plugins/automation/`、`automation.zip`、`index.json` | Go/WASM 示例、iframe HTML、市场示例；打包脚本、单测/备份测试/E2E 使用 | 删除本仓库examples/plugins；hello/验收源码统一到mediatool-plugin/plugins/<id>，没有插件独立包产物 |
| `examples/plugins/filename-rules/` | 旧规则配置示例（不属于可运行 V2 包） | 随examples/plugins删除；如需功能示例，在F6于插件仓库重写 |
| `scripts/build-plugin-example.py` | 构建 WASI 示例 ZIP | 删除旧打包脚本及调用；改用插件仓库测试工具和主程序pin编译门禁，不提供打包上传替代命令 |
| `go.mod/go.sum` 的 wazero | `framework.go`、对应测试唯一执行依赖 | 仓库非测试和测试依赖扫描归零后 `go mod tidy` 移除；PS0 不执行 tidy |
| `web/src/views/PluginsView.vue` | 当前安装/启停/配置/clone/任务 | 原位升级实例管理；不再对用户展示 WASM 沙箱保证 |
| `web/src/views/PluginPageView.vue` | 当前 `/plugins/:id/:page`；iframe + postMessage 桥 | 原位替换 V2 页面；JSON 默认，受信 ESM 可选；移除旧任意 HTML iframe 桥 |
| `web/src/components/PluginRenderer.vue` | 目前只识别 title/description/metrics/rows/actions，**不是通用组件树** | 替换白名单 JSON 组件渲染器和无 eval 条件语言 |
| `PluginDashboards.vue`、`PluginMarket.vue` | 首页单仪表板、市场 UI；被首页/插件页使用 | 多key/meta；PluginMarket改为PluginCatalog，只展示内置定义、权限差异、已安装状态；不展示外部来源/版本选择 |
| `web/src/router.ts` | 已有插件路由；插件侧页还不是动态注册表 | 保留既有路由，新增受控动态侧栏注册，不接受插件任意覆盖核心路由 |
| `web/e2e/framework.spec.ts`、PluginPage 单测 | 旧 WASM/iframe/权限测试 | 更新合同并补真实app worker子命令生命周期/权限刷新UI回归，不删其他功能用例 |
| `internal/platform/backupverify/verify_integration_test.go` | 旧测试读取automation.zip、构造旧密文插件行 | 改成 V2/legacy 归档双向备份验证；保留业务凭据 round-trip 断言 |
| `internal/platform/secrets/rekey.go` | 明确枚举旧插件两列的 AAD | 只调整插件表定位/新增 V2 列；旧 AES/AAD 与其他凭据项不改 |
| `cmd/migrate/main.go` 的 test-only plugin cleanup | 仅测试库允许 truncate 旧 fixture | 更新 V2 测试清理名单与数据库名守卫，生产路径仍不能执行 |
| `docs/PLUGIN_FRAMEWORK.md`、`PLUGIN_DEVELOPMENT.md`、`EXTENSIONS.md`、OpenAPI | contract=1 说明 | F1 标记废止并链接 V2；F2/F5 按阶段补全新接口，不宣称未来已实现 |

**真实集成缺口**：目前 `plugins.Recognize` 的直接调用主要在工作台预览；不能据此宣称入库识别全链都已插件化。`messaging.Dispatcher.Prepare/Deliver` 是出站投递，Telegram adapter 也是出站；没有现成的入站 `/cmd` 分发宿主。`notifications` 主要是模型/接口占位；通知实际接线须通过 messaging/outbox。当前也没有通用工作流、Agent/LLM、外部 auth-provider broker。

### 2.2 原表与只动插件的迁移边界

| 原表/列 | 来源迁移 | 处理（草案） |
|---|---|---|
| `extension_plugins` 及 manifest/config/config_ciphertext/wasm_module/plugin_data_ciphertext/package_*/readme/ui_html | 00025、28、30、31、32 | 禁止执行，归档为 `plugin_legacy_wasm_plugins`，保留插件私有密文；不自动译成 V2 |
| `extension_deliveries` | 00027 | 归档 `plugin_legacy_wasm_deliveries` |
| `extension_runs` | 00028 | 归档 `plugin_legacy_wasm_runs` |
| `extension_repositories` | 00030 | 归档 `plugin_legacy_wasm_repositories`；不自动信任旧仓库内容 |
| `extension_jobs` | 00030 | 归档 `plugin_legacy_wasm_jobs`，不复执行旧调用 |
| `extension_service_ticks` | 00030 | 归档 `plugin_legacy_wasm_service_ticks`，停止旧调度 |

不为了“不兼容”销毁旧插件可能保存的第三方凭据。归档不等于运行时兼容；旧插件须在内置插件仓库按V2重写，随主程序pin发布后重新选择并确认权限。归档清除必须另有管理员显式批准，不纳入 F1 自动 GC。

**迁移不得触及** administrator/media user 凭据、integrations（含 115 授权）、master key、application_settings、native_server_settings、STRM/媒体/扫描/下载/站点业务表；不得重写历史 00025/28 等迁移（它们还包含共享表）。共享 `jobs/outbox_events/audit_logs` 保持表结构与原数据；正常插件任务完成/审计写入由既有服务完成，不 `TRUNCATE`、不按整个队列删除。仅对旧插件 kind 做“已退役、未执行”的安全终结处理。

## 3. ADR A1 / A2″ / A3–A6

### ADR A1：同一app二进制的进程外长驻worker（接受修订2）

- 每个启用实例一个受supervisor管理的子进程：固定 `/app/media-tool plugin-worker --plugin <registry-id> --instance <uuid>`。不由manifest指定路径，不加载Go动态库，不启动额外容器/顶级服务。
- `main`最先识别固定worker子命令并使用独立参数解析器；拒绝未知/重复参数、混入`--role`或配置路径。worker分支不调用现有app的配置加载、日志数据库、DB/Redis、Vault、迁移、115/integrations构造器。
- 启动身份只来自父进程stdio连接/受控继承FD和已claim的generation；命令行中的ID不是Host授权。没有有效父连接/握手时有界退出；直接手工运行子命令不能连接宿主数据库或获得Host能力。
- worker通过registry精确选择一个工厂，隔离/握手完成才构造该实例并InitPlugin。只有此进程能调用插件实例业务；app父进程只读metadata，不调用插件工厂或get_*方法。RPC与业务回调分离，崩溃/停止收敛沿用A3及§5。

### ADR A2″：内置Go插件库与同源SDK（接受；覆盖A2/A2′分发设计）

- 唯一module `github.com/dingding229/mediatool-plugin`，内含 `sdk/`、`plugins/<id>/`、`registry/`；SDK不是另一个go.mod，import路径为 `github.com/dingding229/mediatool-plugin/sdk/...`。插件及SDK不得导入主程序或其internal包；SDK也不得反向import registry/plugins，插件不import registry，注册集中在registry静态factory表，CI检查依赖DAG无环。
- 主程序go.mod/go.sum固定真实提交对应的module版本，经构建编入app；SDK/DTO/事件/contractVersion与插件一起固定。CI用接口断言和Host编译保证一致，启动只读自检再核对合同/schema和注册项，握手验证同一build身份。**manifest不再声明sdkVersion、二进制路径或运行平台**。
- registry列出全部纯metadata＋工厂引用；不使用有副作用的init注册或在全局初始化实例。管理员选择前不创建实例/目录，不运行InitPlugin/业务goroutine/get_*，不会因为新pin增加插件就自动启用。
- Go包变量初始化和init会早于main运行，因此编译链接模型把插件源码及其依赖纳入app可信供应链：禁止插件/SDK/依赖未经审查的初始化I/O、goroutine、环境秘密读取、进程执行及panic。CI AST/依赖检查加人工审核，不能只删init函数就声称所有依赖已安全；worker隔离不能弥补恶意父进程包初始化。
- 只保留“源码修改→插件库推送→主程序bump pin→镜像发布”的更新链。运行期不下载、不编译任何插件/SDK，不接受上传/外部仓库/源码或版本选择。生产镜像无解释器和Go工具链。
- 全部插件的ESM产物随同一pin在构建期加入app前端静态资源；按插件ID命名空间，授权后才执行。删除本仓库plugin-sdk/go、examples/plugins及旧WASM依赖，F6示例只存在插件仓库。

### ADR A3：受信代码的缺陷隔离（审核后简化方案）

威胁模型是维护者审核并编译进镜像的自有插件的缺陷，不是恶意代码。worker与app同UID **65532**；保持生产`cap_drop:[ALL]`及`no-new-privileges:true`，**不做setuid launcher、UID池、跨UID停止helper或seccomp，不为插件修改生产安全配置**。

1. app父进程最早期调用`prctl(PR_SET_DUMPABLE,0)`，配合cap_drop ALL限制同UID worker读取父进程`/proc/<pid>/environ`、mem及ptrace；G2/F1-D在真实Linux实测，并核对Go所有线程/新线程与启动顺序，不能只测一个无Go线程的小进程。dumpable失败或平台/pprof/core等现有能力受影响必须如实报告，不声称已经秘密隔离。
2. worker环境由白名单显式构造，不透传`os.Environ()`；DB/Redis/Vault/115/代理凭据不注入。FD只保留明确stdio/所需FD，其余close-on-exec；Pdeathsig、独立进程组、退出时整组kill并收割。G2验证父Go线程退出语义及孤儿回收，不许僵尸进程。
3. 固定执行同一只读app的plugin-worker命令，ID由registry与父连接确定；worker最早分流，不初始化DB/Redis/Vault/业务配置。参数中的instanceId不能获得Host身份，身份/授权由Host连接绑定。
4. rlimit默认AS8192MiB/NOFILE256/NPROC64，短命exec CPU30s；常驻worker不用30s累计CPU寿命。同UID的NPROC按真实UID汇总线程/进程而非实例独立计数，F1-D必须实测其与app现有线程的相互影响，失败不得伪称独立限额。整棵进程树RSS每200ms采样，单worker256MiB/总1GiB；cgroup可用时可选加强，不要求生产挂载或能力变更。
5. **Landlock尽力加固**：利用容器已有no_new_privs，worker早分流后非特权自限制，只读运行库/证书/DNS和app代码，读写本实例data/tmp及已授权媒体根，拒绝其他/data与秘密文件。内核ABI不支持记录warning后继续，Host scope仍强制；不能fail closed。实现若超过约300行非测试代码或需要新依赖，先停下报告。G1只修订设计，不提前写此实现。
6. 实例目录使用可信root、路径前缀/owner及无符号链接校验，不能扩大媒体DAC权限；SDK/Host按scope、selector、actor、generation执行授权。不因代码内置而给插件SQL/凭据或全表接口。
7. **残余风险**：同UID进程仍可读取app可读文件（例如文件形式的主密钥、日志/备份/其他实例数据），Landlock不可用时尤其如此；dumpable不保护这些路径。相同UID也不是可靠的进程信号/实例机密隔离。没有seccomp时受信代码可能setsid/double-fork，进程组本身不保证收割逃逸后代，需supervisor整树跟踪及代码审核，不能宣称恶意代码沙箱或绝对RSS限制。
8. Go包初始化和同页ESM信任风险仍按A2/§12披露。scope是Host API硬授权，但原生exec/网络/文件不全部经过Host守卫；代码审核兜底，不许借插件提供主程序媒体中转/HLS/转码或运行期二级插件安装。

### ADR A4：typed Host API（接受）

- `mediatool-plugin/sdk/{contracts,events,host}` 为公共DTO/错误码/scope/扩展点和`contractVersion=2`的唯一来源；主程序internal/contracts只做别名、adapter与Host接口实现，不复制公共enum。SDK不导入主程序、DB或runtime实现。
- `internal/platform/pluginhost` 做 capability dispatcher + 显式 adapter 注入，调用各业务 service。现有缺少 Service 的模块先抽窄接口；不得为图省事增加 `sql.query`、`redis.command`、`integration.reveal`、`settings.dumpEnv`。
- JSON 中 ID 为字符串，bytes 明确 base64（上限），ticks int64，时间 RFC3339 UTC，配置缺失/JSON null 分开；列表有 cursor/pageSize、总数/hasMore 合同，不返回 Go struct/数据库 row map。
- 每次调用由连接绑定实例身份，params 里的 `instanceId` 不被当作认证。业务写操作继承服务验证/锁顺序/审计；危险删除仍需业务 preview digest + expectedRevision + confirm，插件 scope 不替代这些检查。

### ADR A5：MP 扩展面（接受，拆清宿主缺口）

- JSON 表单/页/仪表板、ESM、侧页、API、持久服务、命令、广播/链式事件、模块替换均纳入 F2；只先接 3 个真实触发点，其余见逐项矩阵。
- `get_*` 返回**描述符和 handler ID**，不能把Go func/接口对象跨RPC序列化。运行调用`plugin.invoke`，指向已登记handler ID，宿主不做任意`getattr/eval`。Go可选接口返回Form/Page/Dashboard/API/Module/Action等typed描述符，SDK将Go handler放入本地注册表，wire只传handler ID；布局/defaults/meta是具名struct，不把func、接口指针或进程内调度对象送JSON。
- get_actions：F4-0 最小工作流宿主（持久 job + 有界 DAG，F5完善UI），不是先做完整可视化工作流编辑器。
- auth_providers：F4-0 最小外部认证 broker（F5完善UI），需单独 `auth.provider` 高危授权；本地管理员登录永不被插件删除/替代。不让链式 hook 凭布尔值绕过 Argon2、管理员/媒体令牌、CSRF 或下载权限。
- agent_tools：F5 注册并提供管理员手动调用/外部适配入口，JSON schema + 有界工具执行。当前无 Agent 对话/LLM 宿主，`AgentLLMProvider`、token 用量自动事件标 N/A（可预留合同，不能空实现冒充具备）；不顺带新建 LLM 平台。

### ADR A6：不兼容旧格式但保留受保护数据（接受）

- 下一空闲迁移号预计 00062，具体执行前再读最新文件，避免号冲突。只处理插件表/索引/专属密文列；绝不改旧历史迁移中的共享表。
- 旧代码/SDK/示例移除，旧表停用归档而不是执行旧包。只读导出可选，默认不把旧 secret 明文迁入新插件。
- 更新 secret rekey/备份工具的**插件列清单**以支持归档与新表；不修改 Vault 既有算法/AAD 前缀，不重加密无关业务，不更换主密钥。
- 迁移前快照插件专属行与 schema；Down 仅在 V2 无实例/任务时允许，恢复旧表名但保持 disabled。V2 已有数据时明确拒绝有损降级，由完整备份恢复；Up→Down→Up 测试验证空 V2 路径以及带 legacy 密文的无损归档。保留旧 WASM运行兼容不是回滚前提。

## 4. 两仓库结构、加载与生命周期

```text
mediatool-plugin（独立module，单向依赖sdk）
  sdk/{contracts,events,host,transport,plugintest,schema}
  plugins/<id>/{plugin.go,manifest.json,README.md,icon.*,ui/dist/*}
  registry/       # All/Get: 只读metadata及factory引用，绝不调用工厂
        │ 主程序go.mod固定版本、只在构建期取源码/产物
        ▼
app父进程: 内置目录→管理员选择→PG实例/权限/配置/lease
                    │ Go supervisor + reaper
                    ├─ 同一app plugin-worker（实例A）
                    └─ 同一app plugin-worker（实例B）
                       stdio ⇄ JSON-RPC ⇄ Host dispatcher→业务Service
前端: 同一pin ui/dist→构建期插件ID命名空间资源→授权后加载
```

主程序拟目录 `plugins/{catalog,contracts,protocol,runtime,repository,events,extensions,worker}`；没有installer/downloader/market运行包。`platform/pluginhost`是业务adapter，公共contract从SDK导入；业务只依赖窄ExtensionDispatcher，不能导入registry工厂或执行器直接绕过supervisor。

数据布局只含实例数据，不含可执行代码：

```text
/data/plugins/<instance>/
  data/                           # GetDataPath；默认256MiB
  tmp/                            # 私有临时；默认64MiB
  snapshots/<operation-revision>/ # 仅本实例数据快照，非旧插件二进制
```

代码与UI都在只读镜像中；无releases/current/解包staging路径。目录创建/删除使用可信root FD、owner校验、逐段无符号链接打开、原子写入；GC限插件根，不能删除媒体或备份。媒体根不与系统秘密根重叠，不能为插件原生open而递归chmod/chown真实媒体。

### 4.1 实例操作

| 操作 | 固定顺序/失败处理 |
|---|---|
| list catalog | 只读registry metadata分页，最多200；合并已安装状态；不触发constructor/数据写入/网络请求 |
| install | 选择当前registry ID→只读预览metadata/权限→绑定expected definition/grant revision确认→事务创建disabled实例/独立加密配置和授权→有界创建data目录；并发重复用operationKey去重。启用另行明确选择，未确认不留下实例/目录 |
| enable | 再核对定义存在/已授权/合同自检→CAS→先取额度再claim owner lease→环境/资源准备→spawn同UID固定worker→initialize→工厂/InitPlugin→校验get_*子集→原子发布registry |
| disable | 立即撤销新调用/Host write→排空15s→StopService 10s→SIGTERM→5s后整树SIGKILL→回收PID/进程树/lease；迟到写回fence不命中即放弃 |
| config update | expectedRevision→加密保存→失效旧generation→有界stop/init；不在DB事务或全局锁里调用worker，避免配置回写循环 |
| crash | 收割/心跳失败→撤销generation→启动退避1/5/30/120/600s+jitter；10次/小时needs_action；不重放外部unknown副作用 |
| pin upgrade | 随新镜像启动刷新**已安装**定义；无权限扩大且数据合同兼容才按既有enabled状态恢复；权限扩大立即停用、保留数据并等待新确认。未安装定义仅目录可见 |
| plugin removed | 不再解析为可执行工厂，availability=missing、needs_action；停旧worker/调度/路由/Host写，保留ID及原配置/KV/data/最后已批准metadata；不得cascade删除 |
| reapprove | 当前定义与旧批准快照差异→管理员确认expectedRevision→更新grant revision；需要再次明确enable，不从旧enabled状态悄悄恢复 |
| uninstall | disable并收割→撤销API凭据/计划/注册→默认保留实例归档；彻底清理需二次确认和FD/owner校验，不删除相关业务数据 |
| clone | 新UUID/目录及AAD；默认disabled；配置/KV/data复制可选择，不复制凭据token、lease、run或危险授权确认 |
| restore data | 只恢复本实例快照且数据schema兼容；恢复不执行旧代码。无单插件代码回滚，代码回退走整app镜像的标准备份/发布流程 |

### 4.2 定义刷新、指纹与幂等

- `definitionFingerprint`由规范化metadata＋实际pin/build身份＋该插件UI资源摘要计算，只作本地一致性/缓存失效，不是可下载包摘要或UI选择项；`reviewFingerprint`单独覆盖权限/selector上限/API public与执行scope/module/chain/UI信任等执行面。
- 仅比较同scope字符串不够：scope新增、可选资源集合扩张、origin/事件/module/API公共暴露扩大或无法证明为子集均视为扩大；`ui.trusted`的ESM字节变化要求再次确认。纯描述/代码版本更新且权限未扩大、schema兼容可刷新，无须每次无关pin变化全部停用。
- 事务保存旧批准定义/授权和新候选metadata、CAS增加revision并撤销旧generation，再在事务外启停；服务/事件/路由注册带definition+grant revision。并发owner/重启重复刷新不得重复实例、丢配置或重新授予已撤销权限。
- 安全兼容刷新以当前已批准的有效授权集为上限，原子更新实例采用的definition fingerprint与revision；缩权时收窄/持久化有效集，后续再扩回也须重新确认，不能复活历史授权。reviewFingerprint不纳入无关pin变更，避免无权限变化也反复询问。
- 数据schema改变不得自动执行已停用插件做“迁移”；保留旧快照，进入needs_action，管理员在数据备份后明确批准兼容迁移。降级镜像不能以旧代码直接覆盖新schema数据。
- 同一pin重复启动无变化则无业务写入；pin移除后再添加同ID也不复活旧授权/自动启用，须确认当前定义。目录里不存在的实例仍可查看/导出安全状态和卸载，但不可取工厂。

## 5. JSON-RPC 2.0 over stdio 合同

### 5.1 帧格式、标识与版本

采用 **4 字节无符号大端长度 + N 字节 UTF-8 JSON**，每帧恰好一个 JSON-RPC object（不是换行/Content-Length 混用）。长度不含头；禁止 batch、BOM、NaN/Infinity、重复 object key、超过 64 层嵌套。Go SDK与宿主均用`io.ReadFull`读取4字节头和N字节正文，半帧 EOF 是连接故障。控制帧/JSON 默认 1 MiB，API body 专用调用仍不能超过全局 4 MiB；超过在分配前拒绝并关闭连接。

```json
{"jsonrpc":"2.0","id":"h:17","method":"plugin.initialize","params":{"contractVersion":2,"instanceId":"<uuid>","generation":"<lease-token>","configRevision":3,"definitionFingerprint":"<local-definition-fingerprint>","catalogBuild":"<compiled-build-id>","capabilities":["plugin.data.read"]}}
```

- 宿主 request ID `h:<uint64>`，worker request ID `p:<uint64>`；response 原样 echo。id 是连接内关联号，不是数据库幂等键/用户 token。notification 无 id，不返回结果。
- 首包`plugin.initialize`版本精确2；SDK/Host来自同一编译依赖，启动先做contract/schema自检，握手再校验compiled build/registry definition/实例generation；reply `contractVersion/schemaFingerprint/catalogBuild/implementedMethods`。不符unsupported_version，不回退v1，不联网找兼容SDK。
- Host 控制面和业务 result 均显式 DTO；request params 中的 generation 不可伪造连接上下文。未知字段由对应 DTO 的兼容策略处理：安全/manifest/权限严格拒绝，业务只允许文档化的扩展字段。

### 5.2 宿主→插件方法表

| RPC | 结果/使用位置 | 默认 wall timeout |
|---|---|---:|
| `plugin.initialize` | 同一build合同/definition校验，不执行可见业务 | 5s |
| `plugin.init` | 映射InitPlugin，传该实例 config（不含宿主凭据） | 15s |
| `plugin.get_state`、`plugin.get_name` | 状态/显示名，不决定宿主 enabled | 2s |
| `plugin.get_form`、`plugin.get_page`、`plugin.get_render_mode` | 组件树/默认 model、页或受信 ESM descriptor | 3s |
| `plugin.get_api`、`plugin.get_service`、`plugin.get_command` | 路由/服务/命令描述符列表 | 3s |
| `plugin.get_event_handlers` | SDK汇总On/OnChain产生的typed广播/链handler描述符，不是Go动态反射 | 3s |
| `plugin.get_dashboard_meta`、`plugin.get_dashboard` | 多 key/meta/组件树，key 必须已登记 | 3s |
| `plugin.get_module`、`plugin.get_actions` | typed module/action描述符，不能在wire返回Go func/指针 | 3s |
| `plugin.get_auth_providers`、`plugin.get_agent_tools` | F4-0 auth broker / F5 tool 描述符；未实现阶段返回明确 unsupported | 3s |
| `plugin.invoke` | `kind=api/command/service/action/module/tool/auth`、预登记 handler ID + typed input | API 15s、命令30s、服务/动作300s、module≤2s、auth10s |
| `plugin.event` | 广播 event envelope，reply 是 ack/result，不是再次发布 host event | 30s |
| `plugin.chain` | 同步 typed patch/decision，见 §9 | 每 handler200ms，单链总2s |
| `plugin.stop_service`、`plugin.close` | 停止服务/释放资源；可重复调用 | 10s/3s |
| `rpc.ping` | echo nonce；不排在长回调之后 | 2s |
| `$/cancelRequest` | notification，`{id}` 取消相应连接内请求 | 无 reply |

`get_*` 必须是 manifest 声明能力的子集，metadata 回应不能偷偷增加 API/public auth、scope、服务或模块。UI 请求缺少新鲜 metadata 时显示 unavailable，不执行无限 `get_*` 以阻塞。

### 5.3 插件→宿主方法与 Go SDK

- RPC总入口`host.call`，参数`{method, args, expectedRevision?, operationKey?}`；method仅命中§7注册表。实例身份、确认scope、资源selector、发起人权限和generation均来自宿主连接上下文。
- Go SDK辅助方法：`SaveData/GetData/DelData/GetDataPath/PostMessage/GetConfig/UpdateConfig`。它们是本实例Host方法的typed包装，不接受另一plugin_id作为任意读写入口；调用都传context。读取向调用者提供的typed输出解码，不返回任意DB map/连接。
- `events.On(EventType, handler)`登记类型化事件；`OnChain(ChainEventType, handler)`登记链处理。注册只写SDK本地表，init后由`plugin.get_event_handlers`校验manifest/scope并随generation原子发布。Go package init不能调用尚未完成initialize的Host RPC；API不会在导入阶段启动业务任务。
- 事件handler使用`context.Context`、EventEnvelope及生成的payload DTO。SDK提供`events.Typed[T]`解码适配；EventType是有限枚举，kind与payload schema的配对在注册时校验，不假称Go编译器会从每个枚举常量自动推导不同T。不依赖装饰语法、eval或任意方法反射。
- `rpc.log`通知携level/message/安全fields，宿主固定module=plugins/instance/pluginId/definitionFingerprint/generation/runId；日志接口用`log/slog`适配，不允许伪装模块。stdout只承载RPC，stderr有界归集；协议stdout污染按故障处理。
- `rpc.progress`仅更新该run的有界进度，不替代持久成功；`rpc.ping`响应仍使用JSON-RPC对应id。Host reply、cancel和日志背压不能依赖正在阻塞的插件回调goroutine。
- 作者可以在已授权的受限进程内使用Go标准库os/exec、net/http、os等能力；这些原生调用不经过Host API守卫，受信代码风险说明、scope声明及OS隔离仍适用。SDK不提供主密钥、连接凭据、媒体token或115取链捷径。

### 5.4 错误、取消、并发和背压

| code | reason（单一常量） | 含义 |
|---:|---|---|
| -32700/-32600/-32601/-32602/-32603 | parse_error/invalid_request/method_not_found/invalid_params/internal_error | 标准 JSON-RPC；不返回 traceback/secret |
| -32001 | permission_denied | scope/资源/调用主体被拒 |
| -32002 | unavailable | 实例/业务依赖不可用 |
| -32003 | deadline_exceeded | deadline 到期，不能据此断言外部副作用未发生 |
| -32004 | cancelled | 主动取消 |
| -32005 | conflict | config/grant/resource revision 或 fence 冲突 |
| -32006 | busy | 队列/并发满，`retryAfterMs` 有界 |
| -32007 | not_found | 不存在或不允许获知的资源 |
| -32008 | unsupported_version | 协议/能力/数据 schema 不支持 |
| -32009 | upstream_limited | 既有 115/TMDB/站点限流，保留建议间隔 |
| -32010 | needs_action | 人工授权/隔离/依赖处理 |
| -32011 | result_unknown | side effect 结果未知，禁止一般重试 |
| -32012 | payload_too_large | 请求/结果/日志上限 |
| -32013 | invalid_extension_result | 描述符、chain patch 或模块结果不符合 typed schema |

- Reader、writer、request correlation 与用户 callback 分离；双向 request 泵独立。**回调等待 Host API 时 reader 仍收包**，不能单线程相互等死。SDK单callback默认串行，但Host reply/ping/cancel独立；插件 opt-in 最大 callback concurrency 4。
- 每实例默认 in-flight 宿主回调4、反向 Host call8；队列32、写队列累计4 MiB；全局 worker8、Host call32。控制队列单独保留8格；响应优先于新调用，不丢已接受 request。满则立即 busy/HTTP503，不无限 goroutine/内存队列。
- heartbeat 10s，连续3次丢失或主进程退出进入 crash 收敛；ping 不执行用户 hook。停止使用单调时钟预算，持久 next_at/lease 用 DB 时间。
- API断连/timeout先 cancel；不可协作任务超 grace 则整树停止，未完成写操作记 unknown。服务在300s内可报告 progress，但不能无限续约；更长操作拆成 Host jobs，不持有 RPC 或 DB 事务。
- 链/模块总预算取调用方剩余时间与2s上限较小者，先抢额度再占业务 lease；禁用/卸载后旧 generation 的 Host 写入、定时回调与迟到结果都由 fence 拒绝。
- 对标准通知协议不随意 ack；应用事件使用有 id 的 request 才能记录投递结果。禁止自动 retry 所有 RPC；只允许明确纯读或有已持久 operationKey 的幂等写重取结果。

## 6. 内置manifest v2与registry合同

manifest是插件仓库 `plugins/<id>/manifest.json` 的纯元数据，不是安装包、网络目录或用户上传格式。registry在编译期登记metadata与工厂，宿主不执行插件来读取定义。配置/API/资源语义仍需§7授权。

### 6.1 例子（F2完整hello；F1只声明已实现能力）

SDK只有同一Go module解析到的一份类型；兼容性由编译断言＋启动自检保证。无sdkVersion/二进制字段；id必须与目录及registry key完全一致。图标/ESM路径仅指该pin中的本地构建输入，不能填URL或实例数据路径。

```json
{
  "contractVersion": 2,
  "id": "hello-go",
  "name": "Hello Go",
  "version": "1.0.0",
  "author": "Example Author",
  "description": "实例隔离与持久计数示例",
  "level": 1,
  "icon": "icon.png",
  "history": {
    "1.0.0": "Initial example"
  },
  "permissions": [
    {
      "scope": "plugin.config.read"
    },
    {
      "scope": "plugin.config.write"
    },
    {
      "scope": "plugin.state.read"
    },
    {
      "scope": "ui.json"
    },
    {
      "scope": "ui.sidebar"
    },
    {
      "scope": "services.register"
    },
    {
      "scope": "plugin.data.read"
    },
    {
      "scope": "plugin.data.write"
    },
    {
      "scope": "fs.data"
    }
  ],
  "extensions": {
    "forms": true,
    "pages": [
      "overview"
    ],
    "dashboards": [
      "counter"
    ],
    "api": [],
    "services": [
      "tick"
    ],
    "commands": [],
    "events": [],
    "chains": [],
    "modules": [],
    "actions": [],
    "authProviders": [],
    "agentTools": []
  },
  "ui": {
    "mode": "json",
    "sidebar": [
      {
        "page": "overview",
        "title": "示例工作台",
        "icon": "puzzle"
      }
    ]
  },
  "resources": {
    "addressSpaceMiB": 8192,
    "rssMiB": 256,
    "cpuQuotaPercent": 100,
    "processes": 64,
    "openFiles": 256,
    "dataMiB": 256,
    "tmpMiB": 64
  },
  "configSchemaVersion": 1,
  "dataSchemaVersion": 1
}
```

### 6.2 JSON Schema草案（2020-12）

SDK保存唯一schema；Go/Vue测试消费同一份。它校验仓库元数据和Host导出DTO，不构成上传入口。

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "urn:mediatool:plugin-manifest:2",
  "type": "object",
  "additionalProperties": false,
  "required": ["contractVersion", "id", "name", "version", "author", "permissions", "extensions", "ui", "resources", "configSchemaVersion", "dataSchemaVersion", "description", "level", "icon", "history"],
  "properties": {
    "contractVersion": {"const": 2},
    "id": {"type": "string", "pattern": "^[a-z][a-z0-9-]{2,63}$"},
    "name": {"type": "string", "minLength": 1, "maxLength": 120},
    "version": {"type": "string", "minLength": 1, "maxLength": 64},
    "author": {"type": "string", "minLength": 1, "maxLength": 120},
    "description": {"type": "string", "maxLength": 4000},
    "level": {"type": "integer", "minimum": 0, "maximum": 9},
    "icon": {"$ref": "#/$defs/path"},
    "history": {"type": "object", "maxProperties": 100, "additionalProperties": {"type": "string", "maxLength": 4000}},
    "permissions": {"type": "array", "maxItems": 128, "items": {"type": "object", "additionalProperties": false, "required": ["scope"], "properties": {"scope": {"type": "string", "maxLength": 120}, "resources": {"type": "object", "additionalProperties": false, "properties": {"libraryIds": {"$ref": "#/$defs/ids"}, "storageIds": {"$ref": "#/$defs/ids"}, "siteIds": {"$ref": "#/$defs/ids"}, "rootIds": {"$ref": "#/$defs/ids"}, "hosts": {"$ref": "#/$defs/ids"}, "eventTypes": {"$ref": "#/$defs/ids"}, "settingKeys": {"$ref": "#/$defs/ids"}, "workflowIds": {"$ref": "#/$defs/ids"}}}}}},
    "extensions": {"type": "object", "additionalProperties": false, "properties": {"forms": {"type": "boolean"}, "pages": {"$ref": "#/$defs/ids"}, "dashboards": {"$ref": "#/$defs/ids"}, "api": {"type": "array", "maxItems": 32, "items": {"$ref": "#/$defs/apiRoute"}}, "services": {"$ref": "#/$defs/ids"}, "commands": {"$ref": "#/$defs/ids"}, "events": {"$ref": "#/$defs/ids"}, "chains": {"$ref": "#/$defs/ids"}, "modules": {"$ref": "#/$defs/ids"}, "actions": {"$ref": "#/$defs/ids"}, "authProviders": {"$ref": "#/$defs/ids"}, "agentTools": {"$ref": "#/$defs/ids"}}},
    "ui": {"type": "object", "additionalProperties": false, "required": ["mode"], "properties": {"mode": {"enum": ["json", "vue"]}, "entry": {"type": "string", "maxLength": 200, "pattern": "^ui/dist/(?!.*\\.\\.)[A-Za-z0-9_./-]+\\.m?js$"}, "sidebar": {"type": "array", "maxItems": 8, "items": {"type": "object", "additionalProperties": false, "required": ["page", "title"], "properties": {"page": {"type": "string", "maxLength": 64}, "title": {"type": "string", "maxLength": 120}, "icon": {"type": "string", "maxLength": 64}}}}}},
    "resources": {"type": "object", "additionalProperties": false, "properties": {"addressSpaceMiB": {"type": "integer", "minimum": 256, "maximum": 8192}, "rssMiB": {"type": "integer", "minimum": 64, "maximum": 1024}, "cpuQuotaPercent": {"type": "integer", "minimum": 10, "maximum": 200}, "processes": {"type": "integer", "minimum": 1, "maximum": 128}, "openFiles": {"type": "integer", "minimum": 32, "maximum": 1024}, "dataMiB": {"type": "integer", "minimum": 1, "maximum": 2048}, "tmpMiB": {"type": "integer", "minimum": 1, "maximum": 256}}},
    "configSchemaVersion": {"type": "integer", "minimum": 1, "maximum": 10000},
    "dataSchemaVersion": {"type": "integer", "minimum": 1, "maximum": 10000}
  },
  "allOf": [{"if": {"properties": {"ui": {"properties": {"mode": {"const": "vue"}}}}}, "then": {"properties": {"ui": {"required": ["entry"]}}}}],
  "$defs": {
    "apiRoute": {"type": "object", "additionalProperties": false, "required": ["id", "path", "methods", "auth", "executionScopes"], "properties": {"id": {"type": "string", "maxLength": 64}, "path": {"type": "string", "minLength": 1, "maxLength": 200}, "methods": {"type": "array", "minItems": 1, "maxItems": 5, "uniqueItems": true, "items": {"enum": ["GET", "POST", "PUT", "PATCH", "DELETE"]}}, "auth": {"enum": ["admin", "apikey", "bearer", "public"]}, "executionScopes": {"$ref": "#/$defs/ids"}, "summary": {"type": "string", "maxLength": 500}}},
    "ids": {"type": "array", "maxItems": 64, "uniqueItems": true, "items": {"type": "string", "minLength": 1, "maxLength": 200}},
    "path": {"type": "string", "minLength": 1, "maxLength": 240, "pattern": "^[A-Za-z0-9_][A-Za-z0-9_./-]*$"}
  }
}
```

### 6.3 编译期及启动语义校验

- CI验证唯一ID、合法semver/history、metadata目录匹配、未知字段/scope、selector与扩展描述符；icon只能本地受限格式，渲染不执行SVG脚本。每项metadata≤128KiB、目录≤500项、列表分页≤200，资源默认值由Host限额裁剪。
- registry工厂静态实现sdk.Plugin；Host adapter静态实现SDK约定Host接口。sdk.ContractVersion与schemaFingerprint为唯一合同版本；启动校验所有纯metadata与主程序已实现能力集合，UI资源build清单一致。未实现scope/扩展显示unsupported，不能执行或假成功。
- UI mode=vue需entry位于本插件ui/dist、无绝对路径/../符号链接/外部动态import与脚本URL、ui.trusted确认；json模式不自动执行ESM。Host构建阶段检查资源与导出，不在运行期从外部网络补缺。
- metadata、注册工厂、编译期导出的资源清单不一致应阻止该构建发布；意外运行期不一致时仅插件子系统fail closed并报告，不猜测兼容。核心媒体链路继续内置逻辑。
- 重复ID或改ID视为不同定义，不把其他实例配置隐式迁移；同ID代码变更必须提升插件version并记录history，pin更新本身不授予新scope。依赖安全更新也随module pin和镜像发布，不在app运行期安装。

## 7. 权限 scope 全表与 Host API

### 7.1 共同规则

- 以下是 **拟实现的封闭注册表**，不是运行时字符串前缀任意放行。`events.chain.<type>`、`events.subscribe.<type>`、`module.<id>` 仅能展开为 §9/§10 的已登记值；`library.*` 等只作 UI 分组标题，manifest 不接受 `*`。
- R=读，W=写/排队，D=危险（可能删除、移动、执行或改变安全行为）。一个 API 若同时需要两项 scope，必须全部满足；selector 只可缩小范围。所有业务 write 必须审计并带 expectedRevision/operationKey；返回run/job ID不等于操作完成。后台插件使用单独PluginPrincipal，由宿主从批准scope/selector计算权限；调用用户数据/NextUp等用户域服务必须绑定有效授权主体，不能伪造media_users记录或相信插件传入VisibleLibraries；无用户主体时只开放安全管理目录投影，不颁发媒体令牌。
- 请求上下文：`instance + definitionFingerprint + grantRevision + generation + invocationId + parentInvocationId + trace + deadline + phase + actor`，全部来自宿主。管理 UI actor 也不能替插件申请未批准 scope；public API actor 进一步收窄，不能继承整个管理员主体。
- owner/媒体库/目录/站点 selectors 由安装页从真实对象选择后存 UUID，不接受任意云路径/connection 明文。Host adapter 对 ID 再查存活、权限与 revision；插件自带 ID 不是授权。
- 跨实例 KV/配置、管理员账户/数据库/Redis/主密钥/115授权/STRM原文或临时直链不在本表可读取范围。`settings.read` 仅 key allowlist、`integrations.read` 仅安全摘要；所谓“内部信息全覆盖”不是“dump 所有对象/凭据”。
- `phase=describe/chain/module-read` 只允许纯读；候选筛选在锁/事务外执行，返回 patch 后宿主重新校验资源版本。不得从 write 后 hook 回到原事务内再次执行危险动作。

### 7.2 scope → Host API/扩展点（完整注册表草案）

| scope | 拟允许的方法/扩展 | 级别/约束 |
|---|---|---|
| `plugin.config.read` | `plugin.config.get` | R，仅本实例 |
| `plugin.config.write` | `plugin.config.update` | W，revision CAS，不自动扩大运行权限 |
| `plugin.data.read` | `plugin.data.get/list` | R，分页≤200 |
| `plugin.data.write` | `plugin.data.put/delete` | W，key≤128字符，value≤128KiB，默认总10MiB |
| `plugin.state.read` | `plugin.state.get`、`runtime.capabilities` | R，仅本实例、非秘密限额/状态 |
| `plugin.instances.read` | `plugin.instances.list` | R，只有 id/name/version/state，不返回其他插件配置 |
| `fs.data` | `plugin.data.path`、`fs.data.list/read/write/remove` | W/D，本实例 data/tmp；remove 需相对路径和 owner 检查 |
| `fs.media:read` | `fs.media.roots/list/stat/readText` | R，已选 root；文本≤64KiB，排除私有/STRM/授权根；不是任意媒体字节代理 |
| `fs.media:write` | `fs.media.preview/apply` | D，移动/删除均走 filemanager plan+confirm；不直接改变 media 身份 |
| `system.exec` | `system.exec.start/get/cancel` | D，只在同UID实例worker及其工作目录内执行（文件隔离为尽力而为）；每次 max60s、stdout64KiB/stderr16KiB |
| `system.shell` | `system.exec.start(kind=shell)` | D，还需 system.exec；显式支持 `/bin/sh -c`，不使用宿主环境/身份 |
| `network` | `network.http`、允许插件原生网络库 | D，明确 raw 网络的受信风险；Host HTTP 仍有 SSRF/超时/大小校验 |
| `network.private` | `network.http` 的已选内部 origin | D，配合 network 与安装时精确 origin；不包含 DB/Redis/管理秘密入口。不能保证约束已经授权的 raw socket |
| `network.proxy.use` | `network.http(profile=host)` | D，宿主代理配置只内部使用，不返回代理密码 |
| `library.read` | `library.assets/candidates/history/scans.get/list` | R，安全 DTO、selector |
| `library.scan` | `library.scan.start/control` | W，目录与 revision校验 |
| `library.recognize` | `library.recognize/suggest` | R/W（生成建议），不得静默 Confirm/Correct |
| `library.organize` | `library.organize.preview/confirm`、`library.correct.preview/apply` | D，使用现有证据、显式确认和 fencing |
| `library.refresh` | `library.refresh` | W，按库/资产ID，入队不做实时全库扫描 |
| `library.history.write` | `library.history.hide` | W，只隐藏历史，不删除媒体 |
| `library.archive.read` | `library.archive.list/preview` | R，返回路径脱敏或 handle |
| `library.archive.delete` | `library.archive.confirmCleanup` | D，现有归档/保留期/确认保护不变 |
| `classification.read` | `classification.get/match/render` | R |
| `classification.write` | `classification.update` | D，走 catalog.SavePolicy，不能直接 UPDATE 分类/库表 |
| `metadata.read` | `metadata.parse/normalize/search/details` | R，复用 TMDB/缓存/代理/限流；无 token/原始client |
| `metadata.refresh` | `metadata.refresh` | W，有界入队，仅相关资产 |
| `subscriptions.read` | `subscriptions.list/get/history` | R |
| `subscriptions.write` | `subscriptions.create/update/queue/delete` | W/D，删除与业务规则变更需revision；只改指定订阅 |
| `subscriptions.rules` | `subscriptions.evaluateRules/coverage` | R，纯函数校验；不能伪造已有文件证据 |
| `search.read` | `search.list/get/resources` | R，页≤200，资源用 opaque ID |
| `search.run` | `search.start` | W，复用限流/缓存/来源快照 |
| `downloads.read` | `downloads.list/get/progress/files/trackers` | R，不返回下载器密码/带passkey URL |
| `downloads.add` | `downloads.addResource/manualCreate` | W/D，来源快照、HR/重复/目标路径验证不可跳过 |
| `downloads.control` | `downloads.pause/resume/retry/recheck` | W，先检查对应实例允许管理的 downloader/task |
| `downloads.delete` | `downloads.cleanup.preview/approve` | D，停止/删任务/删文件分开，复用现有审批，不隐式 deleteFiles=true |
| `sites.read` | `sites.list/get/metrics/rss.discover` | R，定义摘要/账户 handle，不返回Cookie/passkey/原始集成设置 |
| `sites.request` | `sites.request/checkin` | D，用 accountId 由宿主附加站点认证、精确站点origin和相对path；不返回认证header/Set-Cookie；checkin是F3拟新增service，非现有能力 |
| `sites.content.raw` | `sites.request` 返回原始有界HTML | D，可能包含站点页面私有字段，需醒目同意；仍不交付宿主/115凭据，优先 typed result |
| `sites.refresh` | `sites.metrics.collect/rss.refresh` | W，复用 sitemetrics/scheduler |
| `sites.templates.read` | `sites.templates.catalog/loaded` | R，仅已选定义及公开编译目录 |
| `sites.templates.select` | `sites.templates.load` | D，只选择 pinned compiled Go definitions，不能导入任意远程 Go/继承库 |
| `integrations.read` | `integrations.list/status/test` | R/W(test)，只安全 Record；无 Reveal/授权二维码流程 |
| `storage.read` | `storage.roots/list/stat` | R，受选 connection/root约束，无稳定/临时下载URL |
| `storage.write` | `storage.mkdir/relocate/preview/apply` | D，复用 filemanager；不能请求115授权/全盘删除 |
| `strm.read` | `strm.entries/diagnostics` | R，仅资产ID/状态/相对输出摘要，**不返回正文/apiKey/pickCode** |
| `strm.repair` | `strm.repair.preview/approve/retry` | D，走现有 STRM repair工作流，scope不授权任意覆盖/迁移 |
| `mediaserver.read` | `mediaserver.providers/libraries/items/sessions` | R，按授权库安全DTO；无Playback URL、用户令牌 |
| `mediaserver.refresh` | `mediaserver.refresh` | W，Emby和原生采用其既有队列能力 |
| `mediaserver.users.read` | `mediaserver.users/devices/history` | R，用户摘要/权限；不返回密码hash、令牌、IP明细默认隐藏 |
| `mediaserver.users.write` | `mediaserver.users.update/revokeDevice` | D，不允许插件提取现有密码/颁发管理员凭据；新用户密码不从Host读回 |
| `mediaserver.settings.write` | `mediaserver.settings.update` | D，revision与端口/监听安全验证，不能注册管理API到媒体端口 |
| `mediaprobe.read` | `mediaprobe.status/list` | R，只解析快照/安全错误码 |
| `mediaprobe.retry` | `mediaprobe.retry/stills.retry/gc` | W/D，入既有队列；插件不能调用ffx取链/执行封装 |
| `notifications.send` | `notifications.send` | W，PostMessageDto→outbox→messaging，去重/标题正文上限 |
| `notifications.read` | `notifications.deliveries` | R，自己的发送结果摘要，不返回渠道凭据 |
| `notifications.retry` | `notifications.retry` | D，未知发送结果仍需 acknowledgeDuplicateRisk |
| `settings.read` | `settings.get` | R，只选非秘密key（模板/缓存/刮削/公开地址等） |
| `settings.write` | `settings.patch` | D，只选key、revision，保留原校验/专用域名保存流程；主密钥/DSN/授权/安全监听不属于key集合 |
| `jobs.read` | `jobs.list/get` | R，本实例派生任务或经资源scope批准的任务，payload安全投影 |
| `jobs.cancel` | `jobs.cancel` | D，新增 typed service 仅支持已登记可取消业务，不允许任意kind/queue操作 |
| `cache.read` | `cache.status/list` | R，安全聚合，不返回任意Redis key/value |
| `cache.invalidate` | `cache.invalidate` | W，限定TMDB/搜索/本实例命名空间，不 FLUSHDB/FLUSHALL |
| `system.read` | `system.info/health/modules` | R，不返回环境/日志原文/凭据 |
| `maintenance.read` | `maintenance.health/alerts` | R，安全状态；备份内容、恢复/发布/主密钥操作不暴露 |
| `events.subscribe.<type>` | `events.subscribe` + §9广播类型 | R/被动注册，资源scope限制event payload；未知类型拒绝 |
| `events.publish` | `events.publish` | W，仅 `plugin.<plugin-id>.*` 自定义事件，不能伪造核心下载/入库成功 |
| `events.chain.<type>` | `events.chain.register` + §9可用链 | D，优先级/patch类型固定；不是任意核心权限hook |
| `module.<id>` | `modules.register` + §10 id | D，替代或聚合型由宿主声明，不能任意Go函数名 |
| `ui.json` | 表单/页/多key仪表板描述符、`ui.get` | R，树/props/model受限 |
| `ui.trusted` | 同页ESM、`ui.assets` | D，等同信任管理员页面JS；不声称scope隔离 |
| `ui.sidebar` | `ui.sidebar.register` | W，只有实例命名空间下的页，不能覆写核心导航 |
| `api.register` | `api.register`/`plugin.invoke(kind=api)` | W，路由仅实例前缀 |
| `api.public` | auth=public 路由 | D，必须额外逐路由确认；默认只读Host API |
| `api.public.write` | public路由所列 `executionScopes` 的业务写 | D，仍须对应业务scope；不能转发管理员会话/通用Host proxy |
| `services.register` | `services.register/run/status` | W，持久cron/interval/date/once |
| `commands.register` | `commands.register` | W，消息来源鉴权，不抢占内置命令 |
| `commands.override` | 冲突命令的显式替换 | D，管理员选择；内置安全/登录命令不可替换 |
| `actions.register` | `actions.register/invoke` | W/D，F2元数据/单动作invoke，动作仍需其业务scope；不是工作流Runner |
| `workflows.run` | `workflows.run/status/cancel` | D，F4-0最小宿主，F5 UI；仅已批准workflowIds，按已确认DAG和各动作scope执行 |
| `auth.provider` | `auth.providers.register/verify` | D，F4-0显式启用外部认证流程；不能仅返回isAdmin=true |
| `agent.tools` | `agent.tools.register/invoke` | D，F5描述符/人工工具调用；无自带LLM平台 |

广播scope取§9的V2事件名（如`events.subscribe.library.organized`），链scope取固定wire名（如`events.chain.name.recognize`）。N/A类型不允许订阅/拦截；SDK可导出贴近MP的枚举名，但不接受旧payload。广播/链式类型、模块 ID 的具体展开是完整表的一部分，不允许申请 `events.*`、`module.*`。若 scope 尚未实现，`runtime.capabilities` 明确报未支持，安装启用不得静默通过。

### 7.3 逐模块 Host API 与真实 Go 服务落点（42 个顶层目录）

表中 RPC 均从 §7.2 取 scope；标“新增 adapter/service”表示需 F3 实现，不能直接 import 私有实现到 SDK。所有列表分页≤200、文本查询≤200字符、批量写≤200（业务已有更低上限从其规定）。

| 模块 | 拟暴露 RPC、R/W/D | 现有落点/需补的服务接口 | 阶段 |
|---|---|---|---|
| accounts | 无管理员密码/Session/注册/改密 Host API；外部认证另走 auth broker | `accounts.Repository.Authenticate/ChangePassword` 只由内置账户服务调用，不暴露给插件 | 不开放；F4-0 opt-in broker |
| acquisition | downloads.list/progress R；addResource/control/retry W；cleanup.preview/approve D | `Engine.List/StartExpectedFromRun/Control/ApproveCleanup`，不能暴露 Job Handler Add/Transfer 让插件造内部payload | F3-A |
| cacheview | cache.status/list R；invalidate W | `Repository.List/Get/Delete/Clear` 增加安全命名空间 Service，不全表清缓存 | F3-D |
| catalog | mediaserver.items R，首页/详情安全DTO | `catalog.Engine.List/ListParent/Home/Item` 需主体上下文；不提供 Image/Playback 原始HTTP客户端 | F3-C |
| classification | get/match/render R；update D | `Repository.Get`、`Policy.Match/Render`；保存必须 `nativelibrary/catalog.Repository.SavePolicy` | F3-B |
| dataview | **不开放**任意 Tables/Rows/RedisValue/RawValue | `dataview` 是管理员原始数据视图，不能作为插件信息接口 | N/A：违反A4/秘密边界 |
| downloads | metadata.torrent.parse R | `downloads.ParseTorrent` 纯数据；不能返回torrent认证tracker URL原值 | F3-A |
| emby | 不暴露 Gateway/Proxy/ManagedLink/MetadataHeaders；刷新/查询经 mediaserver | `Gateway` 只供主程序请求链；安全外观服务见 library/catalog | F3-C桥，不开放handler |
| embyapi | 不开放 token解析/RequestContext/Router/ServeHTTP | 媒体端口保持封闭；插件API只注册到管理监听器 | N/A：不能劫持认证/播放 |
| externalapi | 不开放全局Create/Revoke/Admit；插件API使用独立实例凭据 | `externalapi` 的既有全局key不是插件bootstrap凭据 | F2专属plugin_api_keys |
| filemanager | fs.media.roots/list/stat R；preview R；apply D | `Engine.Roots/List/Get/Preview/Execute/PreviewBatch/ApproveBatch`；复制/删除仍保留证据与确认 | F3-B |
| integrations | integrations.list/status/test R/W | `Repository.List` 安全Record、`Manager.Test`；禁止 `Get/Default/Reveal` 返回 Credentials，禁止115授权入口 | F3-D |
| jobs | jobs.list/get R；cancel D | `Repository.Browse/Detail` 安全DTO；新增按owner/可取消kind的Cancel Service，不暴露Create任意kind、Claim/Finish | F1本实例、F3-D业务 |
| library | assets/candidates/history/scans R；scan/recognize/refresh W；organize/correct/archiveCleanup D | `Engine.ManagedAssetsKind/CandidatePage/History/Scans/Start/Recognize/Suggest/Confirm/Correct/QueueRefresh/PreviewArchiveCleanup/ConfirmArchiveCleanup` | F3-B |
| linkcache | 无原始Resolve/直链/Redis访问；仅cache安全状态/失效 | 作为 integrations/playback 的内部缓存，不返回签名URL | 不直接开放 |
| maintenance | maintenance.health/alerts R | `system.Inspect` / `maintenance.Engine.Alerts` 安全投影（maintenance没有Engine.Health方法）；不开放Backup/Restore/Claim/Finish/RegisterRelease/数据库dump/主密钥操作 | F3-D |
| manualdownloads | downloads.manualCreate/retry W；list/progress R | `Engine.Create/List/Retry`，只接受经验证的资源/磁链/受限上传句柄，不执行任意本地路径 | F3-A |
| mediaprobe | status/list R；retry/stills.retry/gc W/D | 子包 probequeue/stillqueue/stillrunner 的管理Service，复用既有HTTP处理使用的入口；SDK不导入ffx/linksource | F3-C |
| mediaserver | providers/libraries/items R；refresh W | `mediaserver.Registry.Descriptions/Provider` 只在host内部，DTO从现有Facade取；不调用Provider.Playback返回URL | F3-C |
| mediausers | users/devices/history R；update/revokeDevice D | `Repository.List/Get/Devices/History/Save/Mutate/RevokeDevice` 的脱敏门面；禁止Login/Session/PasswordHash/token | F3-C |
| messaging | notifications.send W；deliveries R；retry D；commands入口 W | `Dispatcher.Prepare/Deliver/List/Retry`；新增 `CommandIngress` 才有入站命令，不把出站Telegram.Send说成入站支持 | F1通知基础、F2命令注册、F4新入站、F3-D |
| metadata | parse/normalize/episodes R | `metadata.NormalizeEpisodes/Episodes/Item.ShowMetadata`；无数据库/网络对象 | F3-B |
| nativelibrary | libraries/items/discovery/NextUp R；按授权UserData可选W | `catalog.BrowseRepository.Query/Views/Discover/PersonalData`；Host必须提供确实有权的actor，不能信任插件传visibleLibraries | F3-C |
| nativeplayback | sessions R安全摘要；不开放取链/StreamURLs/Session激活/媒体中转 | `runtime/active.go:ListActive` / `SessionRepository.List` 现有管理侧列表入口；核心Resolve/URL构造不被插件替换 | F3-C只读 |
| nativetmdb | metadata.refresh W；stored details R | `Engine.Refresh/Repository.EnqueueAsset` + 已持久快照读取；不导出TMDB client/token | F3-B/C |
| networkproxy | network.http(profile=host)；只返回非秘密代理状态 | `Repository.Get` 内部、`networkproxy.Client` 构造HTTP；不返回配置URL、账号、密码或允许插件改宿主代理 | F3-D |
| notifications | notifications.send 的稳定PostMessageDto | 当前只有通知接口/模型；实际投递接 messaging/outbox，不新建第二套通知队列 | F1基础/F3-D |
| playback | 不开放媒体Relay/Playback Engine；只内部复用URL安全策略 | `playback.Policy.Check` 可作Host HTTP网络保护，不将解析URL直接发插件 | 不直接开放 |
| playbackgrants | **不开放** Prepare/Authorize/VerifyManagedURL/Revoke/Rotate 的凭据或稳定URL | 仅主程序对STRM/播放校验内部调用 | N/A：秘密/播放不变量 |
| playeraccess | 不开放UA白名单写、原始访问日志或冒用clientIP；可给aggregate状态 R | 现有 Repository.Policy/Events 由mediaserver安全投影过滤后使用 | F3-C只读摘要 |
| plugins | 本实例config/KV/state/dataPath R/W；自己的services/actions/events W | V2 InstanceService/DataService；未批准跨实例读写/启停其他插件 | F1/F2 |
| releaseattrs | metadata.attributes.parse/validate R | `releaseattrs.Parse/Valid` 纯数据 | F3-B |
| releasename | metadata.name.parse/render R | `releasename.Parse/Render/RenderPath` 保持纯函数；hook在上层业务服务，不在纯包中引入RPC | F2识别/F3-B |
| search | search.start W；list/get/resources R；rule evaluation R | `Engine.StartCached/StartMediaCached/List/GetFiltered/RunResource`；校验原来源快照，隐藏认证下载hint | F3-A |
| settings | settings.get R；patch D | `Repository.Get/Save` + 专用访问域名/分类/原生设置service；key allowlist，不可dump/修改所有设置 | F3-D |
| sitemetrics | sites.metrics R；collect W | `Engine.Collect/GetSchedule/SaveSchedule/Process` 安全门面；不返回站点凭据 | F3-A |
| sites | sites.list/get/templates R；request/checkin D；refresh/load D | `Repository.List/Get/LoadedGoTemplates/LoadGoTemplates`、`Client.Test/Search/Metrics/RSS/DiscoverRSS`；新增受控站点请求/签到Service，Manager内部附认证 | F3-A |
| storage | storage.list/stat R；mkdir/relocate D | `StorageProvider` 的接口由`integrations.DynamicStorage`实现；写入通过filemanager；无Resolve/Upload任意文件/115token返回 | F3-B |
| strm | entries/diagnostics R；repair preview/approve/retry D | `library.Engine.ManagedAssetsKind/PreviewSTRMRepair/ApproveSTRMRepair/RetrySTRMRepair`；不直接开放Builder.Build/ReadOwned/WriteAtomic原文接口 | F3-B |
| subscriptionpolicy | evaluateRules/coverage R | `Evaluate/Filter/TitleMatches/ValidateKeywords` 纯数据；结果不是替代实际下载/入库证据 | F3-A |
| subscriptions | list/get/history R；create/update/queue/delete W/D | `Engine.List/Get/Browse/History/Save/Queue/Delete`；当前状态是收集/评估/运行，不虚构MP“追剧完成”持久状态 | F3-A，完成observer F4 |
| system | system.info/health/modules R | `system.Modules/Inspect` 安全版本/能力摘要；不提供environment/process dump或root命令 | F1基础/F3-D |

**不直接暴露的公共能力与新增门面**：

- `Host.NetworkClient.Request`：3–15s，单响应≤1MiB、跳转最多3且每跳校验origin/解析IP；拒绝私有/metadata地址，除非精确批准 `network.private`；禁止返回任意文件描述符/客户端secret。自身HTTP实现使用既有proxy与requestbudget，不能把client传给插件。
- `Host.Downloads` 的全下载器列表/暂停/标签：当前业务Repository并非覆盖全部已存在种子；F3需新增窄 `DownloaderService`，用 Manager.QB + Adapter.AllHashes/Inspect/SetPaused/DeleteOwned。**DeleteOwned/HR/源文件所有权检查保留**，插件不能绕过现有保护删不属于本程序的种子/媒体。非owned第三方种子默认只读；扩大管理须另立业务ADR。
- `Host.Sites.Request/Checkin`：当前没有通用checkin公开方法。新增typed、相对URL、accountId绑定的服务，不返回Cookie/Authorization/Set-Cookie；插件用解析规则/状态判定而不是取出账号凭据。需要验证码时 needs_action，不绕过站点验证。
- `Host.Library.ChangeFeed`：目录监控用 `Engine.RunChangeWatcher/WatchWindow` 的事件化安全投影 + 可恢复cursor，插件不得各自无界扫描整盘。插件原生监视本地已授权目录也可，但不把云盘远程路径当本地文件。
- `Host.Subscriptions.Completion`：当前 `completeCollection` 仅表示资源集合收集结束，**不是追剧完成**。F4新增只读 CoverageObserver（`subscriptions/completion.go:Observe`，拟新增），以TMDB季集清单 + 可暴露资产 + 订阅revision推导完成/期待集数；结果存插件专属 observation 表/缓存，不改原 subscriptions表含义。无清单时unknown，不能误发完成事件。
- `Host.Metadata.Watchlist`：TMDB可用既有客户端；豆瓣想看没有宿主OAuth/API，F6示例需用户自提供公开列表或插件自己的合法凭据；不能把任意抓取/绕验证码写成现成能力。

## 8. SDK 能力对照与扩展点框架

### 8.0 官方Go SDK接口、Host分包与测试工具（设计草案）

下列是拟定公共接口形态，不是本轮已经写入SDK的代码；不要求作者实现所有可选接口：

```go
// 省略package/import；所有结构都属于官方SDK，非宿主内部Go类型。
type Plugin interface {
    InitPlugin(context.Context, Config) error
    GetState(context.Context) (bool, error)
    StopService(context.Context) error
}
type FormProvider interface { GetForm(context.Context) (Form, error) }
type PageProvider interface { GetPage(context.Context, PageRequest) (Page, error) }
type APIProvider interface { GetAPI(context.Context) ([]API, error) }
type ServiceProvider interface { GetService(context.Context) ([]Service, error) }
type CommandProvider interface { GetCommand(context.Context) ([]Command, error) }
type DashboardProvider interface {
    GetDashboard(context.Context, DashboardRequest) (Dashboard, error)
}
type DashboardMetaProvider interface {
    GetDashboardMeta(context.Context) ([]DashboardMeta, error)
}
type ModuleProvider interface { GetModule(context.Context) ([]Module, error) }
type ActionProvider interface { GetActions(context.Context) ([]Action, error) }
// GetAuthProviders/GetAgentTools等可选接口按F4-0/F5阶段开放。
```

- SDK通过type assertion识别可选接口，不对任意方法做反射。API/Service/Command/Module/Action描述符可绑定typed Go Handler；这些func在wire JSON中标记忽略，只将稳定handler ID注册到对应generation。可选接口未实现则返回unsupported能力或空注册集合，不能凭manifest声明就绕过检测。
- `Runtime.On(EventType, EventHandler)`与`OnChain(ChainEventType, ChainHandler)`使用enum/context/Envelope及生成payload DTO；`events.Typed[T]`适配器负责解码并与事件schema对应。不同枚举到T的映射不是Go语言自动保证，SDK注册时还需校验。
- Host辅助方法示意：`client.SaveData(ctx,key,value,expectedRevision)`、`GetData(ctx,key,out)`、`DelData(ctx,key,expectedRevision)`、`GetDataPath(ctx)`、`PostMessage(ctx,Message)`、`GetConfig(ctx,out)`、`UpdateConfig(ctx,value,expectedRevision)`。所有读写仅本实例、带取消和typed错误；任意JSON只用于插件自有KV，业务对象用专门DTO。
- F3分包对应`host/library`、`host/subscriptions`、`host/downloads`、`host/sites`、`host/metadata`、`host/storage`、`host/strm`、`host/mediaserver`、`host/notifications`、`host/settings`、`host/jobs`、`host/system`及必要子域client。只发布§7真正开放的方法，不为空的42个模块都制造可调用后门。纯wire/transport包与Host facade分离，不能形成SDK→核心DB/runtime依赖环。
- 比如`downloads.Client.List(ctx,ListRequest) (Page[Task],error)`、`library.Client.StartScan(ctx,StartScanRequest) (JobReceipt,error)`、`subscriptions.Client.Create(ctx,CreateRequest) (Subscription,error)`均对稳定DTO封装，不接受任意method字符串作为正常业务API。底层host.call保留协议解析，但未知方法/缺scope由服务端拒绝。
- `plugintest.NewHost`可设置允许scope、resource selector、revision和预期typed调用，并注入deadline、busy、unknown、撤销及反向事件；`ServeHarness`用内存/pipe peer测试真实SDK状态机，golden规范化关联号/时间后比较。它不提供默认真实DB/网络/文件删除实现。
- SDK、Host分包和plugintest都在mediatool-plugin/sdk；单module不另建SDK tag/module或主程序示例目录。作者添加plugins/<id>实现＋纯manifest＋注册工厂；本地用sdk/plugintest的mock Host/stdio harness执行golden，真实隔离通过主程序同一binary的plugin-worker验收。不再生成独立插件包或cmd/plugin发布入口。

### 8.1 MP 接口 → 本项目实现

| MP接口/概念 | 本程序V2对应合同 | 阶段/差异 |
|---|---|---|
| init_plugin / stop_service / close | 必需InitPlugin/StopService，SDK Serve的Close收尾；wire生命周期与generation | F1；回调返回后仍必须收割后台子进程 |
| get_state / get_name / is_clone | GetState/可选GetName/Runtime.Instance.CloneOf；admin desiredState独立 | F1 |
| get_config / update_config | Go GetConfig/UpdateConfig，typed config + revision CAS | F1；默认仅自己 |
| save_data/get_data/del_data及异步变体 | Go SaveData/GetData/DelData，context取消与goroutine并发，无双套异步API | F1；list分页，不一次返回无限数据 |
| get_data_path | Go GetDataPath，返回本实例隔离data根 | F1 |
| post_message | Go PostMessage(Message)→notifications.send→既有outbox/渠道 | F1基础/F3-D完整，渠道密钥不交付 |
| get_form / get_page | 可选GetForm/GetPage，typed JSON组件树/model或ESM描述符 | F1配置骨架；F2通用渲染 |
| get_render_mode | Go可选RenderModeProvider，manifest UI mode与descriptor一致 | F2；不加载任意HTML字符串 |
| get_dashboard / get_dashboard_meta | 可选GetDashboard/GetDashboardMeta，多key、refresh/border/cols/title | F2 |
| 侧栏独立页 | ui.sidebar descriptor→现有 /plugins/:instance/:page | F2；不是新增任意顶级核心路由 |
| get_api | 可选GetAPI返回typed API/本地Go handler；gateway→invoke | F2；wire只传已注册handler ID |
| get_service | 可选GetService，持久cron/interval/date/once→jobs唤醒 | F2；不在插件中自行再创建APScheduler |
| get_command | 可选GetCommand，CommandIngress→注册表→invoke | F2；当前仅出站通知，必须补入站宿主 |
| MP事件注册 / EventManager | Go On(EventType, handler) / OnChain、typed payload及本地注册表 | F2基础/F4逐项，表§9 |
| get_module | 可选GetModule，有限typed方法注册表，无Go任意反射/猴补丁 | F2注册/F4接线，表§10 |
| get_actions | 可选GetActions，typed动作schema/本地Handler与最小Runner | F2单动作注册/invoke；F4-0工作流宿主；F5 UI |
| get_auth_providers | opt-in外部身份broker（新宿主） | F4-0；不允许替代内置安全校验 |
| get_agent_tools | JSON工具描述符+管理员手动工具执行 | F5；无LLM对话宿主，不导入MP工具基类 |
| self.chain / systemconfig / plugindata | 按F3模块分包的typed Go Host client，不给ORM/session/raw dump | F3；业务能力对等，不兼容MP包导入路径 |
| version/author/icon/level/history | 插件仓库manifest纯metadata＋编译registry；UI不显示源码/commit选择 | F1目录合同/F5插件库管理 |
| MP运行时依赖安装 | N/A；Go依赖在插件module审核并由主程序构建解析，随app静态编译；不现场安装 | 内置库决定，不提供依赖安装器 |

### 8.2 API 路由与鉴权

- 业务路由 `/api/v1/plugin/{instance}/{path...}`；管理路由继续复数 `/api/v1/plugins/...`，二者分开。都不在原生媒体端口/Emby网关注册。
- 管理操作走现有管理员认证、CSRF、revision、审计。`admin` 模式插件API同样要求管理员会话；GET应无副作用，写方法有CSRF。
- `apikey`/`bearer` 使用**实例专属**随机256bit凭据（存SHA256摘要，带过期、允许路由/方法、scope revision）；不复用 STRM全局key、媒体用户token、DB账号或管理员cookie。密钥仅创建时向管理员展示一次，不进入worker request。Bearer格式可接受标准 Authorization；冲突来源拒绝。
- `public` 必须单独scope + 逐路由确认，默认拒绝敏感read/write；Public请求`executionScopes`只能缩小实例授权。Host方法能强制此上下文，原生Go/ESM副作用则仍属受信代码风险。
- HTTP RequestDto只传 method、路由相对path、经过schema筛选的query/body、允许header（无Authorization/Cookie/代理密钥）和安全caller metadata；ResponseDto只支持有界JSON/text/可选受控附件handle，不给媒体流/Reader/HLS/转码。默认响应≤1MiB，不支持chunked流式代理。
- API timeout→504/503；busy→503+Retry-After；未知/禁用实例→404；scope不足403；公开端点按IP/实例限流（默认60/min，body≤256KiB）。核心admin/媒体API不能被注册覆盖。

### 8.3 JSON 组件树与 Vue ESM

组件树格式：`{component, key?, props?, model?, when?, events?, children?}`；最多500节点、深度20、序列化≤256KiB；model≤128KiB。可映射的 Vuetify 风格白名单如下：

| 输入名 | 本程序design-system/Element Plus映射 |
|---|---|
| VContainer/VRow/VCol | 既有panel/栅格，cols/md/lg限制1–12 |
| VTextField/VTextarea | ElInput，固定model指针和长度上限 |
| VSwitch/VCheckbox/VSelect/VRadioGroup | ElSwitch/ElCheckbox/ElSelect/ElRadioGroup |
| VBtn/VIcon/VAlert/VChip | ElButton/受控图标/ElAlert/ElTag |
| VCard/VCardTitle/VCardText/VDivider | 既有panel/标题/ElDivider |
| VDataTable/VPagination | ElTable/ElPagination，数据必须分页 |
| VTabs/VTab/VWindow | ElTabs/TabPane的受控子集 |
| VProgressLinear | ElProgress，范围0–100 |
| Text/Markdown | 文本/清洗后的Markdown，无script、事件属性或原始HTML |

- 每组件有 props schema，不透传任意`on*`、`innerHTML`、`srcdoc`、危险URL/style。model采用受限JSON Pointer；拒绝 `__proto__/prototype/constructor`。条件仅 `eq/ne/and/or/not/exists/in` AST，无JS表达式、模板编译或eval。
- events只绑定预登记action/command/API handler与有限payload template；明确busy/error状态、二次确认、ARIA标签、375/768/1440响应式和keyboard导航。不能把插件报错栈当HTML。
- ESM模式：mediatool-plugin/plugins/<id>/ui/dist保存已构建ESM/assets，主程序构建从同一pin加入前端静态资源，命名空间`/plugin-assets/<id>/<uiBuild>/`。不从CDN或运行期网络加载。宿主固定导出`@mediatool/plugin-ui`（Vue/组件库/PluginApiClient），插件仓库构建external这些依赖，避免重复Vue；UI ABI由SDK合同及主程序构建校验，不运行期协商外部版本。宿主在首个module执行前提供固定import-map，把该bare specifier映射到host-build-revision对应的只读runtime facade；facade与主应用共享同一Vite chunk图。禁止用运行时字符串替换/eval来修import，host/plugin版本不匹配就明确拒绝加载。exports `{Config,Page,Dashboard}`，加载路径含插件ID与编译期uiBuild，pin刷新/unmount必须注销listener/timer。
- **同源ESM拥有管理员页面JS的实际能力**。封装API client的scope检查只是推荐通道，不能阻止它直接fetch管理接口。`ui.trusted`确认必须写明这一点；如果不接受这种信任，只有JSON模式可启用，不将同页ESM伪称隔离。
- 仪表板key≤8，`refresh`最少10s，页面不可见时停poll；border/cols/title受schema约束；不同实例相同key组合成`instance:key`，避免冲突。

### 8.4 服务与命令

- cron复用`platform/scheduling.Next`的现有**五字段、Asia/Shanghai**规则；interval最低60s，date为RFC3339单次；一次性任务必须持久化，不靠worker启动时sleep。F2不假称支持任意时区APScheduler对象，后续有需求再扩合同。
- 服务next_at与run lease只有一个调度事实来源；jobs只唤醒，skip-locked领取、先资源背压再lease，fence写回。默认每服务不并行、misfire coalesce为最近一次；任务历史页≤200，保留期可配置。手动触发同样记run，不直接绕开调度并发限制。
- 现有消息渠道优先 Telegram：先核对现有入站能力；本轮未发现接收宿主，新增有界poll/webhook入口延至F4，回调需来源签名/secret和允许chat/user映射；这个**入站通道的共享连接器令牌只由host持有**。默认不开公网匿名Telegram回调。
- 命令以`(channel, command)`登记，冲突由管理员选择；默认命名空间 `/plugin <instanceAlias> <command>`，不抢占系统登录/危险维护命令。消息主体权限、scope和实例状态每次重校验；不能把接收到的一串消息直接送Shell。
- 未具备的微信/钉钉等渠道，不在F2虚构通用入站兼容；可在稳定CommandIngress扩adapter，不改插件协议。

## 9. 事件总线：逐项 MP 映射

### 9.1 投递与链式语义

- 广播 envelope：`contractVersion/eventId/type/schemaVersion/occurredAt/aggregate{kind,id,revision}/actor/origin/correlationId/causationId/data`。数据按事件DTO allowlist重新构造，不转发DB row、旧outbox任意payload或请求header。
- 既有outbox事件用 `outboxId + type` 去重导入 V2 event 表；新业务事件在业务成功提交的事务内 `EmitTx` 插入插件专属event表。只在实际外部确认后发“完成”；未确认/超时不能伪造success。该SQL不是在事务里调用插件。
- delivery按`(eventId,instanceId,handlerId,subscriptionRevision)`唯一；先取worker额度再领取lease，默认60s、heartbeat、attempt fence。明确idempotent handler可至少一次重试；非幂等处理在断连/过期后unknown，管理员确认后才重放；不承诺exactly-once。
- 新订阅有生效cursor，不能用实例updated_at当全部事件过滤界线，避免改配置漏事件；回放是明确、限量、有资源授权的管理动作。单次claim≤100，每实例pending默认1000，全局10万，超限保留durable source并报警，不吞记录。
- 链式handler按priority降序，同优先级`instance UUID/handler ID`稳定排序。输入快照含resource revision，返回`{decision:pass|replace|reject, patch, reasonCode}`；patch只允许该事件schema字段。核心ID/源文件指纹/授权/所有权/域名白名单不可改。
- 链式不在事务/全局锁内RPC。需要锁的业务先prepare→hook→重取锁/CAS核对原revision→执行；revision变化重新prepare，不能把旧patch用到新文件。链超时/崩溃/非法结果丢弃patch并回落内置逻辑。
- **副作用例外**：写型hook一旦可能已经发出外部请求，超时是unknown，不自动再执行内置写动作。只有纯读或可证明未发送/持有相同幂等键且provider支持幂等时才安全回落；否则会重复下载/通知。这是对A5“失败回落”必须明确的安全限定。
- context有visited handler/module、深度≤8、事件因果链≤16；不允许插件重发reserved核心事件。Host API递归进入同一module可用受控`modules.callBuiltin`（只对当前module/当前调用一次），不得死锁或无限递归。
- F2只先接3个真实点：`NameRecognize`链（工作台和入库识别入口）、`DownloadAdded`广播、`TransferComplete`广播（托管STRM发布完成）。其余注册类型可留合同，但未接线明确not_supported，不虚假ACK。

下表中的业务函数是**拟插入hook的位置**，不代表它现在已发MP事件。非本程序业务的N/A不做假事件；对应差异列入最终验收。

### 9.2 EventType：31/31

| MP EventType（wire） | V2语义/映射 | 触发点：文件:函数（现有或注明新增） | 阶段/差异 |
| `PluginReload` (`plugin.reload`) | plugin.reloaded | 拟新增 `plugins/runtime.Supervisor.Reload`，新generation成功发布后 | F2；崩溃本身不是成功reload |
| `PluginAction` (`plugin.action`) | plugin.action.invoked | 拟新增 `plugins/extensions.ActionDispatcher.Invoke` | F2单动作/F4-0工作流；目标实例限定 |
| `PluginTriggered` (`plugin.triggered`) | plugin.custom.triggered | 拟新增 `plugins/events.PublishCustom` | F2；只插件自定义命名空间 |
| `CommandExcute` (`command.excute`) | command.executed | 拟新增 `messaging/commands.go:CommandIngress.Dispatch` | F2；保留上游拼写对照，host事件采用规范命名 |
| `SiteDeleted` (`site.deleted`) | site.account.deleted | `integrations/repository.go:Repository.Delete`，kind=ptsite提交后 | F4-A；模板定义删除不冒充账号删除 |
| `SiteUpdated` (`site.updated`) | site.account.updated | `integrations/repository.go:Repository.Save`，ptsite安全字段diff | F4-A；无Cookie/passkey |
| `SiteRefreshed` (`site.refreshed`) | site.metrics.refreshed | `sitemetrics/collector.go:Engine.Collect` + `storage.go:Engine.Save` | F4-A；指标成功持久后 |
| `TransferComplete` (`transfer.complete`) | library.organized | `acquisition/transfers.go:Engine.ImportFile`，`library/journal.go:Engine.FinalizeRecovered` | F2；现有media.imported桥；只STRM/托管输出发布，不宣称复制整个影片 |
| `TransferFailed` (`transfer.failed`) | library.organize.failed | `acquisition/transfers.go:Engine.Transfer`、`library/journal.go:Journal.Apply`终态 | F4-B；不把可恢复中间态误当永久失败 |
| `SubtitleTransferComplete` (`transfer.subtitle.complete`) | library.subtitles.copied | `library/artifacts.go:CopySubtitlesOutputsVerified`成功结果由上层commit发出 | F4-B；仅实际发生字幕复制时，不宣称cloud_files已索引外挂字幕 |
| `SubtitleTransferFailed` (`transfer.subtitle.failed`) | library.subtitles.failed | `library/artifacts.go:CopySubtitlesOutputsVerified`错误经上层操作终态提交 | F4-B；不在纯helper内发RPC |
| `AudioTransferComplete` (`transfer.audio.complete`) | N/A | `metadata/episodes.go:IsVideo`：现有整理对象是视频 | N/A；没有音频整理宿主；插件自有音频任务只能发自定义事件 |
| `AudioTransferFailed` (`transfer.audio.failed`) | N/A | 同上，无对应音频任务 | N/A；不得由视频失败伪造 |
| `DownloadAdded` (`download.added`) | download.added | `acquisition/downloads.go:Engine.transition` 的download.started确认点 | F2；qb添加成功，不是仅创建pending记录 |
| `HistoryDeleted` (`history.deleted`) | library.history.hidden | `library/history_manage.go:Engine.HideHistory` | F4-B；本程序隐藏历史，不宣称物理删除媒体 |
| `DownloadFileDeleted` (`downloadfile.deleted`) | download.files.deleted | `acquisition/cleanup.go:Engine.Cleanup`经读回确认 | F4-B；deleteFiles=false不发此事件 |
| `DownloadDeleted` (`download.deleted`) | download.task.deleted | `acquisition/control.go:Engine.Control`、`cleanup.go:Engine.Cleanup`成功确认 | F4-B；已请求/结果未知不是已删除 |
| `UserMessage` (`user.message`) | messaging.inbound | 拟新增 `messaging/commands.go:CommandIngress.Receive` | F2仅注册/已有入站接线；若新增bot轮询则F4，鉴权后默认只选定渠道/发送者 |
| `WebhookMessage` (`webhook.message`) | plugin.webhook.received | 拟新增 `httpapi/plugin_v2_routes.go:pluginRoute`，已注册webhook route通过鉴权后 | F2；不是任意共享无认证入口 |
| `NoticeMessage` (`notice.message`) | notification.queued | `messaging/dispatcher.go:Dispatcher.Prepare`；拟新增NotificationService.Send的提交点 | F4-D；queued不等于已送达，可另有delivered事件 |
| `SubscribeAdded` (`subscribe.added`) | subscription.created | `subscriptions/repository.go:Engine.Save`，新建成功提交 | F4-A |
| `SubscribeModified` (`subscribe.modified`) | subscription.updated | `subscriptions/repository.go:Engine.Save`，已有revision变化提交 | F4-A |
| `SubscribeDeleted` (`subscribe.deleted`) | subscription.deleted | `subscriptions/repository.go:Engine.Delete`提交 | F4-A |
| `SubscribeComplete` (`subscribe.complete`) | subscription.coverage.completed | 拟新增 `subscriptions/completion.go:CoverageObserver.Observe`；读TMDB清单+可暴露资产 | F4-C；不是completeCollection，unknown不发完成，观测存插件专属表 |
| `SystemError` (`system.error`) | system.operation.failed | `jobs/worker.go:Worker.Run`终态错误与拟新增安全错误总线 | F4-D；聚合/节流，排除plugin日志递归，原始异常不扩散 |
| `MetadataScrape` (`metadata.scrape`) | metadata.scraped | `library/scraping.go:Engine.EnrichScraping` + `PersistScrapeReport`提交；`nativetmdb/engine.go:Engine.Process`成功 | F4-C；不在浏览热路径现场刮削 |
| `ModuleReload` (`module.reload`) | module.registry.changed | 拟新增 `plugins/extensions.ModuleRegistry.Publish` | F2；generation原子更新 |
| `ConfigChanged` (`config.updated`) | settings.changed | `settings/settings.go:Repository.Save`、`settings/native_management.go:SaveNativeManagement`；插件config单独事件 | F4-D；只非秘密diff+revision |
| `MessageAction` (`message.action`) | messaging.action.received | 拟新增 `messaging/commands.go:CommandIngress.HandleAction` | F2仅已有入站；新增bot轮询F4，回调签名/过期/一次性nonce |
| `WorkflowExecute` (`workflow.execute`) | workflow.started | 拟新增 `plugins/workflows.go:WorkflowRunner.Start` | F4-0；当前无工作流执行宿主 |
| `AgentTokensUsage` (`agent.tokens.usage`) | N/A | 当前无LLM调用/计费宿主 | N/A；人工工具调用不能产生虚构token统计 |

### 9.3 ChainEventType：19/19

| MP ChainEventType（wire） | V2允许的决策 | 触发点：文件:函数（现有或注明新增） | 阶段/差异 |
|---|---|---|---|
| `PluginDataReset` (`plugin.data.reset`) | 允许/拒绝本实例清理，拒绝说明有界 | 拟新增 `plugins/repository.DataService.Reset`清理前 | F2；管理员强制清理仍需明确确认，不影响其他实例 |
| `NameRecognize` (`name.recognize`) | 补充/替换解析Hint | `httpapi/workbench.go:parseReleaseName`、`library/recognition.go:Engine.Recognize`事务前 | F2；内置Parse回落、ID/路径不可改 |
| `AuthVerification` (`auth.verification`) | 外部provider身份验证结果；不是跳过内置密码校验 | 拟新增 `plugins/authbroker.go:Broker.Verify` | F4-0；当前无provider broker；只能已授权provider/绑定subject |
| `AuthIntercept` (`auth.intercept`) | 内置身份通过后的可选拒绝/附加约束 | 拟新增 `plugins/authbroker.go:Broker.PostVerifyPolicy` | F4-0；不能将401变成成功，媒体令牌与CSRF不受此hook控制 |
| `CommandRegister` (`command.register`) | 建议附加命令/冲突策略 | 拟新增 `plugins/extensions.CommandRegistry.Build` | F2；reserved命令不可覆盖 |
| `TransferRename` (`transfer.rename`) | 改输出相对路径模板结果 | `library/confirm.go:Engine.Confirm`、`acquisition/transfers.go:Engine.ImportFile`的prepare阶段 | F4-B；逐段root/冲突/owner验证，不能在事务中等RPC |
| `TransferRenameBuild` (`transfer.rename.build`) | 对typed模板输入给出建议 | `library/template_outputs.go:ApplyOutputTemplates`上层的拟新增prepare service | F4-B；现helper接tx，不在其内部调用worker |
| `TransferIntercept` (`transfer.intercept`) | 可拒绝该整理计划/说明原因 | `library/confirm.go:Engine.Confirm`、`filemanager/batch.go:Engine.ApproveBatch`进入写事务前 | F4-B；不能放行核心不允许的操作 |
| `TransferOverwriteCheck` (`transfer.overwrite.check`) | 可建议跳过/拒绝，不可放行越权覆盖 | `library/output_layout_guard.go:CheckOutputLayout`上层prepare | F4-B；宿主最后保护检查永远保留 |
| `ResourceSelection` (`resource.selection`) | 对候选ID排序/有界筛选 | `subscriptions/evaluate.go:Engine.Run`、`search/filters.go:Engine.GetFiltered`上层 | F4-A；只可选原始合法候选，不能构造未知下载源 |
| `ResourceDownload` (`resource.download`) | 调整允许的下载选项/拒绝 | `acquisition/downloads.go:Engine.StartExpectedFromRun`、`manualdownloads/tasks.go:Engine.Create`准备阶段 | F4-A；HR/来源快照/所有权仍由内置服务强制 |
| `DiscoverSource` (`discover.source`) | 附加Provider的typed发现候选 | `catalog/home.go:Engine.Home`上层拟新增SourceRegistry、`metadata`搜索门面 | F4-C；后台缓存/分页，不在客户端首页同步等待所有worker |
| `MediaRecognizeConvert` (`media.recognize.convert`) | 合法媒体候选转换建议 | `library/recognition.go:Engine.Recognize`、`confirm.go:Engine.Suggest` | F4-C；TMDB类型/id组合校验 |
| `RecommendSource` (`recommend.source`) | 附加推荐Provider | `catalog/home.go:Engine.Home`上层SourceRegistry | F4-C；当前只有媒体库首页，需新adapter，不假装已有推荐引擎 |
| `WorkflowExecution` (`workflow.execution`) | 对动作上下文建议/拒绝 | 拟新增 `plugins/workflows.go:WorkflowRunner.BeforeStep` | F4-0；在run事务外等待，之后fence核对 |
| `StorageOperSelection` (`storage.operation`) | 从已授权连接/根中选一个handle | `filemanager/engine.go:Engine.Preview`、`library/scan.go:Engine.Start`上层prepare | F4-B；不能返回凭据或任意根路径 |
| `AgentLLMProvider` (`agent.llm.provider`) | N/A | 本程序无Agent/LLM provider宿主 | N/A；F5只tool registry，不顺带承诺LLM运行时 |
| `SubscribeEpisodesRefresh` (`subscribe.episodes.refresh`) | 补充期待季集清单 | 拟新增 `subscriptions/completion.go:CoverageObserver.ExpectedEpisodes`，复用TMDB Episodes | F4-C；不能标记不存在文件为已拥有 |
| `SubscribeCompletionCheck` (`subscribe.completion.check`) | 建议完成/拒绝完成 | 拟新增 `subscriptions/completion.go:CoverageObserver.Check` | F4-C；必须再有宿主文件证据，不能凭插件布尔值禁用订阅 |

N/A汇总：两个音频整理广播、AgentTokensUsage、AgentLLMProvider（4项）无当前宿主；不为“覆盖率”捏造。其余46项都有明确阶段/拟插入点；其中auth/workflow/订阅完成/入站消息等是**需要新最小宿主**，不是简单转发现有事件。F4验收要逐项验证46项的调用者与反例，不能只测试枚举字符串。

### 9.4 未落地事件的验收清单管理

F1把本表导出为纯contract枚举与coverage fixture；F2/F4用一份 `testdata/contracts/plugin-v2-event-map.json` 记录 `implemented/deferred/not_applicable + producer + schema + tests`。每个新增 producer 对应成功、失败、重复、revision变化及未授权不投递的用例；N/A须保持具名原因。稳定别名不意味着照搬MP payload；SDK按本项目DTO书写。

## 10. 模块覆盖、动作、认证和工具

### 10.1 可覆盖的模块方法注册表（有限集合）

| module id / scope | 合同与业务入口 | 执行/失败规则 | 阶段 |
|---|---|---|---|
| `metadata.name_recognize` / `module.metadata.name_recognize` | 输入文件名+安全解析上下文→Hint；`workbench.parseReleaseName`/`library.Engine.Recognize` | 纯读，默认200ms，总2s；非法结果/超时回内置Parse | F2 |
| `metadata.identify` / `module.metadata.identify` | 规范媒体候选→metadata.Item；`library.Engine.Recognize/Suggest` | 纯读，2s；TMDB身份/类型/范围重校验 | F4-C |
| `notifications.channel` / `module.notifications.channel` | 已授权通知→DeliveryReceipt；`messaging.Dispatcher.Deliver`的Provider adapter | 后台10s；可证明未发送才回内置，发送结果未知不重复发送 | F4-D |
| `downloads.provider` / `module.downloads.provider` | Probe/List/Inspect/Add/Pause/Delete的typed downloader合同；Manager.QB调用点上层 | 读15s/写30s后台job；不能用插件假结果绕过delete-owned/HR；未知写入不自动换provider重试 | F4-A |
| `sites.search` / `module.sites.search` | 已选account+query→SearchPage；`search.Engine.Process`/`sites.Client.SearchAll`上层 | 后台30s，并行提供者上限4；失败只该provider失败，可用内置同账户搜索安全回落 | F4-A |
| `mediaserver.refresh` / `module.mediaserver.refresh` | provider/library/asset→RefreshReceipt；`library.Engine.Refresh`/`acquisition.Engine.RefreshEmby` | 后台30s；requestId幂等，未知结果需核对，不能多次扫库 | F4-D |
| `library.output_plan` / `module.library.output_plan` | typed metadata→相对输出计划；`ApplyOutputTemplates`上层prepare | 纯读2s，roots/重复/owner/文件指纹验证，核心WriteAtomic/Journal不能覆盖 | F4-B |
| `storage.selector` / `module.storage.selector` | 已批准candidate handles→selected handle；`filemanager.Engine.Preview`上层 | 纯读200ms；不返回connection对象 | F4-B |

修正 §5 的默认module budget：2s用于普通快速模块；表中后台provider显式15/30s是非热路径独立job预算，不能把同步链总2s扩成全局30s。注册表保存在宿主，不接受动态Go符号名或任意method反射。

优先级[-100,100]，高者先；tie按instance/handler ID。覆盖型first-valid wins，聚合型需显式module定义（如搜索多Provider合并），不能所有方法都隐式取首个。每次改变registry原子生成版本，已有请求使用固定snapshot；失败熔断只该module/instance。插件可通过受控callBuiltin调用本模块内置实现一次，防止Host调用自己递归。

**不可覆盖**：数据库/Redis/Vault、管理员密码验证、CSRF、媒体用户TokenAuthenticator/PermissionChecker、115授权/逐文件grant、STRM原文验证、native stream/PlaybackInfo URL构造、播放UA/限流、ffx执行入口、备份恢复与主密钥轮换。权限校验在模块选择前后均保留。该限制不妨碍对标插件使用这些业务的受控服务。

### 10.2 最小工作流动作（F4-0基础，F5完善UI）

当前无通用工作流宿主。新增ActionDescriptor：`id/name/inputSchema/outputSchema/effectKind/requiredScopes/handler/maxDuration`；ActionContext仅有本workflow允许的resource handles、前序输出与deadline，不含DB/凭据。Workflow是≤32节点的无环图、并发≤2、总时长≤30min；跨实例动作只有在管理员创建/批准DAG并分别确认参与实例权限后可执行，插件不能自行递归调用其他实例handler；人工确认危险节点、可取消、每步lease/fence/日志，重试策略与side effect类型绑定。Go SDK只登记本地typed handler；不把MP ActionContent或Go接口对象跨进程序列化。UI首期JSON编辑/手动运行，不承诺复杂拖拽编排器。

### 10.3 auth_providers（F4-0基础，F5完善UI）

新增可选broker（不是当前已存在）：Begin→一次性state/nonce→callback→provider subject/claims→宿主校验issuer/audience/过期→管理员预绑定本地账号/角色。签发本程序cookie/会话永远由accounts/auth服务完成，插件不拿session key；禁止从插件claims直接授予管理员。禁用provider不影响本地密码救援登录。provider错误fail closed，不回落成匿名成功；这一点不适用一般“失败回内置”的字面解释。

### 10.4 agent_tools（F5）

注册name/description/JSON schema/requiredScopes/effectKind；先提供管理员测试面板及受保护工具调用，完整工具输入/输出有界、危险调用需确认。没有LLM对话、token核算、自动Agent循环；对应N/A保留。若未来引入真正Agent，须另立ADR绑定用户权限与调用预算，不能把插件安装授权当成无限自治权限。

## 11. 持久化 schema、迁移与幂等草案

### 11.1 迁移号与新增表

- 预计 `00062_plugin_v2_runtime.sql`：F1基础表＋旧插件6表停用归档；`00063_plugin_v2_extensions.sql`：F2调度/路由/事件；`00064_plugin_v2_workflows.sql`：F4-0工作流/auth最小宿主。F5插件库管理优先复用F1定义/授权/操作历史；确需额外索引或字段时草案 `00065_plugin_v2_catalog_review.sql`，**不预建市场仓库/下载表**。执行前重核最新编号；PS0不新增迁移。
- 无新数据库扩展、无插件自定义DDL/SQL。JSONB大小Go＋CHECK双重限制，状态/错误常量与CHECK逐项D2比对。未安装catalog来自编译内存，不批量落库。

| 新表（全部 `plugin_v2_` 前缀） | 关键字段/约束/索引 | 阶段 |
|---|---|---|
| definitions | 仅已选plugin_id PK、当前manifest jsonb≤128KiB、version、definition/review_fingerprint、contract_version=2、availability available/missing/unsupported、last_seen_build、revision；没有二进制路径/下载URL/仓库设置 | F1 |
| definition_revisions | plugin_id+definition_fingerprint PK、manifest/grant面快照、observed_at；仅已安装定义的历史；每plugin至多100条未引用＋所有当前已批准引用，用于权限diff/审计而非可安装旧代码 | F1 |
| instances | uuid、plugin_id FK、clone_of、name、desired_state、runtime_state、config/grant/operation_revision、approved_definition/review_fingerprint、generation、owner_lease_until、restart_count、next_restart_at、reason、timestamps | F1 |
| configs | instance PK、revision、schema_version、ciphertext≤256KiB、updated_at | F1 |
| kv | instance/key PK、revision、ciphertext≤256KiB、updated_at；明文单项≤128KiB/实例10MiB | F1 |
| grants | instance/scope/selector_hash唯一、selector jsonb、review_fingerprint、approved_revision/by/at；新定义不覆盖旧批准集 | F1 |
| operations | uuid、instance、kind、business_key UNIQUE、expected_revision、state、lease/fence/attempt、next_attempt_at、reason_code、安全result摘要；due/stale索引 | F1 |
| runs | uuid、instance、kind/handler、business_key UNIQUE、definition/grant_revision、generation、state、lease/fence、side_effect_class、started/finished/duration、安全摘要 | F1/F2 |
| state_snapshots | uuid、instance、operation_revision、config/KV密文引用、data相对路径、size、schema、state/lease；只恢复本实例数据，无可执行产物 | F1基础/F5完善 |
| registrations | instance/kind/key PK、definition_fingerprint、grant_revision、generation、schema、descriptor jsonb≤128KiB、enabled | F2 |
| schedules | instance/service PK、cron/interval/date/once、timezone/next_at/revision/misfire/enabled | F2 |
| events | uuid、source_key UNIQUE、type/schema/aggregate/revision、occurred_at、safe payload≤256KiB、causation；type/time索引 | F2 |
| deliveries | event/instance/handler/subscription_revision唯一、state/attempt/next_at/lease/fence/reason | F2 |
| api_keys | SHA256摘要PK、instance、allowed routes/methods/scopes、grant_revision、expires/revoked/last_used | F2 |
| completion_observations | subscription_id+revision PK、expected/owned摘要、observation_revision、unknown/partial/complete、last_event_key；不ALTER订阅表 | F2预留/F4-C |
| workflows / workflow_steps | run与≤32步DAG、安全输入输出引用、handler/effect、lease/fence/state/revision | F4-0 |
| auth_bindings / tool_calls | provider实例+外部subject摘要→本地账号引用，无密码；工具复用runs及schema | auth F4-0/tool F5 |

所有definition历史只包含纯metadata，绝不能用于“从DB加载旧插件代码”；module pin/build身份只用于内部一致性与审计，不作为管理员可选择源码/提交的字段。代码升级/降级无SQL开关。

V2内部FK只指V2表，保存业务UUID不建立级联修改/删除业务表的反向依赖；原archive保留原FK/密文AAD，rekey仅更新插件列定位。旧市场表只归档，不读其URL、不调度旧下载。

### 11.2 状态常量（草案）

- definition availability：`available / missing / unsupported`；缺失只停用，不删数据。未安装项是registry条目，不伪造persisted instance状态。
- instance desired：`enabled / disabled / absent`；runtime：`stopped / starting / running / stopping / backoff / needs_action`。权限扩大置disabled/needs_action，常量reason `permission_review_required`；pin缺失 `definition_missing`；合同/能力错误 `definition_unsupported`。
- operation/run：`pending / running / succeeded / failed / unknown / needs_action / cancelled`；next_attempt_at唯一调度事实，jobs只唤醒。kind限install/enable/disable/clone/configure/reapprove/uninstall/restore_data，不提供download/install_url/upgrade_version/rollback_code。
- delivery：`pending / delivering / succeeded / retry / unknown / dead`；只有幂等交付lease到期才retry，其他unknown。snapshot：`pending / writing / ready / failed`，writing有lease/孤儿清理。没有package publication状态。
- owner lease30s/10s续约，连续2次续约失败先停本机workers；新owner确认旧generation消亡才启用，不能确认则needs_action。外部RPC不跨DB事务/全局锁，业务操作fence阻止迟到写。
- pin刷新将availability/新metadata/修订与旧generation撤销在短事务内提交；随后进程外清理。重启重复刷新不重置管理员disabled，不新增scope、不重复数据迁移。
- 服务run lease按最大duration＋30s配置（默认330s）；数据clone/restore总预算10min、lease上限12min/15s续约，GC宽限至少25min。lease每次领取新fence，迟到写回须`WHERE lease_token=? AND generation=? AND state=?`命中，否则放弃；先提交撤销引用，再安全删除孤儿数据文件。

### 11.3 加密、调度与保护边界

- configs/KV用现有Vault/AAD机制，以稳定instance UUID/字段语义隔离；clone重新加密，保留旧归档AAD，不重加密其他业务、不更换主密钥。worker只取得本实例允许配置/数据，不获密钥或DB/Redis连接。
- 配置省略保留/null清除，秘密mask不是可写回原值；revision CAS。config schema升级与数据恢复需明确批准，data目录含插件自有秘密时备份按私有业务数据对待，不能进公共CI/release附件。
- grant变更/卸载/缺失定义立即撤销连接generation、路由和凭据有效性；不能只更新前端状态。审计经现有服务记录actor/instance/pluginId/版本/原因，不记令牌原文。
- 复用`secrets.Vault.Seal/Open`且不修改既有`media-tool:integration:v1:` AAD前缀；配置id=`plugin:v2:<instance>:config`、KV id=`plugin:v2:<instance>:kv:<key>`由唯一构造器生成。clone读取旧AAD并按新instance重封；rekey/backup只追加V2/legacy插件列定位，其他集成凭据字节保持不变。
- config/grant/definition三种revision分开；业务幂等键例如`instance:kind:fromRevision:targetDefinition`、`instance:service:scheduledAt`、`channel:messageId:instance:command`，不用随机UUID/now()充当业务键。手动重执行以CAS递增operation revision表达新意图，网络重发返回原operation结果。
- supervisor/PG是调度事实源；插件自有后台goroutine必须StopService有界停止，不能再建立一套抢Host lease的调度器。数据快照tmp写→fsync→原子rename→fenced DB引用，GC宽限/配额/owner校验与worker lease协调；默认保留最近2份＋7天，超额needs_action，不删正在恢复或已引用快照。
- 数据恢复/镜像代码回滚不撤销已经发生的下载、通知或文件删除；业务副作用由既有service审计与独立恢复路径处理，不以“恢复插件快照”抹掉事实。

## 12. 内置库构建发布、同一binary隔离与资源预算

### 12.1 两仓库与镜像边界

```text
mediatool-plugin/                    # github.com/dingding229/mediatool-plugin；一个go.mod
  go.mod / go.sum
  sdk/
    plugin.go                       # 接口/Runtime/Serve/ContractVersion
    contracts/ events/ transport/   # typed DTO/enum/protocol
    host/<domain>/                  # F3分包客户端，不依赖主程序
    plugintest/ schema/             # mock/golden/唯一manifest schema
  plugins/hello-go/
    plugin.go / manifest.json / README.md / icon.*
    ui/{src,dist}/                  # 可选；dist是已构建ESM，构建输入同pin
  registry/                        # All/Get纯metadata＋factory引用
  .github/workflows/verify.yml

mediatool/
  go.mod / go.sum                   # pin上述module的真实不可变版本
  cmd/media-tool/...                # main最早分流plugin-worker
  internal/modules/plugins/...     # host实现、supervisor、目录与实例
  internal/platform/pluginhost/... # 业务适配
  internal/platform/webui/assets/  # 构建时包含同pin插件ID命名空间UI
```

- 插件库尚空，F1才初始化；不在本仓库放SDK副本/examples，也不让插件库import主程序。插件内业务依赖由该module的go.mod/go.sum审核，主程序MVS结果/replace/主module依赖升级必须测试；不承诺仅插件库测试就证明整个app兼容。
- 现Docker backend固定Go1.26.9，release主程序 `CGO_ENABLED=0`；runtime不添加Python/Go/make/git/pip。新库代码与已构建UI增加app/静态资源体积，**没有独立插件运行时层，但不是总镜像零增量**；空库/hello/6插件的app字节、镜像压缩/展开体积与workerRSS在F1/R-F/F6实测，不沿用旧2–15MiB独立包估算。
- 构建阶段从go.mod/go.sum**精确pin**解析源码/资源；不读取远程main、不在runtime取代码。建议在既有多阶段Docker中先物化同一module到临时build stage，纯校验/复制plugins/<id>/ui/dist与图标到frontend输入，再Vite构建并复制回backend。只增加构建stage，不新增生产服务。
- frontend/backend/registry共用一份build清单（pluginId/version、内部definition及UI fingerprint/contract），检测两边pin不一致即失败；UI文件资源≤128项/插件、单项≤4MiB/总16MiB、目录总≤128MiB，路径/符号链接/外部import一律校验。先验上限可按实测申请调整，不能运行期补下载。
- 静态图标catalog可读取，但只有安装且ui.trusted批准的实例允许加载/执行ESM；物理打进镜像不意味着自动import全部插件UI。不允许根网页启动时遍历执行所有ESM。静态字节不包含秘密，不以资源URL保密代替授权；控制的是宿主页面实际挂载和API调用，即使读取到代码也不能获得Host连接或实例权限。

### 12.2 开发、CI与标准更新链

1. 作者在mediatool-plugin新增/修改plugins/<id>、manifest/version/history、registry与测试；SDK变化与Host DTO/事件合同同库更新，不单独发布另一个SDK module。保持无副作用初始化，使用sdk/plugintest跑mock Host/golden；UI源码改动同时按固定前端工具链重建并提交dist，CI检查源码/产物匹配。
2. 插件库CI固定Go版本，执行`go vet ./...`、`go test ./...`、`go test -race ./...`串行，以及metadata/schema、factory接口、ID/权限枚举、DTO/RPC golden、依赖/初始化lint。默认无宿主DB/Redis/115凭据、无真实媒体副作用；race测试binary不是release产物。
3. 测试通过→**推送dingding229/mediatool-plugin**→得到真实不可变提交；然后主程序go.mod/go.sum **bump pin**。两仓库使用用户批准的同一套Git凭据；未验证写权限，F1遇拒绝就停下报告，不能建替代来源或复用其他项目凭据。
4. 主程序CI校验精确pin可编译、Host接口与sdk.ContractVersion/schema一致、无本地replace/浮动源码引用、全部metadata/资源匹配；运行真实app plugin-worker golden/环境与资源实测、build/vet/race/errcheck、Vue/相关E2E。新库自己的CI不能代替主程序门禁。
5. 再走**主程序提交/推送→Actions全部通过→Docker Hub AMD64不可变digest→服务器备份→只更新获批app镜像，不改生产安全配置→验证→清理**。没有“只推插件库让生产自动更新”的步骤；PS0不执行上述操作，F1先框架、R-F才发布。
6. 运行时install只是在当前pin目录选择ID，离线完成；未编入的插件返回not_found，无远程下载/URL/文件上传退路。业务network/115/站点调用是F3能力，不可用于实现二级插件安装/动态编译器。

主程序release继续固定Go1.26.9（后续工具链升级另审）、linux/amd64、CGO_ENABLED=0；代码/manifest/ESM/依赖相同才是同一完整输入。确定性测试改为两目录构建相同app＋内置目录/资源，比较规范化registry清单/UI字节及受控flags的app binary，不再比较插件ZIP。镜像层时间/供应链签名不保证跨CI运行逐字节相同；真实上线信任Actions产出的不可变Hub digest。原生AMD64执行release static app与race分开，ARM模拟不能替代。

### 12.3 Worker早分流与包初始化安全

- 现main在`run`中加载配置/日志/DB等，F1必须将`plugin-worker`解析置于这些代码之前；初始化独立无Host DB配置。工作目录、HOME/TMP、stdin/stdout及允许FD仅属于实例。stdout只给RPC，panic/stderr安全截断。
- 父进程固定环境白名单，不透传`os.Environ()`，不继承DB/Redis/Vault/115/代理密码、app日志/业务socket FD；close-on-exec/显式FD检查。worker不能因SDK导入或main公共helper重新加载app配置文件。
- 构造器/工厂只在worker隔离且握手授权后调用，parent catalog调用只解析纯metadata；constructor错误/崩溃仅损失该worker，不使parent执行插件逻辑。
- **Go包初始化不在上述时间闸之后**。registry import的全部插件及其transitive依赖可能在app父进程main前初始化。禁止插件/SDK自定义init和非纯顶级求值，依赖初始化在pin review中核对；lint发现调用/goroutine/I/O/环境访问/panic等需拒绝或逐项批准安全的标准库初始化，不把这个lint宣称为机器码形式化证明。若将来要隔离不可信包init，必须改独立二进制设计，不能假称同binary已解决。
- 同binary现有主程序及transitive依赖的包初始化也须检查，特别是worker分流前的配置文件/网络访问；不能只审查新插件目录便认为早分流已完成。必要时仅提取相关初始化为正常server路径的显式调用，按任务预算回归。
- 因此“管理员安装受信代码”进一步变为“代码已由维护者审核编译入app＋管理员选择实例能力”；安装确认是实例业务授权，不是允许在运行期注入任意代码。
- 固定同UID app worker，无launcher/UID切换/helper/seccomp。父进程dumpable限制、环境和FD白名单及Pdeathsig/进程组/整树看门狗在F1-D实测；Landlock仅尽力而为，不能把不支持当成禁用插件的条件。app可读文件的同UID泄露风险由审核兜底，不在文档中伪称已消除。

### 12.4 默认资源与不可夸大的保证

| 项目 | 提案默认/上限 | 验证/限制 |
|---|---|---|
| 实例/worker | 安装≤32，每启用1进程，同时≤8 | 先总内存准入再lease；catalog≤500不等于500个进程 |
| 地址空间 | RLIMIT_AS 8192MiB | 真实同app worker验证runtime映射；不是RSS硬限，F1-D实测不满足先说明 |
| Go软内存 | GOMEMLIMIT 192MiB | 仅软目标、插件可更改；不能代替OS限制 |
| RSS | 单worker256MiB，总1GiB | cgroup v2可委派时硬限，否则200ms整树采样看门狗，说明突刺风险 |
| CPU/进程/FD | cpuQuotaPercent100；exec CPU30s；NPROC64；NOFILE256 | 常驻worker不设30s累计CPU寿命；NPROC包含线程需实测；fork/整树收敛单测 |
| 实例数据 | data256MiB/tmp64MiB，总可写4GiB | 代码/UI只在镜像；原子写、快照配额、禁止清业务媒体 |
| 帧/队列 | 默认1MiB/硬4MiB；累计写队列4MiB | 读长度先限额，不猜测恢复污染stdout |
| 日志 | 单行4KiB/尾64KiB/100行min、burst200 | 安全归集panic/stderr，实例精确过滤，不泄露Host secret |
| 身份/根 | app/worker同UID65532；实例0700/宿主根0711 | 同UID仍可读app可读文件；目录mode不是实例间安全边界 |
| 回调/Host | 每实例4/8、队列32、全局Host32 | reply/cancel/heartbeat独立，不跨锁等RPC |
| GC | 每轮≤1000项/100ms，可续扫 | 只实例tmp/快照/明确弃用目录；宽限大于lease与停止最坏预算 |

rlimit不是完整整树内存沙箱；进程组之外double-fork/setsid/子孙由进程树跟踪和受信代码审核兜底，不加入seccomp。同UID不是恶意代码隔离；dumpable/收割实测失败如实报告，Landlock ABI不支持warning继续，不新增privileged/业务容器。runtime允许受信插件自身exec/network/files，但Host scope无法拦截所有原生系统调用，保持原信任说明。

### 12.5 安装确认、定义升级与保密

- 用户只选择内置ID，不选择URL/路径/源码commit/平台。预览纯metadata不构造插件；确认绑定当前definition/review fingerprint、资源selector及配置revision。install默认disabled，安装并启用必须明确同意。
- pin刷新版本/权限diff由Host纯函数计算；权限扩大或ui.trusted资源变化需停用/重新确认，数据保留。pin删除保留不可用实例；审计不能把“definition刷新”写成“插件已运行成功”。
- 新版worker失败没有旧插件二进制可以局部重启：保持needs_action和数据快照，管理员通过标准app镜像回滚处理代码，不提供隐蔽版本下载/选择入口。
- 编译内置不授予秘密读取权；worker环境/Host DTO不含主密钥、115授权、DSN、管理员cookie、媒体token、STRM正文。插件自己的配置可含其独立第三方秘密，备份/日志按私有数据保护。
- ESM同页风险继续通过ui.trusted醒目告知，源码/产物必须同pin、禁止远程脚本。主程序管理员日志原值规则保持，但不新增收集登录body或把私有日志/数据库上传公共CI。

## 13. 前端/API与共享契约

- 现PluginsView内新增“内置插件库/已安装实例”区域，不新开顶级应用；列出当前pin全部ID/name/version/author/icon/level/history/描述/权限/可用状态。只读目录不构造插件，无源码/提交/外部仓库/上传控件。
- 安装/启用展示危险scope、selector和ui.trusted二次确认；未安装默认无数据。支持clone/配置/停用/日志/计划；pin刷新仅展示版本与权限差异，不提供升级到任意版本按钮。
- 已装定义消失显示“当前镜像不含此插件，数据已保留”；权限扩大显示“已停用，等待重新确认”。不能把不可用项从页面直接过滤掉，以免误以为数据丢失。代码回滚指向运营流程，不在页面下载旧版本。
- JSON组件树、多key/meta/受信ESM/侧栏沿用§8；ESM由同pin资源构建，父页面按已安装授权实例动态import，不对catalog全部执行。失效generation后卸载监听器/计时器与页面，防旧权限页继续调用。
- 日志固定 `module=plugins,instance,pluginId,definitionFingerprint,generation,runId`；logging.Query新增精确instance过滤，禁止靠keyword伪装隔离。配置/私有key不出现在目录或审计message。

| 新管理API草案（仅管理监听器） | 合同 |
|---|---|
| GET /api/v1/plugins/catalog | 当前编译目录，分页≤200，合并安装/可用状态；无可变source/commit字段、不联网 |
| GET /api/v1/plugins/catalog/{pluginId} | metadata/权限/历史；不调用worker |
| POST /api/v1/plugins/catalog/{pluginId}/preview | 只接受已知ID/selector与可选目标instance；返回当前definition/review revision和权限diff，不接收文件/URL |
| GET /api/v1/plugins | 已安装实例分页≤200，包括missing/unsupported |
| POST /api/v1/plugins/operations | install/enable/disable/clone/configure/reapprove/uninstall/restore_data；operationKey、expectedRevision、明确confirm |
| GET /api/v1/plugins/operations/{id} | persisted stage/result/unknown，不以HTTP202声称完成 |
| GET/PUT /api/v1/plugins/{instance}/config | 省略保留/null清除、secret保持/重设、CAS |
| GET /api/v1/plugins/{instance}/runs | 有界运行历史 |
| GET /api/v1/plugins/{instance}/extensions | 实际generation注册/冲突/unsupported |
| POST /api/v1/plugins/{instance}/services/{service}/run | 持久手动任务 |
| POST/DELETE /api/v1/plugins/{instance}/api-keys | 实例专属凭据，只创建时展示原文、worker无原文 |
| GET /api/v1/plugins/{instance}/ui/{uiBuild}/{asset} | 已确认内置资源白名单；按实例授权映射到编译资源，不读取data路径 |
| ANY /api/v1/plugin/{instance}/{path...} | 仅预注册方法/auth/执行scope→invoke |

所有管理写操作管理员认证＋CSRF＋审计；plugin业务路由按§8鉴权。**旧包上传/URL安装/仓库配置/下载/独立代码upgrade/rollback接口全部不注册**；不能保留admin-only“隐藏入口”。MEDIA端口无plugin/admin路由。

公共SDK合同/golden/schema唯一位于mediatool-plugin/sdk；主程序 `internal/platform/httpapi/testdata/contracts/plugin-v2-*.json` 为管理HTTP及Host实现对照fixture，校验与所pin SDK版本一致，不手写另一套enum。Vue测试消费同一HTTP fixture；metadata schema从SDK固定输入复制/生成前端类型，CI断言无漂移。PS0不新增JSON Schema依赖，F1选择Go显式校验或新依赖前按规则审核。

## 14. 测试与验收策略

### 14.1 纯函数、协议与SDK单元

- SDK在插件仓库：interface assertions、mock Host、typed event/DTO、帧拆粘/半帧、UTF-8字节长度、重复key、深度/大小、ID/cancel/deadline、背压/心跳、双向callback不死锁；宿主测试同一pin wire golden。
- registry/manifest：重复/未知ID、路径不符、未实现scope、缺工厂/非法元数据、纯catalog不调用构造器；拒绝runtime/sdkVersion/二进制路径/仓库URL/上传等废弃字段，contract/schema自检失败fail closed。
- grant diff矩阵：无变化/权限缩小可刷新；scope/origin/selector/API公共执行面/module/chain扩大或未知变化必须停用；ESM变化需ui.trusted再确认。revocation/fence不能只靠UI。
- JSON组件schema/条件/model原型污染/eval/HTML事件拒绝；module排序/只读fallback/写unknown不重复副作用；危险scope拒绝反例。
- main早分流、禁止有副作用包初始化/全局单例、依赖方向单测；不把字符串grep等同所有初始化安全证明，审查新增依赖。

### 14.2 数据库与存储集成（只测试库，串行）

- 仅media_tool_test与Redis DB15，复用测试锁；两仓库不并发运行会变更DB的集成/E2E。插件仓库默认不连DB或真实外部服务。
- 每个实际新增迁移Up→Down→Up，旧插件密文归档可rekey/备份；非插件schema/数据摘要不变。V2有实例时Down拒绝有损回退，不通过删业务表凑往返。
- 新pin有N定义、无安装时：目录有N、实例/目录/worker/计划/路由均为0；安装1个只创建1个；clone独立UUID/AAD；重复安装operationKey不重复。
- 已装实例pin刷新无变化/缩权/扩权/移除/重新添加；扩权即时停worker/Host写并保留config/KV/data；missing可管理但不可执行；重新启动/重复同步幂等，不自动复活旧授权。
- lease过期、owner竞争/进程崩溃、配置CAS、快照中断/GC/配额、停止期间迟到调用；写run/delivery unknown不一般重试。
- 数据目录逐段无symlink、rename竞争、父proc/环境secret sentinel与同UID残余风险、快照/GC路径越界和宽限。没有ZIP/市场获取测试fixture；替代为旧HTTP上传/URL接口不可达反例。

### 14.3 真实同app Go worker golden

- 在主程序CI用精确pin构建release `CGO_ENABLED=0 linux/amd64` 的**同一个app binary**，先验证静态链接，再启动其plugin-worker接合成Host。真实序列：initialize→InitPlugin→GetState→Form/Page/Dashboard/Meta→API/Command/Service→反向config/KV→StopService/Close；只规范化关联ID/时间。
- 插件仓库hello与host-demo（如需）覆盖必需/可选接口、On/OnChain、typed客户端；golden属于插件库SDK和主程序Host双向合同，不再安装测试二进制/ZIP。
- fmt.Print污染、panic/fatal/os.Exit、goroutine死锁、stderr/输出炸弹、exec子孙及整树回收；父进程运行正常，坏worker不拖垮app；无有效stdio握手直接运行子命令有界失败。
- worker不初始化app配置/DB/Redis/Vault；测试合成环境/FD/proc秘密访问，并明确同UID文件残余风险；子命令混入server参数或任意路径拒绝。不存在自动all角色回落。
- **未选择的插件工厂不调用**，但包初始化已在父进程发生：用lint/人工审核门禁验证无副作用，不用这个测试虚报任意恶意init被进程隔离。
- 原生linux/amd64的同binary dumpable/FD/Pdeathsig/进程组/rlimit/RSS实测必须执行，Landlock不支持仅warning继续，不能被ARM模拟或缺二进制Skip替代。`go test -race`单独构建，不把race binary当静态release证据。
- 固定完整输入两目录构建比较registry/UI清单和受控flags app字节；runtime断网/无编译器情况下选择内置hello仍成功，未知ID始终404，不访问GitHub或执行下载工具。

### 14.4 两仓库CI、前端/E2E与回归

- mediatool-plugin：vet/test/race/golden、metadata/registry、SDK dependency/lint、UI源码dist一致；mediatool：pin compile/Host contract/worker门禁、Go build/vet/race/errcheck与Vue/相关E2E。各项证据分开，不把本轮PS0静态校验当实际CI通过。
- E2E：内置目录→未安装无实例→预览权限→选择安装/启用→hello统计→clone/停用；当前pin无来源/commit/上传控件；模拟扩权停用待确认与missing保留状态；CSRF/admin/媒体端口隔离。
- Vue单测/vue-tsc：共享fixture、多个实例、GetForm model/when、dashboard key/刷新/不可见停poll、同pinESM按需加载/unmount、拒绝未确认ESM。
- secret sentinel扫描响应/事件/日志/公共CI产物：无DSN、主密钥、115令牌、媒体用户token、STRM正文、云路径；测试不复制生产日志、DB或账户。保留管理员原值日志既有规则，不新增导出秘密。
- 回归站点pin/按需加载、HR/owned删除保护、115/STRM/302/用户权限、备份/密钥、非插件调度；原生播放不引入plugin执行/ffx，不转码/中转/HLS。
- 每次实施测试/构建后清理本任务目录/GOCACHE/cache-path与日志JSON；确认无Go进程后go clean -cache，不动modcache或其他项目缓存。PS0仅文档检查，不构建。

## 15. 分阶段任务、预算与发布门禁

估算为**非测试代码**，包含必要前端/脚本/SDK，不含golden/测试/文档；不是承诺每阶段只有1200行。每行子任务是约300–1100行的执行单元，1200是预算上限而非要填满的目标；实际超过内部再拆段连续完成，每任务末统一汇报。新增依赖/数据库扩展、改变已批准安全模型或新增生产访问仍先确认。下列预算有约±25%波动，同UID环境/资源实测纳入F1-D，Landlock按300行/无新依赖边界评估，不单设安全PoC。

### F1：内置库与worker内核，预计6–9k（跨两仓库）

| 子任务 | 非测试行数估计 | 验收 |
|---|---:|---|
| F1-A1 插件仓库初始化 | 600–900 | mediatool-plugin单module、sdk契约骨架/registry/hello纯metadata与factory；本库无主程序依赖，无init副作用；先确认该仓库推送权限 |
| F1-B runtime schema/迁移62 | 600–900 | 旧6表归档、definitions/instances/grants等基础表；Up/Down/Up与非插件数据不变，无packages/market表 |
| F1-C 双向RPC与SDK transport | 600–900 | framing/mux/cancel/heartbeat/队列及合同golden；公共协议在插件库sdk，Host无复制enum |
| F1-D supervisor/reaper | 800–1100 | owner lease/fence/backoff、先准入后lease、固定app子命令；同UID环境/FD/dumpable/Pdeathsig/整树回收及AS/RSS隔离实测，无setuid/UID池/seccomp |
| F1-E catalog与按需实例 | 600–900 | 未选择零实例/数据/工厂；install/enable/disable/clone/config/KV/data目录；admin/CSRF/审计、operationKey幂等 |
| F1-F SDK辅助/测试工具 | 650–1000 | 必需/可选接口、typed On及config/KV等基础Host client、mock Host/golden，插件库vet/test/race |
| F1-G pin定义刷新 | 500–800 | 重复同步/重启幂等；扩权停用、missing保留、CAS/旧generation立即拒绝；无运行期升级入口 |
| F1-H build/资源/依赖门禁 | 500–800 | 前后端同pin、只读资源清单、初始化检查、主程序Host compile/启动自检；runtime无插件编译/下载路径 |
| F1-I 基础管理/日志/移除旧框架 | 500–800 | 内置目录＋已安装状态、权限确认/日志instance过滤；删旧WASM/SDK/examples/上传/市场；审计与秘密sentinel |

F1-A1只初始化新插件库，必须实际推送得到提交才能pin；若本项目凭据无权推送，如实阻塞，不使用临时本地replace作为完成证据。不再有F1-A2安全PoC子任务，隔离实测随F1-D完成。F1基础Host仅本实例config/KV/state/dataPath、system摘要及既有notification outbox，其余scope明确unsupported。PS0及AGENTS草案获批后随F1主程序提交；初始化SDK不等于F3业务全覆盖。

### F1执行分组（本次审核安排）

- **G1（当前）**：先验证插件库push权限→F1-A1/C/F（单module/SDK/双向RPC/测试工具/hello/CI）→插件仓库提交推送取得提交；本组主程序只修订本文，不pin、不改代码、不提交。
- **G2**：F1-B迁移62＋F1-D supervisor/reaper/同UID隔离实测＋F1-E catalog/实例，主程序pin G1提交，但先不推送。
- **G3**：F1-G定义刷新＋F1-H构建依赖门禁＋F1-I UI/日志/删除旧框架；主程序首提交包含PS0及§17 AGENTS正式规则，推送并通过CI；F1不部署，R-F才发布。
- 每组按约1200行内部再分段，每段一行进度，组末①–⑤＋D表；两仓库提交均带指定Co-Authored-By。Go/数据库测试串行，仅media_tool_test/Redis DB15，不连生产；结束清理本任务缓存/产物，不动modcache。
- 本轮只读检查`internal/modules/messaging`及`internal/adapters/telegram`：现有为出站投递，未找到getUpdates/setWebhook/callback_query或CommandIngress接收实现；因此新增Telegram bot轮询排入F4，F2不承诺已有真实入站。

### F2-A审核补充（G2仅记录，不实现）

G1 contracts.Descriptor目前仅ID/Handler字符串/RequiredScopes。F2-A必须提供SDK侧直接Go函数绑定：API含Path/Methods/Auth/Summary/Handler:p.foo，Service含Trigger/Cron/Interval/Kwargs，Command含Cmd/Desc/Category，Action含Handler(ActionContext)。runtime生成稳定handler id，Go函数只保留本地表，wire descriptor补齐元数据并路由plugin.invoke；不能要求作者手工字符串ID而宣称与MP绑定方式对等。本组不修改已审核G1 SDK。

### F2：扩展点框架，预计 4–7k

| 子任务 | 非测试行数估计 | 验收 |
|---|---:|---|
| F2-A registry/descriptor | 400–600 | get_*稳定描述符、版本快照、冲突/priority、扩展许可子集 |
| F2-B API gateway/实例key | 600–900 | 四种auth、public确认、request/result有界，不在媒体端口 |
| F2-C 持久服务 | 500–800 | cron/interval/date/once、手动run/历史、misfire/lease/fence |
| F2-D 广播事件 | 650–950 | EmitTx、outbox bridge、delivery、重复/unknown/回放cursor |
| F2-E 链与module基础 | 500–800 | typed patch、total budget、安全fallback、写unknown例外、递归保护 |
| F2-F JSON渲染器 | 700–1000 | 表单/页面/仪表盘组件与model/when，不eval |
| F2-G ESM/多key/侧栏 | 450–700 | 共享Vue/组件库/API client、trusted声明、按编译期uiBuild加载/卸载 |
| F2-H 入站能力核对/命令接线 | 350–600 | 先核对messaging入站能力；无现成轮询/webhook接收宿主时，通用registry保留，新增Telegram bot轮询延到F4，不偷偷扩充F2 |
| F2-I 3个真实hook接线 | 300–500 | NameRecognize、DownloadAdded、TransferComplete实际业务触发+回归，不改原始STRM语义 |

框架验收必须是“真实插件执行并调Host、真实业务触发、前端可操作、禁用后即时停止”，不是RPC echo测试。auth/workflow/tools的未实现能力明确unsupported；单动作可以复用有界run调用，但不是工作流Runner，不宣称已执行workflow/auth/Agent。

### R-F：框架发布，预计0–600（修复另按任务预算）

F1/F2审核后：先保证所需插件库提交/CI成功→主程序固定pin与前后端清单一致→全量测试→主程序提交/推送→Actions原生AMD64含真实同app worker环境/资源实测、golden与errcheck→Hub不可变digest→服务器Compose/DB/插件目录权限备份→仅更新app镜像，不改变cap_drop/no-new-privileges或增加launcher→验证未选插件不加载/hello实例及原有媒体链路→清理临时实例与缓存。

插件库发布本身不触发生产下载或升级；生产以主程序镜像为唯一代码版本。不能绕过主程序CI/Hub发布。PS0不连接用户给定SSH，后续发布才用本次新密钥；不改target安全配置，不新增容器、launcher或privileged。

### F3：Host API 按业务分组，预计 5–8k

| 分组/子任务 | 估计 | 验收 |
|---|---:|---|
| F3-A1 搜索/规则；A2订阅；A3下载/下载器；A4站点/指标/签到请求 | 各350–600，总1.4–2.4k | scope/selector/secret matrix、真实Service非mock门面、HR/owned/来源快照不回退 |
| F3-B1 文件/存储；B2库扫描/整理；B3STRM/归档；B4metadata/分类 | 各350–600，总1.4–2.4k | preview-confirm、fence、路径/身份不可越界、TMDB/115限流、无rawlink |
| F3-C1媒体查询/用户；C2原生会话/权限；C3probe/stills管理 | 各350–600，总1.05–1.8k | 稳定DTO、T42权限/匿名图片约束保持、只读浏览不现场探测 |
| F3-D1消息/notifications；D2settings/integrations/proxy；D3jobs/cache/system/maintenance | 各350–550，总1.05–1.65k | 业务能力齐全但无凭据/SQL/Redis/发布恢复后门，通知unknown不盲重试 |

每分组交付请求/响应golden、逐scope拒绝反例、敏感字段sentinel；缺Service先补窄接口再接Host，不让pluginhost变成第二套SQL业务层。

### F4：事件/链全覆盖，预计 4–7k

**相对初拟阶段的必要依赖调整**：若把workflow/auth最小宿主全留F5，F4不可能让它们的事件/链真实触发。将原F5-D/E/F的最小运行内核平移到F4-0（不重复计量），F5保留插件库管理/权限变更、数据恢复、工具与相关UI完善；R-F框架发布仍在F2之后。

| F4-0前置子任务 | 估计 | 验收 |
|---|---:|---|
| F4-0A 动作注册/上下文/最小工作流数据 | 450–700 | 宿主不是插件临时线程；有界DAG、schema、权限与确认 |
| F4-0B 工作流Runner | 650–950 | 持久步骤lease/fence、取消/unknown与确定性幂等 |
| F4-0C 外部认证broker | 600–900 | opt-in、身份绑定、nonce、本地登录保留、不能直接isAdmin |


| 子任务 | 估计 | 验收 |
|---|---:|---|
| F4-A1站点/订阅广播；A2候选/下载链与Provider | 各350–600 | 实际提交/上游确认点，失败不重复下载 |
| F4-B1整理/字幕/历史广播；B2重命名/覆盖/拦截prepare；B3存储选择/删除回读 | 各350–600 | RPC不跨事务、路径/owner与revision重校验、媒体保护回归 |
| F4-C1metadata/discovery/recommend；C2期待集数/完成observer | 各350–600 | 不误用completeCollection、不在热路径无界扇出，无证据unknown |
| F4-D系统/通知/module刷新/settings | 350–600 | 事件去秘密、节流、防自激递归、写unknown不自动fallback |

验收以§9的50项覆盖fixture为准：46项具有真实或新最小宿主接线并有测试，4项N/A具名保留；不能为了“全覆盖”触及音频/Agent全平台或修改受保护播放合同。

### F5：插件库管理、升级确认与UI完善，预计2–3.5k

| 子任务 | 非测试估计 | 验收 |
|---|---:|---|
| F5-A内置目录/已安装管理 | 350–550 | 当前镜像全部metadata、安装/可用状态；无来源/commit/URL/上传/下载/版本选择 |
| F5-B pin刷新与权限确认UI | 450–700 | 版本/history与危险scope差异；扩权/missing保留配置数据，确认后明确启用；不能单插件代码回退 |
| F5-C数据snapshot/schema恢复 | 500–800 | 仅实例数据，兼容检查/CAS/崩溃恢复，无业务副作用撤销或旧代码下载 |
| F5-D/E/F workflow/auth面板 | 合并到F5-H，不重复计内核 | F4-0已有最小宿主，只补状态/错误/schema表单，不建全功能可视化平台 |
| F5-G tool registry/人工调用 | 300–500 | schema/预算/审计；无LLM诚实unsupported |
| F5-H UI/文档/收尾 | 400–650 | instance日志/权限提示/响应式可访问/双仓库开发说明；按需复用F1表，不默认再建迁移 |

这里的“安装/升级”是当前内置定义的选择与镜像pin刷新，不是新市场；schema/data恢复不能绕过数据保护。运行期源码编译、代码上传、外部来源、同页ESM沙箱化、完整LLM/工作流平台均不包含。

### F6：插件仓库内6个Go验收插件，预计2.5–4k

| 验收插件（独立原创SDK示例，非复制MP代码） | 非测试估计 | 验收点 |
|---|---:|---|
| 站点自动签到 | 350–550 | sites.request/checkin、cron、凭据只host持有、失败/验证码needs_action、通知 |
| 自动做种清理 | 450–700 | downloads.read/delete、策略预览、HR/owned保护、默认dry-run、未知删除需核对 |
| 媒体库刷新与入库通知 | 250–400 | TransferComplete、mediaserver.refresh、notifications，重复事件不重复副作用 |
| 目录监控实时整理 | 400–650 | changefeed cursor、library.scan/organize、停用后无后台残留；不无界扫盘 |
| TMDB/豆瓣想看订阅 | 450–700 | metadata/网络/订阅、公开列表或插件自己的合法账号配置，缺接口诚实失败，不借宿主115令牌 |
| 下载器助手 | 350–550 | 安全列表/暂停/恢复/标签能力、API+命令+仪表盘+动作，读写scope分离 |

全部源码在mediatool-plugin/plugins/<id>，共用该module的sdk/go.mod/go.sum及registry；每项带manifest/version/history/权限、mock Host/golden/单测，可选同库ui/dist。不在主程序重建examples或独立cmd/bin包。先通过插件库CI并推送，再由主程序bump pin执行真实同app worker端到端。使用合成站点/downloader/115 Transport/小文件，不对真实账户签到/删种/移动媒体；生产动作需管理员选择并显式配置。功能对等不意味着绕过站点限制或管理非owned种子。

### R：最终发布与总预算

- F3/F4/F5/F6全部审核，重新全量门禁→提交/推送→Actions→Hub AMD64不可变digest→备份→app更新→生产安全验证→临时实例清理。保持官方三服务、115/主密钥/STRM/媒体及既有业务不变。
- 最终验收逐项核对：MP接口矩阵、50事件映射（4N/A明确）、42模块Host清单、每scope拒绝矩阵、实例/pin刷新/数据恢复与整镜像回退/停用/清理、真实同app worker及SDK golden/同pin资源一致性、媒体端口隔离、secret sentinel、旧archive可恢复。
- 阶段估计合计约 **24–40k 非测试行**（F1–F6加两次发布各0–600）；主要不确定性是dumpable/Landlock/进程树的内核行为、各业务窄Service提取、双仓库pin/同binary初始化安全/前端资源一致性。此数是计划级范围，不是允许单任务写几万行；每子任务仍约1200。
- 若只做F1/F2预计约10–16k非测试行，即可框架发布；不能把剩余业务覆盖当“几个Host method”不计预算。

## 16. D1–D10设计自检与待审核事项

| D | 本设计对应要求（不是已实现证明） |
|---|---|
| D1 | 插件库sdk是公共contract/schema/DTO/enum/golden单点；主程序Host/管理HTTP/Vue按pin校验，无二进制包版本协议 |
| D2 | definition/instance/run/delivery/错误与CHECK逐一对应；scope固定枚举，不改业务状态 |
| D3 | 错误/fence/Close/审计均检查，写unknown不盲fallback，无假成功 |
| D4 | owner/starting/delivery/snapshot的lease和整树回收；定义缺失/扩权停用、重启幂等 |
| D5 | operationKey/revision，外部副作用不许诺exactly-once；重复刷新不重复实例或迁移 |
| D6 | 帧/目录/列表/队列/资源/日志/GC/外部调用均有界，无运行期下载/上传路径 |
| D7 | 外部RPC不跨事务/全局锁，先背压再lease；父进程catalog不运行插件工厂 |
| D8 | 逐连接scope/selector/actor/grant generation；秘密不下发，ESM/包init信任限制明示；媒体端口无插件API |
| D9 | SDK/插件库不依赖主程序internal；Host适配单向；只mediaprobe可引用ffx，播放不引入执行器、不转码/中转/HLS |
| D10 | 迁移只插件表/旧专属归档；保留配置/KV/data，绝不触及凭据/115/主密钥/STRM/媒体与业务表 |

**内置库与简化A3已按本次审核写入；实施约束和后续验证项：**

1. A3按审核简化为同UID65532、父进程dumpable=0、显式环境/FD、Pdeathsig/进程组、rlimit/RSS；不做setuid/UID池/跨UID helper/seccomp，不改生产安全配置。Landlock尽力加固，不支持warning继续，超过300行或需依赖先报告。隔离实测纳入F1-D，不单设A2 PoC。
2. 同binary要求意味着包初始化在父进程main前执行；本设计通过可信pin审核、无副作用初始化约束控制，不声称任意恶意init崩溃已隔离。若要求连不可信初始化也完全隔离，将与本次同binary决定冲突，须另改架构，不能实施时隐瞒。
3. 同页ESM仍需网页级信任；写副作用unknown不能一般fallback。保留这两项原安全限定。
4. 00062/63/64及可选65的插件专属迁移草案执行前重核；不建package/market表。旧密文归档保留、不运行兼容旧格式。
5. 四项音频/LLM无宿主事件仍N/A，其余新增workflow/auth/入站命令最小宿主按阶段，不捏造能力；两仓库推送权限实际验证留F1，PS0只读不能据公开可读宣称可写。

当前G2完成后按①–⑤＋D表汇报并停下等待G3；不自行提交/推送、访问生产或部署。

## 17. AGENTS.md已批准草案（G3首次提交应用）

已批准在现有site模板规则后增加以下一条，G3首次主程序提交时正式应用；G2不修改实际AGENTS.md。其余维护/部署/数据保护规则不变：

```text
- Plugins are compiled Go definitions from the pinned
  github.com/dingding229/mediatool-plugin module. Its sdk/ owns public contracts,
  typed Host clients and test tools; plugins/<id>/ and registry/ supply the built-in
  library. Only administrator-selected definitions create instances/data, and only
  enabled instances execute through this app binary's plugin-worker subprocess
  under the supervisor. Keep package initialization side-effect-free and preserve
  the worker's secret-environment and resource isolation. Never accept plugin source,
  ZIP/binary uploads, external repositories/URLs, runtime code downloads or dynamic
  compilation. Do not expose source/commit/version selectors in plugin UI.
  On startup/upgrade refresh installed definitions only; expanded permissions stop
  the instance pending administrator confirmation. Removed definitions become
  unavailable while preserving configuration/KV/data. Bundle plugin ESM at image
  build time from the same pin, under plugin-ID namespaces; never load remote UI
  code. New/changed plugins must be tested and pushed to dingding229/mediatool-plugin
  first, then pinned in mediatool and released through its standard
  Actions -> Docker Hub immutable image -> backed-up server deployment flow.
  Use this project's approved Git credentials for both repositories; report missing
  access instead of using other credentials or alternate plugin distribution.
```

F1删除本仓库plugin-sdk/go/examples/plugins及旧上传/市场执行代码，不删除历史备份或受保护业务。此草案不是PS0提交授权，也不改变AGENTS现有只允许官方三服务和不可变Hub镜像的发布要求。

## 18. PS0修订2历史校验记录（2026-10-17，G1前）

- 仅修订本文档；实际AGENTS.md与HEAD逐字节一致，规则仅为§17草案。未改代码/迁移/Compose，未初始化或推送插件仓库，未暂存/提交/推送主程序，未连接生产。
- 文档中3个JSON代码块均解析；Manifest v2的2020-12 Schema、JSON示例及Vue本地资源示例有效，**31个非法metadata反例均被拒绝**（含废弃runtime/sdkVersion/包摘要/来源URL、缺字段、越界与远程UI路径）。这只是文档Schema校验，不是已实现宿主验证器。
- 81项scope及42模块Host API清单、31广播＋19链式事件映射相对修订1完整保留；仅请求上下文的package身份改为definitionFingerprint。SDK接口/辅助方法与D1–D10设计要求仍在。
- 已移除正向ZIP/二进制/外部仓库下载安装、独立SDK module和单插件代码回滚设计；历史审计、废弃字段拒绝与AGENTS禁止规则中的提及有意保留。升级以插件库提交→主程序pin→标准镜像发布为唯一链路。
- worker、SDK、registry、Go测试/race/golden、CI/推送权限及隔离尚未实施/验证，不以本次静态检查宣称F1已完成；仅匿名只读核对新仓库元数据及本地site实现。
- 已删除本任务`/private/tmp/media-tool-ps0-builtins.*`备份/编辑脚本目录及`media-tool-ps0-builtins-path`标记，释放**208 KiB**分配空间。未运行Go构建/项目测试，未产生GOCACHE/cache-path或测试日志JSON；未触碰GOMODCACHE和其他任务缓存。
- Markdown围栏/行尾空白及`git diff --check`通过；工作区只保留本文档，等待PS0审核，不进入F1。

## 19. F1-G1实施记录（2026-10-17）

- 已用本项目现有GitHub凭据验证`dingding229/mediatool-plugin`的push权限；插件库单module初始化，实际提交/推送成功：`7db7256791793ccecce2393604c4e0c017e21887`。提交含指定Co-Authored-By；未在主程序go.mod pin此提交。
- G1按三段交付：①公共contract/错误与长度分帧/双向RPC；②manifest/scope/event枚举与类型化SDK辅助、本地On/OnChain；③无副作用registry/hello、mock Host/逆向调用与故障注入、真实Go测试子进程stdio golden及CI。宿主生产业务桥接和API/service/event执行接线仍属G2/F2/F3/F4，不能以枚举存在声称已接线。
- 双向RPC默认1MiB/硬4MiB帧、重复key/深度/UTF-8检查、有限请求/回调/队列/累计字节、独立响应/控制处理、取消/timeout/断连及错误脱敏。取消不能越过尚未发出的请求；IO泵与回调分离，反向Host调用不死锁。SDK配置/KV辅助带revision，不自动重试可能已执行的写操作。
- 单测实测修正：mock harness等待worker ready后返回；重复Close幂等；bounded callback完成时先释放slot再回复；防止取消帧抢在原请求前发送；panic/错误Message不将原始秘密字符串送入RPC错误。SDK进程退出收尾有界，G2仍必须负责OS级整树回收。
- 本地固定Go1.26.9，AMD64/CGO_ENABLED=0模块编译、vet、27个顶层测试（含子测试84项，0 Skip）、真实测试子进程stdio golden，以及`go test -race -p 1 -count=3 ./...`通过。测试只用内存mock/net.Pipe/受控测试子进程，不访问数据库/Redis/115或真实媒体。主程序真实worker、dumpable/Landlock/rlimit/RSS与迁移62未实施，留G2。
- 插件库CI为ubuntu原生AMD64：格式/静态编译/vet/单测golden/race三轮/无生成产物，run `https://github.com/dingding229/mediatool-plugin/actions/runs/39016523940`；最终结论和缓存清理实测在组末记录。
- 审核要求的A3/§12/§16/F1/R-F与AGENTS草案已修订：同UID65532、不做setuid/UID池/跨UID helper/seccomp；Landlock不支持warning继续，超过300行或需新依赖先报告；app可读文件残余风险明确。现有Telegram仅出站，新增bot轮询延到F4。
- 本组主程序仅本文更新；实际AGENTS.md/go.mod/go.sum/代码/迁移/Compose不改，不暂存/提交/推送主程序、不访问生产、不部署。后续G2须明确下发，G3才首提交主程序并更新正式AGENTS。

### G1最终门禁与清理

- 插件仓库远端main与提交`7db7256791793ccecce2393604c4e0c017e21887`一致，工作区干净。Actions run 39016523940的verify为**success**；格式/无CGO编译/vet/真实Go stdio子进程golden/race三轮/无生成产物全部通过，未跳过核心真实子进程测试。
- 本机确认无Go/compile/link及本任务测试进程后，删除专属`/private/tmp/media-tool-f1-g1.*`（GOCACHE＋门禁日志）和对应`media-tool-f1-g1-cache-path`：244312 KiB；执行Go1.26.9的`go clean -cache`，全局缓存从1176降至1176 KiB。合计释放**244312 KiB（约0.233 GiB）**；未执行clean -modcache，未改modcache/生产/其他任务目录。
- 主程序仅本文留待G3提交；G1结束，等待G2，不自行pin或接线。

## 20. F1-G2实施记录（已完成，2026-10-17）

- 精确pin `github.com/dingding229/mediatool-plugin v0.0.0-20261017081253-7db725679179`，无replace；不修改插件仓库，不实现本次仅记录的F2-A函数绑定。
- 迁移62只归档旧6表并创建V2基础表，增加owner/generation/lease及PID/start-time进程引用；新generation需旧进程引用清理后才可claim。Up/Down/Up、D2枚举与非插件所有表的列/数据摘要验证；有V2实例拒绝有损Down。Docker schema label同步62，不改生产Compose。
- V2配置/KV沿用同一Vault和原`media-tool:integration:v1:` AAD前缀；新行id分别使用`plugin:v2:config:<instanceUUID>`与`plugin:v2:kv:<rowUUID>`，便于既有rekey以固定行id工作。旧WASM密文及旧AAD保持原样，只调整归档列定位，备份验证识别旧名到归档名的对应。不迁移或重加密115/业务凭据。
- 新V2实例操作支持install/enable/disable/clone/configure及operationKey+请求摘要幂等；确认绑定当前定义指纹和全部已声明scope，F1暂不批准尚未实现业务scope。clone复制配置/KV且重新加密、默认disabled；data/tmp独立新目录，不复制活跃文件快照（文件恢复/快照属后续F5）。目录/文件符号链接和越界拒绝。
- Host每次数据库校验generation/owner/lease/grant revision/已批准定义及声明scope，不跨请求缓存。提供本实例config/KV/CAS/list/state/dataPath、system安全摘要和notification outbox；外部身份/SQL/凭据方法拒绝。管理员API在管理监听器，认证+CSRF+同事务审计；旧包/市场入口先410，不等待G3删源码才停用。
- worker在主程序配置/日志库/DB前分流，同UID65532、白名单环境/FD CLOEXEC、Pdeathsig、独立进程组、subreaper、PID/start-time收割、rlimit和整树RSS/CPU采样。父进程dumpable=0；短命exec的CPU接口尚未开放，常驻worker不套30秒累计CPU寿命。cgroup仅可选，当前不调整生产挂载或capabilities。
- Landlock无新依赖、实现约103行，使用已有x/sys与Go AllThreadsSyscall限制所有Go线程；ABI缺失warning继续。Go的CGO/race进程不支持AllThreadsSyscall，测试明确分成CGO降级路径与**CGO_ENABLED=0静态内核门禁**，不得把race降级通过称为隔离通过。G3 F1-H需把原生Linux静态sandbox测试单独加入CI。
- 本机默认ARM Colima内的Rosetta AMD64系统调用返回Landlock ENOSYS，故创建任务专属全系统QEMU x86_64 Colima；其Linux6.8内核Landlock ABI=4。在cap_drop ALL/no-new-privileges/UID65532下完成静态实测：父environ/mem拒读、8个预存Go线程与新线程目录拒读、ABI warning、真实Pdeathsig、FD CLOEXEC、rlimit、double-fork整组kill/reap；Go CPU profiling可用，core dump因dumpable=0禁用。
- 实测真正主程序`media-tool plugin-worker`完成hello init/加密KV，崩溃重启一次且停用后收割；不是只测SDK helper。数据库测试仅media_tool_test，Redis若使用仅DB15，串行执行；无生产访问/主程序提交/推送。
- 旧WASM测试暂保留到G3移除，专用测试fixture在media_tool_test独立schema迁移到61，绝不在public恢复旧表或提供运行兼容路径。其目的仅保持本组完整回归，不重新开放旧格式。

### G2回归与最终结果（2026-10-17）

- 全包race按项目锁串行覆盖；初轮发现filemanager完成事务残留旧extension_plugins查询，已去除旧plugin.dispatch分支，保留原files.*事务outbox与核心文件操作；调度/工作台预览的旧WASM读取也移除，仅保留内置名称解析，F2再接新识别hook。相关filemanager、旧插件独立schema fixture、HTTP包修复后race复跑全绿，其他包与完整migrations race已通过。
- 增加并发6个owner只领到1个lease、过期owner真实进程退出前新owner不得启动、资源限额触发回收、配置/KV/list/state/dataPath/system/notification outbox dispatcher、admin/CSRF/媒体端口及共享HTTP契约测试。工作流/服务/API动态函数绑定不在本组。
- Linux release静态内核测试单独运行，覆盖9项（含三轮重复），与CGO/race降级路径区分；实际main worker init→KV→崩溃重启→owner lease过期切换→disable/reap，以及极小RSS预算强制回收均通过。无物理AMD64硬件声明：测试是在本机QEMU全系统x86_64内核与AMD64容器完成，而非Rosetta syscall翻译层。
- Go build ./...、vet ./...、全仓既定生产路径errcheck及Linux源码errcheck通过；无前端改动，未运行Vue/E2E，旧插件UI迁移/删旧框架与正式AGENTS规则留G3。生产部署留R-F，无提交/推送/生产访问。

### G2最终门禁补充

- 全包race分批串行覆盖109包，其中75包有测试；完整migrations race（118个顶层测试）通过，新G2并发领取/往返测试追加复跑通过。初轮filemanager旧表耦合及旧fixture导入问题修复后，filemanager/legacy plugins/V2/HTTP整包race重跑通过，无剩余失败包。
- 最终原生Linux ABI路径用静态CGO_ENABLED=0二进制执行：内核9项×3轮全过（含父environ/mem/ptrace拒读、预存多线程Landlock、Pdeathsig、FD/rlimit、double-fork、warning、整树统计）；真实主程序worker/Host集成7项全过，含KV、崩溃重启、过期owner切换无重叠与RSS限额回收。macOS中的Linux专项Skip不作为完成证据。
- 主程序代码/迁移/测试保留工作区，未暂存/提交/推送，正式AGENTS未变；没有新的Go依赖（除指定SDK pin）或DB扩展。Landlock103行，不触发300行/新依赖暂停条件。

### G2清理与停止点

- 确认Go/compile/link/测试进程退出后，删除专属`/private/tmp/media-tool-f1-g2.*`的GOCACHE、Linux/Mac构建产物、测试日志/JSON/脚本及cache-path标记，释放3610964 KiB；按任务label删除临时测试容器及其匿名测试卷，并用Colima支持命令删除专属`media-tool-f1-g2` x86_64 VM/磁盘/配置，释放2244432 KiB（清理前分配量）。
- 执行Go1.26.9 `go clean -cache`：全局缓存1176→1176 KiB。上述合计释放**5855396 KiB，约5.584 GiB**。不执行clean -modcache；保留共享Colima基础镜像缓存和原默认VM/业务容器，不做docker prune。
- Docker context已恢复colima；原默认app/postgres/redis未重启或更新（本地旧app显示unhealthy，未将其宣称为本组部署健康验证，也未擅自处理）。主程序无暂存/提交/推送，停止等待G3。

## 21. F1-G3实施记录（2026-10-17）

- 第一段：Host读锁改为KEY SHARE；写使用独立实例advisory lock保护配额/CAS，提交前以generation/owner/grant_revision/lease/权限指纹条件UPDATE做fence，失败回滚整笔副作用。配置Host写同时推进实例revision，管理员配置CAS不会覆盖插件刚提交的配置。每次授权查库，不跨请求缓存。归档两密文列显式加入CipherColumns、保留原AAD，旧表/归档表不存在则跳过；无重复轮换。
- 第二段：仅已选定义启动刷新，重复同步不写；定义/实例revision CAS及锁协调并发。执行面/资源/权限变化均保守地请求重新确认（不仅扩权）；被移除定义标missing，保留私有数据。停止/lease回收不能擦掉permission_review_required。review默认停用，旧generation即时失效。卸载为保留数据的absent tombstone，重装新实例。
- 构建门禁：初始化扫描覆盖registry的全部非标准库传递依赖，禁init和非纯顶级求值；固定pin资源清单由构建期Go工具生成，host与Vue共享同一library.json，资源仅从module-cache中的该pin复制、路径/数量/单文件/总容量有界。不加Python/Go运行时工具链，不提供下载、上传、市场或运行期编译。CGO_ENABLED=0真实同binary worker测试加入原生Ubuntu AMD64 verify，以UID65532、无capabilities/no-new-privileges运行；与race降级测试分开。
- 第三段：PluginsView内置目录、默认停用安装、危险权限确认、启停、分身、配置CAS、按实例KV/日志、保留数据卸载。管理API受管理员/CSRF/审计保护且媒体端口不注册。旧WASM框架、pluginhost、SDK/examples、ZIP/市场API和前端删除；保留旧job空ack以便历史任务收敛，保留files.* outbox。organizeHistory从旧插件源文件无行为改动迁至独立文件，非插件用例保留。
- AGENTS采用§17批准规则；FRAMEWORK/DEVELOPMENT/EXTENSIONS/OpenAPI同步V2。F2-A函数绑定与descriptor元数据仍待实现，本组不实现plugin.invoke、动态API/服务/事件、JSON树或ESM执行；不能把F1称为MP全能力完成。F1不连生产、不部署，R-F统一部署。

### G3本地回归修正记录

- 全仓race首轮所有业务包及427.1秒迁移套件均执行，唯stillstore静态Dockerfile断言因新增plugins目录改变命令串而失败；保留原目录创建/属主链、新目录独立创建，原测试不减断言，stillstore整包race复跑通过。
- 全量106条E2E首轮105通过，新增权限确认UI用例发现交互等待问题；改为可聚焦原生同意框并明确二次确认按钮文本后重跑。定向运行仍需包含首次管理员注册用例，不把缺注册的登录失败当产品缺陷。
- 复跑发现backupverify测试清理会误清native_server_settings默认行，使其后的独立E2E无法启动。测试清理保留该singleton，E2E在media_tool_test中幂等补齐初始singleton；端口探测允许TIME_WAIT重绑定但仍拒绝存活listener。未修改生产路径或放宽产品认证/安全限制。

### G3提交前最终门禁

- Go build/vet、生产路径errcheck及Linux构建标签errcheck通过；全仓race执行（包括427.1秒migrations及tests）。首次stillstore断言修正后该包整包race复跑通过；本轮最终V2/HTTP/rekey/backupverify/logging及stillstore再次串行race回归。
- 迁移62 Up→Down→Up与D2枚举/非插件表不变通过；归档rekey干跑及实转、并发共享读/续租/撤权/写fence、重复刷新/权限确认/missing保留及幂等卸载覆盖。
- Vue typecheck/build、33个文件104项单测通过；Python28项通过；整套E2E最终106/106通过（2.5分钟），原非插件用例保留。UI截图已只读目视核对，沿用现有主题并在390px下无横向溢出。
- 本机非Linux的专项Skip不作隔离完成证据。原生Ubuntu AMD64静态内核＋真实同binary worker门禁必须由本次Actions实际通过，随后记录Hub digest；不部署、不访问生产。
