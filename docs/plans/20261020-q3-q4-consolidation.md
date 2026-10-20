# Q3＋Q4 数据库与流程整合（2026-10-20）

本轮只在本地提交，不推送、不部署、不访问生产。遵循根 AGENTS.md；所有数据库测试复用 `data/.isolated-test.lock`，仅任务临时 PostgreSQL 的 `media_tool_test` 与 Redis DB15。前端不改视觉；历史迁移 1–65 不动。

## Q3a 索引与约束

- 00066 为 goose 非事务迁移：六个 CONCURRENTLY 索引（jobs owner、service；manual job/owner；workflow pattern；完整 Bridge 来源谓词；state snapshot owner 唯一索引），移除 state_values 冗余单列及被完整 Bridge 覆盖的旧 files 部分索引。
- concurrent build 前先 DROP IF EXISTS：中断产生 INVALID 索引或未记录版本的已完成索引均可重跑，不以 IF NOT EXISTS 隐藏 INVALID。正常启动不会重建已记录版本。锁等待 5s、每语句 10min，迁移 CLI 总上下文 30min；失败保留旧数据，只需解决锁/负载后重跑。
- 00067：state_tasks revision、fingerprint、result JSON、snapshot/instance 所有权复合 FK；search account_ids 数组、subscription 三对象、download details 对象 CHECK。均 NOT VALID→VALIDATE，不修正异常历史数据。5s lock_timeout、10min statement_timeout，事务失败全回滚。
- 此组 schema 67（后续追加）；新索引有写放大和磁盘临时成本，CONCURRENTLY 不等于无锁，约束 ALTER 的短锁仍需升级维护窗口。跨版本回滚先下约束再下索引，不回写密文。
- 测试：合成 jobs/manual/runs/outbox 各 40,000 行；使用完整 owned-jobs、service inflight、workflow LIKE 和 Bridge 谓词，未关闭 seqscan。EXPLAIN ANALYZE (BUFFERS) 的单次暖库样本如下，仅验证计划，不宣称生产 SLA。

| 查询 | 65 毫秒 | 67 毫秒 | 实际采用新索引 |
|---|---:|---:|---|
| owned jobs | 48.859 | 6.543 | q_jobs_plugin_owner |
| manual ownership | 1.279 | 0.031 | q_manual_job_owner |
| service | 50.896 | 0.033 | q_jobs_plugin_service |
| workflow | 2.521 | 0.055 | q_runs_workflow |
| Bridge | 1.963 | 0.088 | q_outbox_bridge |

`migrations/q_integrity_test.go` 保存可重复的大样本及 EXPLAIN 测试；同时验证 65→67→65→67、构建中断重跑、跨实例 FK 拒绝、非法历史行中止且原值不变、JSON 非对象/非数组拒绝。生产计划需在 Q6 备份后观察，未运行生产 EXPLAIN。

## Q3b 空遗留结构退役

- 00068 在同一事务按固定顺序 ACCESS EXCLUSIVE 锁住十个待退役表及三个受影响表，随后断言每张表为空、两个引用列全 NULL；任一失败 RAISE，绝不 DELETE/TRUNCATE/CASCADE。5s 锁等待、60s 语句超时。
- 删除六个 WASM 归档表、site_accounts、notification_channels、bootstrap_imports、media_user_legacy_grants；删除 notification_deliveries.channel_id、download_tasks.site_account_id、site_definitions.local_overrides。`common_definition`、`source_*` 不动。
- **local_overrides 与“全 NULL”文字的差异**：历史字段实际 NOT NULL DEFAULT `{}`；本实现只允许空对象 `{}`，JSON null/数组/非空对象均中止。未查询生产这个字段，若生产有覆盖配置升级会安全拒绝，必须由所有者决定迁移配置，不能手工清掉来过门禁。
- 删除运行期 legacy grant 展开桥和旧枚举、rekey 旧密文入口、backupverify 的旧 WASM 改名兼容循环；backupverify 保留完整备份清单，只在升级对比中精确投影已批准的空对象移除（非空表仍拒绝）。旧迁移与历史结构测试继续保留，不改65以前的历史。
- 测试覆盖非空 bootstrap/WASM/delivery/repository/site/channel/grant、非空覆盖配置、写者持锁中断/版本不前进/重跑、当前密文逐字节及 AAD/115凭据/STRM稳定身份不变、全量rekey dry-run与故障回滚。旧WASM rekey用例替换为当前plugin call args/result的双列AAD用例，旧数据保护由迁移拒绝用例承担。
- schema 68；不可通过 schema-only Down 回滚。Q6 必须先停写、备份完整 PG/应用主密钥/Compose与STRM配置、验证恢复克隆；失败或回滚用升级前完整数据库＋对应二进制，不新建密钥、不复用升级后库运行旧程序。本阶段没有触碰生产或生成生产备份。

## Q4a journal / outbox 保留与维护故障域

- 00069 增加 source 的逐事件 cursor receipt、payload compact 标记、journal ingestion 时间与持久 retired cursor；旧 journal 按 event UUID 回填。事务 5s/10min。00070 使用独立非事务 CONCURRENTLY 建 pending-source 与 event FK 访问索引，移除已被 pending-source 索引覆盖的 Bridge 全来源索引；5s/10min，可重跑；最终 schema 70。
- GC 与 Bridge、显式 Replay 共享 advisory lock 7063001；只回收存续订阅已 ACK、无引用 run、且**入 journal 满7天**的payload，每轮最多1000；不越过未到期的ingestion边界。未ACK/disabled存续订阅仍保持cursor安全；pending/running/unknown保留自身journal行和收据，不让单个unknown洞阻塞其它可回收行。明确卸载的实例不再参与ACK下界，配置/KV及unknown记录保留。不会擅自跳订阅cursor或清未知结果。原业务时间仍用于 SDK replayWindow 校验，迟提交不会被原时间/事务UUID高水位漏掉。
- outbox 不删除行：业务 EmitTx 使用 event_type/aggregate_id 保证幂等，删源会重投。源只有在 journal 已退役且通知已 dispatched 后才把 payload 压为 `{}`，保留 id/type/aggregate/cursor 小收据；待发送通知及非Bridge来源（包括 plugin.notice）不压缩。**小收据行数仍增长，这是防重放的有意代价，非宣称数据库零增长。**
- 物理事件回放窗口明确为7天；SDK replayWindow=0 仍是不额外限时的语义，但不代表无限存储。过期游标/超期仍被未知任务钉住的历史，返回 HTTP410 `event_retention_expired` 与 `X-Event-Retired-Cursor`，不会返回“成功0项”或自动前进。需长期回放者必须在发布前接受此保留协议；不提供自动重放恢复。
- `GET /api/v1/plugins/events/retention`（管理员鉴权）返回 stored/limit/retiredCursor/retentionSeconds/state/nextAction；80% warning、100% blocked、回收后自动恢复 ready，不用手动“清除告警”。服务端告警可恢复，不误把容量满当worker损坏。
- state recovery、run recovery、GC、Bridge、Deliver、Schedule 独立执行并聚合带域名错误，各有5s预算，不再让journal Busy中止服务调度。supervisor 不再给所有域共用一个5s截止点；单个域故障不会耗光其它域的预算。
- 测试：50轮/10000事件的持续摄入→GC→重新Bridge、source幂等身份保留、30天旧时间的迟提交、100000满载下Schedule继续、100轮有界GC容量自动恢复、unknown与通知未发送保护、迁移回填与410过期拒绝。上述为合成协议长跑，不冒充7天真实墙钟 soak。

## Q4b 故障分类、链路状态与分页

- 共享 `flow.Status` 只描述观测：phase/retryable/nextAction/reason，不替代授权或状态机。downloads 列表与 transfers 列表新增 `flow` 对象，保留旧 state/phase 字段；下载阶段覆盖 download→organize→STRM→emby_refresh→emby_verify→complete。
- 重试接口与列表共用阶段判定，retryable 必须有**原有且 failed 的对应 job**；缺 job 返回 job_missing/inspect_task，不创建平行任务，不靠“重试搜索/订阅”补缺口。下载来源需确认、输出布局需确认、上传starting/uploading/uncertain（即使有session ID）、暂停/取消/已完成均不允许普通重试，返回对应下一步。
- retry 查询与上传session查询区分 ErrNoRows=404、存储故障=503；列表读取中断不返回“成功半份列表”。提交回执未知返回 verify/commit_outcome_unknown，不宣称失败未执行，不建议盲目重试。
- 保持原有错误信封，增量增加 `error.flow`；浏览器 APIError 保留它且不自动重试。下载页仍使用原有按钮/布局，只根据flow禁用无效重试、显示原因title与在途防双击；无CSS、颜色、布局改动。
- 三种分页共同拒绝重复、空值、非整数、越界和坏URL编码，返回400；保留各自有意的上限（通用100、插件/管理200及各自offset上限）。移除legacy first-value/default分支，不用重复的平行解析器。
- 数据库JSON CHECK使旧“写数组再验证读取错误”夹具不再能写入；测试增强为先断言数组被拒绝，再以对象内错误字段类型验证原字节不被失败报告覆盖，没有删除/放宽原防覆盖断言。
- 新增实际HTTP handler集成回归：无任务404、缺job409、有failed job一次入队/重复拒绝、已知session的unknown不被重放；加纯函数状态矩阵、分页跨入口与APIError传递测试。

## Q4c readiness / capability / ops阶段

- 连接列表增量 `readiness={state,nextAction,reason}`：区分缺官方115授权、已授权但待管理员启用、缺Emby key、未探测/过期探测、探测失败、24h内当前配置探测成功。保存配置会清除旧probe；不主动联网、不自动启用115或插件。
- 新增管理员 `GET /api/v1/readiness`，汇总 open115/CD2/Emby/STRM 的配置与最近probe口径。STRM仅报告stable resolver配置，nextAction=verify_player_302；不把配置齐备说成真实播放器已验收，不输出凭据或临时URL。
- 新增管理员 `GET /api/v1/plugins/{id}/capabilities` 与前端typed入口；使用同pin manifest声明、当前fingerprint grants、generation/lease/snapshot及run状态，区分unsupported / authorization_required / disabled / starting / runtime_failed / ready。读取停止的实例不会伪造空能力列表、创建run或启动worker；未修改插件仓库及pin。
- 维护单JSON统一附加 `flow`；preview→approve→execute→verify→complete，approved不是已执行，executing/failed不建议重放。owner完成收据保留state=completed兼容字段，但phase=verify、业务仍暂停。维护单列表合并为一个查询＋共享scan，删除原N+1平行读取。
- 00071新增 `ops_request_verifications` 小表及FK/摘要约束（5s/60s事务），不把旧completed单自动认证为verified。新增明确确认的 `POST /api/v1/ops/requests/{id}/verify`、`media-tool ops verify-execution -id <UUID>`；核对当前binary revision、schema、原批准摘要和保护数据，写核验收据与审计。不会恢复、回滚或解除维护。
- `Resume`在锁定当前维护窗口下重新核对该维护单收据及当前保护数据；核验后发生保护数据变动会拒绝旧proof，不能用先核验再改数据绕过。前端原有维护单操作区添加明确核验动作/确认文案，completed显示“执行完成（待核验）”，不改样式/布局/CSS。
- 收尾补强Q4a：journal容量进入独立ops监控规则（80%阈值，恢复自动发recovered，指标不可读不阻断其他采样）；journal/run GC只有订阅已ACK才可删终态幂等run。新增“效果成功但cursor更新丢失8天后再Deliver”回归，防止GC删去dedupe后重放；以及明确卸载＋unknown洞不能压住其它可回收payload的回归。retired_cursor是已超期历史的退役边界，保留的unknown洞可能低于此边界，但不变成可自动回放的历史。
- 最终schema **71**，1–65历史原文未改。新增ops核验、readiness、capability、真实handler、Vue确认动作及无副作用测试；真实Linux worker/owned server不在本机ARM64冒充通过。

## 进一步精简候选（Q5前交审核；不是已删除收益）

| 优先级 / 模块 | 候选及证据 | 预计非测试净减 | 风险 / 保留边界 |
|---|---|---:|---|
| P1 acquisition/httpapi | download列表的retry-key SQL CASE与flow映射可改为同一typed阶段目录，pipeline与control错误边界继续统一 | 50–90行 | 中；必须保留未知上传、source/output确认和缺job三种不同恢复路径 |
| P1 plugins/v2 hooks | acquisition/discovery/organizing的Submit→wait→typed receipt校验重复外壳（各`*_hooks.go`） | 150–230行 | 高；链式policy hook与普通read/admission不能共享“失败重试”语义，先测试后抽薄外壳 |
| P1 events/outbox | Bridge事件白名单/小payload CASE与事件map验证，提取当前Go typed catalog，增加SQL谓词一致性测试 | 60–100行 | 中高；历史迁移不重写，不丢事件原始发生时间/实例路由/隐私边界 |
| P2 maintenance/backupverify | 两套inventory/hash/rows扫描外壳与保护投影工具可抽公共只读层 | 80–130行 | 中；完整备份清单、授权迁移投影、保护摘要三口径不可混为“忽略差异” |
| P2 nativelibrary/catalog | 目录sync及媒体投影查询的transaction/scan重复封装，避免重复权限fence与空结果处理 | 90–140行 | 中高；浏览可见性、cache MVCC fence和未授权空结果不能降级成通用缓存 |
| P2 integrations/httpapi | 保存前probe与显式test的结果/错误呈现共享小adapter；连接readiness与UI nextAction文案集中 | 30–60行 | 低中；授权成功≠enabled，probe≠真实播放验收 |
| P3 operator/diagnostics | 已分层CLI内同类JSON/结果输出和窄范围cleanup重复外壳 | 40–70行 | 低中；严格302、Hub immutable和unknown收据不能为了统一而降断言 |

合计约 **500–820 非测试行**，只计可验证的手写外壳；不把生成的CD2协议、历史迁移、受信SDK合约或不同鉴权域误当死码。Q3/Q4已实际删掉legacy grant桥、旧rekey改名路径、分页legacy分支、ops N+1，新增安全协议/迁移及测试的行数单独据git统计，不声称所有业务代码净减。

## 质量门边界与待审核事项

1. **严格全量race门暂不能宣称通过**：现存 `TestRealPostgresBackupDrillAndPermanentKeyProof` 会创建 `media_tool_restore_*` 数据库（`internal/modules/maintenance/maintenance_integration_test.go` 调用真实 Drill/PrepareCandidate）。本阶段明确只允许 `media_tool_test`，因此没有执行该夹具、没有创建任何克隆库；允许范围的全仓race在同一test-lock下仅显式排除这一项。不是修改断言、不是用mock结果声称真实恢复通过。Q5需重构为符合库名边界的独立隔离恢复方案，或所有者明确批准仅临时集群克隆库例外。
2. 本机darwin/arm64不能通过原生Linux AMD64 worker/kernel门；保留原始平台skip并实际执行脚本得到拒绝，cross-build/errcheck不冒充原生运行。发布前仍须官方AMD64 Actions及 owned-server 验证。
3. 首轮全仓race暴露旧插件business fixture仍复制已退役grant表、旧枚举普查仍把历史CHECK算入现行Go enum、历史20→22测试对当前public执行Down三个问题。已补齐fixture、独立断言历史enum不变、把20→22→20→22放到`media_tool_test`内独占schema；没有删掉历史密文/身份保护断言。新snapshot/rekey/restore回归直接升级到71，使用真实复合FK而非LIKE复制表的无FK夹具。
4. E2E的临时官方PG未显式设置POSTGRES_USER时，测试SQL执行器按官方默认postgres用户名回退；只在测试库执行既有SQL，保持ON_ERROR_STOP。没有读取业务服务凭据，没有用放松鉴权/密码长度来过浏览器测试。
5. 生产D1的空表证据来自所有者，本轮未重新连接生产；local_overrides、JSON形状和state_tasks历史合法性仍须发布前只读预检。67/68/69的ALTER与断言事务会持有锁，应停写维护窗口升级；66/70的CONCURRENTLY也需预留磁盘/IO。不得删数据来迁就中止。
6. 7天物理回放窗口、小收据永久保留、unknown自身行/收据保留、未ACK存续订阅钉住历史是本次协议的明确取舍，需审核接受。回收/恢复告警不等于业务重放，更不等于真实115/CD2/Emby播放验收。

## 最终门禁记录（清理前生成的同pin UI包）

| 门禁 | 结果与耗时（wall秒） |
|---|---|
| Go build / vet | PASS；7.17 / 1.46 |
| errcheck（Darwin / Linux AMD64） | PASS；0.66 / 0.57；ignoretests沿用基线，不增加ignore规则 |
| race合法隔离范围 | 最终各包87/87通过；1102顶层PASS、15原有SKIP、无race告警。全仓运行637.21s时仅历史迁移夹具尾部仍误调旧goose全局入口失败；修复后tests包2.571s复跑通过；journal洞补强后受影响v2/acquisition/messaging/httpapi/maintenance分别37.430/10.826/3.351/15.633/3.297s复跑，最终GC边界6.290s复跑。**不是某次完整命令一次exit0，也不标严格全量绿灯** |
| Vue / vue-tsc＋Vite | PASS，47文件159项；3.57 / 10.29。维护确认mock类型错误已修复，最终生产UI包真实重建通过后再跑E2E |
| E2E（单worker、同test-lock） | PASS 122、SKIP 1（原生Linux worker）；168.67 |
| Linux AMD64 CGO=0 cross-build | PASS；5.77；不是原生执行 |
| govulncheck（已有v1.8.0） | 0可达、0导入包漏洞；2.95。require-only GO-2026-5932，x/crypto@v0.57.0 openpgp，Fixed=N/A，不在调用图；go.mod/go.sum不变 |
| plugin check/assets/matrix | PASS；同pin7定义、8资源；schema71 |
| 跨库真实恢复 / 原生kernel门 | **BLOCKED**：前者1项因本阶段库名边界未执行；后者脚本实际拒绝darwin/arm64（0.01s）。不以放宽断言、模拟器或本地镜像验收替代 |

全部数据库写测试均共享原仓库锁，仅任务临时PG的media_tool_test、Redis DB15。结束前检查schema71且额外非系统数据库数为0。未访问生产数据库、未连接服务器、未push或触发Actions/Hub。UI仅交互/文案，历史迁移1–65、根私有Compose、go.mod/go.sum、插件pin均未改。

### 每组改动预算

统计为非测试代码新增＋删除（SQL/CLI/shell/生成标识计入；文档另列，测试文件不计入1200预算）：Q3a +65/-3；Q3b +89/-87；Q4a +209/-24；Q4b +212/-78；Q4c当前 +515/-60，每组均小于1200。Q4c含前组联动收尾与fixture补齐，未重写已接受的Q1/Q2提交。

## 任务残留清理

- 已删除本任务专用GOCACHE、GOTMPDIR/TMPDIR、离线工具二进制、测试原始日志、生成webui和E2E截图/结果。清理前du分配合计 **6,068,732 KiB（约5.79 GiB）**；对应目录已删除，webui仅保留原跟踪的.gitkeep。后续测试请先build-web/make test重建同pin UI产物。
- 仅按创建时精确ID且校验media-tool.task=q3q4-20261020标签，停止并等待自动移除两个临时PG/Redis容器及其匿名测试卷；PG停止前最近一次du约0.94 GiB。此数单列，不当作宿主立即回收的可用空间。共享镜像未删除。
- 没有执行go clean -modcache、全局docker prune；保留模块缓存、既有node_modules、原app/postgres/redis服务、业务卷、外部Emby/CD2和近期回滚备份。共享test-lock文件及原私有配置保留。
