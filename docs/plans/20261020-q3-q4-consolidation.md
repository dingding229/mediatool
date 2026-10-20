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
