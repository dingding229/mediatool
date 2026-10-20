# Q6 发布执行记录（进行中，未部署）

2026-10-20；本阶段已获所有者发布部署授权。先读AGENTS.md与Q6草案。除实际部署验证外，所有回归仍只用media_tool_test/Redis DB15与共享test-lock，不用生产夹具。

## 只读预检与原始推送

- 在仅短暂冻结app写者的窗口执行原docs/sql/20261020-q6-preflight.sql，READ ONLY=on、schema65、state_tasks=1、同实例snapshot匹配1；全部空表/NULL引用/local_overrides/JSON/状态断言通过，ROLLBACK后app已unpause并healthy。
- 预检私有目录：`/root/docker/mediatool/private/q6-update-20261020T061112Z/`。此时只含预检与运行配置收据，**不是完整升级备份**。未导出私有环境/运行日志到Git或Actions。
- 原始15个commit已从74ef5f507a031aca8fbfc96936c739ad3f25a0dc推送至a8e407de29d9e007b639449f0813740e81cde7a4，仓库dingding229/mediatool、分支main；认证账号与仓库push权限已核对。
- 初始Actions run：39379063624。一次监控连接EOF后只读复核，未将网络断线误算CI结论。该run最终verify失败、publish跳过；因此没有使用或宣称该版本Hub发布/服务器部署成功。

## Actions失败与限定修复

- 失败step：Native AMD64 release worker isolation kernel gate (static, non-root)。全仓race与前置实际worker测试已过，但该step还运行静态非race的v2套件；TestDiscoveryPoliciesNarrowTimeoutInvalidAndCommitFence在期望episodes=[1,2,3]时得到回退[1,2]。
- 根因是**测试假peer回复匹配**：调用者200ms等待已结束，持久run仍可能保留总policy deadline；旧helper只排除队列deadline已过的请求，故下一用例的预设回复可被旧的未过期pending行消费。
- 新增确定性回归，预置一条合法但更早的未过期请求。修复前真实失败：新调用失去预设回复，回退全部候选。修复仅在*_test.go中：fake peer在ready前记录既有pending IDs，只回复之后的新调用；更新回复时仍要求pending且未过deadline并检查影响行数。
- 不修改生产hooks/SDK/pin、不增加或重试200ms运行预算、不删失败用例、不放宽期望结果。原expired/readiness/权限及commit-fence断言继续保留。

## 部署前验证输入缺口

当前项目目录尚未发现可用的管理员/原生媒体用户私有验收凭据。已向所有者请求现有私有文件路径（不在聊天收集密码）。未重置口令、创建生产测试账号、伪造会话或绕过认证。在现有凭据落实前，不切换服务器镜像；完整备份/更新/后验收据尚未执行。

## 当前边界

服务器仍是上一官方Hub digest `sha256:0aa7146e4c5ce5ff6303d0e43be4f3cd0efdaf91e3131ca6ce73fecfe2dd6dc4`。根Compose、deprecated fallback环境行、数据库schema及其他服务均未更新。不得把本记录当作Q6已完成或Q7准入。

## 修复提交前本地门禁

- 新增回归在旧helper上先真实失败，再验证修复；受影响用例与expired/readiness/权限围栏回归-race -count=20通过（59.89s）；CGO_ENABLED=0静态模式的policy/discovery回归-count=20通过（24.74s）。没有尝试以本地模拟Linux代替R2。
- 最终冻结树以单条bash scripts/test-release.sh（共享锁）完成build-web、build、vet、全仓race/集成/恢复、errcheck、Vue、plugin check/assets/matrix、E2E、govulncheck，COMPLETE_GATE_EXIT_0与complete-command-exit=0。具体计时见下一表；Mac的Linux专用跳过仍必须由再次Actions消除，未算成通过。

| 门 | 结果/秒 |
|---|---|
| `bash scripts/build-web.sh` | 通过 / 16.81 |
| `go build ./...` | 通过 / 5.75 |
| `go vet ./...` | 通过 / 6.30 |
| `bash scripts/test-go.sh` | 通过 / 747.64 |
| `errcheck -ignoretests ./cmd/... ./internal/... ./migrations/... ./tests/...` | 通过 / 2.43 |
| `npm test --prefix web` | 通过 / 4.65 |
| `go run ./cmd/media-tool-dev plugins check` | 通过 / 0.43 |
| `go run ./cmd/media-tool-dev plugins assets --check` | 通过 / 0.15 |
| `go run ./cmd/media-tool-dev plugins matrix --check` | 通过 / 0.16 |
| `bash scripts/test-e2e.sh` | 通过 / 175.39 |
| `govulncheck ./...` | 通过 / 4.92 |

Vue：47文件159测试；E2E：122通过、1 Linux专用跳过；govulncheck：可达0/导入包0、require-only 1（同Q5c已报告）。生产schema/数据未作为这些测试的夹具。源码修复仅54增/5删的测试代码；无业务代码/模块pin/迁移/权限预算变化。
