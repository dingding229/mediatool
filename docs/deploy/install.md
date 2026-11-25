# 安装与配置

[文档目录](../INDEX.md) · [快速部署](../../README.md)

## 运行服务

正式部署只使用根目录 `docker-compose.yml`：

| 服务 | 职责 | 持久数据 |
| --- | --- | --- |
| app | 管理页面、API、任务、媒体解析、内置媒体服务、插件子进程 | 应用数据、输出、私有文件 |
| postgres | 账号、加密配置、媒体索引、任务和操作记录 | PostgreSQL 数据卷 |
| redis | 缓存与运行协调 | Redis 数据卷 |

所有应用功能都在 app 内运行，不需要额外的任务、插件或转码容器。CloudDrive2 和下载器是已有外部服务；不要删除、替换或清空它们的数据。

## 镜像与初始化

使用已经发布的 `dingding229/mediatool` Docker Hub Linux AMD64 镜像，选择不可变摘要。不要用其他仓库或本地镜像归档替代正式镜像。首次提取宿主工具及初始化步骤见[README](../../README.md)。

`media-toolctl --root "$PWD" init-compose` 只替换公开模板中的占位值，并以 `0600` 权限原子保存。已初始化时拒绝再次覆盖。必须保管原始主密钥、会话密钥、播放密钥和数据库密码；丢失主密钥可能导致已加密凭据无法解密。

不要把初始化后的 Compose、数据库、授权或日志提交到版本库。

首次空库启动仅在 app 的启动 stdout 打印一条 `[ADMIN INITIALIZATION]`，不进 Web 日志。安装者将其中令牌填到首次注册页；默认 24 小时有效，注册成功即失效。令牌过期或未保存时，仅重启该空库实例来签发新令牌，旧令牌随即失效。已有管理员时不签发，不修改账号。不要把启动日志或令牌转存到公开位置。

## 直接配置参数

参数直接写在 Compose 的 `environment`、`ports`、`volumes` 中，不使用 `.env` 插值或叠加部署文件。请按自己的环境填写，文档不提供可复用凭据。

| 参数或配置 | 用途 |
| --- | --- |
| `APP_PUBLIC_URL`、`AUTH_ALLOWED_ORIGINS` | 管理入口及允许的完整 Origin；多个 Origin 用逗号分隔，不带路径 |
| `MEMBER_PUBLIC_URL` | 内置媒体服务对播放器公开的地址 |
| `STRM_RESOLVER_ORIGINS` | 内置播放器额外认可的精确旧解析 origin，逗号分隔，最多16项；空值不增加来源，不接受路径或通配 |
| `STRM_ALLOWED_ROOTS`、`STRM_OUTPUT_DIR` | 允许的云盘根与受管输出位置 |
| `LOCAL_BROWSE_ROOTS` | 文件管理允许浏览的本地容器目录 |
| `PASSWORD_RESET_FILE` | 部署所有者密码恢复标记路径 |
| `OPS_DATA_DIR`、`APP_LOG_DIR` | 私有维护文件及应用日志位置 |
| `TZ` | 时间显示环境；业务调度使用 Asia/Shanghai |

后台“设置 → 通用与访问”可配置各公开域名；明确设置的域名优先于部署默认值。变更域名不自动改写已有 STRM。

## 端口、反代和存储

- 容器默认管理 8080、内置媒体服务 8092。公开地址、反代目标和宿主端口必须一致。
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

## v0.43.0 外部 Emby 对接退役

程序不再连接或代理外部 Emby；原外部网关 8091（本机生产映射 25015）退役，旧 EMBY 配置不再读取。内置媒体服务 8092（生产 25018）的账号、协议别名及 STRM 解析入口不变。无需修改 Compose 或清理外部服务；外部 Emby 服务和数据留待独立维护。

### 插件文件系统沙箱

插件 worker 默认必须成功启用 Landlock，否则不加载插件业务代码，实例保留原有配置及 KV，显示 `needs_action / sandbox_unavailable`。请优先在支持 Landlock 且允许相关系统调用的 Linux 内核运行；修复后在管理端重新启用实例。若明确接受退化风险，可只在私有 Compose 的 `app.environment` 中显式设置 `MEDIATOOL_PLUGIN_ALLOW_NO_LANDLOCK: "1"` 并重建 app 容器，再重新启用实例。每次向 worker 下发该开关前都必须写入 `plugins.sandbox.override` 审计；审计失败则不启动。开关只允许在 ABI 不可用时继续，不放行规则安装失败，也不会取消 worker 的其他限制。处理完毕后移除此设置并重建 app 容器。
