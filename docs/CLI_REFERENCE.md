# CLI / API 现行映射（Q1–Q2）

本页取代旧独立 cmd、Python 和多服务部署命令。正式运行仍只有根 Compose 的 **app / postgres / redis**；Emby/CD2 是已有外部依赖，不创建或删除它们。

## 三类二进制

- `media-tool`：app 内的服务、worker、迁移、管理员维护和诊断；插件仍使用本 binary 的内部 `plugin-worker`，不提供运行期代码下载/编译。
- `media-tool-dev`：源码工作区的构建、资源、测试隔离与合成 fixture。**不装入 runtime 镜像**，不访问真实媒体/生产恢复口令。
- `media-toolctl`：所有者在部署宿主运行的固定 Docker/私有文件编排；应用功能仍调用 app。无 HTTP/daemon/上传执行入口；runtime 镜像附带同源静态 ctl，宿主不必安装 Go。

源码工作区可用 `go run ./cmd/<三者之一> ...`；`make build` 输出三个文件到忽略的 `bin/`。服务器只用已经发布并验证的 Docker Hub AMD64 immutable image，不使用本地构建产物代替发布。

## 旧 19 工具 → 分层子命令

| 旧 cmd | 新入口 | 重要边界 |
|---|---|---|
| media-tool | `media-tool server/worker/healthcheck/strm/account-info` | 既有 `-role/-queue/-cloud-path/-strm-relative` 保留；无参数仍 all；未知命令拒绝启动 |
| migrate | `media-tool migrate` | 原 `-action up/status/down`、`-test-only`、测试插件清理参数保留 |
| admin-reset | `media-tool admin reset` | 必须离线确认、私密密码文件；不是测试夹具，不会启用公开注册 |
| rekey | `media-tool secrets rekey` | 默认 dry-run、全体 worker 停止、显式 apply；当前密文/AAD保留；Q3批准的空旧格式已由断言迁移退役 |
| backup-verify | `media-tool backup verify` | 私有 snapshot、精确克隆名及 test-only 守卫保留 |
| ops-control | `media-tool ops <action>` | info/register-release/check/claim/backup/drill/verify/verify-execution/prepare/finish；verify是备份包核验，verify-execution是维护执行后只读核验，审批不是执行成功 |
| cloud-upload-smoke | `media-tool diagnose cloud-upload` | 自有小夹具、明确写确认、持久 journal、未知结果不重建会话 |
| cloud-events-check | `media-tool diagnose cloud-events` | 只读 1–30s，不能宣称真实变化交付已验证 |
| align-cloud-download-ua | `media-tool diagnose cloud-ua` | 默认只读；apply 验证失败恢复原值 |
| complete-emby-layout | `media-tool diagnose emby-layout` | dry-run/apply 保留，不触碰外部 Emby/CD2 数据目录 |
| gateway-session-smoke | `media-tool diagnose session` | 仅 stdin 接收独立核验的既有 session JSON；绑定仍需明确确认；不自动读外部认证数据库 |
| integration-smoke | `media-tool diagnose integrations` | 使用 app 内现有加密配置做有界检查 |
| playback-smoke | `media-tool diagnose playback` | STRM 原文/授予/可见性；只接受 resolver 302，再取 CDN 206，不接受服务器 relay |
| provider-smoke | `media-tool diagnose provider` | 原 probe/loopback-proxy 参数保留；回环诊断代理不是媒体 relay 功能 |
| secure-cloud-download | `media-tool diagnose cloud-security` | 原 dry-run/apply 关闭 useHttpDownload 的窄变更保留 |
| site-template-sync | `media-tool sites sync` | 无 ids 仅刷新已选项；显式 ids 才加载，不全量装入 |
| plugin-check | `media-tool-dev plugins check` | 仅构建期传递依赖初始化检查 |
| plugin-assets | `media-tool-dev plugins assets [--check]` | 同一个 pin 的清单/本地 ESM，不运行期加载远程 UI |
| site-compat-audit | `media-tool-dev sites audit` | 显式本地源目录的离线审核；源码/commit 参数不进入模板 UI |

叶命令的既有 flag 名和值不变；布局是进程入口适配，不在 server 请求中调用全局 flag/argv 适配器。构建期与 runtime 命令分别链接，没有导入另一个 main 包。

## 开发/CI 与数据库隔离

```sh
make init                  # 仅创建开发 .env，已有文件拒绝覆盖
make test                  # 共用锁的全仓串行 race，再运行 Vue 单测
bash scripts/test-go.sh ./internal/platform/httpapi
bash scripts/test-e2e.sh
bash scripts/test-native-emby.sh
bash scripts/test-plugin-kernel.sh  # 仅原生 Linux AMD64，不能用模拟器冒充

go run ./cmd/media-tool-dev plugins check
go run ./cmd/media-tool-dev plugins assets --check
go run ./cmd/media-tool-dev plugins assets
go run ./cmd/media-tool-dev plugins matrix --check
```

- `.env.test` 设置 `TEST_DATABASE_URL`（数据库必须精确为 media_tool_test）和 `TEST_REDIS_URL`；测试默认强制 Redis DB15。缺专用 Redis URL 时仅从项目 `.env` 取 Redis 连接再重写为15，**不 source 生产配置、不复制应用主密钥/账号**。
- `test-lock` 持有同一个 `data/.isolated-test.lock`，所有 Go/E2E/native/kernel 的 DB 变更串行。`go test` 直接调用不等于完整隔离集成验收。
- runner 固定 `-p 1 -race -count=1 -timeout=20m`，只接受包路径，拒绝调用者覆写并行标志；原生 release worker 的 CGO=0、65532、cap-drop/no-new-privileges/readiness 门禁均保留。
- 工具/矩阵只从现有精确 Go module pin 读取元数据，不执行插件工厂。Node/Vue/Playwright 仍是前端工具链，不用 Go 伪造浏览器测试。

## 宿主运维

```sh
media-toolctl --root /root/docker/mediatool init-compose
media-toolctl --root /root/docker/mediatool image-check dingding229/mediatool:sha-<40hex>
media-toolctl --root /root/docker/mediatool update dingding229/mediatool@sha256:<64hex>
media-toolctl --root /root/docker/mediatool register dingding229/mediatool@sha256:<64hex>
media-toolctl --root /root/docker/mediatool execute <已审批票据UUID>
media-toolctl --root /root/docker/mediatool --archive /私有目录/owned.dump backup-verify
```

- `init-compose` 仅替换 CHANGE_ME 并原子设0600，不覆盖已初始化值；仍使用直接值，无 `.env` 插值。**初始化后的 Compose 不提交 Git**。
- 控制命令先验证三服务、当前 app/同项目 PG、immutable Hub 的 arch/revision/schema/digest，持宿主 `.update.lock`；只有 update/明确批准票据才切换，先留私有备份。
- 恢复/回滚保留原库及旧 Compose/备份；未知切换只留 uncertain 回执，不自动重放或有损降级；业务继续暂停直到管理员复核。
- 克隆备份验证不是普通回归测试：只在所有者明确运行时创建 `media_tool_restore_<12hex>`，只删除自己创建的同名克隆；不得把此工具、真实恢复或生产口令当 media_tool_test 的夹具。
- 新 runtime 附带 `/app/media-toolctl`。首次升级须在 Q6 从**已发布、验证的同一 Hub image**提取 ctl，再执行宿主操作；不能从未发布的本地编译替代 Hub 交付。已升级部署可 `docker compose cp app:/app/media-toolctl ./bin/media-toolctl` 后设置0700并安装到所有者 PATH。不得把 Docker socket 新挂到 app。
- 旧 `scripts/update-server.sh` 仅为以上 Go ctl 的薄包装，优先已有 `bin/media-toolctl`；源码工作区才回退 go run。

## 严格播放诊断

```sh
media-toolctl --root /root/docker/mediatool playback-check \
  --base-url https://<自有管理域名> --gateway-url https://<自有播放域名> \
  --username <管理员> --password-file /私有目录/password \
  --asset-id <已有资产UUID> --item-id <已有Emby条目ID> \
  --user-agent '<实际播放器UA>'
media-toolctl --root /root/docker/mediatool load-check --requests 500 --concurrency 16
```

播放检查从 authenticated preflight/资产 STRM/当前路径开始，保留云盘可读、解析目标、Emby 连接/用户权限映射、无效配置报告；禁止非回环明文传管理员凭据。原媒体用户 token/source 可从原私有 M0 参数配置取值；不要求提供方 Cookie/115 token 到诊断进程。非交互密码只能通过0600普通文件，交互输入关闭回显。

必须 stable resolver HEAD/GET 都302，CDN只带真实 UA/Range 且必须206；每个 CDN hop 做 public DNS 和 dial 地址约束，不转发管理员 Cookie/媒体 token，不接受 HLS/relay/transcode 成功口径。报告0600，只有固定检查结果，没有临时URL/密钥；仍不宣称真实播放器解码、seek、续播、音轨/字幕已验收。配置不足/验证失败退出2。

旧 HLS正向、双架构本地镜像“发布验收”已删除；旧 remote-session 的 server/.env/SQLite 包装属于已取代部署，改用 app 的 session stdin 诊断，不再自动抽取外部 Emby 认证库。

## API 映射

- **现有 HTTP route/method/Envelope、Emby DTO、错误码均保留**；未注册的旧资产列表 handler 和仅测试使用的指标包装不是公开 API，测试改走真实 collector/现有接口。
- ops preview 的 `command` 字符串改为 `media-toolctl --root ... execute UUID`，preview/approve/execute/verify 票据与状态语义不变。
- 已认证管理员读取的私有请求日志，plugin query 也保留原值/完整编码/参数顺序；原登录 body/Cookie/Authorization 不额外采集，恢复口令仍仅 startup stdout，不进入 web store。
- 没有增加远程模板/插件导入接口、source/commit/version UI、运行期编译或新的数据删除 API。

## Q3/Q4 增量API/CLI契约（schema71）

| 入口 | 新增/变化 | 安全语义 |
|---|---|---|
| GET /api/v1/downloads、/transfers | flow: phase/retryable/nextAction/reason | 原state/phase仍保留；只有对应failed job与安全阶段可重试 |
| POST /api/v1/pipeline/{kind}/{id}/retry | 成功data.flow、拒绝error.flow；404只指真实缺行，存储故障503 | 缺job不重建；未知上传/来源/输出确认不被通用重试绕过 |
| GET /api/v1/integrations | readiness.state/nextAction/reason | 原授权/配置保存不自动enable |
| GET /api/v1/readiness | 配置＋最近probe的CD2/115/STRM/Emby概览 | 无联网探测/播放证明、无临时直链/凭据输出 |
| GET /api/v1/plugins/{id}/capabilities | 当前声明/授权/运行能力分类 | unsupported、待授权与运行失败不混同；读取不启动worker |
| GET /api/v1/plugins/events/retention | 80% warning/满载blocked/回收后ready | journal七天ingestion窗口＋订阅ACK；source幂等身份不删 |
| POST /api/v1/plugins/{id}/events/replay | 过期游标410 + X-Event-Retired-Cursor | 不静默跳cursor；unknown仍须明确确认 |
| POST /api/v1/ops/requests/{id}/verify | confirm:true，返回维护单flow/verifiedAt | 核验版本/schema/保护摘要，不执行/解除维护 |
| media-tool ops verify-execution -id UUID | 与上一行相同的只读核验及收据 | 不是原来的ops verify（备份包核验） |

以上HTTP入口均要求管理员认证。分页现在统一拒绝重复/空值/坏编码/越界；各列表有意的pageSize/offset上限不变。journal只回收ACK且无未解决run引用、已到期的旧payload；unknown保留自身行，明确卸载实例不再压住ACK下界；源outbox在已派发通知之后仅压缩payload，小型幂等身份仍持久保留。发布前需接受7天物理回放窗口并检查blocked订阅，不能以清库消除容量告警。
