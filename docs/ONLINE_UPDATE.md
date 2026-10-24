# 在线更新（F8b / v0.28.4 安全修复）

## 权限、凭据和默认状态

- 只有管理员可访问检查、控制和执行接口。**执行开关默认关闭**，更新检查独立保留。开启执行、关闭执行、人工恢复确认都要求当前管理员密码复核、明确确认并写审计；真正更新还需再次确认目标与密码。
- 控制状态保存在 PostgreSQL `app_settings`，与对应审计同事务提交，不增加迁移（schema 79）。执行 API 与 helper 启动准入检查开关，前端按钮同样禁止未开启的执行。
- `manual_recovery` 写入数据库互锁及终态审计；兼容读取旧版未确认的 manual_recovery 审计。**删除 `/data` 任务文件不能解锁**。只有「任务已完成人工确认」显式操作（按任务 ID）可解除，且写 `online_update.recovery_confirmed`；确认不等于开启执行开关。不得以删除文件、重试更新代替恢复核查。
- socket 等同宿主机 root 权限；app 仍 UID/GID 65532，只追加批准的 GID 994，不修改 socket 属主或权限。白名单是正常代码路径限制，**不是应用被攻陷后的宿主机隔离边界**。
- helper 使用**目标 Release 不可变 digest** 内的 updater，故修复逻辑可从目标镜像生效；目标供应链信任也等同 root。启动前检查 Release/Hub digest、AMD64、版本、commit、schema、仓库；helper 内及切换前重查 Release/Hub，拒绝可变 tag 或不一致结果。
- 脱敏 plan、状态、stderr、日志、审计、API 均不保存 DATABASE_URL、密码、Token、Cookie。DSN 只从身份/配置指纹校验后的原容器 inspect 恢复到 helper 内存，用于备份、实际 schema、恢复和终态审计；失败时 app 用自身连接对账。
- 宿主 operator-private 备份保持原始 Compose、渲染 Compose、完整容器快照和 pg_dump，目录 0700、文件 0600；这是经批准的私有明文凭据边界，回滚不依赖 master key 或运行中的容器。不要上传至 Actions/Release/Git。

## 不再依赖 Compose 版本的切换方案

选用**直接从已验证原容器 inspect 生成 create 请求**，而非为 helper 维护与宿主机同大版本 Compose。理由：宿主 Compose 版本不受镜像控制，同大版本仍可能改变序列化；复制原容器有效配置更简单、可精确保留 Env、端口、资源限制、用户、挂载、capability、网络等。不新增依赖。

Compose 仅用于备份时的只读 `config --format json`，没有 `compose up` 更新/回滚调用。实际生产安装仍使用根 Compose，更新只改其 app.image 文本一行，不改变其他服务。

1. 拉取目标 digest、启动临时 helper 后，**先完成无写入 Docker 的 dry-run**：获取目标 image inspect，内存生成原容器的 create 请求，以和实际写入相同的 API 白名单校验。预检前不备份写文件、不停旧 app、不改 Compose、不恢复数据库；仅持久化任务阶段/必要 helper 启动和镜像拉取。
2. 原容器 Config 保持不变，仅 Image、来自目标 image 的 OCI 标签和默认自动 Hostname 更新；保留 HostConfig。网络只保留原网络与经过限定的服务别名，拒绝未知静态网络选项。不接受任意原始 create/API 输入。
3. 挂载按 source、target、RO、propagation、递归、NoCopy、创建源目录语义逐项规范化比较：短 `Binds` 隐式创建 bind source，等价于 typed Mount 的 `BindOptions.CreateMountpoint=true`（Compose `create_host_path`）。false 不等价；未知选项、重复目标、冲突递归或不支持类型在预检拒绝，不能悄悄放宽。实际请求仍沿用原 HostConfig 表达，不需要跨 Compose 重写。
4. 预检通过并记录**更新前实际 schema** 后，备份 Compose/完整 inspect/pg_dump，校验 SHA256 与 `pg_restore --list`。失败直接 failed，旧 app 在线。
5. 替换前再次检查 Release/Hub、原容器指纹及 Compose 未被外部改动，然后原子替换 app.image。**停止旧容器、改为本任务 rollback 名称保留**，创建同名新 app、启动；不删除旧容器。
6. 检查新 app Running/healthy、HTTP `/readyz`、目标 version/commit 和实际 schema；成功才删除保留的旧容器。旧镜像和近期备份仍保留。
7. 失败回滚：停止/删除精确匹配的新 app，检查数据库实际 schema。**schema 与更新前相同则零恢复命令**，保留备份后业务写入；只有目标版本包含前向迁移、实际 schema 已在该前向区间发生变化，才在单事务内恢复经 SHA256 校验的 public 归档，再核对 schema。目标同 schema 却发现版本变化、反向或超出目标范围、查询不确定时 manual_recovery，不猜测恢复。
8. 恢复原 Compose；把保留的同一旧容器 ID 改回原名并启动，等待健康，不用 Compose 重建。回滚失败进入 manual_recovery，不自动再演练。
9. 已在切换阶段的中断任务只走保守回滚，不重放未知 create；只有能重新验证原容器、私有快照和配置指纹才操作。

**限制**：schema 版本号是迁移提交的依据；迁移在同一版本号下私自做已提交 DDL 不受支持。正常事务化迁移失败不应留下半提交 DDL。数据库连接不可达/状态不确定时，宁可要求人工恢复，也不盲目覆盖业务数据。

## 错误证据、审计与清理

- helper **AutoRemove=false**，避免退出即丢证据。每次 pg_dump/pg_restore/psql、只读 Compose config 和 Docker HTTP 失败的脱敏 stderr，带时间/结果存于 `/data/online-update/<id>.evidence/`；只记录诊断，不保存 create 请求和 Env。
- stderr 收集/落盘有长度上限，过滤所有环境值、密码、URL 和认证字段。Web/API 不直接返回这些文件。终态写入后，app 对账持久审计，再精确校验 task/project label 并清理已停止的 helper；运行中或身份不明的不删。
- requested 与 succeeded/failed/rolled_back/manual_recovery 均按任务幂等审计；helper 无 DSN/连接不可用时，app 通过自己的数据库连接补写终态。锁未释放不对账，不清理执行器。
- 白名单拒绝 exec/build/plugin/swarm/prune、卷/网络创建删除、其他服务变更。只拉固定仓库 digest；只允许本项目 app 和任务 helper 的必要 inspect/create/start/stop/rename/delete。没有任意 URL/API 转发入口。
- 私有失败备份目录（包括历史 0 字节 partial）保留证据；有效 bootstrap 备份继续保留。禁止全局 prune、删业务卷或修改 Emby/CD2。

## 回归与本次部署边界

本地按定向规则跑改动包及直接反向依赖 -race、相关 Vitest/guards/build/E2E；全量交 Actions。

`TestRealHelperEntrypointDocker` 从脱敏 plan + 真实 inspect 入口开始，使用固定公开不可变镜像、真实 Docker/pg_dump/readyz，所有数据库操作只在 `media_tool_test` 串行执行。生产形状混合短/长挂载、create_host_path 真/假、10 个挂载；独立 dry-run 单测覆盖 Compose Binds/Mounts 等价与不等价转换。

整链覆盖：预检拒绝（旧 app 全程在线）、新启动失败/健康失败（原 ID 回滚且 DB 不动）、真实 schema 79→80 后失败（恢复到 79 再启动原 ID）、成功及备份失败。持续探测 readyz，严格断言只在 applying 至终态的预期切换窗口内允许中断。单测也覆盖凭据、Docker 白名单、互锁文件删除与显式确认、API 管理员/密码边界。

本次 v0.28.4 按标准 Actions→Hub digest→私有备份→仅 app.image 部署，**执行保持关闭，不发布 v0.28.5、不触发生产演练**。历史任务的人工确认由业主操作；等审核后再决定何时启用与最后演练。
