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

## Q5.5：完整接口契约、前端流程文档与追加安全发现

- 管理端真实注册259个pattern；原生NewServer真实61条注册。OpenAPI3.1/JSON兼容YAML1.2：291个paths、625个标准path/method operation（管理467、原生158；包括GET隐含HEAD、methodless标准方法与原生OPTIONS）。这是展开口径，不是625项独立业务功能。外部Emby动态协议/插件descriptor仅定义挂载边界，未声称穷举运行中的任意动态路由。
- 每operation具有唯一operationId、按业务域tags、安全scheme、源注册/handler位置、参数/请求体和实际wire响应。230个schema；共用具体错误/响应components，移除通用OK。覆盖nullable、结构体/匿名map投影、custom MarshalJSON、RawMessage/动态JSON、二进制图片/tar/STRM、SSE、302Location、状态/flow/readiness/capabilities。原生PascalCase/独立listener、不支持WebSocket/嵌入字幕、旧prefix501/404单列；不会把动态JSON伪装为固定成功DTO。
- API生成器采用Go AST+go/types导出信息，不添加运行依赖。管理端构造器仅抽出routeRegistrar以便**运行时**recording ServeMux独立枚举，原生测试直接读取真实NewServer.router.routes；与生成器的静态路径展开双向比较均缺失0/多余0，避免生成物和生成器互相证明。14个关键DTO做反射＋真实JSON序列化字段比较，包含维护单额外flow、原生UTC/非null Array；另校验schema/response引用、path参数、鉴权scheme及strict302边界。
- 运行时对照确实抓出静态提取最初漏掉的常量STRM endpoint：从258修正为259；不是把缺失项排除测试。初轮Q5.5门也曾因测试源语法错误在vet中止，修复后必须从完整命令首项重跑，不计为通过。
- **新增确认的HEAD/CSRF缺陷**：GET与PUT共用networkProxy handler，旧代码只判断GET；ServeMux让HEAD匹配GET注册，而protected按HEAD只读跳过CSRF，带JSON body的已认证HEAD因而进入保存分支。真实合成库负向测试先失败（revision/代理配置改变），再将HEAD明确纳入读取分支；相同恶意Origin、无CSRF的HEAD只读200、PUT403，revision/密文不变。没有扩大CSRF例外或更改视觉。同时清理config两条仍指向已删除init-env.py的运行时报错提示，统一为media-tool-dev init-env；没有恢复Python入口。
- 新增中文docs/FRONTEND_API.md按12组流程说明顺序、状态机、确认/重试/unknown、Cookie/CSRF、错误与脱敏示例；完整使用清单见FRONTEND_API_ROUTES.md。TypeScript AST静态发现156个直接调用、24个经适配调用、3个资源/链接，3个仅适配层、439个未检出。后者包括HEAD、原生/外部客户端、后端返回资源URL和动态插件，**不是死码证明**，不得据此删除现行功能。
- R4只读SQL已用合成schema65+一条真实state task验证：合法通过，坏revision/非空遗留表RAISE，中止后原数据/版本保持不变；SQL只读执行，不对目标做自动修复。当前schema仍71，没有增加迁移或生产访问。
- 新增scripts/test-release.sh作为可复现的一条fail-fast锁内完整门禁；不安装工具到go.mod。最终各命令结果/耗时与清理量在下节登记。插件pin/站点pin/go.mod/go.sum、根私有Compose及业务卷未改。

## 安全证据索引（完整门禁覆盖，不以静态阅读代替执行）

| 边界 | 新增/保留证据 |
|---|---|
| S1 proxy DNS/SSRF | networkproxy/pinned_test.go：HTTP CONNECT目标IP/Host与秘密隔离、混合/重绑定DNS、拒绝无直连fallback、SOCKS5/5h IP ATYP、头上限与取消；Policy地址校验 |
| S2 原始302 | playback/load_test.go、playback_test.go、emby/direct_only_test.go、tests/m0_protocol_integration_test.go：并发302/零中转、非DirectOnly拒绝、false配置不能重开HLS/媒体fallback、provider headers拒绝 |
| 鉴权/CSRF/session | httpapi/account_auth_integration_test.go、api_test.go；新增head_safety_integration_test.go证明HEAD不走PUT且恶意来源PUT403 |
| S5 路径/编码 | strm/strm_test.go：root sibling、symlink、单次编码/解码、literal percent文件名；文件/原生router现有负向测试 |
| SQL/查询 | dataview/inspect_integration_test.go拒绝table注入/系统表；migrations/native_movie_integration_test.go拒绝SortBy注入；acquisition/flow_test.go拒绝恶意state；operator/swap_plan_test.go验证标识符引用及命名约束 |
| S4 state所有权 | migrations中真实复合FK/constraints验证与state snapshot兼容/AAD/rekey；不把仅LIKE结构的HTTP夹具当真实FK证据 |
| worker隔离 | 环境清理/manifest/secret边界本地套件通过；原生Linux UID/RSS/seccomp/实际worker链路按R2仍由Q6 Actions执行，Mac跳过不算通过 |
| 中断/恢复 | maintenance同库pg_dump/重建public/pg_restore/幂等、snapshot准备态恢复、迟提交/cache fence、outbox/journal长跑与满载现有断言；真实跨库rename不是合成测试通过口径 |

## 精简与行数口径

按git numstat，测试与文档/规范独立，不将OpenAPI/中文文档当业务代码。Q5a非测试130增/145删，净−15；测试181增/45删，净+136。Q5b非测试193增/165删，净+28（其中新增137行代理IP pin安全实现）；测试360增/179删，净+181。Q5c业务文案1增/1删，净0；测试10增/9删，净+1。Q5.5另计运行时路由可观测的小接口、HEAD修复与完整测试脚本，规范/契约测试独立统计。

R5已落实四组共享能力：阶段映射与SQL CASE同源、backupverify/maintenance只读摘要外壳、integrations probe保存回执/回读、operator诊断请求构造；并非以大量删行替代保留功能。maintenance/backupverify扫描两处净−58、operator诊断净−6；其它状态/未知结果/安全边界增加抵消了部分减少。禁止relay旧支路移除带来engine净−102，但代理修复、所有者交换SQL和回执检查有新增成本；不虚报整个阶段是大幅净减。未触及插件hooks、events/outbox或nativelibrary/catalog的进一步合并。

Q5.5非测试44增/9删，净+35（24行可重复完整门脚本、10行构造器抽取及HEAD等小修）；测试2038增/0删，净+2038。四组累计非测试368增/320删，**净+48**；测试2589增/233删，**净+2356**。规范/文档单列，不计上述代码；每组非测试改动量均远低于1200行。不能将新增契约测试或去掉旧通用OpenAPI OK的行数当业务精简收益。

## 最终冻结树门禁（Q5.5全部代码/规范改动之后）

完整命令（已有工具在任务目录，不向go.mod添加依赖）：

```sh
ERRCHECK_BIN="$TASKROOT/tools/errcheck" \
GOVULNCHECK_BIN="$TASKROOT/tools/govulncheck" \
bash scripts/test-release.sh
```

scripts/test-common.sh默认取得原data/.isolated-test.lock并验证media_tool_test/Redis DB15；脚本set -euo pipefail，依次运行下表，未排除恢复/迁移/集成/E2E。最终日志同时出现COMPLETE_GATE_EXIT_0、complete-command-exit=0。最后一次从build-web到govulncheck在同一条命令完成；不是局部门禁拼接。下表为最终冻结树实测，不用较早的Q5c/第一次Q5.5完整门代替。

| 门 | 最终结果 | 秒 |
|---|---|---:|
| build-web（vue-tsc、Vite、同pin资源） | 通过 | 16.07 |
| go build ./... | 通过 | 4.12 |
| go vet ./... | 通过 | 1.59 |
| scripts/test-go.sh（全仓-p1 -race -count=1，含集成/恢复） | 88包通过；2368个PASS含子例 | 667.21 |
| errcheck -ignoretests ./cmd/... ./internal/... ./migrations/... ./tests/... | 通过，沿用原门禁范围 | 1.18 |
| npm test --prefix web | 47文件159测试通过 | 4.06 |
| plugins check | 通过 | 0.26 |
| plugins assets --check | 通过 | 0.15 |
| plugins matrix --check | 通过 | 0.16 |
| scripts/test-e2e.sh | 122通过、1 Linux专用skip | 173.74 |
| govulncheck ./... | 可达0/导入包0，require-only 1（依赖报告已列） | 4.10 |
| 各步骤计时合计 | 单命令exit0，不含初始化开销 | 872.64 |

- Go有21条SKIP记录（含子例、15个顶层）：Linux实际worker/资源用例依R2留Q6 Actions；另外2个顶层是所有者真实Emby导出夹具/子进程helper的非独立入口，不能拿真实生产数据补跑。不是88包全部用例零跳过，Mac没有证明原生Linux门通过。
- 最终全仓中的B2实测：10000影片首Redis miss492.75ms、冷repository命中103.80ms、3 SQL；测试7.85s完成并保留迟提交/直接SQL/权限失效断言。临时PG autovacuum关闭只隔离背景噪声，不能把本轮quiet结果扩展成生产繁忙时的200ms保证；初轮2/5保守miss记录不删除。
- R1真实同库恢复、R4只读SQL、HEAD回归、管理/原生运行时路由对照均包含在这一次全仓race。go.mod/go.sum、插件/站点pin与生产配置未改；尚未push、触发Actions、访问生产、发布Hub或部署。

## 结束清理收据

- 测试与工具进程结束后，核对任务标签/精确ID/autoRemove，仅停止并自动移除本次2个测试容器；1个本次匿名卷自动回收，未手动删除任何业务卷。
- 按任务根与本项目生成目录清理：独占GOCACHE/TMPDIR/GOTMPDIR、工具二进制/草稿/私有合成测试日志、三份已知/tmp编辑脚本/位置指针、webui构建输出（保留.gitkeep）、同pin导出资源/ESM与E2E产物。释放3795480 KiB（约3.62 GiB，以路径du分配量前后差计；不把Docker VM层字节重复叠加）。
- modcache、共享依赖/node_modules、原data/.isolated-test.lock、业务配置/卷、真实媒体/STRM/授权和回滚备份均保留；未执行全局prune、go clean -modcache或生产清理。
