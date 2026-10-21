# 部署

使用仓库根目录唯一的 `docker-compose.yml`，服务为 app、postgres、redis。参数直接写入 Compose，不使用 `.env` 插值。Emby、CloudDrive2 和下载器是外部服务，不删除其配置或数据。

- [安装与配置](../docs/deploy/install.md)
- [升级与版本登记](../docs/deploy/upgrade.md)
- [备份与恢复](../docs/deploy/backup.md)
- [账号与密码恢复](../docs/deploy/security.md)
- [全部使用文档](../docs/INDEX.md)
