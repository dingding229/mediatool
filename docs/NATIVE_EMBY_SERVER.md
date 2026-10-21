# 内置媒体服务协议

内置媒体服务在 app 进程内提供独立的 Emby 兼容端口，用于媒体客户端登录、浏览和播放。它不是管理员接口，也不是完整的 Emby 服务端管理 API。

使用步骤见[Emby 客户端接入](deploy/emby.md)；播放边界见[播放与 302](deploy/playback.md)。

## 账号和目录

媒体用户由管理员创建，库的可见、可播放和可下载权限独立控制。目录只暴露已确认、文件在库且有有效 STRM 输出的资产。账号或授权撤销后不能继续使用原会话访问。

目录和详情采用 Emby 风格的 PascalCase 字段；列表使用对应的查询结果或数组。条目 ID 为不复用的数字字符串，不应转为浮点数。季度、集、继续观看、最近播放和进度以上报及实际目录为准。

## 播放与处理

PlaybackInfo 提供受当前用户、媒体源和设备约束的直接播放地址。HEAD/GET 播放请求经授权后返回无媒体正文的 302，播放器自行访问官方地址。Range 和实际播放器 UA 保留；不输出云盘令牌、管理员凭据或临时地址到目录 DTO。

探测与截图在后台运行，与媒体转发无关。外挂字幕和音视频编码兼容性以接口实际返回为准；不支持的能力明确报错，不使用转码替代。

## 端点清单

### 媒体端口端点

| 方法 | 路径 | 用途与限制 |
|---|---|---|
| GET | `/System/Info/Public` | 公共服务器信息 |
| GET | `/System/Info` | 认证后信息，ServerName/地址同公共发现；不支持重启/更新 |
| GET | `/System/Ping` | JSON 字符串，值为数据库中的 ServerName |
| POST | `/System/Ping` | 与 GET 相同；不收集请求体 |
| GET | `/Branding/Configuration` | 本地空品牌配置，无任意CSS/远程资源 |
| GET | `/System/Endpoint` | 匿名、保守网络默认值 |
| GET | `/Branding/Css` | 本地空 CSS |
| GET | `/Branding/Css.css` | CSS 别名 |
| GET | `/DisplayPreferences/{id}` | 用户认证后只读默认值 |
| GET | `/Users/{userId}/Configuration` | 仅当前用户最小配置 |
| GET | `/Sessions` | 当前用户合法空列表，不暴露他人会话 |
| GET | `/DisplayPreferences` | 认证后只读默认值；不承诺POST持久化 |
| GET | `/Localization/*` | 合法空本地化目录资源 |
| GET | `/socket` | 认证后501，无Upgrade；REST不受影响 |
| GET | `/Users/Public` | `public_user_list=false` 返回空数组，true 时返回最小启用用户字段 |
| POST | `/Users/AuthenticateByName` | 兼容 Pw/Password、MediaBrowser/Emby 认证头，签发 365 天设备令牌 |
| GET | `/Users/{userId}` | 令牌所属用户信息与最小策略 |
| POST | `/Users/{userId}/Password` | 本人改密，保留当前令牌并撤销其他设备 |
| POST | `/Sessions/Logout` | 撤销当前设备令牌 |
| POST | `/Sessions/Capabilities` | 需令牌后返回 204 |
| POST | `/Sessions/Capabilities/Full` | 需令牌后返回 204 |
| GET | `/Library/MediaFolders` | 与 Views 相同的可见库集合 |
| GET | `/Users/{userId}/Views` | 可浏览的逻辑媒体库 |
| GET | `/Users/{userId}/Items` | 分页、排序、过滤和搜索 |
| GET | `/Users/{userId}/Items/Latest` | 最近更新数组，默认按剧聚合 |
| GET | `/Users/{userId}/Items/{itemId}` | 电影、剧集、季、集详情 |
| GET | `/Items/Latest` | 与用户Latest等价，默认20项 |
| GET | `/Items` | UserId 查询参数形式 |
| GET | `/Items/{id}` | 条目详情 |
| GET | `/Items/Counts` | 仅可见范围的 Movie/Series/Episode 计数 |
| GET | `/Items/{id}/Similar` | 同类型同题材可见条目，最多20项 |
| GET | `/Shows/{id}/Seasons` | 跨可见库合并实际存在的季，支持特别篇筛选 |
| GET | `/Shows/{id}/Episodes` | 跨可见库去重、Season/SeasonId与StartItemId分页 |
| GET | `/Shows/NextUp` | 继续观看/下一集 |
| GET | `/Search/Hints` | 标题/原始标题NFKC搜索提示 |
| GET | `/Genres` | 可见范围题材Name/稳定Id |
| GET | `/Items/Filters` | 可见范围题材、年份、分级 |
| GET | `/Items/{id}/Images/{type}` | TMDB 海报、背景、Logo、缩略图；匿名仅限已暴露条目 |
| GET | `/Items/{id}/Images/{type}/{index}` | 指定图片序号 |
| GET | `/Items/{id}/PlaybackInfo` | 只返回 DirectPlay/DirectStream 能力 |
| POST | `/Items/{id}/PlaybackInfo` | 兼容客户端播放协商 |
| GET | `/Videos/{id}/stream` | 受权限和 UA 白名单保护，302 到 CDN |
| GET | `/Videos/{id}/stream.{ext}` | 容器扩展名兼容形式 |
| GET | `/Videos/{id}/original.{ext}` | 原始媒体直链形式 |
| GET | `/Items/{id}/Download` | 需要下载权限，仍只允许 302 |
| GET | `/Videos/{id}/subtitles/{index}/stream` | 内嵌字幕由客户端读直链，此端点404 |
| GET | `/Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{format}` | 内嵌字幕统一404，不抽取或转换 |
| POST | `/Sessions/Playing` | 激活草稿会话 |
| POST | `/Sessions/Playing/Progress` | 10秒节流，状态/seek即时写入 |
| POST | `/Sessions/Playing/Ping` | 续约会话，不修改用户进度 |
| POST | `/Users/{userId}/PlayingItems/{itemId}` | 旧版开始上报，查询参数 |
| DELETE | `/Users/{userId}/PlayingItems/{itemId}` | 旧版停止上报 |
| POST | `/Users/{userId}/PlayingItems/{itemId}/Progress` | 旧版进度上报 |
| POST | `/Sessions/Playing/Stopped` | 幂等停止并保存用户进度 |
| POST | `/Users/{userId}/PlayedItems/{itemId}` | 标记已看 |
| DELETE | `/Users/{userId}/PlayedItems/{itemId}` | 取消已看 |
| POST | `/Users/{userId}/FavoriteItems/{itemId}` | 添加收藏 |
| DELETE | `/Users/{userId}/FavoriteItems/{itemId}` | 取消收藏 |
| GET | `/Users/{userId}/Items/{itemId}/UserData` | 用户条目数据 |
| POST | `/Users/{userId}/Items/{itemId}/UserData` | 更新进度、已看、收藏 |
| GET | `/Users/{userId}/Items/Resume` | 继续观看列表 |

### 管理端口端点

| 方法 | 路径 | 用途与限制 |
|---|---|---|
| GET | `/api/v1/media-server/native/sessions` | 管理员只读活跃会话，分页≤200 |
| GET | `/api/v1/media-server/users/{id}/devices` | 管理员设备分页，≤200，不含设备key/令牌/IP/UA |
| POST | `/api/v1/media-server/users/{id}/devices/{deviceId}/revoke` | 单设备撤销及审计，不影响其他设备 |
| GET | `/api/v1/media-server/users/{id}/history` | 管理员按用户读取最近条目观看状态，分页≤200 |
| GET/PUT | `/api/v1/media-server/native/settings` | ServerName、总开关、revision；GET含安全的媒体地址与端口 |
| GET | `/api/v1/media-server/native/library-mappings` | 分类对应关系白名单，不含任何目录路径 |
| GET | `/api/v1/media-server/providers` | 管理端：媒体服务能力 |
| GET | `/api/v1/media-server/libraries` | 管理端：可授权库 |
| GET | `/api/v1/media-server/users` | 管理端：媒体用户列表 |
| POST | `/api/v1/media-server/users` | 管理端：创建媒体用户 |
| GET | `/api/v1/media-server/users/{id}` | 管理端：单个用户及授权/限制 |
| PUT | `/api/v1/media-server/users/{id}` | 编辑媒体用户及授权/限制 |
| POST | `/api/v1/media-server/users/{id}/password` | 管理端：重置密码 |
| POST | `/api/v1/media-server/users/{id}/sessions/revoke` | 管理端：撤销会话 |
| DELETE | `/api/v1/media-server/users/{id}` | 管理端：删除用户，不删除文件 |
| POST | `/api/v1/media-server/native/probe/{assetId}/retry` | 管理员会话、CSRF、维护模式保护；双开关均开启时才接受；原子审计 |
| GET | `/api/v1/media-server/native/processing` | 三开关、两队列状态统计、截图用量/预留/上限 |
| PUT | `/api/v1/media-server/native/processing/settings` | 三开关、revision 乐观锁、事务审计 |
| GET | `/api/v1/media-server/native/processing/tasks` | kind/state/page/pageSize 分页，单页≤200 |
| POST | `/api/v1/media-server/native/processing/retry` | 单条或确认后的筛选批量重试，≤500，审计 |
| POST | `/api/v1/media-server/native/processing/gc` | 单轮≤200项、沿用2秒预算/游标，报告实际回收 |
| GET | `/api/v1/media-server/native/libraries` | 分类同步后的库列表，含隐藏库、来源分组/规则和目录 |

## 调用约定

独立媒体端口不包管理 API 的 `data` 信封。客户端按公共信息、登录、用户视图、目录、详情、PlaybackInfo、播放和进度上报的顺序使用。HTTP 头部和查询参数按 OpenAPI 及客户端协议校验。

管理 API 只位于管理监听端口；媒体端口不提供数据库、插件管理、账号恢复或任意文件访问。WebSocket、服务器管理和其他未提供的客户端能力不能通过别名绕过限制。

完整字段与状态码见[OpenAPI](../api/openapi.yaml)。
