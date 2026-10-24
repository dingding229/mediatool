# 版本与更新检查（F8a）

## 构建来源

稳定版只接受指向当前 commit 的 `vX.Y.Z` Git tag；本批首版为 `v0.28.0`。Version、Commit、BuildTime 通过 ldflags/build-arg 注入，schema 保持数据库迁移编号，插件 pin 从 Go 构建依赖信息读取。`Revision` 只是现有运维代码对 Commit 的内部别名。

`media-tool-dev build-meta env|json|github|ldflags` 为 Makefile、测试二进制与 Actions 共用元数据实现。非 tag 构建基于 HEAD 已包含的最高稳定 tag 显示 `X.Y.Z-dev+<12位sha>`；首次 tag 之前使用 `0.0.0` 基线。未通过构建工具注入的临时 go run/test 显示明确的 `0.0.0-dev+unknown` 哨兵，不冒充正式版。

- main：推送 `sha-<commit>` 和 `latest` 开发镜像。
- tag：保留 `sha-<commit>`、`latest`，额外推送 `vX.Y.Z` 稳定镜像并创建 GitHub Release。
- `sha-*`/`latest` 是可变别名，main/tag 构建的元数据不同；更新检查只认稳定 `vX.Y.Z` 与 Release digest，部署始终使用不可变 digest。
- Release 正文来自对应 CHANGELOG 中文版本区段，附 AMD64 digest、最低/目标 schema 与机器可核对的元数据块。
- 已发布的稳定 Release 不重推；发布前确认不存在，拒绝网络错误或已有 Release，避免重试覆盖稳定 tag。

API 文档的 `info.version=1.0.0` 表示 `/api/v1` 协议文档版本，不再冒用程序旧版本；运行构建信息通过管理员 `/api/v1/system/version` 获取。

## 只读检查边界

仅访问以下固定公开 API，不读取/发送 GitHub、Docker Hub 账号凭据，不接受自定义仓库或 URL：

1. GitHub：`/repos/dingding229/mediatool/releases/latest`，过滤 draft/prerelease，并严格要求稳定语义版本 tag。
2. Docker Hub：`/v2/namespaces/dingding229/repositories/mediatool/tags/vX.Y.Z`。

Release 元数据必须只有一个合法块，版本与 tag 相同，仓库固定，commit/digest 格式合法，平台为 linux/amd64，最低 schema 不高于目标 schema。Hub tag 的 digest 必须与 Release 一致，并确认 AMD64/Linux 镜像。跨域/其他重定向、超大响应、缺字段、类型/版本异常或 digest 不符均不提示更新；变更说明仅在专用组件内通过精确锁版的 marked 和 DOMPurify 渲染；原始 HTML 和图片在 Markdown 阶段丢弃，再按标签/属性白名单清洗，链接统一以新窗口打开并设置 noopener noreferrer。该交叉检查不等于密码学签名，也不能防御两个发布账号同时失陷。

## 持久化与频率

复用 PostgreSQL `app_settings` 中 `system.update-check` 键，无新增迁移、扩展或依赖。检查结果与开关分别原子更新，避免检查完成时覆盖管理员刚关闭的开关。默认每 12 小时检查，可关闭；手动检查在一分钟内复用结果，跨进程通过固定 advisory lock 合并。后台失败只保留固定状态，不影响请求或播放。获取缓存状态不会访问外网；前端复用可见性暂停的共享刷新 composable，每五分钟读取缓存。

页头铃铛对每个可信新稳定版本原子认领一次提示，跨标签页和 app 重启不会重复认领；关于页始终可查看已验证的新版本。发现不可信数据后会清除旧的可更新结果。测试配置关闭真实后台外网访问，检查器测试使用合成 Release/Hub 响应。

## 本批不做的事

没有 Docker API、docker.sock 挂载、updater 子命令、镜像拉取或执行更新入口，也不修改 APP_ENV 或 IP 库配置。自动替换、备份、回滚、确认与审计均留给 F8b 单独设计、审核和验证。

## F8b：镜像启动契约与手动更新

v0.28.5 起，目标 helper 在备份/停止 app/修改 Compose 之前，比较当前运行镜像（不可变 image ID）和目标镜像的完整 image Config。Entrypoint、Cmd、Env、Healthcheck、User、WorkingDir、ExposedPorts、Volumes、StopSignal、Shell/OnBuild/StopTimeout 等任一项变化都必须**手动更新**，不能直接套用旧容器设置。未来新增 Config 字段也不会被忽略。

只明确排除 Labels：版本、commit、构建时间等是非启动元数据，应保存在 OCI labels 和 ldflags 构建信息中；不得为了版本显示添加随版本变化的运行时 ENV。当前 Dockerfile 符合此规则，基础镜像 Env 仍受比较约束。

拒绝时为 failed，而不是 manual_recovery，原因「镜像启动配置已变化，请按文档手动更新」。详细手动更新边界、凭据/备份和回滚规则见 ONLINE_UPDATE.md。更新检查可继续显示可信发布，但“可信发布”不代表启动契约允许自动更新。不可变 digest 交叉核对与此守卫是两道独立检查，均不可绕过。
