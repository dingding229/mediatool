# Q5＋Q5.5 安全、精简与前端接口契约执行记录

2026-10-20；基线14fdcab/schema71；按Q5a→Q5b→Q5c→Q5.5本地提交，不push、不部署、不访问生产。先读AGENTS.md；DB写入/恢复/性能/E2E共用原test-lock，仅media_tool_test和Redis DB15。Go使用任务独占cache/tmp，不清modcache。R2原生Linux AMD64门留给已有Actions，不在Mac重复尝试。

## Q5a：低中风险精简与恢复夹具

- acquisition下载阶段成为同一私有typed目录，同时生成retry key、flow phase与固定SQL CASE；HTTP失败job指引复用同一分类，paused/succeeded/missing不再被平行分支统一误称active。SQL片段只来自编译期目录，不接收请求字符串。
- maintenance与backupverify共用只读列清单/摘要扫描，保留完整与授权投影两种比较语义；补齐Rows.Err检查，避免扫描中断成为成功的半份清单。clone/drill与同库恢复夹具共用恢复后表摘要/密文/AAD/迁移幂等验证器。
- integrations统一revision-fenced probe记录与保存后回读；配置已保存但回执/回读失败，明确返回validation_record_failed，不鼓励重复保存。operator诊断复用请求构造，不合并HTTP与CDN的不同信任边界。
- R1：真实pg_dump合成数据→在共享锁中重建media_tool_test的public schema→真实pg_restore→完整摘要/永久凭据密文与AAD/STRM原文/二次迁移校验。没有创建额外数据库，也没有假造drill成功；生产仍保留克隆库能力。另发现原有数据库rename测试也会建额外库，已改为同库schema事务交换＋生产DATABASE交换SQL纯契约，真实跨库rename执行不是该测试的通过口径。
- 专用测试角色为DB owner但无SUPERUSER/CREATEDB/CREATEROLE，进一步防止误创建额外库；原生产角色/服务未改。
- B2保留原200ms、最多3 SQL、冷进程快于首miss及权限/直接SQL/迟提交失效断言。初始固定5轮有2轮保护性cache miss（9 SQL、648/919ms），不抹去失败；增加仅记录Redis命令名/键摘要/错误与MVCC的诊断后3/3通过。仅本任务临时PG关闭后台autovacuum并等现有worker结束后，5/5通过：冷命中124.9–164.2ms、均3 SQL，首miss614–826ms。未修改生产catalog/fence、未隐藏内容查询预热、未放宽门限；不能据此承诺有其它写入/后台维护时的命中率，初轮缺少完整诊断不能唯一归因。
- 组件race：maintenance/backupverify/acquisition/integrations/operator/httpapi全通过；原生catalog大模块没有做精简。Go/Vue模块边界、唯一三服务、115授权、STRM稳定resolver、插件pin/worker隔离不变。
