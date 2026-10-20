# 新阶段 Q：整合、优化、测试与生产评估 — Q0 审计与基线

> 范围：只读业务代码审计、隔离基线测试及本报告；不是 Q1 开工单，也不是生产就绪或部署验收。
> 审计提交：`74ef5f507a031aca8fbfc96936c739ad3f25a0dc`。任务指定日期：2026-10-20；测试时间使用 Asia/Shanghai。
> 阶段门：Q0 完成后等待所有者审核；Q1–Q5 仅规划，提交、Actions、Hub 与服务器操作按本次明确顺序留到 Q6。

## 0. 执行边界、统计口径与审核状态

- 已先读取根 `AGENTS.md`。保持 Go/Vue、PostgreSQL/Redis、唯一三服务根 Compose、编译期 pin 站点/插件、原始 STRM 与官方 115 播放器 UA → 302 CDN 约束。
- 未修改业务代码、迁移、go.mod/go.sum、CI、根 Compose、已有凭据、媒体、STRM 或生产数据库；未连接服务器、重置生产密码、提交、推送或部署。
- 基线在 HEAD 的临时 `git archive` 快照执行；仅使用 `.env.test` 指定的本机 `media_tool_test`，Redis URL 强制 `/15`；应用密钥、账号和媒体样本均为临时合成夹具。未把原 `.env` 或其应用密钥复制到快照。
- 快照中的锁文件链接到原仓库 `data/.isolated-test.lock`，实际复用同一 inode 的 `test-lock`，不是另开一把锁。Go DB 测试、E2E、测试库元数据/EXPLAIN 均串行。没有运行真实上传、远程播放、生产备份/恢复脚本。
- 当前 **19 个 `cmd/*/main.go`，不是 26 个**；**23 个 Git 跟踪的 Python 文件 = 12 个工具/夹具 1007 行 + 11 个测试文件 533 行 = 1540 行**；**65 个 SQL 迁移，1–65 连续**。忽略的私有诊断目录不属于这 23 个公共脚本，未执行或纳入公开内容。
- Go 非测试源码共 119708 行，其中两个 CloudDrive2 生成文件 43502 行；不能把生成协议绑定当重复业务代码删掉。行数估计均为量级，不是已实现收益。
- Claude 审核存在阻塞：按委派技能先执行 `codexhost delegate --help`，返回 `unknown command 'delegate'`；当前 `codexhost --help` 也未提供委派能力。**没有创建 Claude 子会话，没有 delegationId/threadId/turnId/deepLink，也没有 Claude 已审核的结论。** 本报告由执行方形成，需补 Claude 规划/审核后批准后续阶段。

## 1. 基线测试

以下耗时为命令实际 wall-clock（秒），含 Go runner 的迁移/串行锁/编译；同一任务缓存逐步变热，不是生产性能基准。

| 门禁/命令 | 结果 | 耗时（秒） | 数量/限制 |
|---|---|---:|---|
| `go build ./...`（darwin/arm64） | PASS | 27.293 | 全仓，不改 go.mod/go.sum |
| `go vet ./...` | PASS | 6.594 | 全仓 |
| `bash scripts/test-go.sh` → `go test -p 1 -race -count=1 -v -timeout=20m ./...` | **最终全仓 PASS** | **642.804** | 79 个有测试包通过、30 个无测试包；1047 个顶层 PASS、15 个顶层 SKIP；含子测试 PASS 2254；无 race 告警 |
| 其中 migrations / tests 集成包 | PASS | 326.843 / 4.303 | 完整迁移/原生媒体/合成协议集成，非仅普通单测 |
| errcheck v1.9.0 `-ignoretests ./cmd/... ./internal/... ./migrations/... ./tests/...` | PASS | 1.398 | 生产错误处理边界；工具构建另耗 5.965 |
| Linux/amd64 `CGO_ENABLED=0 go build ./...` / 同标签 errcheck | PASS / PASS | 42.723 / 1.831 | 仅 cross-build/静态分析，不冒充 Linux 执行 |
| `npm test --prefix web` | PASS | 4.615 | 47 文件、157 项 |
| `npm run build --prefix web` | PASS | 12.651 | vue-tsc + Vite；已有依赖，生成物只在临时快照 |
| `python3 -m unittest discover -s scripts -p 'test_*.py'` | PASS | 3.327 | 33 项；Python 内部报告 3.141s |
| `bash scripts/test-e2e.sh` | PASS（平台 skip 单列） | 183.491 | Playwright 单 worker：122 PASS、1 SKIP（Linux 同 binary worker） |
| 已有 govulncheck v1.8.0 `./...` | PASS，有非可达模块公告 | 3.652 | 0 可达/0 导入包漏洞；1 require-only 公告，见1.2；工具构建另耗5.380 |
| plugin-check / plugin-assets --check / 同 pin 资源生成 / matrix --check | PASS | 0.716 / 0.711 / 0.284 / 0.126 | 构建门禁；无业务实例创建 |
| `bash scripts/test-plugin-kernel.sh` | **BLOCKED（不是通过）** | 23.586（含锁等待） | 前置条件拒绝 darwin/arm64：Native Linux AMD64 release gate required；未执行内核测试 |

首次完整 Go race 为 **FAIL，864.321s**：76 包 PASS、3 包 FAIL（filemanager、plugins/business、migrations），15 个顶层失败，其中 14 个受临时目录保护影响、1 个是缓存性能门禁。仅改 TMPDIR 的两包复核仍失败（86.664s），定位 Go 1.26.9 的 GOTMPDIR 行为后同时修正两变量，**再次完整重跑全部包通过**，不是把“挑失败用例复跑”冒充全仓绿。

缓存性能另做同断言 `-race -count=3` 串行隔离复核，3/3 PASS（整个命令36.939s）：冷进程缓存命中117.295/113.035/133.062ms，均3 SQL；最终全仓中命中103.336ms、3 SQL，首个miss478.716ms。保留首轮1.844s/9 SQL失败证据为稳定性跟踪，不放宽200ms/≤3 SQL门禁。

15 个顶层 Go SKIP 中，13 个为 Linux真实worker/RSS/acceptance 门禁，另2个是显式自有导出夹具/子进程专用helper；不是15个业务断言被手工跳过。E2E的1个Linux测试与独立kernel门禁同样尚待原生AMD64环境。没有新增 skip 或删除测试。


### 1.1 可复现方式与环境修正记录

- 环境：macOS/darwin arm64，Go 1.26.9，Node 26.0.0，npm 11.12.1，Python 3.14.4；本机已有 PostgreSQL 18、Redis 8、ffmpeg/ffprobe、前端依赖和锁文件匹配的浏览器。
- Go 命令使用任务独占 GOCACHE/GOTMPDIR、`-mod=readonly`、`GOTOOLCHAIN=local`、`GOPROXY=off`；没有清理或更新 modcache，没有改项目依赖。errcheck v1.9.0、govulncheck v1.8.0、deadcode（x/tools v0.50.0）均从已有 module cache 编译到任务临时目录。
- 初次环境探测：Homebrew 的实际本地 Go 是 1.26.4；普通 `go version` 自动选择缓存中的 1.26.9，强制 local 后才暴露差异。已显式使用缓存的 1.26.9 工具链 bin 目录，不降 go.mod 版本。工具的 `go run name@version` 离线还会请求 deprecation 元数据，改为在已有工具模块内 `go build -mod=readonly`，未新增模块。
- 临时路径首轮位于 `/tmp`，macOS canonical 路径是 `/private/tmp`，被文件管理器保护规则拒绝（`internal/modules/filemanager/capabilities.go:23`、`automatic.go:39`）；相关用例因此失败，且一个测试继续使用失败构造器的 nil 返回而 panic。第二次仅改 TMPDIR 仍失败；读取当前 Go 1.26.9 的 testing.TempDir 源码确认它优先使用 GOTMPDIR，最终将 **TMPDIR 和 GOTMPDIR 一起**设为系统 `/var/folders/.../T` 下的任务专属目录后全仓复核。没有修改或放宽产品路径保护、测试断言、时限。
- 不把重跑掩盖为首次全绿；首轮、复核及无法执行的原生 Linux 门禁分别列出。性能门禁失败也不通过放宽阈值消除。
- Vue 构建包含 vue-tsc；资源先按同一个 plugin pin 生成到临时快照。原仓库生成资源和 node_modules 不被重装、删除。

### 1.2 漏洞扫描解释

已有 govulncheck 扫描实际完成：可达符号 0、导入包 0；require 的 `golang.org/x/crypto@v0.57.0` 有模块级公告 **GO-2026-5932**（未维护的 openpgp、无修复版本），当前代码未导入/调用受影响包。不是“整个依赖树无公告”，也不应因此删除正常使用的 Argon2 等 crypto 功能。扫描数据库可联网访问；工具及模块来源是已有缓存。Q5/Q6 需重新扫描，不把本次结果当长期保证。

## 2. 重复、死代码、遗留格式与文档

### 2.1 可合并的 helper：先保语义，再减少代码

AST 排除测试与 `.pb.go`，找到 **5 组完全相同函数体**；另有相似但契约不同的实现，不以名字相似直接合并。

| 类别 | 证据位置 | 合并方案/边界 | 预计净减少 |
|---|---|---|---:|
| 文本 ID 校验 | `internal/modules/mediausers/repository.go:31`；`internal/modules/playeraccess/model.go:78` | 完全相同的 15 行；共享特定文本 ID 校验，不等同 UUID | 8–12 行 |
| 标题归一化 | `internal/modules/library/recognition.go:33`；`internal/modules/search/media_match.go:14` | 完全相同；保留 Unicode 字母数字规则；不能替代完整媒体身份匹配 | 4–7 行 |
| 媒体重试表 | `internal/modules/mediaprobe/probequeue/model.go:58`；`stillqueue/model.go:110` | 完全相同；共享建议间隔与 24h 上限；不要把下载未知结果当可重试 | 4–7 行 |
| 时间取小值 | `internal/modules/mediaprobe/linksource/source.go:180`；`internal/modules/plugins/v2/workflows.go:455` | 同体；共享 time.Time.Before/零值语义的小 helper，不能直接对 time.Time 使用内建 min | 3–6 行 |
| UUID 校验 | `internal/modules/plugins/business/chain.go:499`；`internal/modules/plugins/v2/store.go:28` | 同体；共享后再比较 probequeue/stillqueue/linksource 的 UUID 接受范围 | 2–4 行 |
| HTTP/JSON 响应 | `internal/platform/httpapi/api.go:457`、`:465`；`internal/modules/playback/engine.go:338`；`internal/modules/embyapi/errors.go:12` | 可共用底层写 JSON/写失败处理；**保留管理端 Envelope、播放错误与 Emby ResponseStatus 三种线协议** | 30–60 行 |
| 分页 | `internal/platform/httpapi/pagination.go:14`；`media_user_management.go:9`；`plugins_v2.go:198` | 统一有界整数/重复参数解析；保留 page/pageSize 与 startIndex/limit、100/200 上限和插件 offset≤10000 | 25–45 行 |
| 错误/事务包装 | `internal/platform/database/`、`internal/platform/cleanup/`；`internal/platform/httpapi/pipeline.go:102`、`:122` | 优先复用现有 Commit/rollback/CheckEffect，统一 NotFound/Conflict/Unavailable 分类，不建立大杂烩 util | 30–80 行 |
| 退避/路径处理 | `internal/modules/catalog/sources.go:63`；`internal/adapters/tmdb/request_control.go:49`；`internal/modules/filemanager/local.go:19`；`internal/platform/artifacts/write.go:18` | 退避机制可参数化，业务重试资格不共享；os.Root、URL path、115 云盘 path 不混为一种路径 | 40–100 行 |

保守合计约 **150–320 行**；业务状态机、鉴权和安全拒绝策略不为减少行数而折叠。

### 2.2 不可达符号与包

已有缓存构建的 `deadcode ./...` 得到 **109 个生产入口不可达函数**；加入 `-test` 后仍有 **19 个不可达函数**。RTA 结果针对本机 build tags 和本提交的 main/test 入口；接口、测试接缝、平台差异与将来 SDK 契约需人工复核，**不是批量删除授权**。

| `-test` 后仍不可达 | 证据 | 建议 |
|---|---|---|
| DownloaderService.Owned、safeName、intFromString | `internal/modules/acquisition/plugin.go:72`；`transfers.go:118`、`:625` | 删除未用包装/旧 helper，保留实际下载所有权检查 |
| classification.Repository.Save | `internal/modules/classification/classification.go:234` | 核对当前带 revision 的保存路径后删旧入口 |
| isBrowserPlayer、Gateway.DirectPlayOnly | `internal/modules/emby/browser_policy.go:26`；`gateway.go:570` | 保留实际浏览器策略；删未用别名 |
| nfoFiles、writeManaged | `internal/modules/library/artifacts.go:49`、`:166` | 合入实际 Journal/Stage 路径，不删 NFO/字幕功能 |
| mediausers.Repository.Login、Session、ChangePassword、Logout | `internal/modules/mediausers/repository.go:157`、`:207`、`:271`、`:313` | 当前原生认证走 nativeauth；删除平行老认证实现前逐项映射限流/设备/失效契约 |
| HierarchyRepository.GetSeries | `internal/modules/nativelibrary/catalog/hierarchy_repository.go:111` | 与当前 browse/hierarchy 投影核对后去掉孤立入口 |
| nativeFetch、RepositoryCatalog、ImportPinned、ImportSnapshot | `internal/modules/sites/native_repository.go:75`、`:124`、`:127`、`:130` | 删除过时远程 JSON catalog 入口及其仅测试可达实现，保留精确 pin 编译型 GoTemplates |
| nodeAttr、artifacts.Equal | `internal/modules/sites/page_links.go:135`；`internal/platform/artifacts/write.go:57` | 确认无生成/平台引用后删除 |

上述 19 个声明体合计 232 行，连同失去入口的辅助实现/导入，约可减少 **350–650 行**；与下表和 helper 估计有交叉，不能直接相加。

- **生产未被 import 的包**：`internal/modules/notifications/notifications.go:1` 只有旧 Event/NotificationChannel 合约，当前真实消息流为 messaging，可删旧空壳；`internal/platform/testworker` 仅供测试 Ready 屏障，**必须保留**，不是业务死包。
- **仅测试可达候选**：`internal/platform/httpapi/native_libraries.go:51` 的 listNativeLibraryAssets 没有路由注册；`site_metrics.go:99` 的 saveSiteMetrics 与实际 sitemetrics 存储并行；`internal/modules/sites/native_repository.go:36` 起的远程 catalog/import 测试链。建议 Q2 对齐实际 API/后台接线后删除旧实现或把通用逻辑移到真实路径，不能仅保留“测得到、线上用不到”的替代实现。
- **暂不删的生产不可达项**：native services 的空实现/构造器、DTO UnmarshalJSON、契约适配器、test-only helper，以及 Linux 专用隔离路径；先做真实入口/SDK contract 映射。CloudDrive2 生成代码完全排除。

### 2.3 `cmd/` 逐个核对（19/19）

“使用”仅依据仓库直接调用、Dockerfile、CI 与运维文档；没有查服务器 shell 历史，不能宣称从未有人手工使用。所有 cmd 都会被全仓 Go 测试编译；下表“CI/运维”特指显式功能入口，不把这种编译等同真实运维验收。

| 工具 | 用途 | 现有 CI/运维调用证据；主镜像携带 | 建议 |
|---|---|---|---|
| media-tool | server/worker/all/healthcheck/strm/account-info 与隐藏 plugin-worker | `deploy/Dockerfile:29`；`Makefile:11`、`:22`；是 | 保留唯一 app，逐步收敛 role 为子命令；worker 同 binary 协议不变 |
| migrate | goose up/status/down、test-only、清测试插件夹具 | `scripts/test-go.sh:24`、`test-e2e.sh:23`；是 | 合为 app migrate；生产 Down 保持显式危险操作 |
| admin-reset | 离线管理员重置、0600 密码文件、停服确认 | `docs/ADMIN_ACCOUNTS.md:42`；是 | app admin reset；不是测试夹具，不调用生产 |
| rekey | 离线主密钥轮换、默认只校验回滚 | `docs/ARCHITECTURE.md:12`；是 | app secrets rekey，保留全部历史密文 AAD |
| backup-verify | 只读源 snapshot、克隆库迁移/密文验证 | `scripts/backup-restore-check.py:27`、`:49`；是 | app backup verify；宿主备份编排单独 ctl |
| ops-control | 审批、执行票据、维护/恢复状态 | `scripts/ops-owner.py:58`、`:104`；是 | app ops；宿主 ctl 只做 Docker/文件编排 |
| cloud-upload-smoke | 显式确认的自有小样本上传、持久 journal/恢复 | `docs/OWNED_UPLOAD_VALIDATION.md:22`；是 | app diagnose cloud-upload；不能以真实媒体作为回归夹具 |
| cloud-events-check | 1–30 秒只读云盘变更观察 | `docs/CLOUD_CHANGE_SCANNING.md:23`；是 | app diagnose cloud-events |
| site-compat-audit | 本地模板目录/固定提交兼容性离线审核 | `docs/NATIVE_ERROR_AUDIT.md:73`；是 | build/dev site-audit；不把 commit/source 选项加入模板 UI |
| site-template-sync | 刷新已选模板，或显式加载指定 IDs | `docs/PT_GO_TEMPLATES.md:13`；是 | app sites sync；不默认全量加载 |
| plugin-assets | pin 资源清单核验及 ESM 构建输入导出 | `.github/workflows/build.yml:59`、`:60`；否，构建阶段运行 | dev plugins assets；不能运行期下载/动态编译 |
| plugin-check | 初始化副作用/资源 pin 核验 | `.github/workflows/build.yml:58`；否，构建阶段运行 | dev plugins check |
| align-cloud-download-ua | 显式 apply 对齐 CD2 UA，验证失败恢复 | `docs/REFERENCE_EMBY_EXTERNAL_URL.md:30`；否 | app diagnose cloud-ua；保留已有参数和回滚行为 |
| complete-emby-layout | dry-run/apply 已知 STRM 映射与专用 Emby 库 | `docs/PROVIDER_CONFIG_VALIDATION.md:22`；否 | app diagnose emby-layout；先确认当前仍需此迁移辅助功能 |
| gateway-session-smoke | 实际网关会话/已存在会话绑定诊断 | `scripts/remote-session-check.py`；否 | app diagnose session，保留显式会话导入确认；Q0 不运行 |
| integration-smoke | CD2/qB/Emby 配置连通与保存期校验 | `docs/PROVIDER_CONFIG_VALIDATION.md:22`；否 | app diagnose integrations |
| playback-smoke | 有界 STRM/播放器 UA/实际 CDN 读取诊断 | `docs/NATIVE_ERROR_AUDIT.md:61`；否 | app diagnose playback；只认 302/CDN，不恢复 relay |
| provider-smoke | 私有 CD2 探测或回环诊断代理 | `docs/NATIVE_ERROR_AUDIT.md:71`；否 | dev diagnose provider，默认仅回环；旧多容器用途需所有者确认 |
| secure-cloud-download | dry-run/apply 仅关闭 CD2 useHttpDownload | 无仓库调用方；否 | 保留为 diagnose cloud-security 子命令，或经确认删无消费的独立入口 |

目标是 **app `media-tool` + 构建/测试 `media-tool-dev` + 宿主运维 `media-toolctl`** 三个用途明确的入口，而不是把全部实现塞进一个 main.go。宿主 ctl 只编排 app/postgres/redis，应用功能仍在 app。重组不等于删除功能；取消某诊断能力需所有者确认。预计共用配置/DB/JSON 输出引导可减少 **250–500 行**，独立 cmd 数由 19 收敛至 3 左右。

### 2.4 旧格式与文档

- 旧 WASM **没有发现生产运行器或 wazero 依赖**；迁移 62 把旧六表重命名为 `plugin_legacy_wasm_*`。`wasm_module/package_files/ui_html` 是封存数据，不是仍在加载远程插件。`internal/platform/secrets/rekey.go:25`、`:26` 及 `internal/platform/backupverify/verify.go` 仍有意保护旧密文，不能误删。
- `internal/modules/sites/native_repository.go` 远程 catalog 路径已经不可达，仍保留网络下载和“空 ids 加载全部”的旧实现；Q2 删除代码/过时测试，不迁移到新运行时。当前有效入口 `go_templates.go:29`、`:72` 只刷新已选/既有账号依赖项并显式加载。
- `docs/HLS_STARTUP_RELIABILITY.md:1`、`scripts/remote-hls-check.py:1` 指向旧 HLS/编码验收；Q1 改成“禁止 HLS/relay”负向回归并历史归档说明，不能平移成可用转码功能。
- `docs/REMOTE_TEST_DEPLOYMENT.md:7`、`:102` 仍记录 server/多个 worker/外部服务的旧部署；只可作为历史证据，应加醒目标记，不与根三服务部署指南并列当现行步骤。
- `docs/ALPHA_QUICKSTART.md:12`、`:72`、`:73`、`tests/README.md:11` 和 M0/REAL_PLAYBACK 文档引用 Python/旧端口；Q1 同步改入口，保留“真实播放器未验证”的限制。
- `docs/PLUGIN_FRAMEWORK.md` 已注明 F1–F6 历史、V2 和精确 pin，不整份删除；`docs/plans/20261017-plugin-system-v2.md` 是长期阶段记录，不应机械当现行待办。建议将当前约束统一链接到 README、PLUGIN_SUPPORT_MATRIX、DIRECT_PLAY_ONLY、NATIVE_SITE_TEMPLATES。
- `docs/IMPLEMENTATION_STATUS.md`、`DEVELOPMENT_PLAN.md`、`REMAINING_PLAN.md`、`WORK_LOG.md` 与多份 acceptance 文档建立“当前规范/历史验收/已取代”的索引；本轮不改写历史成功记录，也不把旧验收充作 Q7 证据。

## 3. Python 全量替代清单（23/23）

删除列均指 **Go 替代通过等价测试并更新全部调用方之后**；不是 Q0 现在删除。T 类脚本由 unittest discover 间接调用（`.github/workflows/build.yml:87`），不能以没有直接名称引用判定死代码。

| Python 文件（行） | 用途与实际调用方 | Go 替代落点 | 可删除/保留注意 |
|---|---|---|---|
| backup-restore-check.py（68） | 私有 pg_dump、fresh clone 恢复验证；RAW_LOGS_AND_BACKUP/OFFLINE_HARDENING/ALPHA 指南 | ctl backup verify → app backup-verify | 可；保留源 snapshot/checksum、0600、只删自建克隆；不是基线自动测试入口 |
| e2e-emby-fixture.py（14） | 保存期 `/Users` 认证回环夹具；`test-e2e.sh:60` | dev fixture emby 或 Go 测试 fixture server | 可；仅合成 token，不访问实际 Emby |
| init-compose.py（25） | 仅替换根 Compose 的 CHANGE_ME；README/deploy/Compose 注释 | ctl init-compose | 可；原值不变、0600、原子写、无 .env 插值、凭据不 stdout |
| init-env.py（15） | 本地开发初始 .env；Makefile/dev/load-env/config 错误提示 | dev init-env | 可；只本地开发，已存在拒绝覆盖 |
| load-check.py（30） | readyz 有界并发/延迟报告；OFFLINE_HARDENING | dev diagnose load | 可；不把 readiness 吞吐称媒体/CDN 吞吐 |
| ops-owner.py（212） | 宿主审批恢复/备份/回滚编排；SYSTEM_OPERATIONS、API 返回命令、MaintenanceView | ctl ops，调用 app ops | 可；保留 flock、审批票据、immutable Hub、停写/健康/回执、不猜测不确定执行结果 |
| plugin-support-matrix.py（68） | pin 的 SDK/registry/契约清单生成；CI:61、对应测试 | dev plugins matrix --check | 可；不读远程源码/不更改 pin |
| remote-hls-check.py（241） | 历史 HLS 首播/seek/独立会话清理；HLS 文档和测试 | Go 负向禁止 HLS/relay 协议测试 + diagnose playback | **不逐功能复刻旧转码流程**；保留负向保障/只读诊断，历史正向工具撤下需确认 |
| remote-session-check.py（98） | 既有 Emby 私有登录会话/诊断设备；REAL_PLAYBACK_VALIDATION | app diagnose session | 可；保留普通用户门槛、管理员诊断明确标志、不能导出 token |
| test-lock.py（10） | Go/E2E/kernel/native 共用文件锁 | dev test-lock/test runner，Unix flock 系统调用 | 可；同一绝对锁路径/单次继承，不能各开一把“独立”锁 |
| test_bootstrap_policy.py（47） | T：bootstrap/入口参数策略 | cmd/media-tool 或 dev runner 的 Go 测试 | 可；保留拒绝不安全启动/已存在密钥不覆盖 |
| test_browser_policy.py（30） | T：浏览器策略/Node harness | Go testing 调用现有 CJS harness | 可；仍测实际 JS 表达式，不只测源码字符串 |
| test_native_media_ci.py（26） | T：CI 原生 ffmpeg/race/错误边界 | Go CI policy test | 可；保留 native AMD64 与真实媒体强制门禁 |
| test_ops_owner.py（20） | T：宿主镜像/回滚策略 | ctl Go 测试 + fake docker executable | 可；不在单测访问 Docker/生产恢复 |
| test_plugin_support_matrix.py（13） | T：支持矩阵 freeze 一致性 | dev plugins matrix 的 golden test | 可；生成与 --check 使用同一函数 |
| test_plugin_v2_ci.py（41） | T：pin、kernel、资源与同 app worker CI 策略 | Go policy test | 可；继续检查 UID/cap-drop/worker 必跑条件 |
| test_remote_hls.py（157） | T：URI 限界、普通用户、独立会话/片段选择 | Go 播放协议/诊断策略测试 | 可；与旧 HLS 正向工具拆开，不借删除测试弱化禁止转码 |
| test_startup_guard.py（20） | T：浏览器启动脚本 guard | Go testing 调用 Node startup harness | 可；保留未知脚本 fail-closed |
| test_test_go.py（115） | T：测试 DSN/DB15、串行、race/20m、参数拒绝 | Go runner 表驱动/子进程测试 | 可；不允许 override -p1、不加载生产配置、不能用真实恢复密码 |
| test_verify_images.py（34） | T：旧本地双架构 smoke 命令无账户/卷 | Go 镜像诊断参数/策略测试 | 可；和现行官方 AMD64 Hub 验证口径对齐 |
| test_verify_m0.py（30） | T：数据库配置、STRM 引用、凭据不导出 | diagnose playback 的 Go fixture 测试 | 可；把“200 也通过”的旧口径收紧为 stable URL → 302 → CDN |
| verify-images.py（41） | 历史本地 arm64/amd64 镜像无卷 smoke；仅对应 Python 测试引用 | dev diagnose image；Q6 用已发布 immutable Hub 引用 | 可；当前无 CI/现行运维直接调用；不成为本地镜像代替发布的通道 |
| verify-m0.py（185） | 登录/preflight/STRM/CDN 有界读取；ADMIN_ACCOUNTS/ALPHA/M0/OFFLINE/tests README | app diagnose playback 或 ctl 包装 | 可；read limit、禁止跟随不安全目标、真实 UA、私密回执；不宣称播放器解码 |

### 3.1 还必须处理的内联 Python（不包含在 23 文件中）

- `.github/workflows/build.yml:47` 的随机测试配置、`:61` 清单检查、`:87` unittest；目标 workflow 完全不再调用 Python。
- `scripts/test-go.sh:4`、`:8`、`:17`；`test-e2e.sh:4`、`:7`、`:44`、`:58`、`:60`、`:62`；`test-native-emby.sh:6`、`:14`；`test-plugin-kernel.sh:4`、`:16`：锁、DSN guard、DB15、随机密钥/密码、端口检查与 fixture 统一 Go runner。
- `scripts/build-web.sh:6` 的路径限界清理；`scripts/update-server.sh:17` 的根 Compose 单个 image 原子替换；Makefile、dev.sh、load-env.sh 的入口提示全部同步。
- `internal/platform/httpapi/maintenance.go:265`、`web/src/views/MaintenanceView.vue` 和 `web/e2e/framework.spec.ts:1384` 的运维命令字符串也要迁移；只改命令和流程，不改视觉。
- 现有 runtime Dockerfile **本来就没有 Python**，目标是去掉仓库/CI/运维 Python，而不是宣称此前 app 依靠 Python 运行。Node/Vue/Playwright 仍保留，前端不在“Python 全改 Go”的替换范围。

## 4. 数据库审计（65，append-only）

### 4.1 已验证的 schema 与历史迁移

只在隔离测试库执行 `BEGIN READ ONLY` 的目录查询：`current_database()=media_tool_test`，`max(applied version_id)=65`；public 共 **121 张表、238 个索引、119 个外键**。PostgreSQL 18 的目录还包含 NOT NULL 类型约束，不能用全部 pg_constraint 条数宣称 CHECK/FK 数。生产在 65 以任务给定事实为前提，本轮未连接生产复核，也没有读取生产行值/容量。

迁移 1–65 完整列于附录 A。**不重命名编号、不改历史 SQL、不 squash**。Q3 只能新增 66 起的迁移；具体分号以实施时审定结果为准。迁移 62 Down 已主动拒绝有 V2 实例的有损降级，不能把“goose down 成功”当生产回滚方案。

### 4.2 过时表/列：可停用代码，不等于可以删除数据

| 对象 | 当前状态及证据 | Q3 建议/生产影响 |
|---|---|---|
| plugin_legacy_wasm_plugins | 原 extension_plugins；含 manifest/config、config_ciphertext、plugin_data_ciphertext、wasm_module、package_digest/files、readme、ui_html；`00062_plugin_v2_runtime.sql:5` | 活跃运行器已无引用；**保留凭据/配置密文和 AAD**。在所有者确认私有备份及保留策略前不 DROP；可先隔离只读归档。旧二进制/HTML 的删除也需明确确认，不自动抹除整个实例 |
| plugin_legacy_wasm_deliveries/runs/repositories/jobs/service_ticks | 原迁移 27/28/30；62 重命名，仅 backupverify 保留校验 | 确认无业务引用后随上一项归档；不能把这些旧 deliveries 当当前 V2 journal；原扩展表名 public 中已不存在，不能再 DROP extension_deliveries |
| site_accounts | 基础迁移旧账号表；当前账号在 integrations(kind=ptsite)，但 rekey 和 download_tasks.site_account_id 仍引用 | **可能有真实历史凭据**，不得直接删。Q2 去掉旧写路径后，Q3 只做保留式归档/映射校验；FK、主密钥轮换与备份白名单同时处理 |
| notification_channels、notification_deliveries.channel_id | 旧通知配置；当前 dispatcher 使用 integration_id；rekey 仍保护 credentials_ciphertext | 原通道可能仍含凭据、旧投递仍有历史引用。确认无活跃 consumer，再保留式归档；不能 truncate 当前 notification_messages/deliveries/attempts |
| bootstrap_imports | 原一次性初始化收据；生产 Go 没有引用 | 先验证对审计/恢复无价值；可新增迁移删除空/已封存收据，不重跑 bootstrap 重新导入配置 |
| site_definitions.local_overrides / common_definition / source_* | local_overrides 没有生产读取；common_definition/source_commit 仍被仓库、search 快照和指标指纹读取 | local_overrides 候选；common/source **不是现在可直接 DROP 的死列**。Q2 清调用链后再评估，并使旧 fingerprint 失效；保留现有 PT 账号 |
| download_tasks.site_account_id 等旧关联 | 当前下载使用 integrations/resource_id 等，旧列仍有 FK 和历史记录价值 | 查明实际值/映射后才能脱钩，不能以新代码不读为理由擦除历史种子、来源和身份信息 |

### 4.3 缺失/冗余索引：按真实查询筛选

静态检查发现 53 个非归档外键没有普通完整左前缀 btree 覆盖；有些存在部分索引、低基数/小表或历史用途，**不建议补 53 个索引**。优先清单：

| 优先项 | 真实查询证据 | 候选与限制 |
|---|---|---|
| 插件所属 jobs 列表/服务去重 | `internal/modules/jobs/plugin.go:23`、`:29`；`internal/modules/plugins/v2/services.go:112`、`:222` | jobs 的 payload->>'instanceId'、serviceId/snapshotId 与 kind/state 查询缺相应索引；按列表和 active service 两种形状分别评估表达式/部分索引，不盲建 GIN |
| manual request 所属证明 | `internal/modules/jobs/plugin.go:23`；迁移 63 的 manual_requests 只有 key 主键 | 评估 `(job_id,instance_id)` 或 `(instance_id,job_id)`，由 EXPLAIN 与分页/取消两类查询定顺序 |
| journal Bridge antijoin | `internal/modules/plugins/v2/events.go:44` | event_id UNIQUE 已在；outbox 现有专用部分索引只含 files/download 三类，Bridge 已处理更多事件。评估匹配事件集合的 `(created_at,id)`，同时解决容量/保留，不能只加索引掩盖全量反复扫描 |
| workflow runs 列表/取消 | `internal/modules/plugins/v2/management.go:105`；`workflows.go:252` | dedupe_key LIKE 'workflow:<UUID>:%' + created_at，现有普通 UNIQUE 的 LIKE 利用受 collation/计划影响。优先结构化 workflow_run_id 及索引，或评估 text_pattern_ops；不直接凭字符串拼接判断 SQL 注入 |
| 真实高扇出 FK | media_assets.cloud_file_id、media_asset_bindings.media_id、managed_media_artifacts.asset_id；native library/资产 join 与清理 | Q3 用合成大样本验证 parent delete/join；保留已有部分唯一索引的业务约束，不为每个 UUID 列盲补索引 |
| 冗余候选 1 | plugin_v2_state_values(snapshot_id) 与 UNIQUE(snapshot_id,kind,key)，`00064_plugin_state_snapshots.sql:31` | 普通单列被前缀覆盖；核对空间/查询后可删单列索引，保留 UNIQUE |
| 冗余候选 2 | native_tmdb_details(tmdb_type,tmdb_id) 与 identity 唯一表达式索引的相同前缀 | 单独窄索引可能有成本收益，需对比，不立即删除 |
| **不可当冗余删** | native_item_ids 主键(id) 与 UNIQUE(id,entity_type) | 后者被复合 FK 引用，前者保证 id 本身唯一；不能因前缀相同二选一。jobs_available 与 queue_available 的过滤/排序用途也不完全相同 |

本轮已在共享锁内、测试库只读事务执行三个 EXPLAIN（无 ANALYZE、未插入性能数据）：所属 jobs 是 jobs/manual_requests 的 Seq Scan + Nested Loop Semi Join + Sort；workflow history 是 plugin_v2_runs Seq Scan + Sort；代表性的 Bridge 来源子集是 outbox/journal Seq Scan + Hash Anti Join + Sort。测试库当时小/近空，这些计划只支持“值得用合成大样本继续评估”的结论，**不能据此推算生产耗时或强制关闭 Seq Scan**；Q3 必须使用完整生产查询形状与合成规模复评。

### 4.4 约束、外键与类型

- `plugin_v2_state_tasks.revision/fingerprint`（迁移 64:26）没有与 instances/snapshots 一致的 `revision>0`、64 hex CHECK；代码校验不是数据库约束。候选新增 NOT VALID → 检查 → VALIDATE，先识别旧异常行，不静默“修正”为新密钥/指纹。
- state_tasks 的 snapshot_id 与 instance_id 分别有 FK，**没有组合所有权约束**；建议评估 snapshot `(id,instance_id)` 唯一及复合 FK，先以测试证明不能把他人快照拼到本实例任务。API 已做所有权检查，此处是持久层纵深防御，不宣称已发生跨实例读取。
- `plugin_v2_manual_requests.job_id`、`plugin_v2_schedules.wake_job`、ops_backups/ops_drills.job_id 没有 FK；是否新增需先确定 jobs 的保留/删除语义。不能用 CASCADE 删除幂等收据或历史备份票据。`plugin_v2_instances.owner_id` 是运行租约所有者，不是管理员 account FK；provider_file_id、TMDB/Emby 外部 ID 也不应该强连本地账号表。
- `search_runs.account_ids`、subscriptions.identity/rules/coverage、download_tasks.details 混合 JSONB 是历史演进结果。Q3 优先补 JSON 类型/必要字段 CHECK；频繁过滤的 tmdb/type 已有独立列，不为“第三范式”强拆所有 JSON、重写整条订阅/下载快照。
- `player_access_events.client_ip/connection_ip` 是 text，若做网络查询可考虑新增 nullable inet 派生列；必须保留原始输入和 ip_source，非法/代理链值不能强 cast 失败而丢日志。暂不修改仅用于显示的外部 ID text。插件运行租约与瞬时 CDN 链接不得为了建表规范化而成为可复用播放凭据。
- 时刻主体已使用 timestamptz；goose 自身 tstamp 是 timestamp，不迁移第三方版本表。`native_server_settings.server_id` char(32) 可评估 text+长度 CHECK，但收益较低且原稳定 ID 不可重生。
- 跨模块 state/cancelled/canceled 拼写与 reason 枚举分散；新增 CHECK 必须按模块允许值验证，不能把 V2 的 canceled 改成 jobs 的 cancelled 或盲建全局 PostgreSQL ENUM。

### 4.5 生产备份、锁与回滚要求

Q3 的测试仅用 media_tool_test；**不以生产克隆或恢复密码做集成夹具**。上线前 Q6 在私有目录保留当前根 Compose、schema65 一致性 pg_dump、自有 STRM/插件状态快照/主密钥与授权恢复材料的现有保护链、前一 immutable Hub 摘要和备份校验回执；不把密钥放进 Actions/Release。备份解密/一致性验证遵循所有者授权的运维恢复流程，不是扩大普通测试库白名单。

- 先用合成 65 库验证 65→66+、中断恢复、非目标表/密文/AAD/STRM 身份不变，再部署。
- 普通索引/NOT VALID 操作需要明确 lock_timeout/statement_timeout；大表若使用 CREATE INDEX CONCURRENTLY，须专门的 goose 非事务迁移和失败索引恢复，不把它混入事务里。
- destructive Down 不作为回滚承诺：新镜像回滚若不兼容 append-only schema，需停 app/全部 app 内 worker，再恢复**同一备份集合**；不能只倒 image 或只恢复数据库却沿用已变更文件/密钥。
- 保留近期回滚备份；不删除 Emby/CD2 配置、媒体、115 授权/账号/主密钥/业务卷；不执行全局 docker prune。

## 5. 使用逻辑与流程问题（后端优先、严重度排序）

| 编号/级别 | 证据与问题 | 后续措施；现有正确行为 |
|---|---|---|
| F1 / P1 | `plugins/v2/events.go:36` 达到 100000 journal 行就返回 Busy；`maintenance.go:23` 在 Deliver、Schedule 之前直接 return；代码未见 journal GC | 长期积累会使事件投递和服务调度一起停，不只是新事件入队。Q3 设计 cursor/dedupe/迟提交安全的保留协议；Q4 拆分维护故障域、可恢复容量告警；不盲删 journal/outbox |
| F2 / P2 性能稳定性 | 原生库 10000 影片冷进程 Redis 命中性能/SQL 门禁首轮未过，见基线与 B2 | 隔离复测 3/3 已通过（117/113/133ms，均3 SQL），首轮失败不抹去；MVCC fence 对不相关写入也很保守（catalog/cache.go:51），再分解 cache budget/网络/权限 fence 与全仓运行干扰，不把门禁放宽来做“优化” |
| F3 / P2 | `httpapi/pipeline.go:102`、`:122` 把 QueryRow 的任意错误当 404 | 区分 pgx.ErrNoRows 与超时/数据库故障；其他 create/get/retry 错误边界一并列清，避免用户遇到“任务消失”后误重复操作 |
| F4 / P2 | 同一链路分别用 download_state/pipeline_state/transfer state/jobs state，重试还按固定 idempotency_key 找 failed job（pipeline.go:82–158） | 返回结构化 phase/retryable/nextAction/reason；失败但缺 job、不支持阶段、结果未知、需源/输出重确认分别解释；**保留现有未知上传不重建会话、源快照需重确认** |
| F5 / P2 | 通用分页、管理分页、插件分页参数/上限/重复参数策略不同 | 共享解析器但保留线协议；重复参数/越界统一明确 400；清空或最后一页不应鼓励重跑搜索/订阅 |
| F6 / P2 | 115 交换成功只保存并保持未启用（`integrations/open115_authorize.go:275`），模板加载/PT账号/115/CD2/STRM/Emby 是多个状态域 | 这是正确安全默认，不改成自动启用；Q4 加后端 readiness/nextAction：授权→验证→启用→文件身份→STRM→Emby；明确授权成功不等于可播 |
| F7 / P2 | 插件安装默认停用、权限变化 needs_action、missing 保数据；auth provider UI/API 明确 501（`httpapi/plugin_workflows.go:90`） | 保留审批和停用语义；接口能力元数据清楚区分“不支持/待授权/运行失败”，不把 Catalog 中可见等同已安装可执行，不将 501 伪装重试成功 |
| F8 / P2 | ops API 已生成审批票据，但返回的是宿主 Python 执行命令（maintenance.go:265）；approved 不是 completed | Q1 换 Go ctl 命令，Q4 统一 preview→approve→execute→verify 状态与不确定回执；UI 交互提示可改，视觉不动 |
| F9 / P2 | `verify-m0.py:158`、`:164` 允许 200/206/302，通过口径可接受服务器媒体流；历史 HLS 正向工具也仍可运行 | Go 诊断严格分开 stable resolver 的 302 与 CDN 的 206，确保实际播放器 UA；原生/外部 Emby/真实播放器的验收不能互相冒充 |
| F10 / P2 | `Makefile:14` 的 test 直接运行 go test，不经 test-lock/测试 DSN runner；`.github/workflows/build.yml:51` 初始 Redis URL 是 DB0，靠子脚本改成15 | Q1 将 Makefile/CI 测试配置统一收敛到 Go runner，入口默认就是 DB15；未设置集成 DSN 时显式说明 skip，不能把 make test 的绿等同完整集成绿。当前 Q0 始终走原共享锁和 DB15 |

订阅实现已有媒体级 advisory lock、revision 复检、disabled 非预览取消（`internal/modules/subscriptions/evaluate.go:97`、`:119`），手动下载已有未知结果确认/重复风险门槛（`manualdownloads/retry.go:17`）；本次没有证据证明这些门槛缺失，Q4 是保留并补贯通解释，不重写成自动重放。

## 6. BUG 与安全疑点（确认、静态风险、验证缺口分列）

| 编号/优先级/可信度 | 证据 file:line | 结论与复核要求 |
|---|---|---|
| B1 / P1 / 静态确定容量故障路径 | `internal/modules/plugins/v2/events.go:36`、`:39`；`maintenance.go:23`、`:26`、`:29` | 同 F1：满 journal 阻断 Deliver/Schedule；并非鉴权漏洞，但长期运行生产阻塞，Q7 前必须关闭 |
| B2 / P2 / 首轮性能不稳定 | `migrations/native_cache_integration_test.go:67`、`:68`；`internal/modules/nativelibrary/catalog/cache.go:100`、`:155` | 首轮 10000 片缓存冷进程 hit 1.844s、9 SQL，不满足 <200ms、≤3 SQL，且比 miss 1.022s 更慢。随后同断言隔离复测 3/3 通过（117.295/113.035/133.062ms，均3 SQL）；保留首轮记录，不断言是已稳定重现的业务缺陷，亦不放宽门禁 |
| S1 / P2 / SSRF 防御边界疑点 | `internal/modules/networkproxy/host.go:55`、`:102`；`proxy.go:104` | 无代理时 public DNS 验证且 dial 绑定已验证地址；有配置代理时只做宿主解析检查，真实域名由代理再解析。需恶意/分视图 DNS 的合成代理测试和出口策略，不能把可信 CD2/qB 内网端点一概禁掉；未证实可被匿名用户利用 |
| S2 / P2 / strict 302 约束遗留风险 | `internal/modules/playback/engine.go:121`、`:135`、`:197`、`:202`；`httpapi/api.go:650`；`runtime.go:146` | 仍有非 DirectOnly 的 proxy/fallback 支路及“启用 STRM_PROXY_FALLBACK”提示。官方 115 在 `integrations/open115.go:161` 明确 DirectOnly，受保护；未证明官方链路会 relay。Q2/Q5 锁死禁止中转契约/清旧提示，不静默开启任何 fallback/HLS/转码 |
| S3 / P2 / worker Linux 验证缺口 | `internal/modules/plugins/v2/worker.go:61`；`sandbox/landlock_linux.go:14`；`supervisor.go:536` | env 为显式白名单，不继承 DATABASE_URL/master/provider secrets；Landlock best-effort 且明确 trusted-code 告警，不等于网络/syscall 沙箱。macOS 不可能证明 Linux UID/cap-drop/RLIMIT/Landlock 行为；必须保留原生 AMD64 门禁，不宣称完全隔离 |
| S4 / P2 / 数据所有权约束候选 | `migrations/00064_plugin_state_snapshots.sql:20`、`:26` | state task 的 instance/snapshot 组合与 revision/fingerprint CHECK 缺少数据库纵深约束；API 的 ownership/CAS 已存在，需错配 fixture 验证，未证实绕过 |
| S5 / P2 / SQL 与路径回归面 | `internal/modules/dataview/inspect.go:104`；`internal/modules/filemanager/local.go:19`；`plugins/v2/state_files.go:96` | dataview 先核对 schema 表名、引用 pgx Identifier；SQL 条件拼接多是固定片段+绑定参数。未确认 SQL 注入；os.Root/符号链接拒绝要保留，不能因本轮 tmp 夹具失败而放宽。补编码路径、代理 DNS、复合所有权负向用例 |
| S6 / P2 / 文档诊断验收失真 | `scripts/verify-m0.py:158`、`:164`；`scripts/remote-hls-check.py:1` | 工具成功口径与当前严格 302 有冲突；Q1 迁移必须重建验收契约，不只机械翻译 |
| S7 / 无已确认越权 / 现有保障 | `internal/platform/httpapi/api.go:327`、`:418`；`internal/platform/auth/auth.go:34`、`:74`；`cmd/media-tool/main.go:217` | 管理会话/CSRF/临时密码强制修改/维护停写已存在；原始管理员日志/数据/full URL 是所有者明确要求，**不作为应默认脱敏的缺陷**。登录 body/Cookie/Authorization 不额外采集；恢复口令只传 os.Stdout，不进 web store。私有日志/数据/凭据不得提交或进公共 Actions 产物 |
| S8 / P2 / 私有日志策略不一致 | `internal/platform/httpapi/api.go:433`、`:443` | `/api/v1/plugin/` 路径 query 中 api_key/token 等在进入私有请求日志前被替换为 redacted，与所有者要求管理员 full URL/原值存在例外。不是要求把凭据放进公开产物；需 Q4/Claude 审核明确私有管理员记录与对插件公开摘要的边界，默认不擅自扩大脱敏或公开范围 |

本节不是渗透测试或形式化证明。未发现 P0 的可重现匿名鉴权绕过、SQL 注入或官方 115 自动 relay；“未发现”不等同不存在。race 的实际结果单列在基线，跨进程/数据库 CAS/远程副作用不能仅靠 Go race detector 证明。

## 7. 建议的 Q1–Q5 分组与行数预算

计量：`+~` 是新增/改写非测试代码（Go/SQL/shell/配置），`-~` 是移除量；测试代码另计。每个内部块约≤1200 行，内部连续执行，**只在整个 Q 阶段结束汇报并停下审核**。代码估计不包括大规模自动格式化、生成 pb.go、改 UI 视觉；具体删除需审核映射，不以净负行数绕过预算。

| 阶段 | 内部执行组（同阶段连续） | 估计非测试量 | 阶段验收/门 |
|---|---|---|---|
| Q1 全部 Python → Go | Q1a：dev runner、全局 test-lock/DSN/DB15/随机配置/fixture/build 清理与 CI；Q1b：ctl init-compose/ops-owner/update/backup 编排；Q1c：诊断/matrix 与 Python 测试迁移、文档/命令接线 | a +800–1100；b +800–1100；c +500–900；移除 .py 1540 行及内联 Python | 33 项现有 Python 语义映射到 Go，所有 CI/Makefile/现行文档无 Python 入口；真实浏览器/Node guard/原生 AMD64/20m/race/p1 不降级；只保留严格 302 诊断，不复刻 HLS |
| Q2 代码整合精简、保留全部现行功能 | Q2a：上述 5 exact helpers、分页底层/JSON 写层、19 个强死码与旧站点远程入口；Q2b：cmd 分层子命令、测试-only 平行实现/文档索引 | a +150–350、-450–850；b +350–650、-550–1100；超量按独立文件再内拆 | API/CLI/插件合约映射表；Go+Vue+E2E 不减覆盖；模板选择、115、STRM、通知/字幕/NFO、原生媒体能力均保留；删除诊断能力需明确批准 |
| Q3 数据库 append-only 优化 | Q3a：66+ 查询索引/CHECK/组合 FK、测试库 EXPLAIN；Q3b：legacy 归档与调用方更新，仅执行审核同意的数据保留方案 | a SQL+200–400、Go+150–300；b SQL+100–250、Go+200–400；不可逆删表另设审核门 | 65→新版本、断点续迁、约束 VALIDATE/锁超时、非目标表/密文/STRM/授权不变；不 squash；full backup/恢复方案经审核；不先删凭据再补救 |
| Q4 逻辑/流程优化 | Q4a：F1 容量协议/维护故障域与状态告警；Q4b：订阅→搜索→下载→整理→STRM→Emby nextAction、统一错误分类/幂等回执；Q4c：授权/插件/ops readiness 接线 | a +250–450；b +400–650；c +250–450；超量继续内拆 | 结果 unknown 不自动重复、批量操作/危险操作确认、权限扩大停用、missing 保数据、审批≠完成；UI 仅交互/流程逻辑无视觉改版 |
| Q5 全量测试、BUG/安全修复 | Q5a：基线稳定性/性能与 B1/B2 复核；Q5b：proxy DNS、鉴权/302、路径、SQL、worker/资源/断电恢复负向测试；Q5c：针对证实缺陷的小补丁与依赖复扫 | 修复 +300–900；测试不限本预算；若插件代码需改，先插件仓库测试推送，再 bump 精确 pin | 全量 serial race/集成/E2E、native Linux AMD64 kernel、Vue、errcheck/vet/vuln；没有故意 skip 或放宽 SLA；产出剩余缺口/风险接受清单，不提前称已部署 |

Q6 才执行所有者指定的 test→commit/push dingding229/mediatool→等待 Actions AMD64 Hub 发布→私有备份→immutable Hub image 更新 `/root/docker/mediatool`→验证；插件先 dingding229/mediatool-plugin，再主 pin。Q7 在实际证据上评估生产就绪与路径/label 限界清理，不清业务卷、modcache 或近期回滚备份。

## 8. 风险与需决策项

1. **审核链缺口**：当前 Claude 委派接口不可用；需在继续 Q1 前补 Claude 审核，而非让执行方冒充规划/审核通过。
2. **数据保留红线**：是否只读归档旧 WASM/site_accounts/notification_channels，还是在私有可恢复备份后移除非凭据 payload？默认保留密文/配置/KV/data，不接受直接 DROP 六表的方案。
3. **诊断功能口径**：同意废止 HLS 正向诊断与旧双架构本地图像“发布验收”口径，保留当前严格 302 与官方 Hub AMD64 验证；这与“保留全部现行功能”不冲突，但不能擅自保留转码功能。
4. **容量是发布风险**：journal 100000 行跨调度域阻塞需要在 Q7 前修复并覆盖；保留协议必须避免迟提交遗漏/重投，不能直接清空表。
5. **本地非生产验收**：macOS 全量测试、cross-build/errcheck 不能替代原生 Linux AMD64 worker、真实播放器/CDN、服务器备份恢复验证。未访问生产，不能提供生产执行计划、负载或客户端就绪结论。
6. **cmd 删除与合并**：19 个全部已说明，但无调用证据不是无人使用的证明；默认迁移为子命令保留能力，少数旧诊断的永久删除须确认。

## 附录 A：65 个迁移的逐版本状态

“有效”指 schema65 中仍有对应结构/约束或被后续演进使用，不等于迁移应重新执行。所有历史文件保持不变。

| 版本 | 主题 | schema65 状态 |
|---|---|---|
| 1 | 基础应用/站点/账号/订阅/媒体/任务/消息（`00001_foundation.sql`） | 含仍有效基础表及旧 site_accounts/notification_channels/extension 数据 |
| 2 | 统一 integrations、可靠队列与 bootstrap 收据（`00002_integrations_and_task_reliability.sql`） | 主体有效；bootstrap_imports 无当前生产引用 |
| 3 | 站点搜索/资源/下载关联（`00003_site_search_and_downloads.sql`） | 有效，后续迁移继续扩展 |
| 4 | 上传与媒体流水线（`00004_transfer_and_media_pipeline.sql`） | 有效，后续迁移继续扩展 |
| 5 | 订阅和 RSS（`00005_subscriptions_and_rss.sql`） | 有效，后续迁移继续扩展 |
| 6 | 订阅 feed 批次（`00006_subscription_feed_batches.sql`） | 有效，后续迁移继续扩展 |
| 7 | 云盘库扫描（`00007_cloud_library_scan.sql`） | 有效，后续迁移继续扩展 |
| 8 | 文件级播放授权（`00008_file_playback_grants.sql`） | 有效，后续迁移继续扩展 |
| 9 | 托管媒体输出（`00009_managed_media_artifacts.sql`） | 有效，后续迁移继续扩展 |
| 10 | 通知结果/不确定状态（`00010_notification_outcomes.sql`） | 有效，后续迁移继续扩展 |
| 11 | 可撤销管理员会话（`00011_revocable_admin_sessions.sql`） | 有效，后续迁移继续扩展 |
| 12 | 上传端点快照（`00012_upload_endpoint_snapshot.sql`） | 有效，后续迁移继续扩展 |
| 13 | 离线/恢复安全（`00013_offline_hardening.sql`） | 有效，后续迁移继续扩展 |
| 14 | 输出对账（`00014_artifact_reconciliation.sql`） | 有效，后续迁移继续扩展 |
| 15 | 管理员账号（`00015_administrator_accounts.sql`） | 有效，后续迁移继续扩展 |
| 16 | 官方115播放（`00016_official_115_playback.sql`） | 有效，后续迁移继续扩展 |
| 17 | 官方115令牌交换 claim（`00017_official_115_exchange_claim.sql`） | 有效，后续迁移继续扩展 |
| 18 | 云盘变更扫描 hints（`00018_cloud_change_scan_hints.sql`） | 有效，后续迁移继续扩展 |
| 19 | 输出归档 journal（`00019_output_archive_journal.sql`） | 有效，后续迁移继续扩展 |
| 20 | 归档清理审批（`00020_archive_cleanup_approvals.sql`） | 有效，后续迁移继续扩展 |
| 21 | 搜索评估快照（`00021_search_evaluation_snapshots.sql`） | 有效，后续迁移继续扩展 |
| 22 | 下载来源重新确认（`00022_download_source_reconfirmation.sql`） | 有效，后续迁移继续扩展 |
| 23 | 文件管理（`00023_file_management.sql`） | 有效，后续迁移继续扩展 |
| 24 | 账号恢复（`00024_account_recovery.sql`） | 有效，后续迁移继续扩展 |
| 25 | 工作台设置（`00025_workbench_settings.sql`） | 有效，后续迁移继续扩展 |
| 26 | 搜索缓存（`00026_search_cache.sql`） | 有效，后续迁移继续扩展 |
| 27 | 扩展 deliveries（`00027_extension_deliveries.sql`） | 数据已随62封存为 plugin_legacy_wasm_deliveries |
| 28 | 站点指标/旧插件包列（`00028_site_metrics_and_plugin_packages.sql`） | site_metrics有效；旧包列/runs随62归档 |
| 29 | 订阅规则检查（`00029_subscription_rule_checks.sql`） | 有效，后续迁移继续扩展 |
| 30 | 旧插件框架/仓库/服务（`00030_plugin_framework.sql`） | 约束/旧数据保留在 plugin_legacy_wasm_*，不再执行/加载 |
| 31 | 旧插件 UI（`00031_plugin_ui.sql`） | 约束/旧数据保留在 plugin_legacy_wasm_*，不再执行/加载 |
| 32 | 旧 WASM-only 约束（`00032_wasm_only_plugins.sql`） | 约束/旧数据保留在 plugin_legacy_wasm_*，不再执行/加载 |
| 33 | 原生站点来源字段（`00033_native_site_repository.sql`） | 有效，后续迁移继续扩展 |
| 34 | PT账号唯一性（`00034_unique_pt_sites.sql`） | 有效，后续迁移继续扩展 |
| 35 | 管理员选定站点模板（`00035_selected_site_templates.sql`） | 有效，后续迁移继续扩展 |
| 36 | 站点指标定时（`00036_site_metric_schedule.sql`） | 有效，后续迁移继续扩展 |
| 37 | 订阅系统间隔（`00037_subscription_system_interval.sql`） | 有效，后续迁移继续扩展 |
| 38 | 运行时搜索 cron（`00038_runtime_search_cron.sql`） | 有效，后续迁移继续扩展 |
| 39 | 获取工作区优化（`00039_acquire_optimization.sql`） | 有效，后续迁移继续扩展 |
| 40 | 文件/库工作台（`00040_files_library_workbench.sql`） | 有效，后续迁移继续扩展 |
| 41 | 库可见性（`00041_library_visibility.sql`） | 有效，后续迁移继续扩展 |
| 42 | Emby可见范围（`00042_emby_visibility_scope.sql`） | 有效，后续迁移继续扩展 |
| 43 | 云盘刮削/命名历史（`00043_cloud_scraping_history.sql`） | 有效，后续迁移继续扩展 |
| 44 | 原生媒体用户（`00044_native_media_users.sql`） | 有效，后续迁移继续扩展 |
| 45 | 播放器UA访问控制（`00045_player_ua_access.sql`） | 有效，后续迁移继续扩展 |
| 46 | 访问域名与STRM修复（`00046_access_and_strm_repairs.sql`） | 有效，后续迁移继续扩展 |
| 47 | 播放器客户端地址（`00047_player_client_address.sql`） | 有效，后续迁移继续扩展 |
| 48 | 媒体分类/公开API（`00048_media_layout_and_api.sql`） | 有效，后续迁移继续扩展 |
| 49 | 永久API凭据（`00049_permanent_api_credentials.sql`） | 有效，后续迁移继续扩展 |
| 50 | 系统运维（`00050_system_maintenance.sql`） | 有效，后续迁移继续扩展 |
| 51 | 手动下载（`00051_manual_downloads.sql`） | 有效，后续迁移继续扩展 |
| 52 | 通知投递尝试（`00052_notification_delivery_attempts.sql`） | 有效，后续迁移继续扩展 |
| 53 | 管理员限流窗口（`00053_administrator_auth_limits_window.sql`） | 有效，后续迁移继续扩展 |
| 54 | 备份删除/恢复审批（`00054_backup_delete_recovery.sql`） | 有效，后续迁移继续扩展 |
| 55 | 原生Emby schema（`00055_native_emby_schema.sql`） | 有效，后续迁移继续扩展 |
| 56 | 原生库粒度（`00056_native_library_granularity.sql`） | 有效，后续迁移继续扩展 |
| 57 | 资产库归属（`00057_native_asset_assignment.sql`） | 有效，后续迁移继续扩展 |
| 58 | 原生TMDB详情（`00058_native_tmdb_details.sql`） | 有效，后续迁移继续扩展 |
| 59 | probe attempt/租约（`00059_native_probe_attempts.sql`） | 有效，后续迁移继续扩展 |
| 60 | still attempt/租约（`00060_native_still_attempts.sql`） | 有效，后续迁移继续扩展 |
| 61 | 原生播放报告（`00061_native_playback_reports.sql`） | 有效，后续迁移继续扩展 |
| 62 | V2插件基础/旧六表归档（`00062_plugin_v2_runtime.sql`） | 当前V2基础；Down拒绝有V2实例的破坏性降级 |
| 63 | V2扩展/事件/API/服务（`00063_plugin_v2_extensions.sql`） | 当前V2扩展；journal容量与保留策略待修复 |
| 64 | V2状态快照/任务（`00064_plugin_state_snapshots.sql`） | 当前快照；索引/组合所有权/CHECK候选见4.3–4.4 |
| 65 | V2工具调用 kind（`00065_plugin_tools.sql`） | 当前最高版本；tool纳入runs kind，其他数据不重置 |

## 9. 任务产物、清理与最终门禁状态

- 唯一持久新增文件：`docs/plans/20261020-q-consolidation-audit.md`；业务代码、测试代码、go.mod/go.sum、迁移、CI、根 Compose 均未修改。未 commit/push、未启动 Actions、未发布 Hub、未 SSH/部署；等待 Q0 与 Claude 审核。
- 全仓最终 race、Vue、Python、E2E（平台 skip 已披露）、build/vet/errcheck/vuln 已完成；Linux AMD64 内核/真实worker 是明确未完成项，不把 cross-build 当替代。原本 app/postgres/redis 的容器及镜像保持原状。
- 已确认任务测试/fixture 进程结束，删除任务独占 GOCACHE、GOTMPDIR、系统用户临时子目录、源码快照、合成配置/密钥、临时工具、日志/JSON/EXPLAIN 和临时路径指针；没有将原始运行日志/数据库内容写入本报告或公开产物。
- **按清理前 `du -sk` 实际分配量计，释放 2757612 KiB（2.630 GiB）**：其中任务 GOCACHE 2672476 KiB（2.549 GiB）；任务目录含缓存共 2757612 KiB，系统任务临时目录 0 KiB。两处准确路径删除后均验证不存在；这不是把整盘其他进程的可用空间变化当释放量。
- 未清理共享 Go 构建缓存、**未清 modcache**、未 docker prune、未删除业务卷/媒体/STRM/115授权/账号/主密钥、未删除旧备份。复用的原仓库测试锁本就存在，保留不删。
- 结论：**Q0 执行产出完成，保持阶段停止；Claude审核与Linux原生执行缺口未关闭；不能据此宣称Q7生产就绪。**
