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
