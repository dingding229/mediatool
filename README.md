# Media Tool

Media Tool 用于搜索影视资源、管理订阅和下载、整理云盘媒体，并向播放器提供媒体目录和播放地址。管理界面使用账号密码登录，支持浅色、深色和移动端布局。

## 核心功能

- **获取资源**：TMDB 影视搜索、站点搜索、发现推荐、订阅预演、定时搜索与 RSS、下载任务和手动添加种子。
- **入库与文件**：目录扫描、候选识别确认、命名分类、资料与 STRM 输出、上传核对、可恢复文件删除、输出恢复与归档。
- **媒体展示与播放**：内置媒体库和 Emby 只读浏览；独立媒体服务、媒体用户、播放器准入、在线会话、探测和截图。
- **服务与扩展**：115 官方授权、CloudDrive2、Emby、通知渠道、TMDB/代理、站点模板及内置插件。
- **运行维护**：任务队列、告警、备份恢复、原始日志、只读数据库浏览、缓存、独立 API 凭据。

播放时，播放器经固定入口跳转到 115 官方地址；服务器不中转媒体流量，不提供转码或 HLS。媒体库网页只用于浏览，不提供网页播放。

## 快速部署

需要 Linux AMD64 主机、Docker Compose，以及能够使用的 PostgreSQL/Redis 存储空间。部署文件只使用仓库根目录的 `docker-compose.yml`，运行服务恰好为 **app、postgres、redis**。Emby、CloudDrive2 和下载器由你单独提供，不由此 Compose 创建或替换。

1. 将仓库根目录的 Compose 模板放到自己的部署目录。
2. 将 `app.image` 设为已发布的 `dingding229/mediatool` Docker Hub AMD64 不可变镜像，例如 `dingding229/mediatool@sha256:<镜像摘要>`；尖括号内容需要自行替换。
3. 首次安装可从该镜像提取宿主工具并初始化模板：

```sh
docker compose pull app
docker compose create --no-deps app
docker compose cp app:/app/media-toolctl ./media-toolctl
chmod 700 ./media-toolctl
./media-toolctl --root "$PWD" init-compose
```

4. **直接编辑 Compose** 的环境字段、端口、挂载和公开地址，不使用 `.env` 插值。初始化后的文件包含私有配置，保持权限 `0600`，不要提交或分享。
5. 启动服务：

```sh
docker compose pull
docker compose up -d --wait
```

管理端默认容器端口为 8080，Emby 网关为 8091，内置媒体服务为 8092。模板只绑定本机回环地址；远程使用需要自行配置访问地址和 HTTPS 反向代理。数据库和缓存不要对公网开放。详细说明见[安装与配置](docs/deploy/install.md)。

**已有部署不要重新初始化，不要替换主密钥、会话密钥或数据库密码，也不要删除数据卷。** 升级使用[升级说明](docs/deploy/upgrade.md)。

## 首次登录与密码恢复

- 首次打开管理端时注册唯一管理员；成功后公开注册关闭，之后使用账号密码登录。
- 管理员账号与媒体用户是两套账号。媒体用户由管理员在“播放服务 → 媒体用户”管理。
- 忘记管理员密码时，由部署所有者在持久化数据目录创建空的 `password` 普通文件并重启 app。程序只在**启动标准输出**显示临时密码；网页、日志查看器和恢复文件均不会保存密码。临时密码有时限，登录后必须设置新密码。
- 恢复文件路径由 `PASSWORD_RESET_FILE` 指定。具体权限、时限和操作步骤见[账号与安全](docs/deploy/security.md)。

## 推荐使用顺序

连接 115/CD2 → 配置 Emby 或内置媒体服务 → 添加站点账号 → 设置命名分类 → 搜索或扫描 → 先核对、再启用订阅。

“已排队”只表示请求已受理，不表示下载、入库或恢复已经完成。结果不明确时先核对原记录，不重复提交。

## 文档导航

- [文档目录](docs/INDEX.md)：13 个主页面和部署手册。
- [统一界面操作](docs/guide/common.md)：刷新、分页、四种状态、危险操作与核对。
- [播放与 302](docs/deploy/playback.md)、[Emby 客户端接入](docs/deploy/emby.md)。
- [备份与恢复](docs/deploy/backup.md)、[日志与数据查看](docs/deploy/logs-data.md)。
- [命令参考](docs/CLI_REFERENCE.md)、[插件说明与作者接口](docs/PLUGINS.md)。
- [前端 API](docs/FRONTEND_API.md)、[接口使用清单](docs/FRONTEND_API_ROUTES.md)、[OpenAPI](api/openapi.yaml)。
