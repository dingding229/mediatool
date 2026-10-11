# 输出归档管理与明确批准的清理

日期：2026-10-10；源码迁移20。是否已发布，以本阶段部署与验收报告为准。

## 范围与默认策略

管理入口位于“云盘索引 → 旧输出归档”。只列出输出迁移产生、`operation=archive`且已完成的托管归档，不扫描任意旧文件。默认保留30天，可修改为1–3650天；编辑必须带当前revision，过期**不会自动删除**，没有清理Worker或后台破坏性重放任务。

- 只清理当前配置STRM输出根目录中的精确`.media-tool-retired/<archiveId>/<原路径>.retired`。
- 归档须仍属于同一资产的asset作用域，摘要匹配、资产输出ready，没有待恢复写入/归档；当前STRM实际内容还须匹配托管清单。
- 原云端媒体、下载/做种源文件、当前STRM/NFO/字幕、共享tvshow.nfo和目录均不在清理范围内。
- 缺失当前STRM或手工改动会保留归档；更改保留期、资产状态或归档内容后须重新核对。
- 已删除归档的记录保留，`archive_deleted_at`单独记录清理时间，不改变原归档完成日期。

## 明确确认流程

1. 显式选择1–20条已过保留期的归档，点击“预览清理所选”。预览使用回滚事务，只读检查文件/归属/保留期，返回计划摘要与相对路径，不返回STRM正文、文件Key或115Token。
2. 查看物理删除警告和逐项计划。取消/关闭弹窗不批准清理。只有输入“清理归档”后，界面的确认按钮才可使用。
3. POST确认携带预览planDigest、相同归档集合及新requestId。后端重新生成计划，批准请求和逐项操作先持久化，然后仅在这次明确请求内执行。API调用本身即为管理员的显式批准，中文输入是额外的GUI交互保护，并非密码或API授权。
4. 每项分别检查结果；批量允许部分完成，不能把“请求成功”当成所有文件已删除。
5. 请求超时或响应不完整时，保留同一requestId查询结果，不自动换ID/重复删除。相同请求ID/计划可幂等查询；同一ID不能用于另一计划。大小写不同的UUID按相同身份串行处理。
6. 中断/冲突的操作只有点击“继续已批准清理”并再次明确确认才继续。改变保留期之后，已有批准操作重试也受当前保留期限制。

公网HTTP部署没有依赖安全上下文限定的randomUUID；请求ID使用getRandomValues生成UUIDv4，不使用Math.random。HTTPS仍为独立的安全部署验收项。

## 持久化阶段与恢复边界

`approved → quarantined → removed`，失败时记录conflict与非敏感原因。

- 先使用原子no-replace rename，把归档移入同目录的`.cleanup-<operationId>.retired`；碰撞不覆盖，移动后再次核对内容并同步目录。
- **quarantined写入DB并提交之后**才能unlink；请求取消时停在这一阶段，后台不会自动继续。
- rename成功、阶段提交前中断：明确重试仅在原路径缺失且精确隔离路径摘要匹配时认领移动结果。
- 只有已经持久化quarantined，缺失隔离文件才可核对为unlink-before-commit并补齐删除记录。approved阶段两处文件均缺失不是“清理成功”。
- 原归档被重建、隔离文件改动/符号链接、当前输出损坏或待恢复均停止，不删除/覆盖它们。
- os.Root约束路径，最终文件要求常规文件并核对身份/摘要。PG锁串行化本程序的同路径操作；**不声称能抵御拥有同目录写权限的恶意进程在最终核对与unlink之间持续替换文件**。应限制输出根目录写权限，清理时避免外部程序修改托管归档；root攻击者和断电硬件持久化保证不在本轮证明范围内。

## API

所有接口使用管理员会话Cookie，写请求还需同源和`X-Requested-With: media-tool`。Body只允许已定义字段，最多16KiB。

| 接口 | 用途 |
|---|---|
| GET /api/v1/library/archives?page=1&pageSize=50 | 分页归档清单，pageSize 1–100，拒绝重复/空分页参数 |
| GET /api/v1/library/archive-policy | 当前保留期与revision，automaticCleanup始终false |
| POST /api/v1/library/archive-policy | revision保护的保留期编辑，不启用自动删除 |
| POST /api/v1/library/archives/cleanup-preview | archiveIds数组，1–20个不重复UUID，只读预览 |
| POST /api/v1/library/archives/cleanup-confirm | requestId、archiveIds、planDigest，明确批准并尝试执行 |
| GET /api/v1/library/archive-cleanups/{requestId} | 查询同一批准请求的逐项结果；未知请求返回空数组 |
| POST /api/v1/library/archive-cleanups/{cleanupOperationId}/retry | Body为{}，仅继续已有批准的单项操作 |

查询的ID是**请求ID**，重试的ID是**逐项操作ID**，不混用。401表示会话无效，403表示CSRF拒绝，400表示JSON/分页/路径ID格式错误，409表示策略/预览/批准/清理冲突，503表示依赖不可用。预览选择验证失败也返回409，需用户修改选择再预览。

## 验收与真实数据约束

隔离PG和临时目录覆盖：预览不删、默认30天/禁自动、修订/摘要过时、重复/超量选择、UUID规范化、批准回放只查询、rename-before-commit、unlink-before-commit、未持久化隔离时丢失、当前输出缺失/手工修改、保留期变更、待恢复资产/日志、明确重试、审计保留与原活动输出不变。HTTP测试覆盖全部接口会话与CSRF、格式/分页边界。Vue测试与E2E覆盖无randomUUID的HTTP环境、输入确认、取消不提交、完成后列表刷新及375px弹窗。

真实部署不创建示例归档、不缩短真实保留期、不调用确认/重试接口，不删用户文件；发布后仅做真实列表/策略读取及现有播放链路的只读回归。临时测试目录中的物理清理不是用户真实资产清理验收，也不是PT业务端到端验收。
