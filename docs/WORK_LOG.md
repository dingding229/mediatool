# Active goal execution handoff

## Goal (unchanged)
根据计划，完成开发，第一轮至少要到可用的状态。
Goal remains active; alpha operation is not a claim that P0-P7 or real-provider acceptance is complete.

## Current authoritative state
- Project root /Volumes/SSD/项目/media_tool; no Git repository initialized, no applicable AGENTS.md in parents.
- Docker daemon is now running via the existing Colima profile. Do not create another VM or replace/delete these volumes.
- Current Compose image media-tool:local, app + control worker + transfer worker + PostgreSQL18 + Redis8 actually running.
- Validation app is localhost:18080 using deploy/compose.validation.yaml; host preview remains localhost:8080. Both share .env administrator key; .env is 0600, never print it.
- Local development DB endpoints 15432/16379 use compose.dev.yaml. Production Compose does not publish them.
- Production database media_tool migrated to version4. Isolated media_tool_test exists and is also migrated. .env.test is ignored and 0600.
- APP_SECRET_KEY generated without replacing prior keys. Registry credentials AES-GCM encrypted, AAD binds recordID. Public DTO returns only secret field names.
- Added typed encrypted integration management and actual probes; CloudDrive2/Emby runtime reads saved profiles; env fallback retained when no saved profile.
- Added runtime site definition importer (licenseAccepted required), fixed upstream commit, NexusPHP/M-Team core parser, cookies/API authentication, per-site durable search with encrypted download hints.
- Added TMDB identity lookup, qBittorrent adapter, confined shared-file checking, download pipeline.
- Added CloudDrive2 RemoteUpload data/hash channel and persisted session, upload queue, size/hash conflict checks, confined STRM generation, media identity records.
- Emby refresh is not considered completion: emby.confirm checks expected STRM paths before imported state. Dedicated refresh key is not used for play permissions.
- Added station inbox + Telegram Outbox delivery; timeouts stay outcome_unknown rather than blind retry.
- New Vue pages: service connections, site config, search/identity selection, downloads/transfers, media index, notifications; task queue has SSE and polling fallback.
- Current modules not complete: subscriptions/RSS, existing-library scan/editing, subtitles/NFO, pure-v2/hybrid validation detail, broad parser grammar/actual-site compatibility, file-scoped playback grants, dedicated Emby port/complete Web/PlaybackInfo rewriting, Redis context cache, upload unknown-outcome reconciliation, Telegram manual retry/error classification/session revocation.

## Verified evidence
- Docker Compose built and started; /readyz returns ready, PostgreSQL/Redis healthy, worker count2.
- Actual Go migrations 00001-00004 completed.
- Real PostgreSQL encrypted connection lifecycle tests pass.
- Real PostgreSQL pipeline test passes: own PT fixture→search→download→shared-file check→gRPC upload→STRM→Emby visible confirmation→durable notification; ORIGINAL FILE UNCHANGED. Fixtures are not user's real providers.
- Go race tests and go vet pass. Last `go test -p 1 -race -count=1 ./...` with TEST_DATABASE_URL completed.
- Five Chrome E2E tests pass against the latest Docker image on18080, including actual task execution, protected credential CRUD and mobile layout.
- OpenAPI validates31 paths; needs further detailed response schemas and future endpoints.
- Backup made with pg_dump -Fc and restored with --exit-on-error into a temporary DB; schema version4 and28 tables confirmed. Temporary restore DB dropped only by exact generated name; original DB untouched. Archive 0600 at data/backups; key not included. Report in data/diagnostics/backup-restore-report.json.

## Known implementation risks to address next
1. Fix downloader path handling: saved qB save_path can be nested/different; current reconciliation assumes base localRoot + file.name. Validate mapping from actual save_path, no directory escape.
2. Fix Telegram delivery result classification and crash recovery: sending records can remain stuck; client currently maps all errors without clear status to outcome_unknown; manual retry UI missing.
3. Fix upload offset0/length0 acknowledgment, partial block MD5 limits and progress semantics (hash progress currently can reach100 before transfer). Bound memory but permit streaming large reads.
4. Add durable upload endpoint snapshot and safe query/list reconciliation for start outcome unknown, not blind retries. User pause/resume/cancel UI missing.
5. Harden STRM output idempotence: pre-check DB and content prevents known conflicts, but atomic file and DB commit aren't one transaction; add managed artifact staging/recovery.
6. Emby confirmation only searches first200 matching items. Add paging/targeted path identity and realistic server path coverage; scan request key scope and gateway source rewriting remain minimal.
7. Full parser coverage: inheriting arrays/filter chains can diverge from upstream; evaluate unsupported fields and requests honestly. `timestamp/timezone` semantics need explicit correctness.
8. Integration env import/credentials rekey/revision-cache cleanup and audit coverage not complete. Manager retains max32 cloud-client revisions until restart, no safe borrower cleanup yet.
9. Tests: pipeline test uses isolated media_tool_test only after SELECT current_database() guard, truncates test tables. Run tests with -p1. Never point it to real business DB.
10. Postgres schema migration00004 down assumes no NULL notification channelIDs; current application creates integration-based deliveries so rollback needs explicit safe handling.
11. Source health wording and Dashboard progress need current phase truth. UI toolchain updates need browser reinspection.
12. External PT/CD2/Emby/TMDB/Telegram credentials still unconfigured. Do not claim real playback/site acceptance; work can continue on missing code and fixtures without them.

## Useful commands
- `docker compose --env-file .env -f deploy/compose.yaml -f deploy/compose.dev.yaml -f deploy/compose.validation.yaml ps`
- `bash scripts/load-env.sh bash -c 'set -a; source .env.test; set +a; go test -p 1 -race -count=1 ./...'`
- `E2E_BASE_URL=http://localhost:18080 npm run test:e2e --prefix web`
- `python3 scripts/backup-restore-check.py` (new private archive + temporary restore only)
- `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`

## Next progress path
Continue with subscription/RSS and existing-media index/confirmation; fix the listed safety/operability gaps; complete P7 packaging and real acceptance when credentials exist. Keep full goal active until requirement-by-requirement audit proves it. Do not redefine completion to passing this alpha's tests.

## Second continuation handoff (full goal still active)

- Current code0.5.0-dev; PostgreSQL main and isolated test DB version12. Docker latest image/management18080 and dedicated gateway18091; control/transfer/library Worker queues run.
- Subscription module now implements persistent scheduling, exact media identity/title/year matching, conservative H&R/unknown promotion checks, torrent manifest validation, missing episodes/inflight coverage, preview, RSS/Atom with encrypted feed URL and SHA256 seen IDs. Initial history baseline and retries use per-run staged feed entries; previews do not consume feeds.
- RSS partial resource updates do not overwrite search-authoritative promotion/HR data; full search updates clear unknown stale data. Rules use per-media PG advisory lock to avoid parallel subscriptions downloading same movie/episode.
- File tree parser supports purev2 and hybrid aliases; qB InspectAliases persists alternative hashes. Keep both per-provider APIIDs and canonical infohash reconciliation covered in real qB tests later.
- Streaming scan and manual confirmation produce new managed artifacts and file grant. Grants table stores digest plus AES-GCM ciphertext, not raw keys in DTO. New STRM includesassetId andkey, keeps path hint; service indexed path wins and guarded movedfiles retain IDs.
- Grant revocation/rotation UI, stable asset lookup, dedicated gateway and PlaybackInfo response protection implemented; do NOT force support flags or convert HLS into a CDN URL. Large JSON body fallback must retain entire stream.
- Redis cache fixture tested on actual Redis; context/version separated; no unknown-expiry caching; semaphore capped cache-miss sourcecalls. Need custom egress-context/version expansion and trusted-proxy deployment validation later.
- Admin sessions are now persisted digest/expiry; logout revokes, oldcookies rejected. Raw admin Bearer remains bootstrap/API access. E2E7 tests pass.
- Managed NFO/subtitle artifact table validates existing digest and does not overwrite foreign/modified files. Artifacts_pending prevents marking importcomplete before auxiliary writes. Need testretry sourcefile changes, shared-tvshow NFO correction, batch identityedit later.
- Telegram classification, target revision, started timestamp, crash unknown result and explicit duplicate-risk retry implemented/tested. Transfer controlAPI and missing-remote-task confirmation support; never blindly rerun a live upload worker.
- Main pending issues from prior handoff mostly improved, but full periodic/incremental scan, eventPushMessage subscription, rich identitycorrection/manualepisode mapping, realupload task/session discovery, fullsource parser compatibility and credentialrekey still unfinished.
- Same .env secret hygiene and isolatedmedia_tool_test rules apply. Run tests with-p1; scan testchecks current_database and TRUNCATE only exactisolated DB. Never run on business DB.

### Evidence additions
- PG subscriptionpreview createszero downloads; enable queuesone; reevaluate doesnotduplicate; RSS baseline/newitems verified.
- PG scan: failuredoesnotmarkmissing, twocomplete absent scans do, STRMnotdeleted; matchingID+SHA1+size move keepsasset address; revoked/rotatedoldkeys denied.
- PG notification crashsending becomesunknown, manual retry requires riskack; HTTP refusal/rate limit classified, URLtokenshidden.
- RealRediscache UA/context/invalidation/unknown-expiry/outagefallback tests pass.
- FullGo race sequentialPG/Redis suite passes; govulncheck reportsno known reachable vulnerabilities. OpenAPI42paths validates.
- 7 Chrome tests pass after locale-aware dialogclosing fix; managementready; gatewayunconfigured returns503 correctly.

### Next required safe work
1. Finish credential master-key rekey and safe connection cache retirement instead of max32 untilrestart.
2. Fix detailed parser/filter/timezone/orderedcase semantics; verify representativeupstream snapshots with legalowned fixtures; do notclaim allconfigworks.
3. Add permanent scan scheduling/PushMessage reconciliation, paginatedcandidate/assetUI, explicitidentityedit and manualepisode overrides.
4. Upgrade verify-m0.py to use DB-managed configs instead of checking only .env, without ever echoing/secretdumping credentials.
5. Durableartifact staging/recovery journaling: DB+FS isn't atomic; exact-match recovery is implemented but fullmanifest repair still needed.
6. Verify bothLinuximage architectures actuallyrun, pressure/multipleproviders/IPv6/reverseproxy/trustedorigin deployment and realprovider/auth/client matrices.
7. Potential edgecase: EmbyMetadata rewrite identifiesstrm302 path broadly, not yet originpredicate; tightenmanagedsource discrimination whilehidingtokens.
8. Transferreconciliation control partly checked byjoblease, but fullatomic exclusive DB fencing aroundRPCvsoperator races stillneedsattention; paused/cancelledjob retryevent semantics needexplicit classification.
9. Uploadprogress currentlyuses furthestoffset/hashbytes instead of fullydeduplicated transferredranges. UIshowphaseseparately, donotuse100% ascompletion.
10. Resolve confirmedlibrarycandidate NFO conflict retries/manifest ownership with episodeidentity providerand batch corrections; confirmedmeansSTRM/NFOwritten, NOT realEmby clientverified.

Keep original P0-P7 objective intact; external credentials remain unavailable. This continuationmade progress, not blocked, and mustnotmarkcomplete based on its tests.

# 2026-10-09 第三轮：无需账号的可靠性与管理完善

用户明确要求先完成不需要等账号的开发。本轮继续实现，不把真实配置缺失当作所有代码工作均无法推进，也未把整体项目标记完成。

## 当前基线

- 0.6.0-dev；业务media_tool与隔离media_tool_test迁移到14，43张public表。
- Go1.26.9，golang.org/x/net0.60.0/x/sync0.23.0/x/text0.42.0。发现昨日发布的标准库/HTTP2安全问题后升级并重新扫描；不能沿用此前Go1.26.8的“无漏洞”结论。
- Docker实际应用+控制/上传/库扫描三个Worker、PG18/Redis8。管理18080/网关18091仍为回环验证入口；旧宿主8080不是本轮运行验证对象。
- .env/.env.test保持0600，未替用户更换业务主密钥；真实integrations仍为0。

## 实现

1. /diagnostics/playback/preflight输出数据库/环境有效来源与开关、不返回密钥；verify-m0支持assetId、当前路径、独立网关，不强制.env保留提供方凭据。
2. Emby重写使用允许origin+资产归属；撤销/旋转后的托管Key仍隐藏；legacy开关同时约束解析入口与网关，数据库停用不回退旧Emby地址。EMBY_PUBLIC_URL明确固定公网origin，不信任伪造转发头。
3. STRM/NFO加密journal与资产/grant同事务stage，落盘后摘要确认；fsync/rename/父目录fsync，已落盘未确认可恢复；手工修改/归属冲突不覆盖。凭据轮换、媒体修正同样stage；输出恢复列表和重试入口已提供。共享tvshow按TMDB剧集身份归属。
4. 上传变更RPC与控制动作共用PG锁、control_revision和Worker租约检查，暂停/取消使用明确任务状态。人工绑定要匹配目标路径/大小并得到远端暂停确认；未知控制结果保留未知、不自动重试。
5. 上传数据进度使用已确认区间并集，哈希进度独立；恢复前重新计算源SHA1；原下载文件不移动。入库最终状态与通知原子提交，批次STRM状态与Emby刷新任务同事务，不允许重复导入把已确认状态退回。
6. 候选/资产分页、最多20项明确身份批量确认、媒体/季集修正；固定assetId/Key/既有路径不随身份修正变化。跨Movies/TV或不同剧集共享NFO冲突拒绝，不自动搬移或误改其他集。
7. 持久化子目录定时扫描，默认关闭，300~604800秒；失败/部分扫描不能投票缺失，调度并发去重。此能力不是PushMessage实时监听。
8. cmd/rekey：离线全表原子重新加密、默认dry-run、私密新Key文件、AAD/ID不变、错误整体回滚；镜像包含/app/rekey。旧gRPC连接按活动操作排空退休，不再32revision后必须重启。
9. 更新文档和OpenAPI51 paths/61 operations；补主密钥、反代、共享目录、真实验收边界；原M0历史单独标明。

## 验证证据

- 隔离media_tool_test/真实Redis的go test -p 1 -race -count=1 ./...通过；不能并行运行多个破坏式DB套件。
- 新测试覆盖journal提交前不写文件、rename后崩溃恢复、手工修改保护、共享NFO、多表rekey dry-run/失败回滚、RPC互斥/旧revision拒绝、传输范围去重、修正revision/固定链接、扫描计划默认停用/调度去重、有效DB配置/停用、不依赖DB的legacy关闭、文件Key轮换与冲突恢复。
- Go vet通过；修复后govulncheck报告No vulnerabilities found。npm生产依赖audit为0，Vue构建、2项Vitest、2项Python验收工具测试通过。
- Chrome8项实际Docker E2E通过；包括新定时扫描默认关闭/配置门槛/分页参数，375px移动页面截图已检查。未注入默认demo数据。
- 独立ARM64和AMD64镜像已构建并启动/app/media-tool healthz及/app/rekey；AMD64为当前ARM主机模拟执行，不等同物理NAS或业务链路验收。scripts/verify-images.py可重复验证，报告保存imageId。
- 自有fixture IPv6上游、固定origin拒绝伪造转发头、480次并发元数据脱敏及超16MiB元数据fail-closed测试通过。
- readyz 500请求/16并发全部成功；报告明确不等于视频/CDN代理吞吐。
- 数据库0600备份恢复到全新临时库，核验迁移14、43表后删除临时库，未覆盖业务库；APP_SECRET_KEY不包含在备份中。
- verify-m0当前输出needs_configuration、serverChainVerified=false、realPlayerVerified=false；没有真实服务账号或客户端通过结论。

## 剩余范围

继续保留真实提供方与客户端矩阵、广泛站点过滤语义、PushMessage实时增量、完整字幕/输出迁移与删除manifest、单集provider元数据、大库/长时间视频代理/真实NAS压力及多网络部署验收。PG锁不协调其他程序同时修改文件，托管目录需明确写入归属。见REMAINING_PLAN.md。

补充压力证据：TestControlledProxyConcurrentRangeBytesAndCredentialIsolation在race模式通过：96个1MiB Range请求、16并发，字节长度/SHA256、206状态及客户端凭据/上游Cookie隔离均验证。自有本机TLS fixture总计96MiB，不替代真实CDN/播放器压力。源码压力用例仅在_test.go，不进入生产监听器。最终Chrome8项再次通过，Go vet/govulncheck再次通过。

# 2026-10-09：单管理员注册、账号密码登录

用户要求登录改为账号密码，管理员通过注册产生。本轮交付0.7.0-dev/迁移15（45张表）。不再使用ADMIN_API_KEY做管理登录或Bearer授权，旧会话通过迁移撤销；业务凭据/媒体/主密钥未改，.env/.env.test仍0600且未覆盖。

- 新增administrator_accounts单例表、账户绑定/凭据修订的会话、PG持久化认证限流；默认无管理员、无默认密码。用户名3–32位ASCII标识符/忽略大小写，密码15–128个字符/保留空格。
- Argon2id m19MiB/t2/p1、每密码独立随机盐、严格PHC边界与常量时间校验；未知账号同样执行密码计算/统一错误。密码计算并发2，IP20/min、全局120/min持久化尝试上限，过期计数定期清理。
- GET /auth/setup、POST /auth/register、账号密码POST /auth/login、账号GET /auth/session、POST /auth/password。注册与首个会话同事务，PG单例/锁防并发多管理员；注册后关闭公开注册。
- 管理API只接受有效签名且数据库已登记/未撤销/未过期/凭据修订一致的Cookie。服务端SESSION_SECRET_KEY独立签会话，未设置时从已有APP_SECRET_KEY用途派生；签名域升级v2。不能用任何服务器Key或STRM Key登录。
- 前端首次注册/后续登录/密码确认/密码管理器/粘贴/显示隐藏/移动布局；系统设置修改密码，成功撤销全部旧会话并回登录。
- 提供本机离线admin-reset工具：明确停服务、私密密码文件、更新现有账户/撤销会话/审计，不开放公开重置或重新注册。维护镜像包含该工具。
- verify-m0改为用户名+交互密码/私密文件，临时Cookie、结束退出，不再读ADMIN_API_KEY认证。
- e2e迁入media_tool_test/18084临时服务及Worker，随机测试账号只存在隔离库，清理后退出；强制防在业务库预注册。保留测试用SessionReader仅用于已验证业务接口的单元测试，生产入口没有相关开关或绕过。
- 源码、部署、初始化脚本、文档、OpenAPI更新（54 paths）；构建清理旧前端assets避免打包旧Key页面。

验证：全量Go -p1 -race（真实隔离PG/Redis）、Go vet、Vue构建/2项单元、2项Python、11项Chrome真实浏览器通过；含并发仅一位注册、错误密码、Cookie安全标志、数据库会话要求、退出重放、修改密码撤销、重启限流、移动页面。ARM64原生/AMD64模拟镜像及维护工具启动通过。govulncheck 0可达/0已导入包漏洞；模块级GO-2026-5932提示为x/crypto中未导入的弃用openpgp，不隐藏该提示。迁移15/45表备份临时恢复通过。

实际运行：Docker18080/18091及验证属于本项目的旧./bin/media-tool宿主预览8080/8091均已更新；两管理端旧ADMIN_API_KEY Bearer请求实际401、setup均registrationOpen=true。业务库administrator_count=0，尚未替用户注册账号，由用户自行在本地页面设置。无数据卷重置、无业务密码预设、无主密钥轮换。真实115/PT/Emby验收仍独立。

## 2026-10-09：密码长度6–20

按用户明确要求，新注册/改密/离线重置的密码长度由15–128改为6–20（包含6和20）。后台按Unicode字符计数，前端共用password-policy.ts，注册/新密码输入与提示/OpenAPI/当前文档同步。旧长密码登录与当前密码验证保留兼容，不修改既有密码哈希、不重置账号、不调整Argon2id参数。

验证：后端5/6/20/21及中文边界、新注册/改密接口边界、历史长密码兼容；全量Go -p1 -race/真实隔离PG/Redis、Go vet、前端4项单元/构建、Python2项、Chrome11项隔离E2E通过。ARM64原生与AMD64模拟镜像重建/启动通过，当前Docker18080 setup返回passwordMinLength=6/passwordMaxLength=20。无需新迁移，仍为迁移15。源码bin/media-tool/admin-reset已更新，未新增原本未运行的宿主预览进程。已有.env、业务主密钥与账户数据未修改。

## 2026-10-09：用户授权的远端CloudDrive2/Emby部署

已在指定/root/docker/mediatool初始化全新mediatool-test项目，AMD64 Debian12现有Docker上部署应用/PG/Redis/三个Worker/官方CloudDrive2/官方Emby及Emby私有回环适配器。只为新项目生成0600环境密钥、挂载和资源限制；未重启原服务/Docker、未改全局FUSE/挂载、未动已有卷或账户。

应用ready、迁移15、注册开放6–20；CD2网页200及官方gRPC GetSystemInfo成功、Ready=true/IsLogin=false；Emby实际4.10.1.0、网关路由通过。Emby看到的STRM Docker mount RW=false，5秒自有H264/AAC合成视频ffprobe通过。所有新管理端口仅127.0.0.1（25014/25015/19798/8096），用户通过SSH隧道初始化。尚未配置115授权或真实播放，不冒充媒体验收。

来源/版本pin与远端只读报告见REMOTE_TEST_DEPLOYMENT.md及data/remote-deploy/deployment.json。新增cmd/provider-smoke为受限只读gRPC诊断/容器回环测试适配，不创建账号、不处理/记录原始凭据或临时URL。用户自建管理员/Emby/CloudDrive2授权后才能继续真实链路。私钥未持久化至仓库/服务器/镜像，结束清理本地临时文件；应由用户轮换聊天披露的Key。

## 2026-10-09：用户明确要求公网直接访问

仅对mediatool-test项目将25014/25015/19798/8096绑定改为0.0.0.0，APP_PUBLIC_URL及EMBY_PUBLIC_URL同步185.99.135.205地址。公网四项HTTP检查200、公网Origin无效账号401、Emby命名空间新固定地址healthz通过。PostgreSQL/Redis未发布，其他23个容器仍运行，无数据卷删除或密钥变更。远端配置先0600备份；旧STRM数量0，无需文件改写。

当前直接HTTP、未TLS、管理员仍未注册，已提示用户公开初始化/密码传输风险；用户请求不等于真实115播放通过。SSH主机指纹对照上一轮核验，临时私钥使用结束清理，未改authorized_keys。新版远端README/只读诊断与本地REMOTE_TEST_DEPLOYMENT.md记录当前公网模式。

## 2026-10-09：用户完成真实提供方配置后的验证

停止上次未完成的Emby密码恢复：只生成PIN文件、未兑换PIN或改密码；用户已配置，后续不重置账号。现有加密CloudDrive2和Emby连接均启用/此前probe ready。实测CD2登录/允许目录授权有效，Emby受保护Users/VirtualFolders接口200。

发现并补齐Emby缺/strm映射及gatewayEnabled；新建专用Movies/TV库，不改账号/原凭据/源媒体。根目录扫描成功，实际云文件1、needs_confirmation候选1，TMDB已配置但不自动确认身份；资产/STRM仍0。

真实CD2 1.1.2能力诊断表明115支持直链但配置supportDirectLink=false，返回中转路径而非directUrl。需要用户开启相应直链配置/权限，不代购买、不改私有115、不降低安全校验。尚无真实115字节/Emby播放器通过结论。

修复此前LibraryView/API pending筛选与数据库needs_confirmation不一致导致真实候选不可见的bug，保留pending别名并同步分页计数。加实际PG扫描筛选回归。新增不泄露URL/代理/账号/Token的CloudDrive能力诊断回归；部署诊断只读/最多1024字节；配置完成工具显式--apply。不注入示例媒体身份。

补充：实际CloudDrive能力检查支持直链但开关关闭；仅保留中转路径，不修改原115配置或收费权限。电影/剧集专用库和网关路径已配置，实际扫描needs_confirmation候选1。真实调试发现并修复候选筛选名错配（旧pending别名兼容），相关PG/race回归及11项隔离Chrome测试通过，更新实际远端应用镜像和源码映射。用户仅需开启CloudDrive直链并在候选界面人工确认身份；未经确认不生成STRM、不声称播放器通过。

## 2026-10-09：用户完成直链与人工身份确认后的真实媒体检查

真实资产1/STRM1/confirmed/刷新成功、无pending artifact。发现用户CloudDrive2同时开启HTTP下载，程序正确拒绝HTTP源。用现有权限仅关闭useHttpDownload，保持直链/线程/IPv4等参数和账号套餐不变；后续真实HTTPS源Range206/1024字节通过。同步连接revision使旧HTTP缓存失效，不降低播放安全策略。

真实固定文件Key/assetId/origin验证、兼容提供方UA HEAD302及安全目标检查通过；普通UA因CD2绑定UA走受控代理GET/HEAD206/Range1024字节，HEAD0body。错误文件Key401，Emby查询实际找到托管STRM item9，错误Emby用户401。没有用管理员APIKey冒充普通用户、没有额外测试账号或密码重置。普通Emby成功授权/真实客户端解码拖动仍待用户测试，不能据服务端字节报告全部客户端通过。

维护工具HTTPS单项修改/权限尊重/脱敏回归、Go race/vet通过；报告路径及范围见REAL_PLAYBACK_VALIDATION.md。私钥用完清理，未改server authorized_keys。

## 2026-10-09：修复Emby静态302及HLS元数据兼容性

用户反馈播放失败/未302。真实有效会话定位Emby4.10.1.0 Me500、item缺失PlayAccess、mpegts请求与ts扩展不匹配，原重写逻辑覆盖HLS DirectStreamUrl。新增上游成功登录观察及server-scoped SHA256摘要身份绑定（共享Redis、30天TTL、退出/401清除）；每次用调用者凭据验证用户policy/item，缺失PlayAccess必须同用户PlaybackInfo无错误且source一致。不信任UserId/DeviceId，不注入后台APIKey，拒绝冲突Token。TS别名补齐；原HLS/remux/transcoding参数与能力保留，地址归网关；原因响应头不含秘密。

新增纯维护gateway-session-smoke及远端只读会话检查脚本。认证DB localId/GUID核验后导入3个现有会话摘要，不改账号/密码、不输出或落盘Token、不复制认证库。已部署自己的server/workers，PG schema仍15，旧镜像/源码留存可回退，未动其他项目。真实用户静态网关兼容UA302→HTTPS Range206/1024字节；普通UA受控代理206/1024字节；错误Token401。合成浏览器HLS配置真实PlaybackInfo源/网关200、能力/路径/参数保留，playlist200/媒体分片200读取1024通过，仅停止自己的诊断编码。

全量隔离Go-p1/race、vet、Redis绑定回归及11项Chrome E2E通过。真实客户端解码/拖动/音轨/字幕仍未验收，realPlayerDecoded=false。证据及边界见REAL_PLAYBACK_VALIDATION.md；用户应从25015而非8096刷新/登录，不强制HLS/提供方UA受限请求302。临时SSH身份用后清理，不改authorized_keys。

## 2026-10-09：参考embyExternalUrl修复Sen原始流及网页URL语义

读用户指定README及相关primary源码，固定commit91798cc2ce5e14646c6b939b9f5d1d8d137541e9。参考AList取链接时携带caller UA、115缓存按UA隔离、stream/original分流、相对媒体URL与转码回退；不复制无条件SupportsDirectPlay或私有115 Cookie预览接口，不输出全query/完整PlaybackInfo。

实际SenPlayer/6.2.1请求original.mkv，补有权限校验的GET/HEAD raw original路由。Emby网页ApiClient把绝对HLS URL拼为/http:/host/...404，改server-relative并加真实契约回归。Sen播放信息只提示公开UA，不泄露Cookie/Authorization；固定deliveryReason日志/管理诊断无签名URL。CDN在服务器与Mac网络均正确UA206、Sen默认UA403；兼容UA original302/CDN206、无效Token401。现有UA/IP/version缓存保持。尝试CD2customUserAgent单字段对齐未通过并恢复，不改账号/套餐/其他配置。

真实用户Chrome初始出画面、时间前进、快进10秒、暂停和一次退出再恢复到约8分31秒通过。但后续该位置冷恢复出现Emby首分片404/network，部分FFmpeg0帧/不生成分片，故未完成长seek/冷恢复/音轨/字幕验收。原始流1MiB及256MiB Range读取通过，不能泛称所有浏览器操作完成。已结束本次测试播放，保留用户页面/账号。详情与后续项见REFERENCE_EMBY_EXTERNAL_URL.md及REAL_PLAYBACK_VALIDATION.md。

## 2026-10-09：新增115官方独立取链与PKCE授权

用户明确授权考虑115开放平台取链替代CD2固定UA。通过用户已登录的官方开发文档核验API及公开原始Markdown，实际域名为proapi.115.com（非臆测openapi域名）。新增open115适配器，实际播放UA发送downurl，按官方绝对路径查pick_code，核对名称/大小；保留旧STRM及Emby授权，CD2扫描上传不变。选中官方连接后direct-only，最多1KiB同UA CDN206验证后302，验证失败502/强制代理冲突409，无静默CD2服务器中转。

增加自己的AppID的PKCE扫码、加密账号/连接版本绑定会话、本地二维码生成、取消/过期/完成清凭据；Access/Refresh原子加密更新，PG行锁避免多进程重复刷新，60秒冷却，终态40140116/19/20/37持久停止重试；未知刷新结果不重复旧token。迁移16仅添加能力，不启用官方连接，不改现有账号/密码/数据。Vue新增服务类型及显式扫码授权，前端关窗移除二维码并停止轮询。参考文档与待用户凭据步骤见OFFICIAL_115_PLAYBACK.md。

本轮没有真实115官方授权，不能把fixture、兼容CD2 UA或服务端有限字节测试当成Sen实际官方CDN直播放。已询问用户自己的AppID，明确不在聊天发送Secret/Token，不借用CD2/他人的应用。

补充验证：全量Go-p1/race及vet、前端typecheck/单测/构建通过；12项隔离浏览器E2E通过，新增扫码fixture只证明界面取消/清理，不进行真实115授权。升级本项目前0600备份数据库，迁移16部署成功，未启用open115连接，原账户/密钥/其他23个容器不变。实际用户页面已看到官方服务类型，未保存演示记录或凭据。原Sen最新日志确认为proxy/provider_user_agent_required，官方真实播放仍等待自己的AppID/授权。

## 2026-10-10：真实115授权后的官方取链验证

读取用户保存的open115已启用/ready配置；不回显AppID/115/Emby凭据，未改账号或配置。升级维护smoke以按各caller UA分别获取官方签名链接，不沿用CD2兼容UA/代理假设。现有用户Token在远端只读认证DB验证关联且只经stdin使用；item11原始/静态流默认SenUA302至cdnfhnfile.115cdn.net、CDN206/1024，Direct-Verified=true；普通UA分别签链亦302；1MiB/256MiB偏移302→206/1024，无效Token401。Metadata不需要原CD2-UA覆盖。

Mac网络安全公网DNS固定/正常TLS与Sen默认UA直接CDN206样本通过；签名URL仅内存，未拷贝用户Token或媒体。只证明授权网关/媒体字节，不宣称真实Sen解码/音轨/字幕/长seek全通过。未修改PG模式/Emby/CD2/业务密钥/其他项目。结束清临时SSH身份，用户应停止旧播放重新进入，无需再改UA。

## 2026-10-10：首轮全范围审计与授权/rekey可靠性

接续首轮可用目标，不把官方302成功当作整个项目完成。新增FIRST_ROUND_ACCEPTANCE.md，对原始需求和计划第12节逐项记录实现/隔离测试/真实有限证据/未通过项；同步README、ARCHITECTURE、当前状态、剩余计划、快速使用与官方播放说明，保留历史记录。

修复离线rekey漏掉open115_authorizations：8张加密表、open115-auth命名空间AAD、终态/兑换claim空payload保持；隔离PG dry-run/apply/错误密文全表回滚通过，没有轮换业务主密钥。

扫码状态长轮询不再持有播放Token的数据库锁；新增迁移17，兑换前提交exchanging并清verifier，HTTP请求不持数据库锁，完成重查取消/新会话/连接修订/管理员凭据版本。多Manager最多一个兑换claim，客户端断开后的未知结果持久failed，进程崩溃claim不重放并按5分钟到期。Vue显示保存中、清二维码、继续轮询/可取消，OpenAPI同步状态。

本次最初的本机迁移命令仅source .env.test，未替换DATABASE_URL，误将本机开发库从15升级到17（增量结构迁移，未修改账号/凭据/媒体）；TEST_DATABASE_URL仍为16，导致首次授权测试因缺exchanging约束失败。未在开发库执行TRUNCATE或rekey，未盲做破坏性down。已修正：scripts/test-go.sh显式替换DATABASE_URL、Redis DB15、迁移-test-only查询实际库名、-p1；E2E也加-test-only，官方fixture先检查实际数据库。实际非测试库迁移拒绝已验证。Redis路径赋值的初始脚本语法错误亦经5项Python守卫回归修复，不以首次失败作为通过记录。

修复后全量隔离Go-p1/race、vet、5项Python脚本测试、4项Vitest、Vue typecheck/build、12项隔离Chrome E2E通过。二维码E2E明确fixture，包含exchanging持续轮询/二维码清理/取消，已截图检查，不伪造用户再次扫码。linux/amd64镜像构建通过；已读远端仍迁移16、三Worker、4个ready连接（TMDB/CD2/Emby/open115）、2个资产、1个authorized会话、0订阅，其他23容器未操作；无PT/qBittorrent/Telegram配置，不借用其他项目凭据。

本轮发布完成：本项目迁移17、server/三Worker就绪，用户1个authorized官方会话与2个资产保留，23个其他容器ID摘要相同。源码安全解包因远端旧Python不支持TarFile.filter而中断，应用已经部署；经归档SHA256/成员校验后只恢复空stage源码复制，没有重复迁移/重建/覆盖备份。实际真实用户默认Sen UA的original/Static302→cdnfhnfile.115cdn.net、CDN206/1024及1MiB/256MiB偏移复测通过；普通UA302，错误Token401，未导入新身份、未假称解码通过。备份后迁移17副本在独立临时库恢复事实匹配并清理临时库，没有覆盖业务库或执行真实rekey；报告注明尚未验证恢复密文解密。

## 2026-10-10：自有文件真实上传与安全HTTP状态指标

上一goal turn为progress：迁移17部署/真实302复测/备份恢复及授权可靠性已改变权威状态。本轮继续原完整首轮目标，不将成功取链缩减为整体完成。

新增cmd/cloud-upload-smoke（明确写入确认、仅Owned-Test-5s.mp4/2MiB内、配置允许根下固定UUID诊断目录、0600原子/fsync会话journal、未知启动不重建、恢复需显式确认），镜像打包维护入口。真实CD2远程上传自有49,982字节视频、持久会话、云大小/SHA-1一致；再经生产安全播放引擎受控Range206读取整个小文件，SHA-256一致。原文件仅复制，未创建虚假PT/下载/电影身份/STRM数据；秒传/去重可能存在，未把完整文件大小当作实际ReadData上传字节。详见OWNED_UPLOAD_VALIDATION.md。

实际Chrome复测官方模式仍HLS network/空buffer/未出画面。相同一组无凭据HLS参数在普通播放用户校验后，master/main/首个TS200、有效TS同步字节；最近FFmpeg已有帧，没有证据把全部新失败继续归为0帧或内部域名泄漏。Mac此时锁屏、原生App无法操作，Chrome扩展DOM可访问；真实客户端验收需要正常设备状态复测，不能从本次黑屏反推全部服务端故障。

现有安全访问日志无响应status/bytes，无法区分HLS首请求失败和后来缓存成功。增加httpsnoop.CaptureMetrics保留Flusher/Hijacker/ReaderFrom的可用接口，记录status/responseBytes/duration而不记录RawQuery/凭据/body；覆盖206字节、流式Flush能力、panic500及敏感字符串不日志的回归。最初错误固定旧httpsnoop导致Go工具临时降低grpc/goose依赖，未部署；已从当前远端基线恢复go.mod/go.sum并核验所有原依赖版本不变，只新增兼容版本httpsnoop1.1.0和将已有qrcode标为直接依赖。修复后全量隔离Go race/vet、12项Chrome E2E和5项Python守卫通过，amd64镜像构建通过。

补充真实启动时序：新日志捕获first 0.ts约10027ms后502（26字节代理错误）、随后404；8.ts另一次约678ms后502，之后多个分片200。不能凭稍后缓存成功掩盖初始失败，也不支持直接把10秒失败归为30秒ResponseHeaderTimeout。Chrome后来readyState4、640x360、currentTime从约7秒到24秒、缓冲有数据，但Mac锁屏时截图仍黑，未证明正常设备画面/全部操作；保留有限DOM证据，测试已结束，不标为完整客户端通过。

代码审查发现ReverseProxy的http.ErrAbortHandler会被原panic兜底转为JSON错误，可能追加到已写出的TS/SSE数据。现让标准中止信号重新交由net/http结束连接，仅记录固定无敏感上下文事件；普通panic仍安全500。新增部分206媒体字节保持、标准中止不吞、无查询/媒体体日志回归，全量Go race/vet和12项E2E再次通过。这是字节完整性修复，不冒称消除了前述启动502/404。

本轮最终发布/回归已核验：工具、HTTP指标与标准流中止修复在指定项目运行，PG仍17/server及三Worker就绪。正式镜像工具对同一持久session/远端自有文件重验206/49,982字节/SHA-256一致，不重复Start；原始文件/2个资产/1个authorized官方会话保留、其他23容器ID摘要相同。默认Sen UA原始流302→CDN206、非零Range和错误Token401保持通过，未宣称Sen真实解码。备份/回退镜像与旧源码保留。后续首要是Emby分片启动中断/冷恢复；PT/qBittorrent/外部通知未配置的真实链路依然未验收，目标保持active。

## 2026-10-10：HLS 首字节预算与取消语义

上一goal turn为progress（真实自有上传、流中断修复和HTTP指标部署）。本轮真实独立诊断发现首TS在10秒客户端预算内超时，但同一URI/同一编码在约0.7秒后能读200/有效TS。读取当前实际Emby/HLS公开脚本核验fragLoadPolicy首字节10秒、总片段120秒及basehtmlplayer首次NETWORK_ERROR拒绝play promise的规则。

实现输入取消不生成502、固定失败分类且不输出原异常URL/详情；已核验basehtmlplayer SHA绑定的首字节60秒改写，克隆运行时策略不变能力/重试/总预算；未知版本原样透传、不改原Emby文件。仅HLS GET/HEAD片段75秒头预算、其余30秒、共享连接池。Go与Python/Node回归覆盖这些边界，首次Go fixture ETag大小写错误修正后全量通过，不将首次失败作通过证据。

发现初版--offset510诊断仍选0.ts，FFmpeg无seek证据，因此该报告不是8分30秒通过。已按EXTINF时间轴选目标片段并单独生成selected证据，原错误报告留存，未把它用于完成门槛。详情见HLS_STARTUP_RELIABILITY.md。

发布后发现原Emby使用application/x-javascript，第一版严格MIME检查使已知脚本未改写（公开GET仍原hash、无policy头）。没有把意图当上线通过；补兼容该已观察MIME与回归，再构建发布修正。普通诊断45秒预算的首TS约10.573秒后200/有效字节通过。

正确按EXTINF选择510秒的85.ts后上游500，FFmpeg确认-ss 00:08:30.000；不再误把0.ts成功称作冷恢复。近期FFmpeg日志出现无帧及输出header/No such file，结束诊断后的输出目录已被Emby清理，不能反推目录缺失就是首个失败原因。此冷seek仍未通过，需要与编码生命周期/请求中断/清理时序区分。


最终公开脚本GET确认MIME修正后60秒policy/no-store/去validator与22,264字节，尚不是旧页面已加载策略证明。独立10秒断开诊断现日志HTTP stream aborted/request_cancelled，sameURI/编码续读约0.938秒200。正确510秒片段85.ts仍500，真实FFmpeg-ss已证；不误用初版0.ts通过。旧Chromereload/尝试硬刷新本轮没有新的basehtmlplayer请求，页面仍10秒networkError，不能把这归于已加载新策略后结果。结束测试保留详情页，目标仍active，下一步编码seek生命周期与正常设备新策略加载、真实PT业务链路。


## 2026-10-10：继续官方302复核、HLS对照和诊断角色修正

接续前轮先核验/清理旧临时SSH目录及远端临时脚本；随后以用户原授权、已核验主机指纹建立新的临时连接，仅操作本项目。未重放部署、迁移、上传或115授权，不改账号密码、主密钥和原媒体。

真实角色核验发现HLS脚本只看EnableMediaPlayback却输出ordinaryUserScopedSourceVerified=true；实际会话IsAdministrator=true。修复工具默认非管理员且拒绝禁用/缺字段/非布尔/错用户，管理员仅显式选择并如实标记。实际默认守卫未选到非管理员会话；没有代建账号。旧报告保留并明确排除其普通用户/同编码断言，官方302报告ordinary前缀属于UA不是用户角色。

增加严格loopback上游对照、会话查找30秒总预算、HTTPError关闭、仅客户端中断重试（500不立即再启编码）和sameEncodingOnRetryProven=false。新测试含SQLite真实fixture身份选择、拒绝管理员降级、完整run的role/500单次/85.ts/204/0600/无Token报告；全部15项Python/Node通过。初次shell结果变量status触发zsh只读变量错误，测试日志本身通过；改result后复验。严格ResourceWarning发现测试SQLite context仅提交不关闭，改closing后复验无警告。

明确管理员会话的官方播放复核：Sen UA original/Static均302、Direct-Verified、目标cdnfhnfile.115cdn.net、各206/1KiB；错误Token401且无Location。仍realPlayerDecoded=false，不等同实际Sen或非管理员矩阵。

输入隔离检查第一次因只允许/usr/可执行路径而未执行ffmpeg；允许已观察/bin/ffmpeg后指定-ss510/noaccurate_seek/视频copy/null约15.402秒退出0取得包，不是解码或精准seek。HLS直接Emby对照一次16.089秒200、另一次15.108秒500，网关单次15.130秒500；不能归于网关独有或断言固定15秒上限。输出生命周期观察只候选输入/新日志且存在日志延迟，不把清理后目录反推根因。各诊断仅停止自己的device/session并204；未停止用户Chrome/Sen。

本轮无Go运行程序改动或新镜像迁移，不冒称重新跑全量Go/E2E。继续定位Emby seek上游失败、实际网页加载/正常设备播放以及原计划PT链路；整体目标保持active。


最终工具直接上游模式改用根路径，和网关剥离/emby挂载后的实际wire路径对应；前两次直接对照是上游/emby别名，不混作完全同路径。根路径85.ts再得约15.248秒200/1KiB/TS，依旧只是一次管理员会话字节样本，不消除已出现的直接上游500或网页门槛。


收尾核验：本地15项Python/Node与远端9项HLS fixture通过；PG仍17、三个Worker、两个资产、一个authorized官方会话。当前运行镜像与本轮前一致，23个其他容器ID摘要一致，管理/网关可达。只同步诊断源码/测试/文档，未重放迁移或部署。最终状态报告已成功读回；首次本地比较使用不存在的reliability-current文件而中断，随后从已有远端收尾报告读回，并用hls-cancel-after基线比较，没有重复测试/状态写入。


## 2026-10-10：PT配置语义与可复现快照恢复

上一goal turn为progress：官方302复核、管理员诊断纠偏和HLS对照。当前临时SSH marker已不存在，没有把上一轮等待状态当成仍运行。本轮先核对实际代码与固定上游快照，再推进不用账号的真实开发；不把整体范围缩成302单点。

上游固定37bf5c5064ae437f54e26b57406ca84d480daab5（2026-09-21），194个配置文件，193站点；初次请求整仓archive超过20MiB上限即停止，转为GitHub树与3并发固定文件读取，没有取消上限或取图标。参考仅在被Docker排除的data/reference。发现17类过滤器、63处字段case、JSON body/flat_map_fields等实际规则。时间偏移含+19，无消费者语义证据时不能按常见时区猜测或继续no-op伪支持。

实现单次占位符替换、参数与JSON/表单隔离、类型保持、HTML声明编码、多日期格式、数组逐项过滤/计数、跨字段依赖与循环拒绝、父组→子种子展开与500条总上限。资源解析对非有限/无效促销和H&R保持未知，不误判免费/无H&R；标签去空/去重。只求值业务必需字段/依赖，未用时间显示字段不阻断搜索，但兼容仍明确partial。

条件按源声明顺序执行，并在保存前将字段case规范化为内部有序数组，普通body/lookup-case对象不变；真实隔离PG往返通过。旧jsonb多条件对象缺优先级证据时拒绝猜测，列表明确partial；API与Vue可指定原完整提交恢复。批次先下载/验证/核对ID，再单事务写入；提交结果未知时503提醒读回，UI不自动重试。上游授权仍须用户主动确认。

新增静态审计CLI和自建fixture/隔离PG测试，193配置加载、159parser_supported/34partial/0invalid，accountVerified=false。初步全量Go回归通过；Vue恢复fixture第一次30秒超时，页面表明许可证未勾选，定位Element Plus隐藏的原生checkbox不可直接check，改为点击可见标签并断言checked，不force绕过。此失败保留日志，不作通过证据。最后全量回归/截图/镜像结果待收尾核验，本轮尚未远端发布，不触碰实际PT账号或伪造下载链路。


## 2026-10-10：PT兼容最终回归、原生镜像审计与发布

接续goal turn为progress，构建session37703在本轮确认退出0，没有重启未知构建。最终全量隔离Go/race/vet、Vue7单测/类型检查/build、15脚本测试与13 Chrome E2E通过。曾出现已有扫描计划E2E在登录时Failed to fetch；不跳过或改产品限流，保留failed日志，重新完整13项通过。首次移动截图抓到侧栏过渡帧，改等实际settled状态后核验截图；为移动站点恢复列表增加完整卡片/按钮，避免必须横向滚到隐藏列。

本地amd64镜像离线审计最初仅构建成功，实际运行失败。排除输入读权限后曾有map/runtime异常；单协程纯Go最小map样本amd64/arm64均通过，不能据此给根因结论。原生arm64最终镜像实际审计通过；amd64在aarch64 Docker模拟及cpu.all=off诊断仍失败。随后用已核验SSH指纹、仅指定项目私有临时目录，把同一静态参考和工具在真实x86_64执行，得到193/159/34/0；最终原生amd64 Docker镜像也以UID65532/只读/无网络/默认CPU设置得同结果。没有更改Go版本、加全局CPU选项或假称模拟运行通过。

重新核验远端：本项目PT账号/站点配置0，无活跃授权兑换，schema17/三个Worker/2资产/1authorized官方会话，其他23容器。最终amd64镜像和288文件源码归档逐成员SHA256核验；归档不含环境秘密/用户数据/上游配置图标。Docker manifest/config双ID按实际核验，不混作运行tag证据。

真实镜像审计后才备份数据库（gzip校验）/tag旧镜像/保留旧源码；仅server/三个Workerup--no-deps，没有迁移重放、账号/主密钥修改或PT自动导入。deploy receipt为deployed，回退保留media-tool:before-sites-compat-20261010、private/database-before-sites-compat-20261010.sql.gz、private/source-before-sites-compat-20261010。Emby/CD2未重建。公开Vue JS含来源提交/恢复，远端9项角色HLS fixture通过。最终PG17/3Worker/2资产/1authorized和23个其他容器摘要一致。

发布后官方原生检查第一次未确认，不凭失败推定链路已通过；脱敏包装后单独完整复核：管理员登录会话、Sen UA original/Static均302→cdnfhnfile.115cdn.net、各206/1KiB、无效Token401无Location，realPlayerDecoded=false。没有重建STRM/再次授权或借后台Key。

本轮做到已发布的PT兼容与可复现恢复，不将此当整体项目完成；下一步保留网页seek/新HLS策略实际加载/正常设备矩阵，以及用户合法PT搜索→取种→qBittorrent→做种保留→CD2上传→STRM/NFO→Emby→通知与真实订阅。目标保持active，不新增其他线程或借用其他项目账号。


## 2026-10-10：网页启动缓存修复、实际页面加载和有限画面

前轮为progress（PT兼容发布）。重新读当前部署/代码/已清临时身份后，回到网页可用性。核验公开index14939/apploader3080精确SHA与loader urlCacheParam，发现原版本缓存不能让已部署basehtmlplayer策略必然加载。本轮增加独立mt_hls_startup=1，只改精确版本的脚本URL/缓存键，不改Emby版本/能力/许可/授权/原文件；未知版本、MIME/压缩/非200不碰，变更去旧validator/integrity/no-store/正确长度。Go覆盖唯一锚点/未知字节/字段/条件请求边界，17Python/Node覆盖原版本/原默认与有限budget，全量Go/race/vet、13E2E通过。

只读编码配置未找到已证的探测/启动超时可调项，不凭猜测写XML；官方AnalyzeDurationMs文档标Obsolete，不将其当请求参数修复。观察独立新device/session首次510秒无重试：15.124秒上游500，候选输入/输出/片段尚无记录，16.792秒目录消失，观察后仅停止自己编码204；早期诊断Path拼接错误与SSH中断未被算通过。可追踪runner既有PID终态receipt已读回，无blind重建；上游错误形状FfRunException/Error starting ffmpeg位于StartFfMpeg/GetDynamicSegment，根因仍开放。

SSH/SCP传输多次中断导致镜像仅16MiB，未启动发布。逐次检查remote长度/SHA与本地相同前缀；先续接未知结果读回，再4MiB块接收完整后才写入，最终整档SHA通过。最终真实amd64镜像以非root/只读/无网络/默认CPU审计成功后备份PG/tag回退/保留源码，再只up本程序和3Worker，无迁移。回退media-tool:before-hls-bootstrap-20261010，private/database-before-hls-bootstrap-20261010.sql.gz与private/source-before-hls-bootstrap-20261010。

公开实际改写index14956/apploader3100/base22264正文hash/header核验，原Emby4.10.1.0版本仍真值。原用户Chrome tab已消失，不重复猜旧句柄；只打开本项目新temporary tab、复用它自己的已授权会话，不读hidden storage或借另一应用登录。新DOM脚本URL均含revision1，服务器200/字节匹配。第一次网络/Abort错误保持、DOM缓冲不等于成功；真实时间轴6.35不满足0.012步长被拒，检查后用6.348，约510秒缓冲/到511.246，原生截图一次真实影片帧。随后真实恢复仍ready0/暂停/黑帧，不声称持续或反复冷恢复通过；Escape结束自己的测试。

当前Hls1.6.0-beta.2源码与原参考完全相同。离线Node VM初次缺AMD define、第二次exports参数缺失失败，补真实AMD exports工厂后newHls实际config首字节60000/总120000、原默认10000不变。这是额外库配置证据，不替代真实解码；无loadSource或网络/账号访问。

最终PG17/3Worker/2资产/1authorized官方会话，其他23容器摘要一致；Sen UA原始/Static302→cdnfhnfile.115cdn.net/2061KiB、坏Token401无Location，明确管理员会话/非Sen解码。下一步持续网页/冷恢复与首次非fatal网络处理有限预算，以及原始计划真实PT/订阅/通知/客户端矩阵。整体目标仍active，不凭新缓存或一帧画面改为complete。


## 2026-10-10：V2有限启动保护与真实首播进展

上一goal turn为progress（缓存策略发布/实际模块加载/一次510秒出画面）。本轮读当前代码和原始完整bind回调，确认fatal||reject与>=400处理会提前销毁即使SDK非fatal重试。实现自有embedJSguard；只有明确SDK1.6beta2动作5/次数有效、startup未完成、非fatal网络与瞬时状态可继续，原权限/fatal/media/取消/销毁/已播放语义保留。成功或终止清90秒总timer和listener，destroy微任务避免覆盖原servererror。无需手动startLoad/loadSource或更新能力/许可，不换session。

35自建状态和当前原回调函数离线执行；三个锚点完整核验后原子rewrite。实际SDK自建XHR503→200只是原库内部retry、公共ERROR0，不冒充guard数据；实际ErrorController在networkControllers而非publicerrorController，核对源码后取到fragmentaction5/count0/max6。离线window.status包装初次缺失导致ReferenceError，补浏览器标准属性后通过，未修改库。全量Go/race/vet、18脚本、13E2E、amd64镜像通过。

镜像4MiB接收完整才append、每次PrefixSHA读回，SSH中断的unknown结果不盲写；同Offsetfalse后真实读回再接最终整档checksum一致。真实amd64nonroot/只读/无网络/默认CPU审计193/159/34/0后备份/回退/source逐成员核验再发布，仅本程序与3Worker、PG17。所有原账号/主密钥/授权/媒体与23其他容器保持。Sen原始/Static302→115CDN2061KiB/坏Token401保持，仍管理员会话/非实际Sen。

真实Chrome新tab测试启动URL曾三次模块LoadFailed（后台approuter/gesture对应200字节），不认定具体网络/参数根因，也不重复无限重载。按正常原入口进入主页和详情，实DOMscriptRevision2/version4.10.1.0。首0.ts50015.128s、5009.016s时guard两次等待；随后有影片帧与time35.298→71.541→158.785，证明一次连续首播两分多钟。点击video暂停，隐藏/失效button失败后按当前video区域恢复，无强制DOMplay状态。

拖动实际滑条6.348到510.046失败；Exit/Escape后恢复新启动有四次等待，170.ts连续约15s500，90秒终止/缓冲空/ready0，不无限重试。错误提示消失后Escape正常返回，不杀用户Sen/其他编码。保留成功frame与失败cold图和chronology，不把初始连续通过改成seek全部通过。整体目标仍active，后续已播放seek/上游探测启动/实际PT合法资源链路、订阅/通知/客户端完整矩阵仍需完成。

## 2026-10-10：按用户新目标检查item17并改为严格直链

本轮目标已由继续网页HLS可靠性改为“检查媒体信息/STRM入口差异，不需要转码”。不继续以网页转码成功作为本目标验收。上轮实测V2兼容网页恢复510秒出画面/14分钟拖动后持续两分钟；独立单次HLS仍15秒500，不能称稳定。直接验证成功缓存已发布，但尚未发布的官方缓存修订接线修复随本轮一起测试/发布，正确按官方提供方选择/凭据修订隔离，避免仅CD2版本的旧URL缓存。

item17上游源实际是.ts；旧诊断错误使用original.mkv会触发原容器不匹配上游回退500。磁盘STRM、上游Path和选定asset一致，网关Path是刻意隐藏文件Key的用户鉴权入口而非字节relay证据。新strict设置、拒绝媒体回退/转码/封装、DTO关闭transcoding/remux且不捏造DirectPlay、Vue明确控制和状态均已实现。首E2E失败定位为隐藏combobox点击未打开，改用已有ArrowDown交互；不存在“取消”按钮改为真实关闭对话框。最终14项E2E、Vue7项测试/构建、18脚本、全Go race/vet通过。

首次严格发布actual item17三入口同临时URL/302/115CDN206/TS同步，真实DTO false transcode/remux、坏Token401、六种媒体否定场景409且窗口新增FFmpeg日志0。不是Sen客户端解码/非管理员矩阵。再加固默认拒绝所有视频/音频项下未知媒体输出（仅字幕/附件/附加元数据例外），避免未来端点或Audio/universal绕过，STRM也回显direct-only。最终加固发布/回退/状态以本轮receipt为准；不会改为“全部项目开发完成”。

## 2026-10-10：用户明确改为STRM原链接

用户明确“不需要25015/Videos/.../stream，直接使用STRM自带链接”。严格模式据此保留媒体来源Path中的原固定链接（编码/查询顺序不重构），对外声明IsRemote/Http，不添加Emby Token，也不在元数据阶段为客户端预先取115临时链接或绑定UA；真正播放仍由STRM入口按实际客户端UA取链并302。仍拒绝转码/封装/媒体代理回退，不伪造DirectPlay能力。

这项需求明确改变此前“隐藏文件Key”的策略：元数据权限检查后，可读用户会得到文件级Key，持有固定链接者可访问对应文件直至撤销；已向用户说明，且请求本身明确授权这一显示方式。凭据/URL不进入访问日志/报告；不暴露115 Token、Cookie或管理密钥。默认兼容模式保留旧网关入口，指定部署严格模式采用原链接。新测试覆盖两个User-Agent、详情/列表/PlaybackInfo、无媒体ID与未经认证/非托管来源，保证不预取绑定UA、原链接字节一致及拒绝匿名Key读取。

## 2026-10-10：继续剩余内容，补CloudDrive2变化推送

新目标“继续完成剩余的内容”，上一工作为progress（STRM原链接已发布/实测）。不再恢复HLS/转码验收方向。先完成剩余计划中的PushMessage事件触发：官方proto适配、仅目录dirty提示、10秒合并、连接修订/允许范围过滤、默认off配置与Vue状态、coordinator/库Worker分离、限定流生命周期/重连/unsupported周期回退及只读能力CLI。当前粒度为用户配置的子目录，不冒充更细多层增量队列。

同时将扫描/Job/dirty消费放进同一PG事务，已进行扫描不吞新变化；连接停用或改变期间不提交缺失投票。gRPC响应头可能早于trailers-only拒绝，因此只在首次成功响应帧后标记连接/安排重连对账，unsupported不生成假dirty。隔离PG18/race覆盖开关、事件风暴、跨范围rename、现有扫描、根目录/凭据修订和回退；新开关默认关闭，不自动改用户扫描计划。最终发布和真实只读诊断以本轮报告为准，整个目标保持active。

## 2026-10-10：扫描变化推送发布与管理页刷新修复

Cloud变化阶段已发布，PG17→18为纯新增列/索引，默认所有旧扫描watch_changes=false；既有扫描计划0且未自动建立。真实CD2只读8秒PushMessage收到响应帧、范围事件0；不能称真实文件变化送达已验收。实际原STRM元数据路径item11/17仍与磁盘一致、302/115 CDN206保持，HLS/转码拒绝保持。原Emby/CD2与23其他容器未操作，三Worker/两资产/一份授权保持。当前本项目PT/qBittorrent/Telegram配置均0，订阅0；后续真实全链路需要用户自己的配置，不能借其他项目。

随后修复已经实际复现的管理SPA history路由冲突：25014/emby直接访问/刷新改为管理界面；专用25015 root通过显式root-origin上下文仍走Emby，旧/emby/子路径API保持兼容。新增HTTP验证分流与Chrome E2E直刷，避免用户必须绕道dashboard才能查看策略。新版本发布结果以cloud-events-route-*为准，整体剩余目标仍active。

## 2026-10-10：继续输出迁移与完整字幕清单

上一goal turn为progress：PushMessage默认关闭功能/PG18、真实响应帧、原STRM/禁止转码回归、临时库恢复及管理SPA刷新修复均已发布。本轮继续剩余计划中的跨类型/跨剧集输出：显式migrateOutputs+只读preview+proof绑定，按同一事务提交新输出与旧输出依赖归档，asset/Key/云端/做种原文件保持。旧输出保留.retired字节而非物理删除，共享tvshow.nfo/目录不自动清理。

发现普通journal Stage提前拒绝手工文件会破坏既有Key轮换的“留下conflict供恢复”协议；已把预检限定迁移写入，保持既有写入恢复语义。归档不只看DB已应用，还查新文件实际内容；原子no-replace系统调用防并发目标覆盖，rename-before-commit可恢复。字幕新增完整位图idx/sub配对/PGS与文本清单，整个清单事务提交。多集NFO技术查证存在消费者差异，未把重复XML episode标签当作Emby保证，保持单集NFO/数据库覆盖，后续真实命名/单集metadata矩阵保留。

首E2E定位为使用了不真实的按钮标签，已按现有“修正身份 / 季集”“查询TMDB”交互修正；明确预览→确认/取消、桌面/手机截图与静态测试业务分开。最终测试与发布以output-migration-*为准，整个剩余目标保持active，不执行用户实际资产自动迁移。


## 2026-10-10：单集元数据与多集命名

上一goal turn为progress（输出迁移、PG19、只读现有资产预览及稳定URI/备份已验证）。本轮用官方TMDB季/单集API和Emby多集命名资料校验，补限定季集字段/缺集与错误响应拒绝、NFO与资产metadata、共享整剧数据剥离、连续/稀疏命名与管理只读预览。对已入库媒体不自动改名或重写NFO，迁移证明仍覆盖生成内容。内部/隔离测试覆盖错季/错剧/重复/未知日期与字段、HTTP错误、NFO/XML、PG存储隔离及新预览；真实消费者尤其非连续覆盖仍独立验收。共享Go缓存回归遇到缺包，使用项目独立cache重建而不清全局缓存。整体目标active，PT/qB/外部通知缺配置不以fixture勾选。


## 2026-10-10 归档管理阶段：安全清理、恢复测试与HTTP界面修正

新增源码migration20，归档清单/默认30天/修订保护保留期、1–20项只读预览、持久化明确批准、先quarantined提交再unlink、幂等状态查询/人工重试及审计保留；没有自动清理Worker。补测批准中断/回放不继续、rename-before-commit、unlink-before-commit、缺失当前输出、保留期变化及待恢复状态，不把unknown disappearance算成功。全部管理接口覆盖会话/CSRF、无效ID/JSON与分页/选择边界。UUID大小写规范化避免不同锁身份。

Vue归档弹窗375px与桌面图已人工检查，append-to-body保持确认按钮可见。改用getRandomValues生成UUIDv4以兼容用户允许的公网HTTP，不降低随机性；修复确认后applying状态阻止列表刷新的问题，空/不完整响应不能显示全部成功。增加同上游主机、无item身份情况下STRM原链接逐字保留的回归测试，继续禁止媒体中转/转码和UA预绑定。

完整隔离Go/race、vet及Vue单测/构建通过。真实部署与后续原STRM/302/无转码回归以data/remote-deploy/archive-cleanup-*报告为准；发布脚本不建立示例归档、不更改真实保留期、不批准/重试真实清理，原媒体/当前输出/账号/主密钥/115授权与其他容器保留。协议、TOCTOU边界与接口见ARCHIVE_CLEANUP.md。完整真实PT/下载/订阅/外部通知与真实设备矩阵仍待完成，不标记项目全部完成。


## 2026-10-11 冷取链超时阶段

基于上轮真实10秒502后恢复的证据，不预断115/DNS/令牌根因。源码审查确认总取链10秒/缓存源12秒与两个官方串行API不协调，以及每次调用新建Transport的连接池问题。本地实现30秒默认/5秒–1分钟总预算、官方有界共享连接池、阶段固定枚举与数字日志、超时504/用户取消中断、有限固定入口写期限。新增隔离TLS实际连接复用/请求凭据隔离、10.2秒真实时钟冷取链成功、Redis共享等待取消/排队期限、管理短写期限与敏感日志过滤回归。所有调整保持STRM原链接、实际UA、115文件身份校验和不转码/不回退relay。数据库仍20，是否已发布以resolve-budget-*报告为准；持续偶发超时和全设备/PT业务验收不得据此标记完成。


### 本阶段发布收尾（2026-10-11北京时间）

已发布resolve-budget-20261011，PG保持20；完整Go/race/vet、9Vue单测/构建、18脚本、19E2E、实际amd64镜像/两套网络隔离native测试通过。真实11/17条目原生/浏览器UA首次与立即重复共八次302/CDN206，再次11两次复读缓存通过。首次自动刷新Token后revision变化造成一次正常缓存miss，后续warm命中；无重新扫码/换账号/借用身份。阶段日志核对不含选中文件Key、EmbyToken、选中网盘路径或临时URL。HLS/转码拒绝、既有5个输出/资产绑定授权/计划/管理员/.env/主密钥/compose、Emby/CD2及其他23容器摘要保留。本轮未确认全部历史超时的上游根因，未宣称持续压力、实际客户端解码或完整PT订阅/下载/外部通知完成。文档状态使用独立补充证明同步，初始镜像/源码/manifest继续保留，运行代码不再改动；SSH临时凭据收尾删除。


## 2026-10-11 同版本Emby实际消费与非连续合集修复

先用实际Emby 4.10.1.0、内部网络、自有5秒视频和去Provider ID的虚构样本验证旧命名。连续范围/单集/特殊单集正确，非连续E01E03在有/无NFO时均误扩为1–3；仅代表首集候选漏3，未作为完整方案。新投影为每个声明集号生成索引、共用asset/原固定链接，明确播放整文件不切片；13个Episode/11场景的同版本实际扫描证明普通/特殊稀疏1,3和连续1–3精确覆盖。临时Emby/HTTP样本容器、独立网络与配置已按owned label清理，原用户Emby未动。首次诊断启动失败和修正后的结果都保留，不把静态Parser测试作消费验收。

同步补确认/下载入库/明确迁移、别名合并正文/Key一致、字幕放大限制、旧链拒绝无迁移、NFO单集身份及whole-file警告、按输出路径而非asset计数的Emby可见确认。新GUI提示不补集/不剪辑，现有资产不自动迁移；发布状态以emby-projection-*报告为准。保持数据库20及当前STRM/115实际UA/不转码策略；真实设备、权限、PT下载/订阅/通知门槛不勾选。


## 2026-10-11 旧输出布局与专用入库恢复

在上轮遗留边界继续推进，旧任务拟写的新集合须与完整已有集合一致；在网络调用前及资产锁内双重检查，冲突持久化needs_output_reconciliation并让Job paused。普通上传控制/重试不绕过；已有迁移proof修复后，read-only输出预览/显式控制revision+digest批准专用transfer.import.resume，不重新上传/覆身份/改STRM。摘要覆盖NFO/字幕/原链接/Key、源目录及源字幕；相同批准重放不新增，变化后再次批准使用新控制修订幂等键。候选后台Finalizer增加消费布局和实际托管文件门槛，旧稀疏链不confirmed，UI提供已有asset聚焦。补回上传围栏/入库锁交接，原全流程fixture通过，未降低租约或暂停规则。

本地全量Go/race/vet、Vue/21E2E和保护验收按output-recovery-*记录，真实发布不创建/恢复用户下载或改其媒体；数据库仍20，无新主密钥/账号/授权或自动迁移。完整真实PT/订阅/外部通知与设备权限仍未全验，保持目标未完成。


## 2026-10-11 原STRM链接专项复核

按用户要求只核对“Emby返回磁盘STRM固定解析地址”，未继续本轮无关PT兼容调查。既有output-recovery生产镜像已具备此逻辑，当前生效策略来自已启用Emby连接中的directPlayOnly=true，而非环境变量默认值。未重建或重启服务、未修改生产代码/配置/已有输出。条目11/17按SenPlayer及浏览器UA读取媒体详情、GET/POST PlaybackInfo共12项，Path逐字等于原STRM，未提供Videos/.../stream替代，未预绑定UA、CDN地址或RequiredHttpHeaders。四组只请求原固定链接且不带EmbyToken，均302→cdnfhnfile.115cdn.net→206，Range读取1024字节；HLS/转码请求仍409/direct_play_required，错误Token401，测试窗口无新增FFmpeg日志。5个原输出和.env/compose摘要、所有容器身份保持。

本地扩展strm_metadata_test.go覆盖POST PlaybackInfo，两类UA下不替代Path且不提前调用provider；Emby/playback/STRM/config四包隔离-race回归与对应go vet通过。未改运行时行为，故没有新生产部署。只读脱敏证据见data/diagnostics/strm-original-link-live-20261011.json。首次诊断只看环境默认值，错误判定策略未启用；原观察保留为env-only-probe，后续修正诊断按实际连接设置优先级读取，全部检查通过，不为此改动生产设置。使用现有管理员会话的HTTP验证，不宣称实际播放器解码、网页编码兼容、非管理员矩阵或整个项目已验收；临时SSH凭据收尾清理。


## 2026-10-11 RSS核心兼容与订阅基线阶段（候选，未部署）

上一goal turn完成原STRM专项复核和POST PlaybackInfo回归，属于progress；本轮回到未完成PT链路。先核对参考配置和原RSS实现，再读取固定UNIT3D提交的实际RSS模板/路由，明确发现HTTP200登录XML会成功返回空源、路径下载/纯数字GUID被丢弃。用上一部署源码中的rss.go进行隔离复现，原错误行为证明和临时样本目录清理均记录，而非只写假设。

补识别严格RSS/Atom根/完整token边界、500项/64层、命名空间/重复标量/CDATA/XHTML、相对URL/重定向后feed地址/xml:base、自己配置域的有限路径ID和键下载URL。保留旧account+种子ID摘要，GUID不覆盖已确认Nexus ID；错误/冲突不返回半份结果。公开详情不使用键下载路径，download hint由已有Vault加密。UNIT3D的contentlength、Nexus明确声明的RSS size/促销字段在单entry求值，不猜timezone、不把未知免费/H&R补成安全。RSS-only不再从上一run合并过期大小/做种/免费/H&R/标签，同run已确认搜索仍可补全。

PG回归覆盖首次无效源不初始化、合法恢复先建历史、路径资源加密/去重、错误源保留已建立基线与原提示、元数据同run范围；全量隔离Go -p1/-race/-count1及vet、Vue9项/构建、Python18项和Chrome21E2E通过。UI无改动，无新迁移、PG20不变。没有借用其他项目凭据、接受上游许可证、自动导入/配置或启用订阅。静态审计仍193/159/34/0，accountVerified=false，时间偏移语义与専用框架门槛保留。

本轮当前为候选源码，正式部署仍output-recovery；Docker/原生amd64隔离验证与受保护备份发布是下一步，未把本地样本验收冒充用户真实PT业务。仍需实际搜索→下载→做种保留→CD2→STRM/NFO→Emby→通知，以及真实缺集/促销和设备权限/HTTPS/持续稳定性，整体目标active。


### RSS候选构建收尾（2026-10-11，仍未发布）

已完成linux/arm64与linux/amd64候选Docker构建；原生aarch64 Docker运行配置审计通过（UID65532、只读根/只读参考卷、无网络，193/159/34/0、accountVerified=false）。首次SSD bind挂载被Colima拒绝，保留失败日志；改为复制到本轮独立参考卷并只读挂载，未改原参考文件权限，临时容器/卷已删除。此处不是原生amd64运行、RSS实际账号或NAS设备验收。

准备了linux/amd64的sites/search/subscriptions三份独立测试二进制及校验摘要，下一步在指定x86_64服务器的临时网络/独立PostgreSQL库运行，再进行受保护发布。源码/镜像/测试manifest见data/remote-deploy/rss-core-candidate-manifest.json；诊断见data/diagnostics/rss-round-audit-20261011.json。镜像构建后的补充只涉及文档状态和一个负例XML转义修正（避免跨域URL负例先在XML语法处失败），运行代码未变；修正后的RSS回归单独通过并重新交叉构建sites测试二进制。初始源码包/构建证据保留，最终当前源码与构建源码差异由manifest明确列出，不谎称两者全部文件逐字相同。当前未创建SSH身份、未连接或更改正式部署，原STRM/不转码策略保持。


## 2026-10-11 RSS原生验证与保护发布

上一goal turn已实现RSS模块修复、全量回归和候选双架构构建，属于progress。本轮先逐文件核对337份最终源码、两架构镜像档案及三份原生测试二进制摘要；重新核对指定服务器实际基线，未使用历史配置数代替实时状态。原生x86_64配置加载审计及sites/search/subscriptions的32项顶层测试（28/1/3）全部通过，独立内部PG与临时密钥/容器/卷/网络清理，未访问生产测试库或PT账号。原生二进制为CGO关闭、不带race，本地race结果单列。

发布前备份在另一独立PG恢复，并在该克隆离线dry-run检查8类密文后回滚，没有改生产主密钥。备份/旧源码/原镜像回退点留存，仅更新server和三个Worker。发布后12项原STRM媒体详情/GET/POST PlaybackInfo一致、四组实际UA头的HTTP请求302→cdnfhnfile.115cdn.net→206、HLS/转码409，测试窗无新增FFmpeg日志；仍是管理员会话/HTTP验证而非真实设备解码。PG20、密码记录/.env/compose、资产绑定/播放授权/扫描计划/现有5份输出和Emby/CD2/其他23容器保护摘要保持。

证据为rss-*发布报告、native日志摘要及独立docs-supplement；文档状态不覆盖最初镜像/源码manifest。没有配置PT/qB/外部通知、接受许可证或启用订阅，实际配置数仍0，不据fixture勾选真实业务门槛。剩余PT请求/重试一致性、实际业务和设备/权限/HTTPS/持续稳定性仍保留，目标active，不因这次发布标记项目完成。SSH临时材料在收尾删除。


## 2026-10-11 每轮结果快照/多站点重试（候选，继续开发）

上一goal turn原生验证及保护发布rss-core属于progress。本轮逐文件核对已发布清单后，新增隔离PG失败测试确认两种跨run污染和RSS重试旧/新条目联合；不是从状态说明直接假设缺口。候选迁移21仅在media_tool_test应用，正式服务器仍20，本轮未创建SSH身份或发布。

已补run独立公共结果/加密提示/账号与定义来源证明/观测时间，旧links不假回填；公共Get不读取全局可变字段，部分RSS只继承同run快照。实际HTTP客户端证明在保存事务复核；共享行锁短暂保护账号/定义，网络不持锁。收集阶段失败重试清空未决策集合，完整集合冻结后决策错误重试保留响应；空源证明也校验，配置变化/过期暂停且通用重试不绕过。下载预检/创建/Worker绑定searchRunId，订阅入队事务复核启用/revision，CryptoColumns增至9并覆盖AAD rekey测试。Vue固定弹窗来源并在对话框显示可访问的重新搜索错误，needs_refresh标记中文化。

两个原失败搜索隔离测试、多站点失败后新集合与不推进旧seen、完整收集后错误不重取、空源账号修订变化不建基线、旧快照/加密隔离/过期及第9类rekey已验证。修改未发布的迁移草稿后，仅重建隔离测试schema：第一次DO语句引号错误在DROP前中止，保留日志；随后带current_database保护的事务重建成功，未清用户业务库/全局Go缓存。另一次本地补丁脚本语法错误发生在执行前，没有部分源写入，随后修正。

当前候选仍缺已排队任务暂停后的明确来源重新确认与更多并发/恢复门槛，下一步继续实现而非宣称全部恢复完成。全量Go/vet/Vue/E2E及渲染结果在本轮收尾记录。实际PT/qB/外部通知、设备/非管理员/HTTPS/长时稳定性依旧独立，整个目标active。


本候选本地收尾：全量隔离Go -p1/-race/-count1、全量vet、Vue9项/类型检查/构建、Python18项、Chrome22E2E通过。新增搜索下载场景用明确fixture验证提交选中searchRunId、409重新搜索错误保留对话框，并检查375px与桌面截图：确认按钮可见、错误靠近确认操作、没有横向裁切。没有把这些样本当用户真实账号。

本轮未构建或发布新镜像、未升级生产库、未建立SSH身份。正式部署仍rss-core/PG20。本地迁移21仍为候选，下一步优先补“已经排队的下载因来源快照变化而暂停”的专用重新确认/恢复路径和重复任务媒体/InfoHash冲突验证，再做并发/中断、原生amd64、迁移/密钥回退与保护发布；不能凭当前绿色测试宣称这个完整恢复流程已完成。


## 2026-10-11 下载来源专用恢复与重复任务身份校验（本地候选）

上一goal turn快照与RSS重试数据隔离是progress，本轮从实际源码/任务队列约束继续补恢复，未用“暂停”代替完整操作。候选22加来源门槛/控制修订/接受receipt；新任务先pinInfoHash，复用已有任务须同媒体/季集/Hash。来源预览不写本程序状态/不提交qB，但会读取PT种子和下载器，PT自身计数或生成token可能有变化，未宣称完全无远端影响。提交重新取证、校验digest/revision/lease/账号及定义/下载器与路径、订阅原规则，恢复原任务，串行与并发同确认不重复入队。无原Hash的旧待提交任务要求额外明确首次身份确认，非“同Hash已证明”。候选模型加source_recovery_required，通用重试与Add禁止绕过，旧accepted确认不重置再次暂停的新一代状态。

将原订阅Title/Free/H&R/做种规则抽为共享subscriptionpolicy并保持订阅API包装；恢复与原评估同规则。加强qB按hash读取：错误/重复的返回hash不当成功；原测试fixture此前返回固定“hash”而非请求ID，调整fixture模拟真实过滤并增加错hash拒绝测试，没有放宽逻辑。首次测试亦暴露TMDB请求ID2但响应ID1的身份校验缺口，生产入口现明确拒绝不一致，测试响应按请求返回ID以检验真正的不同媒体冲突。运行lease负例按真实数据库约束设置token/until/owner，不伪造不合法running行。

全量隔离Go race、vet、Vue9项/类型检查/构建、18脚本及最终严格串行23E2E通过；375px和桌面新来源弹窗已人工检查，按钮可见，hash换行、明确同身份确认可键盘/点击。首次checkbox由于隐藏代理input/滚动容器无法正常测试可见交互，改为有可见尺寸/焦点样式的native checkbox，不用force点击绕过确认。随后一轮E2E曾与另一个PG测试重叠导致队列样本被清，属于测试调度错误，不作为业务失败根因猜测；停止两句柄后重跑完整Go→vet→E2E串行全部通过，保留失败日志，不跳过测试。OpenAPI新增路径初次位于components下，已移到paths并解析核对。

本轮未发布、未创建SSH身份、未升级正式PG；生产仍rss-core/20，候选21/22仅隔离库。整体实际PT→qB→做种保留→CD2→Emby→通知/缺集促销与设备/权限/HTTPS/容量门槛保留。下一步继续未知提交/恢复及源修改竞争、更宽实测，准备原生amd64和新schema备份/轮换/回退再保护发布，不标项目全部完成。


## 2026-10-11 来源恢复未知提交、竞争与迁移验证（待原生/发布）

上一goal turn专用恢复本地实现与23E2E属于progress。本轮核对候选摘要后补HTTP真实时钟样本：qB已接受但返回503的重试按落库Hash核对，过期旧来源不触发重复Add；旧receipt不能重置新暂停代。新增竞争测试先失败（预览期间账号修订变化仍返回nil），修复在PT/qB网络结束后再次ValidateRunResource，并绑定网络开始前snapshot digest，另测同run提示替换不能与旧torrent混合。本轮全流程fixtures不使用PT账号或真实文件。

添加tests隔离迁移往返20→22→20→22，旧链接不填当前快照、原密文/task身份保持；每次只连接media_tool_test，defer恢复候选版本。下一步真实x86_64/备份克隆/9类密文dry-run及受控生产迁移，未据本地结构回退测试宣称生产回退已验收。


## 来源恢复发布收尾：2026-10-11

已发布media-tool:source-reconfirm-20261011，schema20→22，仅停止/更新server、worker、transfer-worker、library-worker；账号/主密钥/115授权/媒体身份不改，无PT账户或规则自动导入。全量本地race/vet、Vue9项/构建、18脚本、23E2E；原生x86_64五包30项顶层测试无跳过，使用新内部网络/独立PostgreSQL，非生产库/非真实PT资源。原生跨编译fixture不带race，本地race证据另列。ARM64原生只读无网络配置审计通过，193/159/34/0仍只是静态支持，不提高accountVerified。

补验证qB接受但返回503的未知结果按已落库Hash核对、无第二次Add；旧receipt不恢复新暂停代，来源revision/同run提示在网络读取中变化被拒绝；读取无效task详情后不以空结构覆盖原数据。初始候选不包含最后这一读取失败保护，故保留初始包并生成validated独立源码/镜像/fixture清单；正式切换的是validated版本，不替换或冒充历史证据。

发布前备份已在另一个独立PG恢复，升级22并执行9类密文dry-run回滚，再结构降20/升22验证。第一次SSH克隆派发失败后读取证实无marker/process/目录/容器/网络/卷；第二次执行输出丢失，后续发现服务器已完成的passed回执，采用回执而没有第三次执行。发布亦以持久化receipt核对，不盲目重放。临时资源、含密钥目录与SSH材料在收尾清理。

发布后12项原STRM媒体详情/GET/POST PlaybackInfo逐字一致，四组代表性播放器UA头请求302→cdnfhnfile.115cdn.net→206，HLS/转码拒绝，验证窗口无新增FFmpeg日志；不是实际设备解码或非管理员权限验收。原5份磁盘输出、密码记录/.env/compose、资产绑定/播放授权/扫描计划与Emby/CD2/其他23容器由摘要核对保持。API仍要求管理员登录，首次注册已关闭，密码新设置6–20位。

回退镜像media-tool:before-source-reconfirm-20261011；备份/root/docker/mediatool/private/database-before-source-reconfirm-20261011.sql.gz；旧源码/root/docker/mediatool/private/source-before-source-reconfirm-20261011。发布/原生/克隆/保护证据为data/remote-deploy/source-reconfirm-*报告与validated-release-manifest。文档收尾另用docs-supplement，不修改运行代码或再次重启。

真实PT/qB/外部通知连接与订阅当前仍0；合法资源搜索→下载→做种保留→CD2→STRM/NFO→Emby→通知、真实缺集/促销与设备/权限/HTTPS/容量稳定性仍未完成。下一步继续pre-dessert请求体/分页/框架等更宽契约，以及上述实际验收，整体目标active。


## 2026-10-11 PT请求体与分页候选（继续推进，未发布）

上一goal turn已完成source-reconfirm保护发布，属于progress。本轮逐文件核对其已发布清单后，读取固定pre-dessert mteam/yemapt等请求定义并审计声明。新增HTTP负例先复现mTorrent覆盖body/form导致mode/categories/filter/page忽略，再将明确载荷放在schema默认之前，冲突/类型不支持报固定错误，空/null保留，不重复替换用户注入的{page}文本。明确UA不被固定程序UA覆盖，query与载荷并存。JSON必须声明结果数组，尾随JSON/缺失结果不当空源。

有界多页只跟随请求中的页变量或既有mTorrent默认（5页/500不同资源），不猜短页/offset/cursor/第一页基数；重复页/上限/后页失败及无分页声明的非空单页显示partial/原因，auth失败丢弃集合。有效部分结果不是failed，RSS both不遮盖搜索不足。重复搜索清本account未消费阶段，冻结/已被下载使用的run禁止变化。精确选中sourceSnapshotDigest和按语义而非cipher nonce计算的证明避免同run替换/主密钥轮换错误，隔离PG与9类rekey验证。尚无此新摘要字段的已排队run任务需专用重新确认，当前正式PT账号/任务状态不从历史报告推断。

全量隔离race/vet、Vue9项/构建、18脚本和24E2E通过。早期E2E一次删除连接等待未完成，保留日志不猜业务根因；另一次全量fixture期待旧单页succeeded，现精确改测partial/单页说明，完整下载上传STRM通知样本继续通过。第一次UI补丁找错panel类在SearchView写入前中止，随后按实际模板修正。移动截图曾处于响应式过渡帧、侧栏遮挡，测试改等待实际边界并启用reduced-motion，最终截图人工核对消息与徽标可见，不强制点击/跳过。runtime最终加入UA后再完整串行回归；截图测试增加稳定等待不改变runtime。

静态审计158 core/35 partial/0 invalid/193加载，新增sunny的X-Csrf-Token鉴权限制说明及zhuque同类notes：之前required-header解释器已拒绝，不能为维持159而隐瞒真实缺口。timezone +19等未假装UTC、未引入PCRE来虚增数量。源码候选未发布、本轮未建立SSH身份/读取其他项目或改正式库，已发布基线记录仍source-reconfirm/22。下一步完善混合来源/原生/保护发布与真实账号/设备业务门槛，整体goal active。


## 2026-10-11 用户要求原STRM链接的定向复核（无新部署）

按最新要求，不把`25015/Videos/11/stream?...`作为媒体信息中的播放地址。当前严格模式已返回STRM原文，未重复修改运行逻辑/生产配置；补充根网关与/emby挂载、四类元数据请求、两类UA共16组fixture断言，保证查询编码/顺序、Remote/Http、真实DirectPlay能力，且无流/转码入口、Emby Token或预绑定提供方头。DIRECT_PLAY_ONLY.md加入明确契约，参考项目历史排查文档加当前方案说明，避免旧HLS/相对stream分析被当作当前要求。

2026-10-11 11:53服务器只读验证12项媒体详情/PlaybackInfo原STRM一致，4组代表性UA直接原STRM302→cdnfhnfile.115cdn.net→206/1024字节；错误Token401、HLS/转码409，严格模式开启，配置/原5份输出/所有容器身份保持、窗口无新增FFmpeg日志。首个主机预检未通过，未执行认证操作；新的只读扫描核对既有主机指纹后仅派发验证一次，不盲重放部署。临时SSH目录/identity/known_hosts已删除，日志不输出Key/用户Token/签名URL。

相关三个包的定向race先运行，初次未配置Redis的那一次有1项身份缓存集成测试跳过；随后使用既有隔离脚本（media_tool_test/Redis15/-p1）三个包均通过，定向vet通过。未运行/宣称全项目测试或真实播放器解码、非管理员矩阵验收。本轮证据为data/diagnostics/original-strm-{live-recheck,live-ssh-preflight,current-unit-summary,isolated-regression,targeted-vet,requirement-audit}-20261011.*。本轮仅测试/文档与只读验收，未部署待发布PT请求/分页候选。

整体goal仍active。上轮新加的both搜索失败+空RSS修订证明characterization已确认失败（Process错误地返回nil），日志pt-mixed-empty-proof-characterization-20261011.log保留；其ValidateRunSources过滤/混合证明覆盖修复仍待后续完成，不把该未发布问题归因于本轮播放回归或宣称所有剩余项完成。


## 2026-10-11 混合来源证明/旧任务兼容加固（继续推进，未部署）

上一goal turn增加原STRM两入口回归、真实只读12媒体信息/4代表性UA链路复核，为progress。按未完成候选恢复当前故障测试与源码：both搜索失败导致成功空RSS证明被state过滤漏检；后续空RSS覆盖先前空搜索来源/时间。新PG负例分别先复现revision/definition覆盖、3种警告状态观察时间刷新、残缺证明/无site记录及未来时间接受，再修复记录证明独立于展示状态、client完整门槛、保持首份来源和时间。首次聚焦search/subscriptions均通过。

扩展审核发现最后基线提交仅查订阅revision，未再次锁定来源。用ACCESS EXCLUSIVE覆盖表关系锁和pg_stat_activity精确观测“已通过首轮来源校验、等待coverage”的窗口，改变空账号/定义，两项旧运行均返回nil并落基线，独立失败日志保留。现收集冻结与最终提交调用ValidateRunSourcesTx，按账号排序读取并持有run/site/账号/现存定义共享锁；过期/变化则rollback后needs_refresh，不提交RSS baseline/seen。另4项实际PG lock_timeout对抗证明锁随提交保留、提交后释放，不用计时睡眠猜保护生效。

旧run-bound任务去除sourceSnapshotDigest后，不取种/不Add，重复失败仅一次控制修订。显式预览批准后绑定新摘要并复用原Task/Job，重复receipt/Add仍只有一次qB提交。已知Hash远端接受但503的旧缺摘要/过期来源任务，则复核qB实际存在、不再次取种或Add。第一版fixture误把初始source_control_revision视为0，实际迁移22默认1；已改为读取初始值并断言只加1，不改迁移或运行控制语义；该失败日志留存。

最终三包隔离PG聚焦及全量race/vet通过；Vue9/脚本18/构建通过。第一次完整E2E 23通过/扫码exchange等待5秒超时。加仅fixture轮询receipt/页面可见性诊断，未修改授权组件、未放宽等待或跳过，下一次完整24通过；因只grep授权会漏掉首次管理员注册，尝试的5项均在login失败，未到轮询，不能冒充原故障复现。测试脚本临时转发参数已恢复原文。另一次紧邻清理的端口预检拒绝启动，随后lsof/实际bind证明无存活监听，再启动完整流程；未强杀其他服务或盲重播业务。初始/专项/端口失败证据全部保留。授权exchange和partial搜索手机截图已人工核对，仅fixture，不是用户实际扫码/设备解码。

证据前缀data/diagnostics/pt-mixed-*；本轮未建立SSH身份、未接触正式库/授权或其他项目。原生/双架构与备份保护发布仍是下一门槛，不将source-reconfirm旧镜像测试用于新候选，也不把当前0账号状态从旧报告推断。无新迁移、STRM原链接无转码要求保持，真实用户账号/资源全链路与设备/普通用户/HTTPS/长期容量仍未完成，整体goal active。


## 2026-10-11 来源一致性候选：原生验证、备份与保护发布完成

上一goal turn修复混合来源/提交窗口/旧任务摘要、全量本地测试和构建双架构，属于progress。本轮先校验380份源码、16份归档/测试包摘要与当前工作树，保留不可变manifest；正式只读重新确认9c54.../schema22、3Worker、2资产、1授权115、5输出及PT/qB/通知/订阅均0、无活动维护。不得借用其他23容器配置。

124MB整包SSH断开后明确读取回执/锁/文件，0已完成且无活动进程才改分块。4MiB与1MiB部分连接又失效：采集内核FLOCK的PID/出生时间、部分前缀摘要，不盲重放；一个确定存活上传随后自然终止，未终止服务器或其他进程。最初观察脚本/proc/locks索引错误已修正，再加真实flock尝试，未使用错误观察作为重启依据。分块观察编号曾跨运行重用，改唯一runId并保留早期目录；实际偏移以新读回而非编号排序为准。最后复用本任务专属SSH控制连接，10件artifact全部校验落盘。传输失败/实际进程与校验记录保留，不替换成假成功。

真实x86_64默认CPU条件、fresh internal PG/network/volume、只读运行镜像、六包83顶层项全部通过且无Skip：sites39/acquisition17/search13/subscriptions8/secrets4/tests2。193/158/35/0静态审计、accountVerified=false。Mac的AMD64仿真regexp SIGSEGV在此次物理执行未复现，不宣称所有模拟器/Go问题根因已确认。原生是CGO0跨编译、非race，race证据为本地全量日志。六份fixture原始日志与摘要已取回留存；测试没有访问正式数据库/账户。临时资源与含测试密钥目录清理。

发布前已有项目备份在新的内部PG恢复，schema22保持；候选migrate为幂等无变更，九类密文dry-run未提交，各类字节摘要和原资产/授权事实保持。不是为制造“升级成功”而把当前数据强制降20；镜像回退留在22。原主密钥未导出/轮换，next-key仅用于孤立克隆并清除。Native/clone/切换均单次派发、有PID出生时间与持久回执；无观察丢失后盲目第二次执行。

13:24仅停止和更新本项目server/worker/transfer-worker/library-worker，实际image a425bc...、release media-tool:pt-mixed-proof-20261011，schema22无迁移；保存旧镜像before-pt-mixed-proof-20261011、原源码和private/database-before-pt-mixed-proof-20261011.sql.gz。13:26十二媒体信息原STRM一致、四组代表性UA原URL302→115CDN206/1KiB，拒绝HLS/转码/坏Token。13:28保护对比全部通过：密码记录/.env/主密钥/live compose、115授权身份/资产/绑定/播放Key/扫描计划、原5输出、Emby/CD2/PG/Redis/migrate以及其他23容器均保持，三个Worker在线。API仍需管理员鉴权、注册已关闭、密码6–20位。不是实际设备解码/非管理员矩阵/真实PT链路验收。

文档更新以独立docs-supplement记录，保留原源码包/镜像/构建与发布manifest，不为同步文字再次重启。整体目标仍active；下一步继续不依赖账号的更宽站点/部署/容量验证，以及用户自己PT/qB/通知配置后的合法资源、做种/订阅/设备/权限/HTTPS业务门槛。


## 2026-10-11 PT声明头与origin协议继续推进（未部署）

上一goal turn实际完成pt-mixed-proof原生83项/备份克隆/保护发布，为progress。本轮先核对计划、源码与固定配置：sunny是Authorization而非X-Csrf-Token，zhuque是X-Csrf-Token，快照没有header生产请求。公开repo/Go标准库primary文档与本机Go1.26.9 client.go用于确认HTTP机制，不把当前上游版本替换为固定commit，也未据README的许可信息自动接受/导入。没有借用其他项目凭据。

新增自有HTTP/TLS负例先复现明确required头被拒、跨origin自定义头泄漏、HTTPS跳HTTP、307原Body泄漏及origin默认端口大小写问题；错误日志保留。request_headers.go统一类型/名字/值/大小写去重/模板完整与origin判定，APIKey保留原账户字段优先，只有已提供声明可用、无自动CSRF/Bearer。CheckRedirect在离开origin后持续去凭据，防止Go后续重复制；同origin正常流保留、跨origin公开GET/HEAD许可，拒绝携Body/非读取方法及TLS降级。Cookie只发账号origin，关闭拷贝client的环境Jar，不破坏调用者Jar；新增回原origin/ambient Jar/无效声明/绝对额外origin APIKey边界和静态兼容证明。未修改115/Emby实际链路。

四包隔离PG/full race/full vet与24E2E通过；静态审计193/158/35/0仍accountVerified=false。缺少header producer的sunny/zhuque仍partial，不删除限制说明虚增覆盖率，时区/专用鉴权不猜。日志前缀pt-declared-headers-*。本轮无SSH身份、无正式库或服务更改、无schema迁移；当前正式镜像保持pt-mixed-proof/22。候选原生/保护发布及完整自有账号/订阅/做种/通知/真实设备/非管理员/HTTPS/NAS容量目标仍未完成，goal active。


本候选最终补检还复现初始绝对additional-domain请求携带已声明头/私密Body的漏洞：不是只有重定向需要origin边界。现当请求target不同于所选base/API origin，只有无声明头、无Body的GET/HEAD公开取链可继续；任何定义私密材料或写请求在HTTP前拒绝。最终代码再次全量隔离race/vet通过。第一次最终E2E为22通过/2个UI等待超时（登录和稀疏集合行点击），已保留原错误上下文；确定旧进程/端口已清理后原样完整复核24通过，没有增加超时、修改UI或强制点击，未据后一次通过宣称最初根因已解决。

## 2026-10-11 附加鉴权头账号入口继续推进（候选）

上一goal turn以自有HTTP/TLS复现并修复声明头/origin协议为progress。本轮补通操作员配置路径，而非停在无账号可填写的解析修复：PT凭据新增requestHeaders，严格平面JSON对象/16项/16KiB/单值8KiB、拒绝原始与大小写重复键、控制字符/非字符串和代理/转发/传输/Cookie等保留头。AccountHeader值只作为字面值，不替换{passkey}、不补Bearer/CSRF。现有AES-GCM封装和九类rekey覆盖此字段，无新表/迁移。仅required_headers对应名字用于本请求base/API origin；APIKey字段原优先、附加头其次、明确配置头最后，未用头不发、跨域不恢复。

隔离PG验证保存加密、公有DTO/审计不含值、Manager解密与只读认证、编辑保留、明确{}清除、改endpoint不隐式带走秘密、凭据修订拒绝旧run证明/不合并新旧来源，以及rekey默认dry-run字节不变。第一轮正例先证明旧Validate拒绝requestHeaders；错误证据保留。前端新增掩码/可显式显示字段、局部aria-invalid/关联错误、JSON形状/原始重复键和大小写别名提示、字节界限；不写localStorage、不回显已存值。提示经HTTPS管理地址填写，不把数据库加密说成HTTP传输加密，不悄悄改变用户原来批准的公网设置。

UI QA初次长表单使标题/关闭入口在屏外，已改为专属有界滚动body；手机保存按钮ratio=0.9929的失败保留，添加实际底部留白后同一ratio=1断言及关闭按钮可视门槛通过，没有放宽断言或force点击。桌面/375px字段、提示、关闭/保存均手工核对。首次25项E2E为24通过/原115流程总时间超时，保留原error-context，未伪称其根因已解决；随后当前候选25项流程通过。初版Vitest测试文件写入中截断在工具写入后立即修复，未发布；最终14项前端单位测试、构建、18脚本、全量隔离race/vet均通过。最终传输安全提示与最新Bundle再次E2E另列。

静态配置193/158/35/0及accountVerified=false保持；sunny仍缺少自动Authorization producer，zhuque仍缺少自动CSRF producer/专用框架，不因手工头而更改为真实验证。候选还须双架构/新原生/保护发布；当前生产仍pt-mixed-proof/22，本轮此处未建立SSH身份或读取/借用任何其他项目凭据。用户自己的合法PT→qB/做种→CD2→STRM/NFO→Emby→外部通知、实际订阅与设备/非管理员/HTTPS/NAS/容量等仍未完成，整体goal active。


本候选补齐“先验证后启用”PT流程：明确的管理连接检查可只读验证未启用账号，但Site业务入口仍拒绝未启用账号，不改变Enabled或账号revision，不创建搜索/下载/订阅。隔离PG正例先复现旧检查失败，再按私有只读构造路径修复；测试验证检查前后业务拒绝和编辑开启后的凭据/来源保护。最终代码再跑全量隔离race/vet，E2E/新原生与候选发布状态以本轮记录为准。之前一次系统检查UI等待超时后读取隔离库，job实际在约2秒内succeeded；这不是证明UI定时/SSE根因已解决。追加只在失败时输出无secret状态/页面可见性诊断，未延长等待或改变Worker行为，之后完整25项通过。

本候选最终增加未启用PT账号只读验证隔离回归后，full race/vet通过。E2E出现过原扫码总时间超时、系统检查旧UI等待、登录Failed to fetch、站点快照与旧入库弹窗等待等不同失败，原证据保留，不归因于附加头也不宣称已消除根因。仅增加按请求类别/相对时间/状态/固定网络错误枚举的失败诊断，不记录URL/query/body/cookie或增加重试、超时、强制点击。最后当前完整25项通过，14前端单位/18脚本/构建通过；本轮新Secret字段使用与25项界面入口测试保持已验证，真实账号矩阵仍另行验收。候选将一次性包含上一轮origin/声明头与本轮加密入口，构建七包AMD64测试二进制后再做新原生/备份保护切换，绝不借用上一镜像83项证明新代码。此前错误上下文偶含本次随机E2E密码（不是用户密码），已在复制证据时去除并保存为私有文件；未把该模拟账号或其会话带到服务器业务配置。


## 2026-10-11 PT附加鉴权头/origin完整候选：原生、备份与保护发布完成

上一goal turn补完整账号/Manager/HTTP/Vue入口、只读验证与本地测试/双架构，为progress。本轮先逐字核验388份源码和所有归档/七包编译二进制，未修改运行逻辑；重读正式基线a425.../22，3Worker/2资产/1已授权115/5输出、PT/qB/外部通知/定义/订阅0、无活动维护。临时0700/0600 SSH身份使用用户提供的授权与既有SHA256主机pin，任务专属控制连接与1MiB校验分块；11件artifact一轮完成，无不明结果重放。没读其他23容器Env或借用凭据。

真实x86_64默认CPU、fresh内网PG/volume/network、只读UID65532，七包115顶层全部通过且无Skip：sites56/integrations15/acquisition17/search13/subscriptions8/secrets4/tests2。新增integration完整凭据生命周期/禁用验证与新sites头/origin都按当前二进制验证；本地race证据单列，原生CGO0不是race。七份原始fixture日志和SHA取回，新静态193/158/35/0/accountVerified=false不虚增。临时资源、测试密钥目录已清理，native不访问正式库。

新的生产备份在独立内部PG恢复，schema22保持，候选migrate幂等无变更，九类密文表dry-run未提交并比对每类现存字节摘要及原资产/授权事实。生产主密钥/凭据未轮换或导出到本地，不人为降库做假升级。Native/clone/部署都一次派发且保存PID出生时间/持久回执，旧镜像/源码/备份保留。16:17仅切server+三个Worker到41ac...，media-tool:pt-request-headers-20261011，schema22无迁移。

16:18十二原STRM媒体信息/四组实际HTTP UA302→115CDN206/1KiB，拒绝HLS/转码/坏Token、窗口无FFmpeg新增；16:19保护全部通过，账号密码记录/.env/主密钥/compose、授权身份/资产/绑定/Key/扫描/原5输出、Emby/CD2/PG/Redis/migrate及其他23容器保持、3Worker在线。16:21登录/主bundle/服务连接模块资源200，含新secret字段/HTTPS提示/有界dialog，未绕过管理登录或读取已存secret，也不冒称真实浏览器登录验收。注册保持关闭/密码6–20位。

rollback镜像before-pt-request-headers-20261011、源private/source-before-pt-request-headers-20261011、备份private/database-before-pt-request-headers-20261011.sql.gz。发布后的文字通过独立docs-supplement同步，原镜像/源码包/manifest保留、不重建或重启。剩余完整合法PT/qB/做种/CD2/STRM/NFO/Emby/外部通知、实际订阅与设备/非管理员/HTTPS/NAS/容量验收未完成，goal仍active；已有授权与配置不用重设。


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

## 2026-10-11 六项文件/媒体管理更新候选

新增filemanager与catalog模块、迁移23：115/CD2与明确本地根目录的管理、影视自定义文件名/模板与分类移动、同名字幕/NFO、回收/恢复、持久预览/核对进度；文件身份/索引保护，不改原Key/STRM，不自动改用户影视。独立服务路由、115无Token待授权保存、显式登录/CSRF/no-store密钥回显且不回放未改的刷新Token、程序/Emby媒体库浏览与后端海报读取。功能/安全界限见FILE_MANAGEMENT.md。

隔离PG/本地实际文件系统/gRPC/HTTP回归新增源变化/到期/冲突/做种输出保护、未知rename已提交核对、云索引/资产保持与旧路径替换拒绝、Token草稿/回显字段审计脱敏、匿名API拒绝及Emby安全DTO。UI采用原有视觉体系、独立配置tab/路由和移动端文件操作卡片；radio/tag等组件已显式注册。首次E2E暴露旧字段label选择器、组件未注册与checkbox操作问题，第二轮暴露新radio与textbox同名定位，均保留失败证据并修复，不加超时/重试/强制点击/跳过。

本轮不自动迁移正式媒体文件；生产镜像是否包含候选以新发布记录为准，不能引用旧pt-execution-contract的128项原生测试替代。


## 2026-10-11 文件/媒体管理六项更新已发布

指定服务器当前正式镜像`media-tool:files-management-20261011`，ID`sha256:1391e89d6c90a6b83fb900e2e0da9083d42fb8429042fa9f62fbfc6f81ff6ce2`，数据库22→23。六项用户更新均已接通：115/CD2影视自定义命名/模板/分类移动及侧车、独立服务配置路由、115与明确本地根目录管理/回收恢复、程序与Emby媒体库浏览、115无Token保存待授权连接、显式会话/CSRF/no-store密钥回显。具体入口、界限和操作方式见FILE_MANAGEMENT.md。程序不会自动重命名用户现有影视，必须选择文件预览并确认；无永久删除、无视频中转或转码。

本轮最终全量隔离race/vet、Vue17项、Python18项及完整28E2E通过；桌面/375px截图已实际核对，手机文件操作改为卡片，预览取消没有文件副作用。新物理x86_64默认CPU、只读UID65532、fresh内部PG+独立Redis15、无测试端口发布，13包207个顶层测试全部通过无Skip：sites66/integrations16/acquisition17/search14/subscriptions8/secrets4/协议迁移2/离线审计2/filemanager8/catalog1/library25/clouddrive2 16/httpapi28。library的既有显式fixture exporter只在该测试容器的空/owned-fixtures tmpfs运行，未排除用例；13份原始日志独立核验。初始两次隔离环境在migrate连接检查失败、未开始测试包，已清理且失败记录保留；第三次使用TCP和认证SQL就绪检测后通过，未改变应用镜像/二进制或放宽测试，不将旧Unix就绪报告当成真实网络连接证明。

新生产备份在fresh PG恢复，22→23/idempotent23及九类密文dry-run通过，迁移及dry-run全部原字节保持；旧镜像的九类密文只读工具也能读取23，未自动降库/恢复生产库。回退镜像`media-tool:before-files-management-20261011`，旧源码`/root/docker/mediatool/private/source-before-files-management-20261011`，备份`/root/docker/mediatool/private/database-before-files-management-20261011.sql.gz`（SHA256 `5bca7248a7d2d634a4b6e9b7aa3d59488ccc17143a691791491cce2f816fec71`）。新目录/文件操作记录有下迁移门槛，不在仍有本地连接或未核对操作时破坏性回退。

仅切server与三个Worker；管理账号密码、主密钥/配置、115授权身份、资产/绑定/播放Key/计划与原5输出字节保持。24个其他容器的ID从本轮最初清单到切换后一致。Emby/CD2/PG/Redis/Emby-loopback保持，migrate一次性容器本轮前后均不存在，不能凭旧发布记录假定存在。12项媒体信息/PlaybackInfo为STRM原链接，四组代表UA原URL302→cdnfhnfile.115cdn.net→206/1KiB通过，坏Token401、HLS/转码409且无新FFmpeg日志。

公网新页面与对应服务bundle200且含实际功能入口；匿名文件/媒体库/回显API401，注册保持关闭、密码6–20位。服务静态探测初版错误依赖会被压缩的JS变量名，改为稳定功能文案/路径后通过，未修改运行代码；同样未绕过真实管理员登录或声称真实影视批量整理。目录权限、客户端解码/全部字幕音轨、完整PT/qB/通知业务与HTTPS/NAS容量验收仍是整体门槛，不凭207个fixture宣布全项目完成。
