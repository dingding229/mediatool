# 安装与配置

[文档目录](../INDEX.md) · [快速部署](../../README.md)

## 运行服务

正式部署只使用根目录 `docker-compose.yml`：

| 服务 | 职责 | 持久数据 |
| --- | --- | --- |
| app | 管理页面、API、任务、媒体解析、内置媒体服务、插件子进程 | 应用数据、输出、私有文件 |
| postgres | 账号、加密配置、媒体索引、任务和操作记录 | PostgreSQL 数据卷 |
| redis | 缓存与运行协调 | Redis 数据卷 |

所有应用功能都在 app 内运行，不需要额外的任务、插件或转码容器。Emby、CloudDrive2 和下载器是已有外部服务；不要删除、替换或清空它们的数据。

## 镜像与初始化

使用已经发布的 `dingding229/mediatool` Docker Hub Linux AMD64 镜像，选择不可变摘要。不要用其他仓库或本地镜像归档替代正式镜像。首次提取宿主工具及初始化步骤见[README](../../README.md)。

`media-toolctl --root "$PWD" init-compose` 只替换公开模板中的占位值，并以 `0600` 权限原子保存。已初始化时拒绝再次覆盖。必须保管原始主密钥、会话密钥、播放密钥和数据库密码；丢失主密钥可能导致已加密凭据无法解密。

不要把初始化后的 Compose、数据库、授权或日志提交到版本库。

## 直接配置参数

参数直接写在 Compose 的 `environment`、`ports`、`volumes` 中，不使用 `.env` 插值或叠加部署文件。请按自己的环境填写，文档不提供可复用凭据。

| 参数或配置 | 用途 |
| --- | --- |
| `APP_PUBLIC_URL`、`AUTH_ALLOWED_ORIGINS` | 管理入口及允许的完整 Origin；多个 Origin 用逗号分隔，不带路径 |
| `MEMBER_PUBLIC_URL` | 内置媒体服务对播放器公开的地址 |
| `EMBY_PUBLIC_URL`、`EMBY_GATEWAY_ADDR` | 外部 Emby 网关的公开地址与监听地址 |
| `STRM_ALLOWED_ROOTS`、`STRM_OUTPUT_DIR` | 允许的云盘根与受管输出位置 |
| `LOCAL_BROWSE_ROOTS` | 文件管理允许浏览的本地容器目录 |
| `PASSWORD_RESET_FILE` | 部署所有者密码恢复标记路径 |
| `OPS_DATA_DIR`、`APP_LOG_DIR` | 私有维护文件及应用日志位置 |
| `TZ` | 时间显示环境；业务调度使用 Asia/Shanghai |

后台“设置 → 通用与访问”可配置各公开域名；明确设置的域名优先于部署默认值。变更域名不自动改写已有 STRM。

## 端口、反代和存储

- 容器默认管理 8080、外部 Emby 网关 8091、内置媒体服务 8092。公开地址、反代目标和宿主端口必须一致。
- 模板绑定回环地址。远程访问请用自己的 HTTPS 反代，保留真实客户端 UA、Range 和授权相关头；不要将媒体服务和管理入口混用。
- PostgreSQL/Redis 不向公网开放；不挂载 Docker Socket、宿主根目录或其他项目私有目录到 app。
- 默认应用用户为 UID/GID 65532。使用绑定目录时，只对实际需要写入的本项目目录设置权限，不递归修改整个媒体盘。
- 下载器共享的源文件目录可只读挂入 app；云端上传和入库不会自动删除做种源。
- 文件浏览只能看到真实挂载且在允许根范围内的路径；私有维护目录不是文件管理入口。

## 启动后

```sh
docker compose up -d --wait
docker compose ps
```

打开管理端完成注册，再按[连接](../guide/connections.md)、[站点](../guide/sites.md)、[设置](../guide/settings.md)配置。先用自己的已入库文件确认播放，再开启自动订阅。

`/healthz` 和 `/readyz` 可供容器或反代检查进程及依赖状态，不代表每个外部服务或播放器都可用。
