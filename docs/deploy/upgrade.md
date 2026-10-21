# 升级与版本登记

[文档目录](../INDEX.md) · [备份与恢复](backup.md) · [命令参考](../CLI_REFERENCE.md)

## 升级前

1. 选择已发布的 Docker Hub Linux AMD64 不可变镜像。只接受 `dingding229/mediatool` 的摘要地址或符合工具要求的固定镜像标签。
2. 保留私有 Compose、原主密钥、账号、115 授权、STRM 原文、数据库卷、媒体和近期回退备份。
3. 查看运行任务和需要核对的上传，避免在未知外部操作上重复执行。
4. 使用与部署匹配的宿主 `media-toolctl`，不把 Docker Socket 加到 app。

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
