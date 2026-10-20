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
- GC 与 Bridge、显式 Replay 共享 advisory lock 7063001；只回收所有订阅已 ACK、无引用 run、且**入 journal 满7天**的连续前缀，每轮最多1000。未 ACK/disabled 订阅、pending/running/unknown 都会钉住，不擅自跳 cursor、不清未知结果。原业务时间仍用于 SDK replayWindow 校验，迟提交不会被原时间/事务UUID高水位漏掉。
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
