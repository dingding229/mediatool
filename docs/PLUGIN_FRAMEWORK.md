# 内置插件框架 V2（F1）

## 来源、发布与管理员选择

唯一代码来源是 `github.com/dingding229/mediatool-plugin`，当前 pin 为
`v0.0.0-20261017081253-7db725679179`。主程序和前端资源清单使用同一个 pin；
`internal/modules/plugins/v2/library.json` 是构建期生成、CI 校验的只读清单。
插件与 Go SDK 编译进 app，镜像不增加 Python 或 Go 工具链。

新增/修改插件：插件仓库 vet/test/race/golden → 推送插件仓库 → 主程序 bump pin →
初始化副作用及资源清单门禁 → Actions → Docker Hub 不可变镜像 → 标准备份部署。
**F1 仅发布构建，不部署；框架部署留 R-F。** 不存在上传、远程市场、外部仓库 URL、
单插件运行期升级、运行期源码编译或下载，不在 UI 暴露源码/提交/版本选择器。

目录查询不调用工厂，不创建实例或目录。管理员安装前确认当前 fingerprint 的所有权限；
安装默认停用。支持启停、分身、加密 JSON 配置、按实例查看 KV 与日志、保留数据卸载。
卸载是 tombstone，不删除私有数据；再次安装新建实例。分身重新加密配置/KV，不共享目录。
管理 API 仅管理端口、管理员会话+CSRF，写操作带 operationKey、CAS revision 和审计。

## 生命周期、升级与授权

`plugin_v2_definitions/instances/configs/kv/grants/operations` 是基础存储。
旧六表归档为 `plugin_legacy_wasm_*`，不再执行或分发；归档密文加入主密钥轮换，
保留原 `plugin:<id>` / `plugin-data:<id>` AAD。迁移62不修改非插件业务表。
有 V2 实例时 Down 明确拒绝有损降级，应恢复备份。

启动 SelfCheck 检查 SDK contractVersion=2、pin 和清单一致。Refresh 只刷新已选定义：
无变化不写；元数据更新换 fingerprint，旧 grant_revision 失效；执行面/资源/权限变化
保守地停用待管理员重新确认。移除的定义标记 missing，配置/KV/data 保留。
审批未完成不会因重复同步被复位。锁与 revision CAS 保证并发刷新/操作的一致性。
Host 每次从数据库读取最新授权，不跨调用缓存。读用 KEY SHARE，不互相串行或阻塞续租；
写的配额/CAS由实例专用 advisory lock串行，提交前条件 UPDATE重新校验generation、
grant_revision、lease和指纹，撤权竞争失败则回滚副作用。已提交完成的调用不追溯撤销。

## 执行边界与残余风险

每个启用实例同 binary `plugin-worker`，配置/日志/数据库初始化之前早分流，
JSON-RPC 2.0 长度帧双向stdio。固定白名单env、CLOEXEC、Pdeathsig、独立进程组；
整树RSS/CPU监测、rlimit、owner lease/fence、退避重启、整组kill/reap。
worker与app同UID65532；父进程最早期dumpable=0阻止同UID读父environ/mem/ptrace。
pprof CPU采样保持可用；core dump被有意关闭。不引入setuid、UID池、helper或seccomp，
不要求放宽生产cap_drop ALL/no-new-privileges。

Landlock用已有x/sys尽力自限制，ABI不支持发warning继续。CGO/race不支持
AllThreadsSyscall时仅为降级测试；CI另用CGO_ENABLED=0、原生Linux AMD64、
非root、cap-drop ALL/no-new-privileges执行内核及真实同binary worker门禁。

这是**受信代码缺陷隔离，不是恶意代码沙箱**：同UID仍可能读取app可读的文件
（如文件形式主密钥）；Landlock不可用时不能保证实例间文件隔离，由维护者代码审核兜底。
同页ESM亦是trusted UI，不是iframe沙箱。密钥/连接/115授权不进入worker环境。
管理员原始业务日志按既有政策可见，但不能公开提交到Git/Actions工件。

## F1 管理 API

| 方法/路径（均以 `/api/v1` 开头） | 行为 |
|---|---|
| GET /plugins/catalog | 只读编译目录；pageSize≤200 |
| GET /plugins/resources | 同pin的只读资源/摘要清单 |
| GET /plugins | 分页实例，pageSize≤200 |
| GET /plugins/{instance}?after=key | 配置及KV（每页≤50，单值≤128KiB） |
| POST /plugins/operations | install/enable/disable/clone/configure/review/remove；幂等+CAS+审计 |
| GET /logs/query?module=plugins&instance=UUID | 有界日志窗口，instance精确过滤 |

F2再实现函数绑定、plugin.invoke、动态API/服务/命令/事件、JSON树和本地ESM执行。
F3业务Host、F4事件全覆盖、F5库管理完善及F6验收插件未实现，不能宣称MP功能已对等。
详见 `plans/20261017-plugin-system-v2.md`。
