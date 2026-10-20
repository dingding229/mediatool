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

## Q5b：安全负向测试与修复

- S1确认并修复：插件host HTTP原先本地检查public DNS后，将原域名交给HTTP/SOCKS代理再次解析，存在分视图DNS绕过。现在直连复查并pin IP；HTTP/HTTPS代理CONNECT到验证过的IP，保留目标HTTP Host与TLS SNI；socks5/socks5h发送IP地址类型，不再发目标域名。代理密码只用于代理握手，不进入目标请求/插件结果。
- 新增恶意/分视图DNS、public+private混合、IPv4-mapped、NAT64、代理拒绝不直连降级、SOCKS5/5h IP握手、CONNECT头32KiB上限、握手取消测试；既有插件RPC的精确origin、无响应正文/秘密、302不跟随及权限scope断言保留。
- S2确认并修复：删除playback媒体字节中转及403重取重放分支；所有Serve都只能严格302或拒绝。旧ProxyFallback不生效，显式ForceProxy只能拒绝，旧环境开关不能开启relay；移除“启用STRM_PROXY_FALLBACK”提示，状态能力固定false。GET/HEAD之外在取链前405。
- Emby网关构造器不可变地锁定DirectPlayOnly，历史DB/环境false不能打开HLS/remux/转码/媒体fallback；后置授权失败不再重复调用上游身份验证。保留元数据代理/登录/图片能力。UI只禁用原有直链策略复选框，后端独立锁定，无视觉/CSS变更。
- 按已禁止功能退役4个relay正向用例及旧relay吞吐用例；替换为96次并发严格302、非DirectOnly来源零媒体读取、GET/HEAD/非法method与provider headers拒绝、Emby false策略不可重开媒体代理。旧“保留转码/原媒体fallback”断言改为明确409且上游零媒体；账号、权限、撤销、logout、STRM来源校验等负向断言未放宽。
- S5新增单次解码/编码穿越测试，保留字面percent文件名，不做危险的二次解码。S4真实复合FK/state跨实例、snapshot兼容/链接/可执行文件拒绝、准备态交换崩溃恢复，以及鉴权/CSRF/会话、worker环境等现有套件全部实际执行。Linux内核/资源隔离依R2留Actions。
- 组件race最终通过：playback/strm/networkproxy/auth/accounts/business/v2/httpapi/integrations及迁移state/ownership用例；针对旧断言/编译遗漏发现的失败均已修复后复跑，未把初轮失败计为通过。尚不声称Q5c全仓一次性门完成。

## Q5c：依赖复扫与一次性完整门

- 单次完整命令：任务专用media-tool-dev test-lock bash "$TASKROOT/gates-q5c.sh"；脚本set -euo pipefail，顺序执行下表全部命令。真实日志以COMPLETE_GATE_EXIT_0和complete-command-exit=0结束；没有跳过恢复夹具/集成/E2E，也没有用多次局部成功拼接门禁。MEDIA_TOOL_REQUIRE_FFMPEG=1，race默认-p1/-count=1/-timeout=20m。
- 耗时（秒）：build-web 12.13；go build ./... 4.79；go vet ./... 0.86；scripts/test-go.sh全仓race 653.99；errcheck 1.18；Vue 3.76（47文件159测试）；plugins check 0.28、assets --check 0.14、matrix --check 0.14；完整E2E 172.29（122通过，1个原有Linux专用case按平台跳过）；govulncheck 3.94（0可达/导入包漏洞）。原生Linux资源门仍是Q6 Actions阻断项，不能把Mac跳过算成通过。
- 全仓发现M0协议/E2E仍期待已禁止relay和可切换直链策略；更新为header-requiring来源409/零媒体读取、fresh302、Emby302、HLS409，以及直链checkbox必须checked+disabled。原鉴权、私有原值日志和媒体来源断言保留。没有CSS/视觉改动。
- go list -m -u -e -json all：138模块、0查询错误，完整版本表见20261020-q5-dependencies.md。未修改go.mod/go.sum，未升大版本或插件pin；require-only GO-2026-5932不在导入/可达包内且无已发布修复，按报告保留风险，不假称所有模块均无advisory。
- Q6只读预检SQL与发布步骤草案随附；当前没有生产查询、push、Actions触发或部署。Q5.5将以合成schema65夹具实际验证SQL的READ ONLY、断言中止与不改数据。
