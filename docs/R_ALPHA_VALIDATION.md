# R-α 验证与候选发布记录

## 范围与证据

- 仅开发、文档、测试和候选镜像发布，不访问生产、不部署。R-β 待单独授权。
- 源码候选 0.27.0 / schema65 / contractVersion2；无新迁移、依赖或DB扩展。
- 插件仓库文档提交 `4ad0d7c57697a29e0c828756ca15a8a58c12827d`；Actions `39273339935` 全绿后才更新主仓精确 pin。
- 六个验收插件与 hello 的版本/history/权限不变；当前表见 `PLUGIN_SUPPORT_MATRIX.md`。

## 先复现，再修复

原实现的两条新增回归先失败：服务端已返回安装成功、目录同步失败时没有“已安装”回执；网络异常不区分拒绝和结果未知，并直接显示 Error.message。
历史“首次已安装但后端无请求”的点击原因无法从服务端缺失记录推断，不把猜测当已复现根因。
修复只针对可确认的流程问题：

1. 尚未确认/提交中/已提交/同步失败/结果未知/明确拒绝分开；提交成功即显示实例ID。
2. 同步失败保留成功回执和已知实例，刷新仅重读列表；并用epoch拒绝迟到列表覆盖新快照。
3. 结果未知只显式重试原意图，同页Map及安装的标签页journal保留同一key。journal只有公开定义身份+不透明key，最多32条，不存配置/actor/凭据。
4. 已安装时按钮标“新增实例”，仍支持合法多实例；未核对安装优先标“核对上次安装”。不同定义pin的旧未知意图需人工核对，不猜测迁移重放。
5. 安全失败日志只含固定结果码/requestId及标准日志元数据，不记输入、输出、配置、key或原始错误。

停用态不请求动态表单、更不隐式启用；保留JSON配置保存。离线schema表单仅给设计建议。
现有外部/管理状态接口版本统一读取buildinfo.Version，避免发布版本与硬编码dev字符串分叉。

## 本地完整门禁

- 插件仓：CGO-free build、vet、unit/golden、race -count=3通过；无数据库依赖。
- 主仓：test-go.sh 全仓 -race -p1 -count=1通过；迁移整包474.469秒，仍使用20分钟既定包上限。
- vet / errcheck / plugin-check / exact-pin资源清单 / 支持矩阵校验通过。
- Vue：43文件144项通过；vue-tsc+Vite构建通过；Python：33项通过。
- 完整浏览器E2E：115通过、1项Linux平台条件跳过；不删旧业务用例，不加自动重试。
- 两个新增UI合同E2E使用真实HTTP .test origin（isSecureContext=false、原生randomUUID不可用，未改这两个浏览器属性），仅静态传输定向本机以避开DNS/系统代理。所有API为明确的UI测试夹具；另有真实Host提交后丢响应/刷新重试E2E证明operationKey对应一个真实DB实例。
- 390px截图人工复核，无横向溢出；确认文案、成功回执、实例ID、JSON停用提示可见。
- 初次过滤式浏览器命令遗漏首次注册夹具，已停止该次调用；完整套件包含首次注册。非安全origin初次受本机DNS代理影响，改为仅测试静态传输定向loopback；未修改OS代理、生产配置或测试预算。

## 性能：不伪造生产样本

本地环境是darwin/arm64，所有DB测试只用media_tool_test和Redis DB15，共用test-lock串行。

| 项目 | 本次证据 |
|---|---|
| 无插件hook热路径 | 10,000次识别2.289555041秒；额外SQL=1，因跨过既定2秒absence TTL。TTL内重复项零SQL，不再每项chain+module两次查询 |
| Host读并发 | 32个同实例并发读，另持KEY SHARE锁，349.135ms完成；实时授权，续租/撤权/写fence由同包回归覆盖 |
| SourceRegistry | TTL内不重复provider调用；空结果120/240/480/900秒有界退避、失败30/60/120/240/300秒；并发lease与配置/停用失效回归通过；上一成功结果最多保留120+300秒 |
| Linux worker冷启动/资源/链回落率 | 本地无样本；仅交叉编译不能当原生证明。由Actions中的真实静态worker测试逐次输出R_PERF，最终审核汇报引用实际数据 |

原生AMD64 CI强制同binary真实worker、专用UID的race host、CGO-free nonroot/cap-drop/NNP内核门禁，以及真实HTTP/服务/事件/停用收割。
`TestRRealWorkerColdStartResourcesAndChainSamples` 在统一testworker.Ready（完整provider/handler注册+只读form往返）之后，直接采25次链调用，不重试计时样本；记录首次链耗时、p50/p95、deadline收据、回落日志、RSS和任务数。
冷启动指标是进程启动到注册及form就绪，不宣称冷磁盘/冷页缓存。deadline收据终态等待在计时样本之外，不扩大生产200ms/2s预算。
本地没有链样本，所以不写0%；历史F4生产首调23–86ms/回落0%不是本次R或当前生产回落率。

## 实际非测试代码行数（物理行，明确口径）

- R-α 生产/构建代码：新增 **183**，删除 **20**；文档、测试、golden、生成清单、go.sum不算代码行。
- 主仓相对已批准V2前基线 `7d8c67ca157d5c08137d311b154a814c0a2de731`：累计新增 **17089**、删除 **2729** 个非测试代码行，含旧框架删除；不能把净增量当实现工作量。
- 插件仓当前 sdk/registry/plugins 保留 **7807** 行非测试Go（79文件）；不包含golden/manifest/资源/文档。
- 以上统计扩展名为Go/TS/TSX/Vue/Python/Shell/SQL，排除测试目录、测试命名和测试worker工具。生成支持表另列，不用净增量掩盖旧代码删除。

## 发布边界

主仓提交后的Actions/Hub结果及原生R_PERF具体数值以最终审核汇报和对应Actions日志为准，本文件不预填尚未发生的CI结果。
候选镜像发布不等于部署；R-β仍须备份、保护表/STRM摘要、不可变digest更新、健康/302、用户浏览器验收。
不触碰保留的absent实例、既有测试快照、生产账号/主密钥/115/STRM/媒体；不自动执行签到、删种、订阅创建或整理。
