# 在线更新（F8b）

## 权限边界与启动方式

- 只允许管理员；执行前再次验证当前账号密码，并确认版本、变更说明、播放中断与数据库恢复风险。请求不接受镜像、URL、命令或路径。
- `/var/run/docker.sock` 等同宿主机 root 权限。应用仍为 UID/GID 65532，只追加宿主机 docker 组（当前 GID 994）；不 chmod/chown socket。白名单是正常代码路径限制，**不是 app 被攻陷后的宿主机隔离边界**。
- app 只通过 Unix-socket 白名单客户端启动目标不可变镜像的临时 `updater` 子命令；更新容器带项目/任务 label，自动删除，不是常驻 Compose 服务。这是三服务规则的唯一更新执行例外。
- 临时更新器需要 root 读写 root 所有的私有 Compose/备份目录。只挂本项目工作目录（容器内保留相同绝对路径，兼容相对挂载）、已有 `/data` 和 socket；只连接现有项目网络，rootfs 只读、cap_drop ALL，仅加 CHOWN/DAC_OVERRIDE 以读写原 UID 所有的私有状态、no-new-privileges。
- 本批运行镜像增加 Docker CLI/Compose（Alpine 官方包），用于按原 Compose 重建 app；没有新增 Go/npm 依赖。Compose 仅能连接更新器的本地 Docker API 白名单代理，不直接连接宿主机 socket。plugin-worker 的 seccomp TSYNC 禁止创建 AF_UNIX socket/socketpair，避免继承 docker 组扩大插件权限；stdio Host RPC 与 TCP/DNS 保持。

## 完整流程

1. 固定 GitHub 仓库公开 Releases 和 Docker Hub 公开 API，重新核验稳定版本、commit、AMD64 digest、schema。**不使用更新检查缓存**。digest 不一致即拒绝并持久审计。
2. 只拉 `dingding229/mediatool@sha256:…`，绝不按 tag 拉镜像；更新器启动后、真正替换前再校验 Release/Hub 和已确认 digest。
3. 持久化私有任务与阶段日志到 `/data/online-update`，返回后台可读的脱敏状态。参数中不携带密码/Token/数据库 URL。每次只允许一项任务。
4. 更新器在宿主机 `private/online-update-<UTC>-<ID>/` 备份 Compose 和 pg_dump（校验归档目录与 SHA256），保留近期备份。备份包含敏感业务信息，仅 operator-private。
5. 原子替换 Compose **app.image 一行**，检查渲染后的完整 Compose 只有 image 不同，再运行固定 `docker compose up -d --no-deps --no-build --pull never --wait app`。不操作 postgres/redis、Emby/CD2。
6. Docker health、HTTP readyz、目标版本/commit、数据库 schema 全部匹配才成功。任务/审计独立于 app 生命周期；杀死 app 不会杀死临时更新器。
7. 失败时恢复原 Compose，停止且仅停止本项目 app，先用 pg_dump 恢复数据库（即使迁移不可逆，也恢复到更新前 schema/业务快照），再用旧 digest 重建 app并验证。恢复数据库会丢弃备份后产生的业务写入，确认页必须提示。失败回滚也失败则标记需运维处理，保留所有备份，不宣称成功。
8. app 重启后检查未完成任务；临时执行器仍在则不重复启动；未启动/异常退出则从私有阶段记录恢复。已经替换镜像的中断任务先走保守回滚，不盲目继续迁移。

## Docker API 白名单

- 无任意 URL/原始 API 转发入口。拒绝 exec、build、plugin、swarm、系统设置、prune、镜像/卷/网络删除等。
- 仅 pull 已确认的本仓库 digest，inspect 既有本项目资源；仅创建/启动/停止/删除精确项目 app 或固定更新任务容器。
- Compose 代理强制 project/service label 与精确容器 ID/name；创建 app 校验 image、User、Env、mount、port、capability 等不扩大原配置。Compose 的发现只返回该项目资源，既有网络/卷只读，不允许创建/删除。允许必要的 ping/version 握手（info 仅本地合成最小能力）；stop/rename 仅用于该 app 的替换。超时回滚使用独立且限时的恢复上下文，不沿用已取消的请求。
- 只使用 Compose 的 `--no-deps` app-only 操作。清理仅自动删除带项目+任务 label 的临时更新容器；不做镜像全局清理，保留旧 digest（至少最近两个）和备份。

## 首次安装与演练版本

F8a 的 v0.28.0 不含更新执行器，不能在不覆盖已发布 tag/digest 的前提下自行更新。先按常规流程从 v0.28.0 部署 F8b 的 `0.28.0-dev+<sha>` 引导版本（如实显示版本，不重写 v0.28.0 Release），再发布 v0.28.1，通过后台真实更新到 v0.28.1。后续稳定版直接一键更新。不允许降级，也不向同版本旧 Release 回退。

## 测试与运维

本地只跑定向 -race、相关 Vitest/guards/build/E2E；全量由 Actions 运行。模拟 Docker API/流程覆盖拉取失败、健康/迁移失败回滚与数据库恢复、app 被杀恢复、非法 API 与不可变 digest。生产不故意制造失败。没有 socket/权限不足时仅禁用执行，保留公开检查。

CI 通过固定 v0.28.0 镜像 digest 提取并缓存已有 ffmpeg/ffprobe 和 musl 依赖，不启动 app、不读取业务卷、不使用凭据；官方 actions/cache v5 固定 commit。只删除带 CI 专用 label 的临时提取容器，失败才走原 apt 后备。测试与预算不变。

断连恢复：inspect/start 响应丢失不是“容器不存在/启动失败”。按固定任务名重新核对已有 helper，不复制执行器，不覆盖它已写入的阶段。只有明确失败且复核为未运行的本任务容器才删除；Docker 状态不明时禁止恢复数据库，转为待运维处理。
