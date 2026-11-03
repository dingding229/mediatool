# 升级与版本登记

[文档目录](../INDEX.md) · [备份与恢复](backup.md) · [命令参考](../CLI_REFERENCE.md)

## 开发验证与正式发布

- `dev`：日常开发分支，推送只运行完整验证（含浏览器门禁），不推送 Docker Hub 镜像，不创建 Release。
- `main`：默认分支及生产发布分支。发版需将已验证的 `dev` 快进合并到 `main`，不 squash、不 rebase 重写历史，并在 `main` 打 `vX.Y.Z` tag。
- `main` 推送与稳定 tag 延续既有发布门禁；pull request 只验证。正式部署必须等待 Actions 全部通过并取得 Docker Hub 不可变 digest，先备份并验证数据库，再仅更新 app，保留近期回滚备份。

## 升级前

1. 选择已发布的 Docker Hub Linux AMD64 不可变镜像。只接受 `dingding229/mediatool` 的摘要地址或符合工具要求的固定镜像标签。
2. 保留私有 Compose、原主密钥、账号、115 授权、STRM 原文、数据库卷、媒体和近期回退备份。
3. 查看运行任务和需要核对的上传，避免在未知外部操作上重复执行。
4. 使用与部署匹配的宿主 `media-toolctl`，不把 Docker Socket 加到 app。

## 管理入口迁移到 HTTPS

1. 只读核对既有 STRM 的 origin（协议、主机、端口）和后台的播放域名。固定路径 `strm.Endpoint` 不包含主机信息，不能表达旧来源。
2. 如果现有播放域名与旧 STRM 来源不同，在**私有 Compose** 中设置 `STRM_RESOLVER_ORIGINS` 为仍需保留的精确旧 origin，例如 `http://192.0.2.10:8080`；最多16项，逗号分隔。不可使用通配符、URL路径、凭据、查询或片段；不自动从文件内容信任任意地址。
3. 先备份数据库及私有 Compose，再与 app 镜像更新一起设置 `APP_ENV=production`、HTTPS `APP_PUBLIC_URL` 和上述精确来源。保留显式播放域名、MEMBER_PUBLIC_URL、原 STRM 正文和密钥不变。
4. 先通过内置播放器验证全部原文件仍被认可并302至官方CDN。任一失败即恢复旧Compose和上一版镜像，不改写STRM或关闭来源/签名校验。development保留原管理来源回退；生产不再额外信任管理地址。

## 更新应用

在自己的部署目录运行，替换下列镜像占位内容：

```sh
./media-toolctl --root "$PWD" image-check 'dingding229/mediatool@sha256:<镜像摘要>'
./media-toolctl --root "$PWD" update 'dingding229/mediatool@sha256:<镜像摘要>'
```

工具锁定部署目录，检查镜像架构、数据结构、来源标签和 Hub 摘要，核对现有 app/PostgreSQL，保留私有备份后受控更新。不要同时运行多个更新或恢复命令。

现有部署可使用 `docker compose cp app:/app/media-toolctl ./media-toolctl` 获取容器附带工具，并设置执行权限。不要替换初始化后的 Compose 为公开模板。

## 更新后核对

- 检查服务健康、管理员登录、连接状态和任务记录。
- 查看媒体目录、已有 STRM 和真实播放器跳转情况。
- 核对未完成上传和批次；显示已排队或结果未知时继续跟进原记录。
- 保留近期备份和原镜像引用；未完成核对前不要清理回退资料。

## 版本登记与回滚

```sh
./media-toolctl --root "$PWD" register 'dingding229/mediatool@sha256:<镜像摘要>'
```

登记版本供“运维 → 备份与告警 → 恢复与回滚”选择，不会直接切换应用。网页预览、审批与宿主执行分开。回滚不能只降低数据库版本；需要匹配的数据库、密钥、输出和应用镜像，并满足保护条件。

执行结果不明确时，先检查工具私有记录、容器和维护单，不能自动再次切换或删除原库。不能以容器启动成功代替业务核对。


## 在线更新（F8b）

见 [ONLINE_UPDATE](../ONLINE_UPDATE.md)。仅管理员重新输入密码并确认后执行；Docker socket 等同宿主机 root 权限。app 保持非 root，operator 显式授权精确 socket GID，不更改宿主机 socket 权限。临时更新容器为三常驻服务以外的唯一例外，使用同一已发布目标 digest，结束自动删除。没有 socket 时保留更新检查。更新失败会恢复预更新数据库及旧 Compose，备份后写入可能丢失；保留近期备份与至少两个旧镜像。不支持任意镜像/URL或降级。
