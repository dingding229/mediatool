# 闲时自动更新（T14）

默认关闭，开启/修改启用配置须管理员当前密码及无人值守风险确认。授权绑定管理员ID、credential_version和独立grant ID，不保存密码/会话Token。普通会话到期不撤销授权；主动关闭、改密、重置、owner recovery、账号失效、失败或人工恢复锁使授权失效。执行以 `system:idle-update:<attempt-id>` 记审计，关联授权人。

## 判断和时区

独立协调器使用T9统一解析器的每分钟Cron，保留Recover/SkipIfStillRunning。时间段和每天次数按服务器本地时区（当前Compose的TZ=Asia/Shanghai），采用开始包含/结束不包含，跨午夜用OR；相同开始结束不允许。

所有条件逐项检查：启用、时间段、授权、可信且更新的稳定Release、无有效原生会话、无近期签发、无关键任务、无manual_recovery、Docker环境可用/无正在更新、每日额度、目标启动配置。已验证表示严格稳定tag、非draft/预发布、固定Hub digest与Release相符、AMD64/Linux及schema前提满足；Start再次VerifyFresh。

- 原生会话（含暂停）沿用现有 **5分钟** last_progress_at 心跳过期规则。
- 任意媒体302或成功PlaybackInfo签发写入独立持久时间戳；最近N分钟无签发，N默认60、15–360整数。失败/拒绝响应不计；记录失败期间保守等待完整安静窗口，不靠异步统计日志判空。迁移初次安装从当前时间开始计安静窗口。
- 检查jobs运行/未知状态、实际下载/手动下载/传输/整理状态，以及备份/恢复/维护；常驻插件service不单凭常驻运行阻塞，但其实际关键业务任务会阻塞。外部Emby不新增依赖，不作为条件。
- 最终复查到Start交接使用进程内准入闸。媒体入口、队列领取和已有同步写入协调入口共享闸；不等待活动请求结束，发现活动则延后。上限5分钟，正常/失败立即释放，重启不保留；不修改ops_mode，不把闸保持到整个更新完成。

## 持久化与失败

schema88新增idle_update_policy、idle_update_activity、idle_update_attempts。按目标version+服务器local_day唯一，准备镜像前登记，跨日则最终复查延后，当天已登记不会因关闭/重新开启而重置。准备镜像在闸外，复用F8b启动配置比较；最终再次检查并交给原Start。镜像配置变化须手动更新。F8b核心预检、备份、替换、回滚、安全守卫未改。

`/data/online-update/idle/<attempt-id>.json` 原子写入并fsync，保留不随业务DB回滚丢失的尝试/授权关联；启动先对账，按执行actor关联F8b job，响应丢失不重复Start。不确定记录安全关闭，不自动重放。失败/rolled_back/manual_recovery自动关闭；站长核对后重新验密开启，manual_recovery还须原有恢复确认。成功/失败经outbox发站内消息及已启用Telegram/通知插件，按attempt/grant去重。若app不可用，恢复后补齐记录与通知，不承诺离线即时送达。

迁移Down仅删除T14三表，无NULL转换；重新Up默认关闭。磁盘安全标记不由迁移删除。清理历史必须保留每日配额和未对账标记，不提供自动清理。

## API与验收

管理员GET/PUT `/api/v1/system/online-update/idle`；POST `/api/v1/system/online-update/idle/check` 只读返回11项条件，不创建尝试/不拉镜像/不触发Start/不写审计。目标镜像未缓存时启动配置项显示阻塞，调度会在其他条件通过后登记尝试并拉取校验；实际替换前必须全项通过。OpenAPI及前端路由登记同步。

生产只验证验密保存、逐项dry-run及关闭，确认无新attempt或F8b job，不做真实自动更新；最后保持关闭。截图位于scratchpad/t14，仅查看文件尺寸与DOM几何，不读图片。
