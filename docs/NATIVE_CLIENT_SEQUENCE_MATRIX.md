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
- 参数名、类型值、排序名大小写不敏感；逗号分隔多值、分页、enableUserData=false 均可解析。F2 后未知类型保留零匹配语义，未知排序忽略；SQL 只来自固定映射。
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

## F2：STRM 媒体源与宽容查询（2026-10-22）

### 取证、审批和字段契约

隔离的官方 Emby 4.8.11.0、人工合成 STRM 的实测结果：详情和 PlaybackInfo 都把 STRM 内容 URL 放入 `MediaSources[].Path`。本程序修复前详情省略 Path；PlaybackInfo 的 Path 则带媒体用户令牌。仅服务端差异已证实，**未把客户端 UI 对某字段的依赖推测当作真机结论**。

生产 STRM 原文包含可独立播放的每文件 bearer grant；直接返回原文会绕过媒体用户授权生命周期。业主批准改用以下无凭证包装 URL，不再要求逐字等于 STRM 原文：

```text
https://native.synthetic.invalid/Videos/100001/stream.mkv?MediaSourceId=synthetic-source&Static=true
```

| 字段 | Emby 4.8.11 详情 / PlaybackInfo | 修复前本程序 | F2 本程序详情 / PlaybackInfo |
|---|---|---|---|
| Path | STRM 原文稳定 URL / 同左 | 详情缺失 / 带媒体用户令牌的 native URL | 配置媒体地址生成的无凭证稳定 native 包装 URL / 同左 |
| Protocol / IsRemote | Http / true | Http / true | 不变 |
| Type | Default | 缺失 | Default（固定能力） |
| Name | 条目名 | 版本 N | 条目名；版本用各自 Source Id 区分 |
| Container | strm / 探测出的 mkv | mkv / mkv | 保留原始媒体容器与探测结果，不改成 strm |
| SupportsDirectPlay / DirectStream | true / true | true / true | 不变 |
| SupportsTranscoding | true | false | **false，不引入转码** |
| DirectStreamUrl | 对照样本缺失 | 用户令牌的 native 相对 URL | 保留原有用户令牌的 native 相对 URL；Path 本身无令牌 |

保留详情 Container 为原媒体容器的取舍：它表达可 DirectPlay 的真实格式，与 PlaybackInfo 一致，避免客户端把 `strm` 当作不支持的播放格式；不改变 STRM 正文或原始媒体。无公网地址配置时媒体源响应失败关闭，不从 Host、X-Forwarded-* 或请求来源回退生成 Path。

### 服务端客户端序列回归

`TestF2ClientPathPlaybackAuthenticationSequence` 串行使用测试数据库、原始 STRM、真实 HTTP 鉴权和目录权限、合成官方取链响应，分别用 SenPlayer/6.2.1、Infuse/8、通用 UA：

1. 已授权详情 → PlaybackInfo；Path 同一稳定、无凭证地址，Type=Default，Name=条目名。
2. 客户端直接使用 Path：X-Emby-Token、X-Emby-Authorization、api_key、Authorization: MediaBrowser 四种正常媒体用户鉴权均为 302，响应无媒体正文；官方取链边界收到实际 UA。
3. 同一 Path 不带鉴权为 401；只有浏览权限时为 403；隐藏条目详情为 404。
4. 原 STRM 文件前后 SHA256 相同。无 relay、HLS、转码及临时 115 URL 写入 DTO。
5. Host/X-Forwarded 注入测试、DTO/golden 和播放序列同时覆盖此契约。

这些是协议序列测试，并不宣称控制了 SenPlayer/Infuse 真机。业主需在真机刷新该条目详情，查看**媒体信息底部播放链接**，再播放电影和一集剧集，确认正常；如客户端自行无鉴权请求 Path，应停止并报告，不能改回暴露凭据。

### 查询宽容与边界

| 请求条件 | 预期 |
|---|---|
| IncludeItemTypes=BoxSet,Trailer,Audio,Person,Playlist | 200 空集；不会退化为全部条目 |
| IncludeItemTypes=Movie,BoxSet | 只返回有权看到的 Movie |
| ExcludeItemTypes=Unknown | 忽略未知项，不改变已知项排除 |
| SortBy=Unknown,DateCreated | 忽略未知排序；DateCreated 使用原位置对应的 SortOrder |
| SortBy 全未知或未实现项 | 回退默认 SortName 升序，原输入不进入 SQL |
| SortOrder 单值 / 多值不足 / 多余 | 单值应用全部；缺位置默认 Ascending；多余位置忽略；**不去重** |
| Limit=500 | Items/Resume 上限 200，Latest/Hints 100，Similar 20，返回 200 |
| Resume 未知 MediaTypes | 未知项零匹配；与 IncludeItemTypes 求交集 |
| NextUp/Seasons/Episodes/Similar + 全未知 IncludeItemTypes | 保持空集，不被专用路由默认类型扩大 |
| 非法数值、非法 ID、过多输入、非法布尔/排序方向 | 仍可 400 |

隔离 Emby 对任意未知 SortBy 曾实测为 500；这里按业主批准的更安全语义忽略并回退，不复制该异常。各点位均含表驱动/SQL 集成测试，不依赖 UA 特判。

### F2 本机 SenPlayer 复验与审核补记（2026-10-22）

已在本机实际 SenPlayer 重新进入详情：电影、剧集媒体信息底部均出现无凭证包装链接；电影真实播放成功，服务端实际 SenPlayer UA 请求为302。部署前已打开的旧详情需退出重进，不能拿缓存页面判定新响应缺字段。BaseItem.Path 未改成原 STRM URL，也未为显示功能暴露文件授权。

该集原账号 `f:series` 的 `can_play=false`，真机403与权限一致。业主审核明确此项属于配置而非代码缺陷，F2通过；待业主自行开启播放权限后再复验该集，不阻塞F3。严禁擅自改写原账号 grants。真机截图及只读权限证据仅存私有 `data/private/f2/`。
