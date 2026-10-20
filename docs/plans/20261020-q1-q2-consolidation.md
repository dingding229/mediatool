# Q1 + Q2 执行记录与审核交接

本记录是 Q0 之后的当前交接，不是发布/部署证明。基线提交 `74ef5f507a031aca8fbfc96936c739ad3f25a0dc`；只做本地提交，未 push、未运行 Actions/Hub 发布、未访问或更新生产服务器。当前规范入口见 [CLI/API 映射](../CLI_REFERENCE.md)、[文档索引](../INDEX.md)。

## 审核决策落实

- Q0 的 Claude 审核已由用户经 codex queue 确认通过；不再把 codexhost 不提供委派命令视为审核阻塞。本轮没有另启模型代替审核方。
- **D1 留 Q3**：规划/审核方提供生产旧六表、site_accounts、notification_channels、bootstrap_imports、media_user_legacy_grants 为0行及 site_account_id 全 NULL 的只读结论。本轮未连接生产复核、未改 SQL 迁移、未删除这些表/列或其 rekey/backupverify 保护；Q3 新迁移必须在事务中锁定目标后断言空/全NULL，否则 RAISE，不能 TRUNCATE/DELETE 或 squash 历史。
- **D2 已执行**：删除 HLS/relay 正向脚本和旧双架构本地图像“发布验收”；Go 诊断只认 stable resolver 302 → CDN 206；metadata/客户端解码不冒充真实播放器验收。
- **D3 已执行**：19 个工具收敛为三类可执行文件与分层叶命令，保留实际应用能力；旧 server/SQLite 会话包装退出，但 gateway-session 的显式验证/绑定 stdin 能力保留，未读写外部 Emby 数据库。
- **D4 已执行**：移除 plugin query 的额外 redacted 例外。新回归验证完整 URL、参数编码/顺序、重复参数的原值；不额外记录登录 body/Cookie/Authorization，不将恢复口令送入 web store，不把私有日志/数据输出到公开产物。
- **D5 留 Q4**：journal 100000 行导致维护链阻断的 F1/B1 没有在本轮偷偷重写或“清表绕过”；仍是发布前必修。

## 分组提交与预算

Q1c 的新增 Go 与旧脚本清退分两笔内部提交，避免单笔超出约1200行非测试代码；无阶段中途审核请求或 push。预算按 Git rename-aware 新增+删除统计，排除 `_test.go`/测试用例与 Markdown，计入脚本/配置/内嵌策略资源；未改的整文件迁移单独计数，不把重命名当数千行重写。

| 组 | 本地提交 | 非测试代码 + / - / 净 | 非测试改动量 | 测试 + / - / 净 |
|---|---|---:|---:|---:|
| Q1a | `0cf00cb` | +379 / -115 / +264 | 494 | +421 / -279 / +142 |
| Q1b | `9c115e2` | +798 / -268 / +530 | 1066 | +225 / -1 / +224 |
| Q1c-1 | `ad3d49a` | +811 / -30 / +781 | 841 | +212 / -0 / +212 |
| Q1c-2 | `9bb61d4` | +1 / -732 / -731 | 733 | +0 / -254 / -254 |
| Q2a | `a00eaa7` | +192 / -673 / -481 | 865 | +128 / -258 / -130 |
| Q2b | `本报告同笔提交` | +277 / -107 / +170 | 384 | +128 / -8 / +120 |

Q2b 包含本记录；其最终 SHA 见 `git log` 和聊天最终交接，避免在文件内自引用自身提交哈希。

## 门禁与环境

所有实际 DB 变更套件共用原仓库 `data/.isolated-test.lock`，固定 media_tool_test 与 Redis DB15。最终执行使用本任务独立、loopback-only 的临时 PG/Redis 容器，复用本项目已有服务的缓存基础镜像 ID，避免共享 PostgreSQL 集群的后台事务干扰 MVCC 缓存性能；没有构建/替换本地 app image，也没有操作现有 app/PG/Redis 数据卷。

Go 使用独占 GOCACHE，TMPDIR/GOTMPDIR 都在系统任务临时目录（避免 macOS `/private/tmp` 的路径保护冲突）；模块/工具均来自已有 modcache，未引入新依赖/更改 pin。测试环境只读专用 DSN/Redis 连接，应用密钥由 Go 重新生成，不 source 操作者生产配置。

首轮最终门禁的 E2E 暴露移植夹具错误：把64位应用密钥当成注册密码，超过产品既有6–20字符限制。没有放宽产品密码策略/浏览器断言；Go 独立 fixture-password 恢复原16字符格式，并新增与真实 ValidatePassword 的回归校验。该失败轮被明确中断，清理了其独立测试容器，之后重新执行完整门禁；下表只在最终轮完成后填入。

| 门禁 | 结果/数量 | wall-clock秒 |
|---|---|---:|
| Go build ./... | PASS | 3.776 |
| Go vet ./... | PASS | 0.889 |
| Go全仓串行race/集成 | PASS：87测试包、1083顶层PASS、15顶层SKIP；含子测试2320 PASS；无race告警 | 606.591 |
| errcheck v1.9.0 -ignoretests（cmd/internal/migrations/tests） | PASS | 0.742 |
| Linux/amd64 CGO=0 cross-build | PASS（非原生执行） | 2.803 |
| Linux标签errcheck | PASS | 0.784 |
| Vue单测 | PASS：47文件157项 | 4.629 |
| vue-tsc + Vite | PASS | 11.035 |
| Playwright单worker全套 | PASS：122项，1项Linux专用SKIP | 171.578 |
| govulncheck v1.8.0 | 0可达/导入包漏洞；1条require-only公告GO-2026-5932（openpgp，无已知修复，不在调用图） | 3.443 |
| 同pin初始化检查 | PASS | 0.230 |
| 资源清单 --check | PASS | 0.173 |
| 同pin ESM/资源导出 | PASS | 0.189 |
| 契约矩阵 --check | PASS，原契约摘要不变 | 0.188 |
| RTA -test | 仅1项GetSeries，保留接口约束 | 4.458 |
| Linux真实worker/kernel | BLOCKED：darwin/arm64，前置拒绝；不是通过 | 0.347 |

迁移包独立耗时303.678s；10000影片缓存 first miss482.062ms、冷进程Redis hit104.903ms、3 SQL，原<200ms/≤3 SQL断言不变。15个Go顶层skip=13个原生worker/RSS/acceptance门禁+2个fixture/subprocess辅助入口；浏览器1个Linux同binary门禁skip。未新增skip、未设Playwright重试或放宽时限。

实际执行了build-web默认入口（包括Go资源门禁、npm ci、vue-tsc/Vite）及工具策略race复核后，重新跑完以上最终整轮。首次容器启动仅因本地基础镜像tag已不存在而受阻，后改为复用本项目已有服务的缓存镜像ID；失败环境轮不计作通过，也不拉取/替换业务app镜像。

门禁中的真实媒体仅合成 ffmpeg/ffprobe 夹具，测试不使用真实媒体修改或生产密码恢复。Go/Node 的浏览器策略与启动 guard 仍执行自身 JS 表达式/harness，不下载或运行完整第三方 JS。

默认 `make test` / `make test-integration` 先构建同 pin 前端资源，再进入串行测试，避免 clean checkout 缺少 SPA embed 导致伪失败；build-web 的具体 Go 资源步骤与 CI 相同，去掉重复 npm 安装。全仓执行所调用的 test-go/E2E 脚本未放宽 race、20m、UID/readiness、错误边界或性能阈值。

## 删除、合并与保留理由

| 范围 | 非测试代码 + / - / 净 | 测试 + / - / 净 | Markdown + / - / 净 |
|---|---:|---:|---:|
| Q1 移植 | +1961 / -1117 / +844 | +858 / -534 / +324 | +387 / -21 / +366 |
| Q2 整合 | +469 / -780 / -311 | +256 / -266 / -10 | +333 / -1573 / -1240 |
| 合计 | +2414 / -1881 / +533 | +1111 / -797 / +314 | +714 / -1588 / -874 |

累计 27 个文件被 Git 识别为重命名；Go protocol 生成文件、go.mod/go.sum、65份历史SQL及模块pin未修改。

1. **Python 23/23**：12 个工具/夹具（1007行）、11 个测试文件（533行）全部退出 Git；CI/Makefile/shell/当前文档无解释器入口。锁/DSN/DB15、随机配置、fixture、清理、运维、备份、负载/播放诊断、矩阵及有效回归都落在 Go。
2. **明确退役**：remote-hls-check 及其9个旧正向用例、verify-images 的本地双架构口径删除；严格302、公开/私有DNS、凭据不进CDN、Hub arch/revision/schema/digest 的 Go 负向/正向测试取代这些不再允许的验收口径。
3. **旧 remote-session 包装**：依赖不存在的 server 服务、`.env` 部署以及自动读取外部 SQLite 认证库，既不属于当前三服务部署，也不应继续自动抽取用户会话；删除包装，不删 app session 的受控 stdin 验证/明确绑定/私密 capability 输出能力。
4. **18个强死函数**：删除无消费者的下载/分类包装、旧媒体用户 Login/Session/ChangePassword/Logout、旧 NFO/文件 helper、路径 nodeAttr/Equal、旧站点远程 fetch/catalog/import；NFO/字幕、当前 nativeauth、账户与授权的真正实现保留。
5. **GetSeries 例外**：RTA 仍标记 `HierarchyRepository.GetSeries`，但它满足 HierarchyService 编译期接口；保留此方法及原断言，不用删断言来让死码工具“归零”。没有把生产入口109个不可达项一键删掉；纯测试接缝、DTO/SDK/unsupported契约也不等于可删功能。
6. **平行接线**：删除未注册的 native assets handler、指标 API 测试包装与空 notifications 契约包；指标断言改走真实 sitemetrics.Engine.Save，危险目标/URL断言改直接测真正 playback.Policy，保留原边界，不用空实现“测绿”。
7. **5组同体 helper**：共享 canonical UUID、Unicode 标题 key、媒体 backoff 表、Earlier deadline、JSON transport；分页共用整数解析但保留管理/插件/旧 page 三类线协议与边界。Emby DTO/admin Envelope/播放错误不合并成一种协议；URL path、os.Root、云盘 path 不强行共用不安全清理逻辑。
8. **文档**：删除10份过时 HLS/多服务/阶段验收/计划指南；WORK_LOG、NATIVE_ERROR_AUDIT、IMPLEMENTATION_STATUS 移入 docs/plans 历史；原 plans 全加 superseded 状态，保留原事实。当前 CLI/API、部署、安全与业务专题以索引为准。

Python → Go 的类型/错误处理可能增加实现行数；因此分别列 Q1 移植与 Q2 精简的净变化，不以删除文档或整文件移动伪装业务代码减少。

## CLI/API 映射边界

完整19行对照见 [CLI/API 映射](../CLI_REFERENCE.md)。runtime 只有 media-tool + 同源宿主 ctl，构建工具不进入 runtime；原 `-role` 与 plugin-worker 子进程协议不变，危险 CLI 的密码文件、offline-confirmed、apply、write-confirmed 和结果不确定不重放仍有效。

HTTP route/method/Envelope、Emby协议、下载/上传/订阅状态机没有换协议；ops preview 只换宿主执行命令字符串，私有日志按 D4 保留原 URL。没有添加远程源/上传执行/版本选择 API。根 Compose 仍为占位模板，直接值与三服务结构没有改变。

## 风险与后续门

- Q3 只能66起追加迁移，按 D1 锁/断言空后删除；生产零行结论不是未来写入后的删除授权。
- Q4 必须关闭 journal 容量故障域；本轮未删除 journal/outbox 或放宽容量保护来做测试。
- macOS 测试与 Linux cross-build 不证明真实 UID/cap-drop/RSS/Landlock/同 binary worker 行为；原生 AMD64 门禁仍保留，Q6 必须执行且通过后才可部署。
- 新宿主 Go 运维编排以合成 Docker/HTTP/文件夹具验证，不冒充生产恢复演练；首次升级需从已发布 Hub image 提取 ctl，再备份、更新与核验。真实播放器、外部 Emby/CD2、115账号与媒体未用于本轮修改测试。
- 旧原生接口 getter 暂留是编译期契约保留，不意味着新的公开功能。其它鉴权/SSRF/错误分类/性能风险按 Q4/Q5 清单继续审定。

## 清理与停止状态

- 最终轮独立PG/Redis容器2/2已删除；前一失败轮2/2也已删除，累计4/4用精确ID验证不存在。仅使用media_tool_test/Redis15，其匿名测试卷按--rm清理；现有app/postgres/redis、外部Emby/CD2、业务卷和回滚备份未动。
- 已清理任务GOCACHE、TMPDIR/GOTMPDIR、临时工具/驱动脚本、日志/JSON/清单/指针及本轮Playwright产物；移除本轮新增的107个本地构建文件，保留原webui embed sentinel及预存资源。Go module cache、共享Go缓存、npm缓存与业务数据不清理。
- 按清理前后`du -sk`实际分配量计释放 **3394056 KiB（3.237 GiB）**：任务目录3359300 KiB（其中独占GOCACHE 3314448 KiB）、系统任务临时目录8956 KiB、项目构建生成物1936 KiB、QA产物23864 KiB；不把Docker VM容量或全盘其他进程变化混入该值。
- 最终代码/配置与门禁启动时的公开源码哈希逐文件一致；无go.mod/go.sum、模块pin、65份SQL、根Compose值修改。只提交源码与文档，未提交测试日志/数据库/密钥/初始化Compose。


Q1+Q2 完成后停止，等待规划/审核方验收；不进入 Q3，不 push、不部署、不清 modcache/业务卷或近期回滚备份。
