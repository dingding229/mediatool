# 原生媒体客户端浏览序列矩阵

## 证据与边界

- 原有 `TestNativeClientSequenceInfuse` / `TestNativeClientSequenceSenPlayer` 是合成协议回放，不是真机抓包。后者使用的 `SenPlayer/1.0` 不能代表当前真机参数组合。
- F1 增补使用管理员私有生产日志中 **2026-10-13 至 2026-10-16** 的 SenPlayer 6.2.1 / 6.2.3 真实浏览参数。历史请求当时经过 Emby 网关；不能把它们描述为 2026-10-22 原生端口故障现场的完整抓包。原生端口基线未记录成功浏览的完整访问日志。
- 公共回归夹具只保留协议参数，使用合成 user/library/item ID 占位符；原始完整 URL、令牌、用户及媒体数据、响应正文和生产取证文件均留在私有目录，不提交或上传为 CI 工件。登录请求体不纳入取证日志。
- [真实参数夹具](../internal/modules/embyapi/testdata/client_sequences/senplayer-6.2.1.json) 标记 `observed` 与 `supplemental`，不把补充探测冒充真机请求。`TestF1NativeSenPlayerBrowseSequence` 经真实 HTTP 登录、PostgreSQL 目录和权限执行全部步骤，并以另一普通 UA 再回放一次。

## 浏览路径

`{user}`、`{movieLibrary}`、`{tvLibrary}`、`{movie}`、`{series}`、`{season}` 均在隔离测试中创建/发现；不固定真实业务 ID。

| 阶段 | 端点与关键参数 | 来源 | 断言 |
|---|---|---|---|
| 登录 | `POST /emby/Users/AuthenticateByName` | 合成测试身份 | 返回当前用户和设备令牌 |
| 库发现 | `/emby/Users/{user}/Views?IncludeExternalContent=false` | observed | QueryResult；CollectionFolder；CollectionType 为 movies/tvshows；Id 为稳定字符串 |
| 剧集最近更新 | `/Users/{user}/Items?ParentId={tvLibrary}&IncludeItemTypes=Series,Movie,Video,MusicVideo,MusicAlbum&Recursive=true&SortBy=DateLastContentAdded,DateCreated,SortName&SortOrder=Descending&StartIndex=0&Limit=30` | observed | 200；Series 非空；TotalRecordCount 正确 |
| 电影列表 | `/Users/{user}/Items?ParentId={movieLibrary}&IncludeItemTypes=Movie,Video&Recursive=true&GroupItemsIntoCollections=true&SortBy=DateCreated,SortName&SortOrder=Ascending&StartIndex=0&Limit=30` | observed | 200；Movie 非空 |
| 电影最近更新 | 与剧集最近更新同一组参数，ParentId 换电影库 | observed | 200；Movie 非空 |
| 电影详情 | `/Users/{user}/Items/{movie}` | observed | 单个 Movie DTO |
| 剧集详情 | `/Users/{user}/Items/{series}` | observed | 单个 Series DTO |
| 季 | `/Shows/{series}/Seasons?Fields=BasicSyncInfo,CommunityRating,ProductionYear,EndDate,Container&UserId={user}` | observed | Season 非空，SeriesId 正确 |
| 下一集 | `/Shows/NextUp?Limit=1&SeriesId={series}&UserId={user}` | observed | 正确 QueryResult；无继续观看历史时允许空 |
| 集 | `/Shows/{series}/Episodes?SeasonId={season}&UserId={user}`，保留真实 Fields/EnableImageTypes 组合 | observed | Episode 非空，SeriesId/SeasonId 正确 |
| 逻辑库 | `/Library/MediaFolders?UserId={user}` | supplemental | 与 Views 的可见库集合一致 |
| 最近更新数组 | `/Items/Latest?UserId={user}&ParentId={library}&Limit=20` | supplemental | 数组，电影/剧集分别非空 |
| ParentId 层级 | `/Users/{user}/Items?ParentId={series}&IncludeItemTypes=Season` → `ParentId={season}&IncludeItemTypes=Episode` | supplemental | 电影库与剧集库均可走通列表/详情；季集层级不脱离其剧 |

真实 Fields 包含 BasicSyncInfo、CanDelete、EndDate 等未知可选投影字段，继续按既有协议忽略不支持的投影字段；不会下载动态代码或打开转码能力。

## F1 修复与防退化

基线在 `embyapi.ParseQuery` 将标准的 Video/MusicVideo/MusicAlbum 类型及 DateLastContentAdded 排序判为非法，导致已有数据、有授权的请求仍为 **400**。这是通用 Emby 查询兼容问题，不是 SenPlayer 专属 UA 规则。

- 保留多类型过滤中的每一项；本目录不存储的标准类型仅匹配零行。**不能**删除未知类型后退化为未过滤的全库查询，也不能把 Video 偷换成 Movie。
- DateLastContentAdded 使用当前可见库内物理成员的最大 created_at；DateCreated 仍使用最早 created_at。`TestF1LastContentAddedUsesLatestVisibleMember` 验证二者顺序不同，其他库的新版本不污染当前 ParentId 排序。
- 参数名、类型值、排序名大小写不敏感；逗号分隔多值、分页、enableUserData=false 均可解析。任意未允许类型/SQL 排序片段仍拒绝。
- 测试覆盖仅 Video / MusicVideo,MusicAlbum 的空结果、Movie,Video 的非空结果、排除未存储类型不隐藏 Movie、授权撤销和 hidden 库拒绝访问。
- 不修改 STRM、PlaybackInfo、播放器 UA 取链、302、115 CDN 或播放上报实现。原有客户端播放序列继续验证直链边界。

## Emby 4.8 实测对照

本次对照使用一次性的本地 **Emby 4.8.11.0** 官方镜像、独立配置及人工生成的短视频。不得使用生产 Emby/CD2 配置或媒体卷建立这个测试实例。服务端与私有对照输出不属于应用发布内容；核验后删除任务容器。

实际对照：Views 返回两个 CollectionFolder，CollectionType 分别为 movies/tvshows，字符串 Id 为 `3` / `5`，ChildCount 各为 1；MediaFolders 返回相同两个库但省略 ChildCount。带标准多类型组合、DateLastContentAdded、分页的电影/剧集 Items 均为 200、TotalRecordCount=1；小写参数名/枚举值及逗号多值请求同样为 200。`/UserViews` 返回 404。

本程序沿用 CollectionFolder、movies/tvshows、数字字符串 Id 和正确 QueryResult 计数，但 Views 目前不投影库 ChildCount，enableUserData=false 也只校验参数、不关闭既有 UserData 投影；这些不是本次证实的 400 原因，本次未扩展到完整 Emby 投影实现。不要把 Emby 的 ID 字符串误写成只能 32 位 hex：本程序使用不复用的数字字符串，客户端必须按不透明字符串处理。`/UserViews` 是单独补充探测，不据名称臆造受支持路由。

## 本地与发布验收

- 所有改库测试串行，仅使用 media_tool_test 和 Redis DB15。改动包及 `go list -test` 求得的反向依赖使用 `-race`；没有前端修改或新迁移。
- 发布后，用原 SenPlayer UA 重放经过 ID 替换的真实查询，分别检查电影、剧集、季、集的条数；补查 Latest/NextUp/MediaFolders 和权限边界。
- 单独核对原媒体用户实际 grants。无授权库的空结果不能归因于协议，测试账号的扩展授权也不是更改原用户授权的理由。
- 真实播放验收只请求一次原始播放 302，不跟随/中转媒体正文。最后删除明确命名的临时媒体用户。
