# 部署说明

唯一Compose在根目录 `docker-compose.yml`，仅app、PostgreSQL、Redis。当前参数直接编辑该文件，不依赖外部.env或覆盖文件。

详细安装、已有实例迁移和Hub镜像更新见 `docs/WORKBENCH_RELEASE_GUIDE.md`。密码恢复、日志及数据浏览安全边界见 `docs/WORKBENCH_SECURITY.md`。

公开仓库Compose是占位模板；`go run ./cmd/media-toolctl --root "$PWD" init-compose`只替换占位符并设0600。**不要提交初始化后的Compose，不要覆盖现有主密钥或数据库密码。** 服务器已配置时不要重新初始化。

Emby/CloudDrive2保持外部依赖；不要通过--remove-orphans删掉已配置服务，不对公网开放数据库端口，不挂载Docker Socket或宿主根目录。
