# 内置媒体库（Emby 兼容）

> **状态：P0/P1 已审核通过（T11–T19、T41、T42）；P2 的 T21 ffx 进程封装已实现待审核，T22–T26 未开始。播放仍未接入，既有开关、真实令牌与直链安全边界不变。** 本文是内置媒体库的唯一协议与边界文档；开发任务、阶段验收和 D1–D10 防缺陷清单见 [`docs/plans/20261016-native-emby-server.md`](plans/20261016-native-emby-server.md)。

## 用途与范围

内置媒体库使用 Go 在主程序 app 进程内运行，通过独立媒体服务端口提供 Emby 兼容接口，供 Infuse、SenPlayer 等客户端按“Emby 服务器”添加。它只暴露已确认的 STRM 媒体投影、TMDB 元数据和媒体用户能力，不暴露后台管理接口、云盘路径、pickCode、STRM 全局 apiKey 或管理员凭据。

所有媒体播放必须保持：实际播放器 UA → 现有 115/CloudDrive2 解析 → 302 到 CDN。内置媒体库不做服务器媒体中转、HLS、转码或 ffmpeg 播放。ffprobe/ffmpeg 只用于后台媒体信息预处理和单集截图，并通过 `internal/modules/mediaprobe/ffx` 受控调用。

## 端口与架构

- 端口：沿用 `MEMBER_ADDR`。2026-10-16 只读核对服务器 `/root/docker/mediatool/docker-compose.yml`：容器监听 `0.0.0.0:8092`，公网 `0.0.0.0:25018 → app:8092`；`8092` 是内部端口，不是服务器公网访问端口。本任务没有修改服务器配置。
- 公开地址优先级：后台域名设置 `application_settings.access.memberUrl` → `MEMBER_PUBLIC_URL` → 请求的 HTTP(S) Host（未配置时记录一次警告，不相信客户端随意提交的 Forwarded Host/Proto）。本次只读核对：`memberUrl` 为空，`MEMBER_PUBLIC_URL=http://185.99.135.205:25018`，暂未设置独立域名。
- 运行方式：与主程序、PostgreSQL、Redis 同属 app/数据库/缓存三容器架构，不新增媒体库容器。
- 路由：大小写不敏感；同时支持无前缀和 `/emby/` 前缀；查询参数名大小写不敏感。
- 认证：支持 `X-Emby-Authorization`、`Authorization: MediaBrowser ...`、`X-Emby-Token`、`X-MediaBrowser-Token` 和 `api_key`；媒体用户令牌只保存摘要，按设备签发并可撤销。

```text
Infuse / SenPlayer / 其他 Emby 客户端
                 │ 独立媒体端口（MEMBER_ADDR）
                 ▼
        internal/modules/embyapi
        路由、认证、DTO、分页、错误映射
                 │ 仅依赖数据库无关接口
        ┌────────┼────────────────────────┐
        ▼        ▼                        ▼
 nativelibrary mediausers/contracts   nativeplayback
 库/条目投影   用户/设备/权限契约     302 解析契约
        │        │                        │
        └────────┴─────────────┬──────────┘
                        后续任务注入实现
                               │
      classification / library / tmdb / jobs / playeraccess
                               │
                      PostgreSQL + Redis

后台预处理 jobs → mediaprobe → mediaprobe/ffx
                  ffprobe 信息 / 单集截图
                  （不进入 embyapi 或播放依赖图）
```

## 数据模型（T03 + T14b / 迁移 00055–00058）

本阶段只确定表、约束、索引与枚举常量，尚未接入库同步、认证、探测或播放业务；阶段内不向生产部署此迁移。`cloud_files.id` 为单列 UUID 主键，探测表可直接引用。

| 对象 | 主键与关系 | 主要字段、默认值与约束 |
|---|---|---|
| `native_item_seq` | 所有实体共用 bigint 序列 | 从 100000 开始、最小值 100000、NO CYCLE；允许空洞，运行时不得重置 |
| `native_item_ids` | bigint `id` 默认取共用序列；`(entity_type,internal_key)` 唯一 | `library/movie/series/season/episode`；内部键 1–512 字符；`retired_at` 为空或退役时间；映射只退役、不删除或复用 |
| `native_libraries` | 分类文本 `id`；唯一 `item_id` 引用 ID 表的 library 实体 | `g:<groupId>` / `r:<groupId>:<ruleId>`，ID 与分组/规则列必须一致；分组/规则标识 1–80 位字母数字/下划线/短横线；名称 1–256 字符；相对目录 1–1024 字符，拒绝绝对路径、遍历、反斜线、控制字符；`collection_type=movies/tvshows`；`hidden=false`、排序≥0、策略修订>0、创建/更新时间 |
| `media_user_legacy_grants` | `(user_id,media_type)`；用户删除级联 | 暂存 `movie/series` 浏览授权及原始 `can_play`；不在迁移中虚构新分类库 |
| `media_user_library_grants` | 保留 `(user_id,library_id)`，外键改为新库 | 保留 `can_play`；新增 `can_download=false`；库隐藏不删除授权；library_id 索引 |
| `native_asset_libraries` | `asset_id` 引用资产，删除资产级联；库外键 | `source=classification/output_directory`；策略修订、资产修订均>0；更新时间；库/资产索引 |
| `native_server_settings` | 仅 `singleton=true` 一行 | `server_id` 为唯一 32 位小写十六进制，迁移时生成并持久化；名称 1–100 字符；版本默认 `4.8.10.0`，四段数字、总长≤32；`enabled=false`、`probe_enabled=true`（Schema59，仅原生服务启用后生效）、`public_user_list=false`、`revision=1` |
| `media_users` 新增列 | 保留账号主键与全部既有凭据 | `max_streams=2`、范围 1–20；`max_parental_rating` 可空、范围 0–100；`expires_at` 可空 |
| `media_user_devices` | UUID；用户外键级联；`(user_id,device_key)` 唯一 | DeviceId 1–256 字符；名称≤256、客户端≤128、版本≤64、UA≤2048；IP 使用 inet（IPv4/IPv6）；创建/最后活动时间；按用户/活动时间索引 |
| `media_user_tokens` | 32 字节摘要主键；用户及同用户设备外键级联 | 不存明文 Token；凭据版本>0；创建/活动/到期/撤销时间，到期晚于创建；未撤销用户、设备、到期时间索引 |
| `media_user_item_data` | `(user_id,item_id)`；用户删除级联；条目外键 | `position_ticks≥0`、`play_count≥0`，已看/收藏默认 false，最后播放/更新时间；条目、续播、收藏与历史索引 |
| `media_probe_results` | `cloud_file_id` 引用文件，文件删除级联 | Schema59：目标 `file_fingerprint`；`pending/probing/ready/failed/needs_action`；attempts、next_attempt_at、probing_started_at、lease_until/lease_token；错误只存枚举class/reason，不存原文；独立成功快照 `result/result_fingerprint/probed_at`，结果≤4MiB；尝试失败不清空快照 |
| `media_stills` | `item_id` 只引用 episode 实体 | `pending/grabbing/ready/failed/skipped`；尝试次数≥0、下次尝试时间、`grabbing_started_at`；相对文件路径≤1024 字符，拒绝 `..`、绝对路径、反斜线、冒号与控制字符；摘要为空或 64 位小写十六进制；宽高 0–16384；ready 必须有路径、摘要与正宽高；错误≤8192 字符 |
| `native_playback_sessions` | UUID；用户、同用户设备、条目外键 | 媒体源 ID 1–512 字符；`playing/paused/stopped`；开始/进度/结束时间、非负进度；仅 stopped 有结束时间；未停止用户与活动时间部分索引、设备/条目索引 |
| `native_tmdb_details` | `(tmdb_type,tmdb_id,season_number,episode_number,language)` 通过生成 `identity_key` 唯一 | `movie/series/season/episode`；payload 为≤4 MiB JSON 对象；`pending/fetching/ready/failed`；fetching 必须有开始时间，ready 必须有完成时间；失败重试与 30 天过期索引；不保存 Token、路径或播放地址 |

- **枚举单一契约**：实体、库类型、TMDB 详情类型/状态与资产归属来源在 `internal/modules/nativelibrary/states.go`；探测/截图状态在 `internal/modules/mediaprobe/states.go`；旧媒体类型与播放会话状态在 `internal/modules/mediausers/states.go`。D2 测试读取全部 Go 字符串常量，与按迁移顺序生效的全部枚举 CHECK 做完整集合比较。
- **授权迁移**：迁移事务先锁定账号及旧授权表，锁等待上限 5 秒、单条语句上限 60 秒；锁超时整轮回滚，避免快照与删除之间的并发写入丢失。Up 将旧 `movies/series` 授权连同 `can_play` 移至 legacy 表，再删除旧授权行和固定库表。T11 首次同步应在同一事务里按媒体类型向所有匹配分类库展开授权，再删除相应 legacy 行。用户、密码哈希、凭据修订以及旧会话表保留。
- **类型与归属约束**：库条目与截图条目通过固定生成列和复合外键分别限制为 library / episode。Token 和播放会话的 `(user_id,device_id)` 必须引用同一用户的设备，不能跨用户绑定。
- **恢复预留**：probing / grabbing 必须有开始时间，且对应开始时间有部分索引；播放会话有最后进度索引。T23-A已提供探测lease过期批量回收仓库方法，运行时调度留给T23-B；截图/播放的收敛仍分别留给T25/T34。
- **Down 边界**：完整回退新表、列、索引和序列；未展开的 legacy 授权可原样还原。已经展开或编辑的新分类授权、原生用户进度/设备/Token/探测数据无法无损还原到旧模型，Down 会删除这些新格式数据，回退前必须备份，不推断扩大授权。普通服务重启/重复 Up 不改变 ServerId 或授权。
- **阶段限制**：旧 `mediaserver` 仓库仍引用固定库表；本任务不改业务查询。新库同步与调用方切换必须在后续任务中完成，不能以 T03 单独上线。

## DTO 与序列化契约（T05 锁定；T07 仅公共信息接入）

- 定义位于 `internal/modules/embyapi/dto`；只依赖 Go 标准库，不查询数据库或调用媒体处理器。
- 字段名保持 Emby PascalCase；所有 ID 字段为字符串。可选的零值数值（季/集索引、播放位置、计数等）使用指针区分缺失与显式 0；Ticks 使用 int64，不经过 float64。
- 数组使用 `Array[T]`：nil 也序列化为 `[]`，不会输出数组字段 `null`。可选空数组可使用 `omitempty` 完全省略。
- `UTCDateTime` 固定输出 `YYYY-MM-DDTHH:MM:SS.fffffffZ`。构造时转换 UTC 并显式截断至 100ns；解析允许 RFC3339 时区偏移，但拒绝低于 100ns 精度的非零尾数，输出始终使用上述唯一格式。
- BaseItem 类型只允许 CollectionFolder、Movie、Series、Season、Episode；其中 CollectionFolder / Series / Season 是文件夹；MediaType 恒为 Video、LocationType 恒为 FileSystem。不一致的对象无法序列化。
- `BaseItemDto.Path` 可省略；若输出，必须是合成的 `<标题>.<扩展名>` 叶文件名，**任何位置都不允许 `/` 或 `\`，没有开头例外**。`MediaSourceInfo.Path` 则必须由 T31 唯一构造函数生成的本端口 `http(s)` 绝对播放地址，路径以 `/Videos/` 开头并带 `Static=true`、`MediaSourceId`、用户作用域 `api_key`；两者都拒绝 STRM 正文、云盘路径、pickCode、全局 apiKey、115 CDN 地址和路径遍历。
- T31 的 `nativeplayback.StreamURLs` 唯一构造函数同时生成绝对 Path 与相对 `DirectStreamUrl`（`/Videos/...`）；DTO 校验两者 RequestURI 必须一致。T31 已在唯一入口绑定公开地址与当前 T41 用户令牌，组合根还显式拒绝全局 STRM key（即使手工 token 表出现同摘要也拒绝）。
- MediaSourceInfo 的能力通过只读方法提供：Http、远程、可 DirectPlay/DirectStream，不可 Transcoding、不需要 Open/Close。序列化不依赖调用者设置；零值也不能打开转码，恶意 JSON 能力覆盖会被拒绝，TranscodingUrl 没有可写字段且永不输出。
- 项目中的 PlayRequest 是 `/Sessions/Playing` 开始上报体（对应 Emby PlaybackStartInfo 的共同字段），不是远控播放命令。开始/进度/停止上报解码忽略未知字段、字段名大小写不敏感，整数接受数字或字符串数字；拒绝溢出、小数、冲突的大小写别名及超过 64KiB 的报告，失败不部分修改对象。
- 合成 golden 位于 `internal/modules/embyapi/dto/testdata/emby/`，每种 DTO 都有 full/minimal；默认测试只读。显式更新命令：`go test ./internal/modules/embyapi/dto -run '^TestGoldens$' -update`。
- 测试同时执行规范化 JSON 精确对比、结构体反序列化后的再次序列化、递归结构检查（PascalCase、数组类型、字符串 ID、整数 Ticks、UTC 日期）。空切片 nil 与空数组按相同的线协议语义处理。

## 领域接口（T06，空实现）

`internal/modules/embyapi` 只依赖以下数据库无关的领域契约；组合根注入实现，当前默认实现全部安全失败，不访问 PostgreSQL/Redis、不监听端口、不读取媒体。

| 包/接口 | 责任 | 任务 |
|---|---|---|
| `nativelibrary.LibraryService` | `ListLibraries` / `GetLibrary`，按媒体用户列出/读取逻辑库 | T11/T17 |
| `nativelibrary.BrowseService` / `catalog.BrowseRepository` | `Views` / `Query` / `Counts`；T17-A 统一 SQL 分页与批量 DTO 装配；T17-B 已接通用 HTTP | T17-A/T17-B |
| `nativelibrary.DiscoveryService` / `catalog.BrowseRepository.Discover` | 专用层级、Latest、Hints、Genres/Filters/Similar | T18/T19 |
| `nativelibrary.ItemService` | `Query` / `Get` / `Seasons` / `Episodes` / `Latest` / `Search` | T12–T19 |
| `nativelibrary.UserDataService` | `Get` / `Update` / `MarkPlayed` / `MarkFavorite` / `Resume` / `NextUp` | T34–T35 |
| `nativelibrary.LibraryDTO` / `MovieDTO` | T14 纯函数：库与电影 → Emby DTO，不做外部调用 | T14 |
| `nativelibrary.ItemDTO` / `UserDataDTO` | 其余映射仍为未实现，后续按任务接入 | T15/T17/T35 |
| `nativelibrary.EnsureIDs` / `catalog.IDQueries` | 数据库无关 ID 批量契约与 pgx 适配，批量≤500 | T13 |
| `catalog.MovieRepository.Query` | 内部按库查询，聚合可暴露电影版本；T17 调用前必须授权库，不直接作为 HTTP handler | T14/T17 |
| `catalog.HierarchyRepository` / `nativelibrary.HierarchyService` | 剧/季/集层级投影与可见库过滤；T17/T18 接入 HTTP 前复用 | T15/T17/T18 |
| `nativelibrary.ItemQuery` | 与 T04 `embyapi.Query` 字段一一对应的安全查询值 | T06/T17 |
| `mediausers/contracts.TokenAuthenticator` / `nativeauth.Store` | 登录、令牌摘要、设备绑定、滑动续期、撤销与请求级校验 | T41 |
| `mediausers/contracts.PermissionChecker` | `VisibleLibraries` / `CanBrowse` / `CanPlay` / `CanDownload` / `ItemAllowed` / `ParentalAllowed` / `StreamSlots` | T42 |
| `mediaprobe.Prober` | 受控媒体信息预处理结果 | T23 |
| `mediaprobe.StillGrabber` | 受控单集截图结果 | T25 |
| `mediaprobe/ffx.Runner` | `Probe` 原始 JSON / `Still` 单帧 JPEG，固定 argv、进程组取消、有界 I/O 与全局并发 | T21/T25 |
| `nativeplayback.StreamResolver` / `nativeplayback.Service` | 原接口保留兼容；新Info/Stream契约只允许媒体信息或302目标，无媒体字节 | T31/T32 |

接口约定：
- `Library.ID` / `Item.LibraryID` 使用分类库内部键（`g:...` / `r:...`），`Library.ItemID` / `Item.ID` 及层级 ID 使用不复用的原生数字字符串。`Item.Hierarchy` 表达剧/季/集关系；`Episodes` 的季筛选由 `ItemQuery.ParentID` 传入。实际投影与权限过滤在 P1 实现。
- `ItemQuery` 与 T04 的 13 个字段一一对应，转换时复制切片和可选布尔指针；fixture 位于 `embyapi/testdata/contracts/item-query.json`。SearchTerm/Genres/Years 的 HTTP 参数扩展留待 T17/T19，本任务不擅改 T04 查询契约。
- 构造函数 `NewLibraryService` / `NewItemService` / `NewUserDataService` / `NewProber` / `NewStillGrabber` / `NewStreamResolver` / `NewTokenAuthenticator` / `NewPermissionChecker` 提供无副作用空实现：返回空值和各包 `ErrNotImplemented`，不把未实现写入误报为成功；`embyapi.NewServices` 组合这些接口，未接入监听。
- 认证契约放在 `mediausers/contracts`，`mediausers` 根包导出接口别名与构造函数；协议层不引入包含 pgx 仓库的根包。既有可信代理/IP/UA 规则提取至 `playeraccess/clientinfo`，根包保留原 API 包装，不更改信任边界。
- `StreamSlots` 返回容量快照，不代表成功预留；T42 仅统计最近 5 分钟的活动会话，T34 必须原子执行真正的并发会话预留与超时收敛。Token 长期续期、撤销及无进度会话收敛仍按已锁定的 T41/T34 要求实现。
- `RedirectTarget` 只有目标 URL 和到期时间，均不参与 JSON 序列化；没有媒体字节或可读流。T32 已复用现有 playback/storage 解析及 playeraccess 白名单，失败不得中转。
- T21 已实现 `ffx.New(Config)`、包级及 Runner 的 `Probe(ctx,url,Options)` / `Still(ctx,url,offset,Options)`，原任意 `Run(args)` 占位入口已移除；不是旧格式兼容层。只有 mediaprobe 及其子包允许导入 ffx，增加全仓非测试源文件 AST 扫描（覆盖当前平台不编译的构建标签文件）；embyapi 全依赖图仍不得出现 pgx/redis/ffx/os/exec。

`nativeplayback` 错误到 Emby HTTP 状态的约定：权限不足/UA 拒绝/分级限制/并发超限为 403，媒体不存在为 404，上游失败为 503，未实现为 501，其他内部错误为 500。`embyapi.DomainErrorStatus` 只输出状态码，禁止把底层错误文本返回给客户端。

## T13 稳定条目 ID（内部服务，未接 HTTP）

`EnsureIDs(ctx, q, []Key)` 返回 `map[Key]int64`，每批最多 500 个输入 Key，先验证/排序/去重，以单条批量 `INSERT ... ON CONFLICT DO NOTHING` 分配，再通过独立 `SELECT` 回读。分两条语句是有意的：READ COMMITTED 下等待并发冲突结束后，第二条语句的快照能读到另一事务刚提交的 ID；同一批按固定顺序写入，避免交错批次反向占锁。

| entity_type | internal_key（锁定） |
|---|---|
| library | `g:<groupId>` / `r:<groupId>:<ruleId>` / `f:movie` / `f:series` |
| movie | `tmdb:<tmdbId>` |
| series | `tmdb:<tmdbId>` |
| season | `tmdb:<seriesTmdbId>:s<季号>`（季号可为 0） |
| episode | `tmdb:<seriesTmdbId>:s<季号>e<集号>`（集号≥1） |

- 媒体实体只从可暴露资产证据分配 ID；同一电影的多个文件/版本共用一个 Movie ID，以资产 ID 区分 MediaSource。电影和剧即使 TMDB 数字相同，因 entity_type 不同而拥有不同 ID。
- 多集文件如 S01E01-E02 只投影首集 `...:s1e1`，`IndexNumberEnd=2`，不另外分配第二集条目；本任务增加 DTO 可选字段并锁定规则，剧集实际投影留给 T15。不会修改 STRM 文件名、正文或路径。
- ID 永不删除/复用。default worker 复用现有 cron，每轮只收敛最多 500 个已分配媒体 ID（单次 3 秒预算）；实体全局完全没有可暴露资产时标记 `retired_at`，其他库仍有可暴露版本则不退休。重新出现/Ensure 时清除退休标记。按 ID 游标遍历，进程重启从 0 幂等重扫，不引入必须人工恢复的中间状态。

## T14 电影与库投影（内部服务，未接 HTTP）

### 已持久化元数据来源

- 身份与主标题/年份：`media_items.media_type/tmdb_id/title/release_year`。
- `media_items.metadata` 保存 `metadata.Item`：`originalTitle`、`overview`、`genres`、`posterPath`、`backdropPath` 等已有字段。`sortName` 缺失时用标题；`premiereDate`、`communityRating`、`officialRating`、`imdbId` 若已存在则读取，否则留空。当前 TMDB adapter 的普通持久化转换不保存这四个扩展字段，`metadata.Item.Scrape` 为 `json:"-"`，不可假定刮削临时数据已进入数据库；本任务不改变 TMDB 存储格式，也不在浏览时请求 TMDB。
- `media_probe_results` 结果读取留待 T24；查询中暂不虚构时长。纯映射接受已知的可选 `RunTimeTicks`，0 值可输出。

### 查询与输出边界

- `catalog.ExposableAssetsSQL` 是 T12 归属、库/资产列表、T13 生命周期与 T14 电影查询共用的证据定义：ready binding + present cloud file + 至少一条有效 STRM 输出。查询再要求 `native_asset_libraries` 归属到所查库且库未隐藏。
- `catalog.MovieRepository.Query` 是内部仓库服务，必须给定一个库 ID，ParentId 为当前库的数字 CollectionFolder ID；多个库中的同一电影 ID 一致，但媒体源和 DateCreated（该库内最早可暴露资产时间）均按当前库的资产计算，防止跨库泄露媒体源。T17 必须先经过库授权再调用；本任务不接 HTTP、不改变现有 401/503 安全门。
- 单页 1–200 条、StartIndex 0–1,000,000，单次 4 秒预算；每电影最多 100 个媒体源（数据库最多读取 101 个以检测超限，超限明确报错而非静默截断）。支持 SortName、DateCreated、ProductionYear、CommunityRating 双向排序，同值以 TMDB ID 升序稳定排序。
- `MovieDTO` / `LibraryDTO` 为纯映射；电影字段使用 PascalCase，数字 ID 为字符串，日期固定 UTC 七位小数带 Z；UserData 暂给未播放/未收藏空值对象，T35 接用户数据。数组保持 `[]`，可选未知字段省略。
- 图片路径只用于 SHA-256 摘要生成 ImageTags / BackdropImageTags，实际图片端点由 T16 实现。BaseItemDto.Path 仅为清理后的合成叶文件名（无目录），MediaSource 不携带云盘路径或 STRM 正文；播放 URL 唯一构造函数留给 T31，本任务媒体源 Path 保持空。
- 契约样例：`internal/modules/nativelibrary/testdata/projection/{movie,library}-{full,min}.json`；重新生成使用 `go test ./internal/modules/nativelibrary -run '^TestNativeProjectionGoldens$' -update-native-projection`，默认只比较、不覆盖。T05 DTO golden 同步覆盖 SortName 和 IndexNumberEnd。

## T41 Emby 认证与设备令牌

- 注入 `mediausers/nativeauth.Store`，协议包只依赖 `mediausers/contracts.AccountService`，不引入 pgx/Redis。T16 之后先完成 T41、T42，再接 T17–T19 浏览端点。
- `POST /Users/AuthenticateByName` 接受大小写不敏感的 Username/Pw，兼容 Password 字段；同次提交冲突密码字段拒绝。密码沿用既有 Argon2id 和 6–20 字符规则，原样保留空格和中文，不额外采集登录 body。
- 256 bit 随机令牌，以 64 位十六进制返回；**明文只在成功登录的 AccessToken，以及T31按协议构造的用户作用域播放URL中返回**。DB 仅保存 SHA-256 摘要，日志不打印令牌、认证头、登录 body 或含令牌的查询串，只记摘要前 6 字节。User/Device/Token 请求上下文均禁止 JSON 序列化。
- 每个受保护请求直接查 token→user→device，检查 enabled、expires_at、must_change_password、credential_version、token 撤销/过期；不做认证短缓存，因此顺序完成的禁用/改密/撤销下一请求立即失效。请求路径中的 userId 和查询/头中的 UserId 不得指向其他用户，返回 403。
- `(user,DeviceId)` 设备登录复用设备行，更新客户端名称、版本、UA、实际客户端 IP，撤销同设备旧令牌。令牌 365 天有效，每 10 分钟最多更新一次 last_seen/expires_at；图片可无 DeviceId 头，令牌仍按已绑定设备查询，提交不同 DeviceId 则拒绝。
- 本人 POST Password 成功后，用户 credential_version 递增；当前令牌更新到新版本并续期，其他设备令牌及旧网页会话撤销。POST Logout 撤销当前令牌并返回 204；没有通过认证的请求不能改密或登出他人令牌。
- 后台创建/重置密码默认是管理员设置的最终密码，不强制首次改密；保留“要求修改密码”选项。该标记为 true 时 Emby 登录返回 401 和管理员改密提示，不自动清除已有标记。
- 登录失败复用 media_user_auth_limits 的 HMAC scope 与一分钟窗口：IP 20 次、用户名 10 次，达到限制返回 429。成功登录不调用失败计数；Argon2 处理并发≤2。
- Users/Public 默认 []；启用 public_user_list 才返回启用且未过期用户的 Name/Id/HasPassword（最多500），不包含策略、密码哈希或 Token。GET Users/{id} 只返回令牌所属用户的最小 UserDto。

## T16 图片端点

- 已注册 `GET/HEAD /Items/{id}/Images/{type}` 与 `GET/HEAD /Items/{id}/Images/{type}/{index}`，支持 `Primary`、`Backdrop`、`Thumb`、`Logo`、`Banner`；尺寸参数大小写不敏感，未知参数忽略。
- 图片只由服务端根据数字条目 ID、实体类型和已暴露资产查询 TMDB 路径；客户端不能提交远程 URL，因此没有 SSRF 路径。CollectionFolder 暂不提供图片，统一 404。
- Movie/Series 使用 TMDB poster/backdrop/logo；Season 使用季海报并回退剧海报；Episode 使用 TMDB still，并回退季/剧海报。`media_stills` 表的 ready 读取接口留给 T25，截图生成不在本任务实现。
- 匿名图片访问是有意兼容 Infuse/SenPlayer 的选择：仅返回已暴露条目的公共 TMDB 图片字节，不返回条目元数据、路径、播放地址或凭据；未暴露/不存在统一 404。T41 已加入有效令牌识别和匿名按 IP 限速；库级权限由 T42 的统一检查器判定（匿名公共海报的既定边界不变）。
- 匿名图片每 IP 每分钟最多 600 次（计数器上限10000个IP，过期回收），超限429；带**有效**媒体令牌不受匿名额度限制，伪造令牌不能绕过。使用既有可信代理规则决定客户端IP，不能通过伪造 X-Forwarded-For 更换额度。
- 仅请求 tag 与当前 ImageTag 完全一致时返回 `public, max-age=31536000, immutable`；无 tag 或旧 tag 返回 `public, max-age=3600`，仍返回当前图片。
- 图片请求使用 TMDB 范围代理、Redis 缓存和 singleflight；尺寸只映射到 TMDB 原图尺寸，不在服务端缩放。ETag 使用与 T14/T15 `ImageTag` 完全相同的路径摘要；tag 不匹配当前图片时仍返回当前图片，If-None-Match 命中返回 304，HEAD 不返回 body。

## T15 剧集投影（服务层，未接 HTTP）

- `catalog.HierarchyRepository` 实现 `nativelibrary.HierarchyService`，所有查询接收 `VisibleLibraries`；请求库必须属于允许集合，T42 由请求级权限服务生成该集合。当前不接 Emby HTTP，T17/T18 再接入。
- 剧按 TMDB ID 聚合；当前库至少有一个可暴露集才显示，`ParentId` 为当前库 CollectionFolder。季只显示有可暴露集的季，季 0 固定为“特别篇”并输出 `IndexNumber=0`。集从资产 `media_identity.episodeDetails` 投影，缺失时使用绑定的 season/episode 作为最小身份，不调用 TMDB。
- 同集多个资产版本合并为一个 Episode 条目并输出多个 MediaSource；多集文件只生成首集条目，`IndexNumberEnd` 为覆盖末集，后续集不重复生成。跨库同集继续使用同一 `(entity_type,internal_key)` 数字 ID，各库只改变 ParentId 和可见 MediaSources。
- 剧整体信息来自 `media_items` 与 ready 的 `native_tmdb_details(series)`；季信息来自 `native_tmdb_details(season)`，无快照时回退为季号/特别篇名称。集名称、简介、日期和 still 来自 `media_identity.episodeDetails`，runtime 优先使用资产信息、缺失时使用季快照 runtime。
- 剧支持 SortName、DateCreated、ProductionYear、CommunityRating、DatePlayed（T35 前回退 DateCreated）；集按季号、集号升序稳定排序。所有列表分页上限 200，源资产读取有 2000 条上限。
- `Series/Season/EpisodeDTO` 是纯映射；剧填充季数/集数和 `UnplayedItemCount=0` 占位，季/集使用指针保留季 0 等有效零值。图片只输出 TMDB/still 摘要标签，不输出路径或播放凭据。
- `ListEpisodes` 先在 SQL 中对 `(season,首集号)` 集键分页，再读取本页资产；每集最多取最新 100 个版本，超出时在首个 MediaSource 名称中标记限制。series 快照每次请求只读取一次，season 快照按本页季号各读取一次；不会因物理资产数或 4 MiB payload 重复传输而阻塞长篇剧。
- golden 位于 `internal/modules/nativelibrary/testdata/projection/*-hierarchy.json`；数据库集成覆盖可见库、计数、特别篇、多版本、多集、跨库 ID 和缺失季详情回退。

## T14b TMDB 元数据补全（迁移 00058）

### 现有字段盘点

| 字段 | 来源/状态 |
|---|---|
| 电影/剧集 TMDB ID、标题、媒体类型、年份 | `media_items.tmdb_id/title/media_type/release_year`，已持久化 |
| 电影/剧集简介、原始标题、题材、海报/背景路径 | `media_items.metadata` 中的 `metadata.Item` 字段，已持久化 |
| 电影首映日期、投票评分、分级、IMDb ID、runtime、演员/导演/编剧、logo | 原有 `metadata.Item` 未完整保存；T14b 存入 `native_tmdb_details.payload`，ready 后供投影使用 |
| 剧集首播日期、评分、分级、外部 ID、演员/主创、图片 | 原有整剧 metadata 未完整保存；T14b 存入 `native_tmdb_details` 的 `series` 快照 |
| 季名称、简介、季海报 | 当前只在 `metadata.ScrapeBundle.Season` 和 NFO/刮削输出流程中使用，未形成可复用的季详情记录；T14b 使用 `season` 快照 |
| 单集名称、简介、播出日期、runtime、still 路径、单集 TMDB ID | 已保存在资产 `media_identity.episodeDetails`；`docs/EPISODE_METADATA.md` 规定季详情一次获取并按显式集号校验；T14b 另保留 `season` 快照供后续投影复用 |

### 运行边界

- 只为 T12 可暴露资产涉及的电影、剧集和季排队，不全量扫描 TMDB；新资产由现有周期调度优先发现，资产绑定完成后也会合并排队。
- `native_tmdb_details` 的身份键由类型、TMDB ID、季号和语言组成；重复排队不会堆积任务。ready 数据 30 天后可刷新；running 时再次请求设置 jobs payload 的 `rerun=true`，由既有 jobs 完成逻辑原子重排。
- TMDB 请求复用 `integrations.Manager.TMDB`，因此使用已有 Token、语言和 `networkproxy` 的 TMDB 范围代理；进程级并发不超过 2，请求有超时，429 使用有界指数退避。失败不影响浏览，字段保持已有值/空值。
- fetching 超过 1 分钟由调度收敛为 failed 并重新安排；payload 上限 4 MiB。数据库只保存规范化 TMDB 字段，绝不保存 Token、Cookie、115 路径、STRM 正文或播放地址。
- 电影投影读取 ready 快照；探测 runtime 优先，TMDB runtime 仅在没有探测结果时兜底。浏览热路径不请求外部 TMDB。当前手动刷新契约为 `POST /api/v1/media-server/native/tmdb/refresh`，只接受已暴露的 movie/series/season 身份并返回队列 ID。

## T12-R3 分类策略解码兼容

`classification.DecodeRecord` 保持线上兼容语义：顶层 JSON `null`、缺失 `groups` 或 `groups:null` 使用默认分组；对象内分组/规则省略字段可能继承默认分组/规则元素字段，这是已知兼容问题，留待后续单独评估。本次只读生产核对仅汇报形状：当前值为顶层 JSON `null`，没有顶层键；未输出策略内容。旧/新解码对 policy、合成 Match、Render 的结果均一致。

## T12 资产归属

- 可暴露资产必须同时满足：`media_asset_bindings.state='ready'`、`cloud_files.state='present'`、存在有效 `.strm` `strm_entries`。未确认、缺失、回收或无 STRM 输出的资产不会进入 `native_asset_libraries`。
- 归属使用实际 STRM 输出目录的路径段最长前缀匹配；规则字符串前缀不会误匹配相似目录。分类匹配仅用于写入诊断，不覆盖输出目录归属。无分组/规则目录匹配时按媒体类型使用 `f:movie` / `f:series` 兜底库。
- `native_asset_libraries` 记录 `output_directory`、`policy_revision`、`asset_revision` 和诊断 JSON；T12 通过数据库触发器将绑定、STRM 输出和云文件状态变化转成 `native.assign` jobs，幂等键为 `native.assign:<assetId>`；变化合并排队，已完成/失败/重试任务重置为 pending，running 任务设置 rerun 并在完成后原子重排（T12-R1）。全量重算按最多 500 个资产一批、使用 ID 游标可中断恢复。

## T11 分类到库同步

- `native_server_settings.library_granularity` 支持 `rule`（默认：每个启用子规则一个库，无启用子规则时使用分组库）和 `group`（每个启用分组一个可见库，历史规则库隐藏）。
- 库 ID 使用 `g:<groupId>` 或 `r:<groupId>:<ruleId>`，通过 `native_item_ids` 稳定分配 `itemId`；分类禁用/删除只隐藏库，不删除 ID 或授权，重新启用复用原 ID。
- 目录使用与 `classification.Render` 相同的分组/规则相对目录；显示名重复时追加 ` · <分组名>` 消歧。
- 分类保存、粒度变更和启动同步均在单个事务内读取策略/设置并更新库。首次同步将 legacy movie/series 授权展开到当前可见电影/剧集库并删除对应 legacy 行；之后新库默认无授权。分组有启用子规则但均未命中时，资产按 T12 实际输出目录进入相应兜底库。
- 管理端 GET `/api/v1/media-server/native/libraries` 返回分页库列表和每库 `assetCount`；GET `/api/v1/media-server/native/libraries/{id}/assets` 返回资产标题、STRM 输出路径、归属来源和诊断信息；媒体端不会暴露这些接口。

## T07 运行与冒烟边界

- `cmd/media-tool` 的 MEMBER_ADDR 已使用 `NativeMediaHandler`，完全独立于管理 handler；保留 5 秒 ReadHeaderTimeout、20 秒 ReadTimeout、30 秒 WriteTimeout、60 秒 IdleTimeout、32KiB MaxHeaderBytes。302 响应很小，无须放宽 WriteTimeout。
- `settings.NativeServerRepository` 从真实 `native_server_settings` 读取 ServerId、ServerName、Version、enabled 等字段，另读取后台 memberUrl；250ms 短缓存，单次数据库读取最多 1.5 秒，过期后读取失败不使用旧 enabled=true 快照。没有后台轮询 goroutine，缓存命中无全局锁/事务；未来 T45 的写入入口须主动失效或遵守该生效窗口。
- 测试命令：`scripts/test-native-emby.sh`。脚本使用全项目数据库测试锁，只允许 `media_tool_test`，不加载生产 `.env`；在本地临时 TCP 端口启动同一 NativeMediaHandler，遍历本页媒体端点（无前缀、/emby、/mediabrowser 与 GET 对应 HEAD），再测试开关和端口隔离。不会访问生产或启动业务 worker。
- 管理端不接入本套原生 Emby handler；既有 `/emby/` 外部 Emby 网关挂载保持原样，不在本任务删除。

## 阶段状态

| 阶段 | 范围 | 状态 |
|---|---|---|
| P0 | 协议框架、端口接入、空实现、契约骨架 | 已审核通过 |
| P1 | 目录分类映射、库与条目浏览、TMDB 图片 | T11–T19、T41/T42 已审核通过 |
| P2 | STRM 预处理、ffprobe 信息、单集截图 | 后端 T21–T25 已审核通过；T26 管理界面/API 已审核通过。生产发布及 AMD64 验证留 R2 |
| P3 | PlaybackInfo、302 播放、字幕、播放进度和用户数据 | T31/T32 已审核；T33/T34 已审核；T35/T36 已实现待本组审核 |
| P4 | Emby 登录、媒体用户、设备、会话和权限 | T41–T45 已审核，P4完成 |
| P5 | 客户端启动兼容、缓存、安全与负载加固 | 本地实现与验证完成，待审核；4.8 HDR/真机证据未核实 |
| R2 | 全量测试、AMD64 镜像发布、服务器只读验收 | 未实现 |

## Emby 端点清单

### 媒体端口端点（T07 已注册）

Public Info、GET/POST Ping、图片与 T41 认证端点已实现；通用 Items/Views/Counts 浏览已接入，层级/发现端点已接入；播放仍未接入。`native_server_settings.enabled=false` 时，除 Public Info/Ping 外的已注册端点统一 503。开启时，除 Public Info/Ping、Users/Public、AuthenticateByName 与图片外，其余端点要求真实媒体用户令牌；后台 Cookie、管理员会话或 STRM 全局 apiKey 均不算媒体用户认证。OPTIONS 只返回不带凭据 CORS 的方法声明。

表中的 `{userId}`、`{itemId}`、`{id}`、`{type}`、`{index}`、`{ext}` 是参数；`*` 是 Localization 的末尾路径通配。GET 自动支持 HEAD，不写正文。媒体端口不提供管理、工作台或 PWA 路径，未知路径始终 404，不因数据库或开关状态改变。

所有空列表模板为 `{"Items":[],"TotalRecordCount":0}`；单条模板 404、写模板 501。这些模板不能越过前述认证/开关保护。

| 方法 | 路径 | 阶段/任务编号 | 状态 | 备注 |
|---|---|---|---|---|
| GET | `/System/Info/Public` | P0/T07、P5/T51 | 已实现 | 公共服务器信息 |
| GET | `/System/Info` | P4/T45、P5/T51 | 已实现 | 认证后信息，ServerName/地址同公共发现；不支持重启/更新 |
| GET | `/System/Ping` | P0/T07、P5/T51 | 已实现 | JSON 字符串，值为数据库中的 ServerName |
| POST | `/System/Ping` | P0/T07、P5/T51 | 已实现 | 与 GET 相同；不收集请求体 |
| GET | `/Branding/Configuration` | P5/T51 | 已实现 | 本地空品牌配置，无任意CSS/远程资源 |
| GET | `/System/Endpoint` | P5/T51 | 已实现 | 匿名、保守网络默认值 |
| GET | `/Branding/Css` | P5/T51 | 已实现 | 本地空 CSS |
| GET | `/Branding/Css.css` | P5/T51 | 已实现 | CSS 别名 |
| GET | `/DisplayPreferences/{id}` | P5/T51 | 已实现 | 用户认证后只读默认值 |
| GET | `/Users/{userId}/Configuration` | P5/T51 | 已实现 | 仅当前用户最小配置 |
| GET | `/Sessions` | P5/T51 | 已实现 | 当前用户合法空列表，不暴露他人会话 |
| GET | `/DisplayPreferences` | P5/T51 | 已实现 | 认证后只读默认值；不承诺POST持久化 |
| GET | `/Localization/*` | P5/T51 | 已实现 | 合法空本地化目录资源 |
| GET | `/socket` | P5/T51 | 已实现拒绝 | 认证后501，无Upgrade；REST不受影响 |
| GET | `/Users/Public` | P4/T41 | 已实现 | `public_user_list=false` 返回空数组，true 时返回最小启用用户字段 |
| POST | `/Users/AuthenticateByName` | P4/T41 | 已实现 | 兼容 Pw/Password、MediaBrowser/Emby 认证头，签发 365 天设备令牌 |
| GET | `/Users/{userId}` | P4/T41 | 已实现 | 令牌所属用户信息与最小策略 |
| POST | `/Users/{userId}/Password` | P4/T41 | 已实现 | 本人改密，保留当前令牌并撤销其他设备 |
| POST | `/Sessions/Logout` | P4/T41 | 已实现 | 撤销当前设备令牌 |
| POST | `/Sessions/Capabilities` | P5/T51 | 已实现兼容接收（无状态204） | 需令牌后返回 204 |
| POST | `/Sessions/Capabilities/Full` | P5/T51 | 已实现兼容接收（无状态204） | 需令牌后返回 204 |
| GET | `/Library/MediaFolders` | P1/T17-B | 已实现 | 与 Views 相同的可见库集合 |
| GET | `/Users/{userId}/Views` | P1/T17-B | 已实现 | 可浏览的逻辑媒体库 |
| GET | `/Users/{userId}/Items` | P1/T17-B | 已实现 | 分页、排序、过滤和搜索 |
| GET | `/Users/{userId}/Items/Latest` | P1/T19 | 已实现 | 最近更新数组，默认按剧聚合 |
| GET | `/Users/{userId}/Items/{itemId}` | P1/T17-B | 已实现 | 电影、剧集、季、集详情 |
| GET | `/Items/Latest` | P1/T19 | 已实现 | 与用户Latest等价，默认20项 |
| GET | `/Items` | P1/T17-B | 已实现 | UserId 查询参数形式 |
| GET | `/Items/{id}` | P1/T17-B | 已实现 | 条目详情 |
| GET | `/Items/Counts` | P1/T17-B | 已实现 | 仅可见范围的 Movie/Series/Episode 计数 |
| GET | `/Items/{id}/Similar` | P1/T19 | 已实现 | 同类型同题材可见条目，最多20项 |
| GET | `/Shows/{id}/Seasons` | P1/T18 | 已实现 | 跨可见库合并实际存在的季，支持特别篇筛选 |
| GET | `/Shows/{id}/Episodes` | P1/T18 | 已实现 | 跨可见库去重、Season/SeasonId与StartItemId分页 |
| GET | `/Shows/NextUp` | P3/T35 | 已实现 | 继续观看/下一集 |
| GET | `/Search/Hints` | P1/T19 | 已实现 | 标题/原始标题NFKC搜索提示 |
| GET | `/Genres` | P1/T19 | 已实现 | 可见范围题材Name/稳定Id |
| GET | `/Items/Filters` | P1/T19 | 已实现 | 可见范围题材、年份、分级 |
| GET | `/Items/{id}/Images/{type}` | P1/T16 | 已实现 | TMDB 海报、背景、Logo、缩略图；匿名仅限已暴露条目 |
| GET | `/Items/{id}/Images/{type}/{index}` | P1/T16 | 已实现 | 指定图片序号 |
| GET | `/Items/{id}/PlaybackInfo` | P3/T31 | 已实现（已审核） | 只返回 DirectPlay/DirectStream 能力 |
| POST | `/Items/{id}/PlaybackInfo` | P3/T31 | 已实现（已审核） | 兼容客户端播放协商 |
| GET | `/Videos/{id}/stream` | P3/T32 | 已实现（已审核） | 受权限和 UA 白名单保护，302 到 CDN |
| GET | `/Videos/{id}/stream.{ext}` | P3/T32 | 已实现（已审核） | 容器扩展名兼容形式 |
| GET | `/Videos/{id}/original.{ext}` | P3/T32 | 已实现（已审核） | 原始媒体直链形式 |
| GET | `/Items/{id}/Download` | P3/T32 | 已实现（已审核） | 需要下载权限，仍只允许 302 |
| GET | `/Videos/{id}/subtitles/{index}/stream` | P3/T33 | 已实现 | 内嵌字幕由客户端读直链，此端点404 |
| GET | `/Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{format}` | P3/T33 | 已实现 | 内嵌字幕统一404，不抽取或转换 |
| POST | `/Sessions/Playing` | P3/T34 | 已实现 | 激活草稿会话 |
| POST | `/Sessions/Playing/Progress` | P3/T34 | 已实现 | 10秒节流，状态/seek即时写入 |
| POST | `/Sessions/Playing/Ping` | P3/T34 | 已实现 | 续约会话，不修改用户进度 |
| POST | `/Users/{userId}/PlayingItems/{itemId}` | P3/T34 | 已实现 | 旧版开始上报，查询参数 |
| DELETE | `/Users/{userId}/PlayingItems/{itemId}` | P3/T34 | 已实现 | 旧版停止上报 |
| POST | `/Users/{userId}/PlayingItems/{itemId}/Progress` | P3/T34 | 已实现 | 旧版进度上报 |
| POST | `/Sessions/Playing/Stopped` | P3/T34 | 已实现 | 幂等停止并保存用户进度 |
| POST | `/Users/{userId}/PlayedItems/{itemId}` | P3/T35 | 已实现 | 标记已看 |
| DELETE | `/Users/{userId}/PlayedItems/{itemId}` | P3/T35 | 已实现 | 取消已看 |
| POST | `/Users/{userId}/FavoriteItems/{itemId}` | P3/T35 | 已实现 | 添加收藏 |
| DELETE | `/Users/{userId}/FavoriteItems/{itemId}` | P3/T35 | 已实现 | 取消收藏 |
| GET | `/Users/{userId}/Items/{itemId}/UserData` | P3/T35 | 已实现 | 用户条目数据 |
| POST | `/Users/{userId}/Items/{itemId}/UserData` | P3/T35 | 已实现 | 更新进度、已看、收藏 |
| GET | `/Users/{userId}/Items/Resume` | P3/T35 | 已实现 | 继续观看列表 |

### 管理端口端点（不注册到 MEMBER_ADDR）

以下保留既有管理接口与管理员会话校验。媒体端口访问同一路径必须 404；表中的“未实现”指后续原生模型扩展，不代表将接口暴露到媒体端口。

| 方法 | 路径 | 阶段/任务编号 | 状态 | 备注 |
|---|---|---|---|---|
| GET | `/api/v1/media-server/native/sessions` | P3/T34 | 已实现 | 管理员只读活跃会话，分页≤200 |
| GET | `/api/v1/media-server/users/{id}/devices` | P4/T43 | 已实现 | 管理员设备分页，≤200，不含设备key/令牌/IP/UA |
| POST | `/api/v1/media-server/users/{id}/devices/{deviceId}/revoke` | P4/T43 | 已实现 | 单设备撤销及审计，不影响其他设备 |
| GET | `/api/v1/media-server/users/{id}/history` | P4/T43 | 已实现 | 管理员按用户读取最近条目观看状态，分页≤200 |
| GET/PUT | `/api/v1/media-server/native/settings` | P4/T45 | 已实现 | ServerName、总开关、revision；GET含安全的媒体地址与端口 |
| GET | `/api/v1/media-server/native/library-mappings` | P4/T45 | 已实现 | 分类对应关系白名单，不含任何目录路径 |
| GET | `/api/v1/media-server/providers` | P4/T45 | 管理端已有 | 管理端：媒体服务能力 |
| GET | `/api/v1/media-server/libraries` | P4/T45 | 管理端已有 | 管理端：可授权库 |
| GET | `/api/v1/media-server/users` | P4/T44 | 管理端已有 | 管理端：媒体用户列表 |
| POST | `/api/v1/media-server/users` | P4/T44 | 管理端已有 | 管理端：创建媒体用户 |
| GET | `/api/v1/media-server/users/{id}` | P4/T42 | 已实现 | 管理端：单个用户及授权/限制 |
| PUT | `/api/v1/media-server/users/{id}` | P4/T42/T44 | 管理端已有（已扩展） | 编辑媒体用户及授权/限制 |
| POST | `/api/v1/media-server/users/{id}/password` | P4/T44 | 管理端已有 | 管理端：重置密码 |
| POST | `/api/v1/media-server/users/{id}/sessions/revoke` | P4/T44 | 管理端已有 | 管理端：撤销会话 |
| DELETE | `/api/v1/media-server/users/{id}` | P4/T44 | 管理端已有 | 管理端：删除用户，不删除文件 |
| POST | `/api/v1/media-server/native/probe/{assetId}/retry` | P2/T23-B | 已实现 | 管理员会话、CSRF、维护模式保护；双开关均开启时才接受；原子审计 |
| GET | `/api/v1/media-server/native/processing` | P2/T26 | 已实现 | 三开关、两队列状态统计、截图用量/预留/上限 |
| PUT | `/api/v1/media-server/native/processing/settings` | P2/T26 | 已实现 | 三开关、revision 乐观锁、事务审计 |
| GET | `/api/v1/media-server/native/processing/tasks` | P2/T26 | 已实现 | kind/state/page/pageSize 分页，单页≤200 |
| POST | `/api/v1/media-server/native/processing/retry` | P2/T26 | 已实现 | 单条或确认后的筛选批量重试，≤500，审计 |
| POST | `/api/v1/media-server/native/processing/gc` | P2/T26 | 已实现 | 单轮≤200项、沿用2秒预算/游标，报告实际回收 |
| GET | `/api/v1/media-server/native/libraries` | P1/T45 | 管理端已有 | 分类同步后的库列表，含隐藏库、来源分组/规则和目录 |

## T02 移除的旧入口

整套 member 网页和 member 专用 API 已移除，包括登录、会话、改密、退出、目录、海报和预留播放接口。媒体用户统一通过本端口的 Emby 认证（T41）接入；后台 `/api/v1/media-server/*` 管理接口保留。后台工作台 PWA 继续由管理端口提供；独立媒体端口不提供 Manifest、Service Worker 或离线页面，T07 之后这些管理/PWA 资源仍返回 404 JSON（HEAD 无正文），只有新注册的 Emby 端点由独立路由器处理。

## 安全与明确不支持项

- 不把 STRM 原文、115 全局解析密钥、pickCode 或管理员凭据返回给媒体用户。
- 播放失败时返回明确错误，不回退到服务器媒体中转、HLS 或转码。
- 所有列表必须分页并有上限；所有外部请求必须有超时；播放热路径不得持有全局锁或长事务。
- 未实现端点返回合法 Emby 风格错误或空结构，不得 panic。
- 真机 Infuse/SenPlayer 添加与解码需要用户最终确认；本阶段不伪造真实客户端验收。


## T42 统一权限模型

- 实现为 `mediausers/nativepermissions.New(db)`。监听组合根通过 `embyapi.ServerOptions.Permissions` 每请求创建检查器，认证成功后放入 `RequestContext.Permissions`（不参与 JSON 序列化）。协议包不依赖 SQL；所有后续浏览/播放端点必须调用该入口，不能自行解释授权行。
- 每请求、每用户只读取一次账号与库授权快照；并发调用用请求内 `sync.Once` 合并。没有跨请求缓存，没有全局锁或写事务。再次检查认证用户和数据库用户的启用、过期、强制改密标记与凭据版本。授权集合上限 200（与管理输入一致），超出时失败关闭，不静默截断。
- **授权行存在即允许浏览**，不要求 can_play=true；未授权/隐藏库不可见。播放要求 can_play；下载必须同时满足 can_play 与 can_download。隐藏库的授权行保留，恢复显示后仍可使用。`VisibleLibraries` 返回新切片，直接用于电影 `MovieQuery.Visible` 和剧/季/集的 `VisibleLibraries` 参数。电影查询也强制验证所请求的库在该集合中。
- `ItemAllowed` 要求条目的实际归属库至少一个可浏览，并调用 `ParentalAllowed`。传入的 LibraryID/LibraryIDs 必须来自服务端投影归属，不能直接采用客户端值；剧集层级需使用已持久化的剧级 OfficialRating。未设置 max_parental_rating 时不限制；设置后无分级、NR、未知分级默认隐藏，不能用预留的数值字段绕过映射。
- 分级映射集中在 `nativepermissions/ratings.go`，是本程序的保守“年龄×5”数值策略（0–100），不是对中国大陆存在官方分级制度的声明。比较采用 `level <= max_parental_rating`，相等允许。

| TMDB 分级/标签（可带 CN:/US: 前缀） | 本程序等级 |
|---|---:|
| G、TV-Y、TV-G、ALL、全年龄、普遍级 | 0 |
| 保护级、6+ | 30 |
| TV-Y7、TV-Y7-FV、7+ | 35 |
| PG、TV-PG | 40 |
| 12+、辅12级/輔12級 | 60 |
| PG-13、13+ | 65 |
| TV-14 | 70 |
| 15+、辅15级/輔15級 | 75 |
| 16+ | 80 |
| R | 85 |
| NC-17、TV-MA、18+、限制级/限制級 | 90 |
| 空值、NR、其它未知值 | 未知；受限用户默认隐藏 |

- `StreamSlots` 只统计同一用户 state 非 stopped 且 last_progress_at 距今不超过 5 分钟的会话（paused 也占名额），达到 max_streams 即返回 ErrStreamLimit；这只是读取快照，不能当作并发预留成功。T34 仍需原子开会话并收敛过期会话。
- 浏览/普通权限拒绝映射为不透露存在性的 404。播放调用方显式通过 `nativeplayback.PermissionError` 转换后，`embyapi.WriteDomainError` 返回 403 和固定可读原因（权限不足、分级限制、超过同时播放数），不回显内部错误。未认证仍由 T41 返回 401。
- 管理端 GET/PUT 单个用户与原列表增加 canDownload、maxStreams、maxParentalRating、expiresAt；maxStreams 为 1–20，分级为 0–100 或 null。PUT 省略 nullable 限制字段时保留，显式 null 时清除。更新权限/限制继续递增 credential_version 并撤销原令牌。现有 Vue 页面编辑时原样保留这些字段；完整可视化编辑留给 T44。
- 共享契约：`internal/platform/httpapi/testdata/contracts/media-user-permissions.json`；Go 严格解码与真实 GET/PUT 回读、Vue 实际 PUT JSON 均断言该契约。T42 不接入 T17–T19 浏览端点，不启用播放。


## T17-A 统一分页查询服务（T17-B 已接入通用 HTTP）

- `catalog.BrowseRepository` 扩展 T14/T15 的服务层，复用 `ExposableAssetsSQL` 和 `LibraryDTO/MovieDTO/SeriesDTO/SeasonDTO/EpisodeDTO`。只读已有媒体与 TMDB 快照，不实时请求 TMDB，不读取 STRM 正文或传输媒体流。HTTP 路由、端到端认证链与响应 golden 已在 T17-B 接入。
- 输入必须是 T41 用户与 T42 请求级 PermissionChecker。`VisibleLibraries`、按同一 `ParentalAllowed` 映射生成的分级 SQL 条件、父级与项目过滤在分页前执行；页内 `ItemAllowed` 是防御性断言，拒绝时整次请求失败，绝不靠内存删除条目改变页长/总数。CollectionFolder 没有媒体分级，仅使用库级浏览授权。
- ParentId 使用原生数字字符串 item_id，不使用 g:/r:/f: 内部库键。库父级默认列电影/剧；剧父级默认列季；季父级列集。无父级且非递归默认列库；递归可按 IncludeItemTypes 查询电影/剧/季/集。库父级筛选在资产聚合之前执行，不借用其它可见库的版本、集或计数；仅剧/季全局 ID 未指定库时，按所有可见库取并集。
- 全局按 `(entity_type,internal_key)` 去重；同一部电影、同一部剧或同一集跨库只出现一次。全局 Movie/Series 的 ParentId 选择可见归属库中 sort_order、库键排序最前者。多版本最多装配最新100个，超出时在首个媒体源名称标记；多集文件仍只以首集为条目，并输出 IndexNumberEnd。
- 支持混合类型 Ids（含 CollectionFolder，最多200，重复/不可见/不存在的ID不重复或不返回）、Genres（忽略大小写、任一匹配）、Years、IsFolder、字面标题子串 SearchTerm、Include/ExcludeItemTypes、StartIndex≤1000000、Limit≤200（0表示只计数）。SQL 同一次查询用同一 filtered CTE 得到总数与分页，空页/越界末页仍返回准确总数。
- 排序支持 SortName/Name、DateCreated、PremiereDate、ProductionYear、CommunityRating、DatePlayed 等固定白名单；同值最终按数值型 ID 升序，未知年份/日期置后。Random 不支持，明确忽略该项及其对应方向；若无剩余排序，则回退 SortName 升序。DatePlayed 在 T35 前回退 DateCreated；IsPlayed/IsFavorite=true 在 T35 前返回空结果，false视作全部未播放/未收藏。
- T17-B 已把全库缺失ID分配移出浏览热路径：native.assign 在归属事务提交前为该资产确保电影或剧/季/首集完整ID家族，失败则任务报错并回滚归属；周期任务每轮补最多500个缺失ID，T13生命周期继续处理退休/恢复。正常浏览只读ID表；仅当前页（≤200项及必要祖先，最多600个键）缺失/退休时分批≤500兜底分配，并按服务实例记录一次不含条目/路径/令牌的警告。稳定分页以后台预分配为正常前提，遗漏时页内兜底不扫描整个库；后台补齐后恢复完整ID排序。
- 页面最多200个逻辑条目，TMDB快照按所需(type,tmdb,season)一次取回、只取本页需要的季集字段；版本一次批量取回，单条最多100，不做逐ID查询。Overview/People仅在Fields显式请求时保留（Overview最长16384字符，People最多60）；其它固定数组仍遵守T05的[]而非null契约。
- `Counts` 默认统计全体可见范围 MovieCount/SeriesCount/EpisodeCount；可接收一份可选 ItemQuery 复用完整的授权、分级、父级、类型、Genres/Years等过滤，忽略分页窗口，仅统计匹配总数。
- Views 按 sort_order、库键排序；保留普通空库，兜底库只有未隐藏、已授权且存在有效内容时显示。不可见、隐藏、不存在或受分级限制的父级均返回 ErrNotFound。所有服务方法有5秒预算，不跨事务调用外部服务，不引入全局锁。
- 新增集成用例覆盖跨库去重/库内版本隔离、父级校验、混合IDs、分级与分页总数、过滤排序、兜底与失效资产、版本/重字段上限以及10000条合成电影预算。测试仅在media_tool_test独立schema执行；实测耗时见本任务汇报，不作为所有部署硬件的性能保证。


## T17-B 通用浏览 HTTP

- 已接入 GET/HEAD：`/Users/{userId}/Views`、`/Library/MediaFolders`、`/Users/{userId}/Items`、`/Items`、`/Users/{userId}/Items/{itemId}`、`/Items/{id}`、`/Items/Counts`。MediaFolders 与 Views 返回同一 QueryResult 内容；Items 的 UserId 查询参数可指定本人，也可由有效令牌推导本人。
- 统一使用 T41 真实认证与 T42 请求级权限。跨用户路径/查询参数403；不可见父级/条目404；非法查询400；数据库不可用/服务超时503。enabled=false 时上述端点503。详情未提供 Fields 时默认含 Overview/People，列表仍按显式 Fields 控制重字段。HEAD 与 GET 同样校验且不写正文。
- 元数据请求总预算6秒，在设置读取之前建立，覆盖可信IP设置、Token认证、权限、服务查询及编码。各数据库子调用继承该上下文；最多32MiB缓冲JSON，完成前不提交成功响应，超时或超限统一503，避免部分200或内部错误泄露。该预算不改变T41独立登录或T16图片端点的既有超时。
- `native.assign` 后台预分配与周期缺失ID补齐使正常浏览不再执行ID INSERT/UPDATE；仅页内兜底分配保留。10000条预分配电影的首次浏览耗时由数据库测试实测，正常热路径零ID写入断言在测试中固定。
- 合成HTTP golden：`internal/modules/embyapi/testdata/browse_http/{views,items,detail,counts}.json`，默认只读比较；使用该包测试的 `-update` 显式更新，不包含登录令牌或真实媒体数据。
- 集成测试通过实际 NativeMediaHandler、真实Argon2媒体登录与X-Emby-Authorization请求完成 Views→库→剧→季→集→详情→Counts，覆盖前缀/大小写、HEAD、混合Ids、分页、跨用户拒绝、隐藏/未授权父级及禁用服务。T18/T19 已接入专用 Shows/Latest/发现端点；NextUp、Resume、用户数据与播放仍等待后续任务。


## T18/T19 层级与发现

- 已实现上述八条路由，继承 T17-B 的真实Token认证、请求级PermissionChecker、隐藏库与分级SQL过滤、6秒请求处理预算（服务5秒）、有界响应、HEAD和三个路径前缀。新增接口不把后台会话/STRM全局密钥当作媒体认证。
- Seasons/Episodes 复用 T15 的稳定剧/季/集身份与投影、T17 统一SQL分页，在所有可见库内合并去重，避免只查第一个库或先全量读入。Seasons 返回 QueryResult，按季号升序，IsSpecialSeason=true仅季0、false排除季0。IsMissing 接受合法布尔值但忽略，不虚构缺失季/集。
- Episodes 的 SeasonId 必须是该剧可见的季；Season 是非负季号，二者同时存在时必须一致。StartItemId 必须属于最终可见结果，表示从该集开始（包含该集），StartIndex 在此基础上偏移；SQL row_number/同一filtered CTE处理，不在内存中全量查找。未知剧/季/集、受限条目统一404；无季过滤时跨库合并所有集。
- Latest 返回数组而非 QueryResult。Limit默认20、最大100；默认将Episode请求映射为Series，返回最近有新可暴露资产的剧；GroupItems=false时返回集。排序单独使用实际资产max(created_at)，不改变原DateCreated的最早资产语义，Series.ChildCount/RecursiveItemCount继续保持季数/集数语义。ParentId若提供必须是可见库，库内聚合只使用该库资产；同剧只出现一次。
- Hints 返回 SearchHintResult{SearchHints,TotalRecordCount}；SearchTerm去首尾空白后至少1字符、最多256字节，Limit默认20、最大100。SQL用 normalize(...,NFKC)+lower+字面子串匹配标题及已持久化TMDB/媒体原始标题，兼容大小写与全半角，中文不分词，%/_不是通配符。SearchHints含Id/ItemId/Name/Type与可用的年份、主图/缩略图tag和Series信息，不返回远程原图URL。
- **索引选择**：本轮不新增pg_trgm扩展、索引或迁移；查询先限定可见库/类型/分级与有效资产。Hints、Genres、Filters 的候选上限为50000个逻辑条目，取50001项检测溢出；超出返回400，不能返回静默截断的错误总数。可用ParentId/IncludeItemTypes缩小范围。即使未命中任何搜索词，超过Hints候选上限也明确失败。查询保留总超时，不承诺任意规模的无索引模糊搜索；未来扩大范围应单独评估持久化搜索投影或trigram索引。
- Genres 返回 QueryResult<NameIdPair>。Genre Id=`genre:`+SHA-256(数据库NFKC+小写+去首尾空白的题材名)，64位十六进制完整摘要，不占用五类内容实体的数字序列；同题材在库变化时ID稳定，不允许拿Genre ID进入播放/图片条目路径。规范化相同的名称合并，展示名取稳定字典序。Filters返回 Genres:[]、Years:[]、OfficialRatings:[{Name,Value}]，只含可见且通过分级的实际条目值。题材/年份最多512种、分级最多128种，超过明确400，不静默截断。
- Similar 先验证目标可见性，要求相同Type且题材交集，排除目标自身，按评分/名称/稳定ID排序，Limit默认20、最大20；无题材或无相似项返回空QueryResult。
- 七份合成HTTP golden位于 `embyapi/testdata/discovery_http/`（seasons/episodes/latest/hints/genres/filters/similar），默认不更新，使用已有-update标志显式生成。集成测试覆盖真实SenPlayer格式登录和认证头、跨库层级、Latest两模式、中文/英文原始标题/全半角、隐私范围、Similar及候选上限。没有真实客户端设备兼容验收或播放实现。


## T21 后台 ffx 受控进程封装

- ffx 保持独立进程边界，不把 `mediausers`/浏览/播放接到 ffmpeg；业务适配已由 T22–T25 接入。Probe 返回有界原始 ffprobe JSON 字节，Still 返回 JPEG 字节；ffx 本包不解析 MediaStreams、获取 STRM 直链或创建 jobs，这些由后续已完成的分层负责。
- `Config`：FFprobePath/FFmpegPath 可给受信任的绝对二进制路径，否则通过 PATH 查找；New 只验证配置，不启动或要求二进制存在。调用时找不到/无法启动返回 ErrUnavailable，供后续任务安全降级。生产默认不设置 AllowLocalFiles；仅合成媒体测试显式开启该项，且本地输入必须是存在的绝对普通文件路径。
- URL 仅 http/https，拒绝凭据型 URL、控制字符、file:/concat:/pipe:/data: 等输入。固定 `-protocol_whitelist http,https,tcp,tls`；仅显式本地测试增加 file。Still 的 `pipe:1` 是封装内部固定的 stdout 输出，不是允许调用者传入的输入协议。
- 不经 shell，直接 exec.CommandContext + argv；Options 不含任意参数/过滤器/输出路径。默认 probesize=8MiB（32KiB–16MiB）、analyzeduration=5秒（1毫秒–30秒）、rw_timeout≤10秒并受总超时约束；UA最长512字节且禁止控制字符。Still 只取第一个视频轨的一帧，禁音频/字幕/数据输出，固定 mjpeg/image2pipe；尺寸默认640×360、最大1920×1080，quality默认3、范围1–31；offset非负且≤24小时。
- Probe 默认30秒、Still默认45秒，Config或Options可调整，最高2分钟。建立超时后再等待全局信号量，排队也消耗同一预算。默认全局Probe=2、Still=1；ConfigureConcurrency 可设两类各1–2，并且只允许在没有运行或等待请求时修改，创建新Runner不能扩大额度。
- Linux/macOS 使用Setpgid建立独立进程组；取消先向整组SIGTERM，给100毫秒回收窗口，再向整组SIGKILL。cmd.Wait必定调用以回收直接子进程，WaitDelay=1秒限制残留pipe等待；成功退出后也清理遗留组成员。不支持进程组语义的平台拒绝执行，不静默退化成只杀父进程。
- stdout默认8MiB、最高16MiB，超限立即终止整个组并返回ErrOutputLimit，不返回部分结果。stderr只保留最后4096字节；截断后丢弃不完整首行，脱敏所有远程URL的query/fragment/凭据，以及源URL参数值。ProcessError.Error只含固定操作、错误类别与去query的目标；Stderr提供单独的脱敏尾部诊断，错误结构不参与JSON序列化。**成功Probe原始JSON可能含ffprobe自身的filename字段（带临时URL），仅给后续受信任解析器使用，禁止直接写日志、数据库快照或HTTP响应。** T24应提取白名单字段。
- Runtime Dockerfile 只增加apk包ffmpeg，frontend/backend构建阶段不变，Schema仍58。按固定Alpine AMD64基础层和2026-10-16 v3.23主仓/社区仓APK依赖元数据，增加约104个包，下载包归档总和47.12MiB、安装大小总和126.47MiB（ffmpeg 8.0.1-r1）；这只是依赖元数据估算，不是Docker构建后的精确OCI层增量，仓库更新/文件压缩/触发器可能使实物不同。本任务未启动Docker、未构建或发布镜像；R2需记录实际镜像大小。
- 本机Darwin/arm64已有ffmpeg/ffprobe 8.1，已实际生成3秒lavfi H264+AAC+mov_text字幕轨MP4，验证本地/HTTP Probe JSON、时长与轨道标识及Still JPEG、实际UA。无工具环境测试明确Skip；本机未Skip。伪进程测试不经shell，覆盖整组超时/取消/强制kill、子进程回收、stdout上限、stderr脱敏、全局跨Runner并发与等待取消。

## T22 探测链接获取（后台可信入口）

- `mediaprobe/linksource.NewForManager` 构造受信任后台依赖；`Source.Fetch(ctx, assetId)` 返回内存 `Ticket`。`Claim()` 只能消费一次，并同时交出临时 URL 与 UA；后续调用 ffx 时必须将这一对原样用于输入和 `Options.UserAgent`。默认 UA=`MediaTool-NativeProbe/1.0`，可通过 linksource.Config 配置，最长512字节、禁止控制字符。Ticket 格式化/JSON均隐藏链接；到期不可再领取。
- 复用 `catalog.ExposableAssetsSQL`：绑定 ready、云文件 present、有效 `.strm` 输出。每次只读一条实际输出；要求 `managed_media_artifacts` 的资产所有权与32字节SHA256摘要匹配。使用 `os.Root` 约束相对路径、普通文件检查、最多8192字节UTF8正文；拒绝越界、符号链接文件与异常正文，不修复/重写文件，不创建/旋转文件令牌。
- 网关和后台复用 `strm.ParseStable` 的 origin/参数/全局旧钥匙或单文件 grant 校验。正文 assetId/path/pickCode 必须与数据库证据一致；取链后再次校验数据库证据和实际文件摘要，避免已替换/已失效资产继续探测。不一致返回需人工处理，不把数据库值静默覆盖进 STRM。若存在多个有效输出，确定性选择路径排序第一条；异常时失败，不猜测另一个文件。
- Fetch 用包内不可由 HTTP 头、UA 或用户参数构造的可信上下文进入专用 resolver；HTTP 的外部 UA 白名单保持不变。绕过的仅是外部播放器准入，不是文件 grant、资产身份或 CDN 目标安全策略。直接调用现有 `integrations.DynamicStorage`（官方115优先、既有CD2路径保留），不回调自己，不调用媒体响应/中继逻辑，不经过会向Redis存储直链的 `linkcache.Cached`。
- 固定探测 UA 用于115取链；上游若要求不同UA或额外Header则安全失败，不自行换UA。复用官方路径/下载身份检查、Token刷新及刷新冷却。对返回目标沿用 playback.Policy 的HTTP配置、来源域名和公网DNS/IP策略，额外拒绝指回STRM解析域名；不会跟随来自 STRM 正文的任意远程地址。
- 取链全过程使用 requestbudget（默认30秒，配置上限60秒，受父ctx更短期限约束）。专属全进程并发默认1、可在空闲时 ConfigureConcurrency(1–2)，与 ffx/外部播放并发独立；繁忙立即返回可重试/建议5秒。115限流（HTTP429/官方代码/刷新冷却）返回可重试/建议60秒，并对本进程后续探测取链冷却60秒；不额外重试取链请求。
- 错误为固定 Go 常量，T23 才持久化：`permanent`（`not_exposable`、`file_missing`）；`retryable`（`rate_limited`、`timeout`、`unavailable`，附 RetryAfter）；`manual`（`authorization`、`strm_mismatch`、`ua_mismatch`、`unsafe_target`）。一次性票据不可用返回永久 `ticket_unavailable`。数据库不可用不能伪装成授权已撤销。
- 临时链接只驻留内存，不写数据库、Redis、文件、日志或错误。日志只含 assetId、固定阶段/结果类别与耗时；覆盖继承的解析observer，沿用R1-F的DNS/TCP/TLS/首字节/总耗时数字字段，无URL/路径/令牌/UA/pickCode。现有官方授权刷新可能继续加密保存新的授权对，这不是持久化临时链接。
- 验证使用合成受管 STRM、假解析服务及完全截获的假115 Transport，不访问真实115/CDN。本小节描述T22链接来源范围：它不执行ffprobe、不创建jobs、不写media_probe_results。T23-A仓库及后续运行接线见下节。

## T23-A 探测尝试与成功快照（Schema 59）

- `mediaprobe/probequeue.Repository` 提供 `Enqueue`、`Retry`、`Claim`、`Succeed`、`Fail`、`RecoverExpired`、`Current`。T23-A交付范围仅含有界数据库操作；T23-B新增的运行接线见下节。T24 的媒体流解析未开始。
- **唯一调度事实来源**：`media_probe_results.file_fingerprint/state/attempts/next_attempt_at/lease_token/lease_until`。state 为 `pending/probing/ready/failed/needs_action`，表示当前目标指纹的尝试状态。`jobs` 只是唤醒；幂等键 `probe:<assetId>:<fingerprint>`，队列 `probe`、kind `native.probe`、max_attempts=1。失败探测写入表中退避后，其唤醒 job 必须成功结束；不得把探测错误交给 jobs 通用重试。无效/过期唤醒不能越过表中的状态和 next_attempt_at。
- **成功快照独立保存**：`result/result_fingerprint/probed_at` 始终代表最近一次成功，`file_fingerprint` 是当前目标。改指纹/手动重试/超时/失败/needs_action 均不清空成功快照。`Current` 只检查成功快照是否存在，不按 state='ready' 过滤；T24 投影必须遵守此读取语义，不能在重探时产生空窗。新的成功结果才原子替换快照。持久化入口只接收强类型 `Result{DurationTicks int64}`，不接收原始 ffprobe JSON、URL或任意文本。
- **指纹**：所有查询使用同一个 SQL 函数 `native_probe_fingerprint(size_bytes,provider_file_id,metadata)`，SHA256 摘要覆盖文件大小、provider_file_id、持久化哈希（实际 metadata 是哈希 map，键 `2` 为 SHA1、键 `1` 为 MD5，兼容 sha1/md5 命名字段）以及存在时的 mtime/modified_at/modifiedAt。忽略扫描 updated_at 和文件路径，哈希值大小写与 JSON key 顺序不造成重探。若上游没有哈希/mtime，同一 provider ID 下的同大小内容替换无法仅凭这些证据识别；不得用每次扫描都会变化的 updated_at 冒充内容版本。
- **领取与 fence**：短 SQL 使用 `FOR UPDATE SKIP LOCKED`，仅领取 pending、已到期、当前仍可暴露且指纹匹配的文件。每次领取生成新 lease UUID，attempts +1。成功/失败写回都要求 token、attempt、目标指纹、probing 状态与未过期 lease 一致；成功还重验当前可暴露资产及云文件指纹。影响0行返回 `(false,nil)`，表示放弃，而非强行重试写入。手动重试或新指纹立即清除旧 lease，旧 worker 不能覆盖新尝试。
- **恢复与退避**：`RecoverExpired` 每次最多200条并 SKIP LOCKED，过期 probing → pending，保留旧快照及 attempts，清空 lease，记录固定 `retryable/lease_expired` 码。取消可中断，再次调用可继续。建议 lease=取链预算+ffx超时+15秒，默认75秒，最大配置组合195秒；仓库领取上限4分钟。B必须先取得可执行槽再领取，不持 lease 排队等待外部并发额度。
- 可重试失败 → pending + next_attempt_at；无上游建议时依次1分钟、5分钟、30分钟、2小时、12小时，之后24小时封顶；有正的 RetryAfter 时优先采用建议间隔，同样最多24小时。永久失败 → failed；人工处理 → needs_action。相同指纹的普通重复入队不清除终态/退避；仅新指纹或显式 Retry 重置 attempts 并回到 pending。
- 手动恢复与 `native.probe.retry` 审计在同一短事务提交，审计仅含 assetId 和指纹。不存在/不可暴露的资产拒绝恢复，不写成功审计。B的管理接口仍须管理员认证且仅管理端口开放；T23-A未开放它，T23-B接线见下节。
- Schema 59 新增错误 class/reason 枚举 CHECK、lease和成功快照配对约束及过期 lease 部分索引，移除自由文本 error 列。D2按迁移顺序处理 DROP/重建约束，比较探测状态、T22分类/原因以及本包 `lease_expired/process_failed/invalid_result` 常量的完整集合。Up恢复旧 probing，并将原failed归一为permanent/process_failed；原自由文本error不保留。Down保留成功快照但丢失新增诊断/lease，并把 needs_action 降为 failed；重新Up可继续入队。

## T23-B 后台执行与默认关闭

- **Schema仍59，未发布前修正00059**：增加约束前先统一归一化旧尝试。ready 缺 probed_at 或 result={} → pending、next_attempt_at=now()；所有旧开始时间清空，旧probing转pending并记录固定中断码。缺完整成功凭据的残留result/probed_at清空；failed/pending已有完整成功凭据则保留为独立快照。旧自由文本错误删除。迁移测试以NOT VALID旧约束模拟脏数据，覆盖ready/failed/pending/probing共9种组合。
- **双开关**：`native_server_settings.enabled` 保持默认false；新增 `probe_enabled boolean NOT NULL DEFAULT true`。有效条件为二者同时true。后台入队事务、SQL领取及成功写回均检查此条件，读失败按关闭处理，不依赖设置短缓存。关闭时不新建native.probe任务、不获取直链、不执行ffprobe；仅允许纯DB过期lease收敛。完整开关UI已由T26接入。
- **触发/补漏**：native.assign提交成功后尝试入队，不把探测入队失败变成整理失败。默认调度周期调用ScheduleBatch：先回收≤200条过期lease，再按UUID游标读取最多200条缺结果/指纹不符/重试到期的可暴露资产；每次总预算30秒，可中断、重启游标重扫，幂等键不堆积。UUID入参规范化为小写，避免同资产因大小写产生两个业务键。
- **真实额度前置**：`ffx.ReserveProbe`使用T21实际全局probe信号量（默认2），`linksource.Reserve`使用T22实际取链准入（默认1，包含115冷却）；两者都成功后才Claim探测lease。等待资源时不创建probe lease、不增加attempts；放弃/取消释放未消费许可。上下文key私有且许可只能消费一次，提前close不能释放正在使用的许可。普通Probe/Fetch的准入语义不变，不另造影子计数器。
- **执行链**：两种许可 → Claim → linksource.Fetch → Ticket.Claim（同时得到URL/UA）→ ffx.Probe（同一UA）→ ResultParser（T24-A替换原DurationParser）→ 带fence写回。取链许可在Fetch退出时释放，ffx许可在实际进程退出时释放；两个执行worker仍可并行探测。等待资源有独立上限，探测lease只从真正有执行额度后开始计时，保留取链预算+ffx超时+15秒余量。
- **运行与关闭**：统一app的supervise新增一个probe运行器，内部恰好两个queue=probe的唤醒消费者；也支持worker --queue probe，不新增Compose服务。构造函数不做外部I/O、不查找ffmpeg二进制；缺工具不阻止默认关闭的启动。执行期间每500ms检查双开关（DB检查预算1秒）；关闭/DB不可用时取消子上下文，ffx沿用T21整组回收。源与进程错误只映射固定class/reason；Process始终以成功结束唤醒job，写库失败由探测lease回收与补漏收敛，不调用jobs通用退避。
- **T23占位解析（历史，已由下节T24-A替换）**：只校验≤8MiB JSON对象并提取format.duration（数字或字符串数字），转换为int64 durationTicks；缺失/N/A为0，负值/非有限/溢出/非法JSON拒绝。filename、streams、chapters和其余原文全部丢弃，不持久化、不记录。没有实现T24 MediaStreams/完整媒体信息解析。
- **管理员重试**：POST `/api/v1/media-server/native/probe/{assetId}/retry`，无需正文，返回202 `{data:{assetId,queued}}`；非法ID400、不可暴露404、关闭或不可用503，错误不含内部详情。仅管理端注册，媒体端全部方法404；沿用管理员会话/CSRF/维护窗口保护。Retry和native.probe.retry审计同一事务；审计只含资产ID与指纹。Go严格解码共享契约，Vue API client的vitest使用同一份契约；完整页面已由T26接入。
- 新增执行测试全部使用假linksource/ffx，无真实115/媒体修改；既有T21真实ffmpeg合成媒体回归保留。日志只记录资产ID及固定结果类别，临时链接和Probe原始JSON不进入日志、数据库、jobs或HTTP响应。


## T24-A 探测成功快照契约（已由 T24-B 接入浏览）

- `mediaprobe/probeinfo` 是纯数据/解析包，只用标准库及既有 x/text 语言数据，不访问网络、磁盘或数据库；catalog 只能导入此纯包，不得导入 probequeue/ffx。D9 同时扫描全仓非测试源码（包括其它平台文件）与实际传递依赖。
- `probequeue.Result` 为纯模型别名；默认 `ResultParser` 在内存读取 ffprobe JSON，提取白名单后，经校验及原 T23 fence 写入 result 成功快照。`schema_version=1`；旧快照缺此字段视为版本0，durationTicks继续可读。A不改变补漏SQL或重试时刻；B负责一次性版本补探，禁止反复重置退避。
- 白名单：容器、durationTicks（int64）、格式码率/大小；原始 Index、类型/codec/profile、视频level/尺寸/码率/平均及实际帧率/pix_fmt/位深/宽高比/SDR-HDR信息/交错/旋转；音轨声道/layout/采样率、ISO639-2语言（chi→zho、fre→fra、ger→deu，und不猜语言）、标题、default/forced；字幕srt/ass/ssa/mov_text/webvtt/text/pgssub/dvdsub/dvb_subtitle及文本/听障标记。没有保存filename、任意tags、封面/字体附件、data流、extradata或原始side_data。
- 输入≤8MiB，原始流≤64（含被过滤流），单流JSON≤64KiB；Index为0–65535且不可重复，不重编号。宽高1–32768、声道/位深1–64、采样率≤768000、帧率≤1000，溢出或异常可选值省略；duration负数/非有限/溢出、无效Index/JSON整体拒绝，错误只含固定码。codec≤32字节，profile/pixel/layout等≤64字节，title/DisplayTitle≤256字节；超长或包含URL、目录分隔符、控制/格式字符、pickCode等敏感标记的文本丢弃，不存原文。
- 容器：matroska→mkv、mov/mp4家族→mp4、mpegts→ts，只有本地/远程输入文件名的安全扩展名为.m2ts时才映射m2ts（只在内存判扩展名，绝不持久化原URL）。不认识的容器省略。音视频codec名保留有界安全短字符串，字幕codec严格白名单。
- HDR语义已在T24-B修正：VideoRange只取SDR/HDR；VideoRangeType取SDR/HDR10/HLG/DOVI及其兼容变体，详见下节。旋转取Display Matrix，回退tags.rotate；interlaced只认tt/bb/tb/bt。
- `DisplayTitle` 为唯一标题策略：视频“高度p CODEC RANGE”（如1080p H264 SDR）；其它“英文语言名 CODEC 声道标签 Default Forced Hearing Impaired”，缺项省略，声道常见值Mono/Stereo/5.1/7.1。原始轨道title另存，不拼入生成标题。
- DTO仅扩展optional字段：BaseItemDto的Container/Size/Bitrate/Width/Height、MediaSourceInfo的Width/Height，以及MediaStream的完整技术字段；有效0值数值用指针。A不填充浏览结果，也不改Path/DirectStreamUrl/capabilities，既有浏览与DTO golden保持不变。新增单独的扩展golden验证零值与结构。
- 合成契约包含手写完整/最小、DOVI/HLG/HDR10/PGS及现场生成的3秒H264+AAC+SRT多音轨多字幕MKV、封面MP4、HEVC10bit HDR10。真实媒体golden仅规范化编码器相关的时长/码率/文件大小/profile/level，锁定语义轨道字段；时长和大小另行断言，数值和字符串继续受解析上限保护；无ffmpeg/对应encoder时明确Skip。所有图片/媒体为测试临时生成，未访问真实115。


## T24-B 浏览探测投影与版本补探

- **未发布语义修正，schema_version仍为1**：VideoRange只能为SDR或HDR。VideoRangeType为SDR、HDR10、HLG、DOVI、DOVIWithHDR10、DOVIWithHLG、DOVIWithSDR。有效DOVI配置优先（dv_profile 1–10），dv_bl_signal_compatibility_id为1/4/2时分别映射HDR10/HLG/SDR兼容型，0、缺失或其它值为DOVI；这些类型的VideoRange均为HDR。无DOVI时smpte2084（PQ）即为HDR10，arib-std-b67即为HLG，不依赖color_primaries，primaries只供辅助诊断。DisplayTitle中的范围名为HDR10、HLG或Dolby Vision。**Emby 4.8实际下发取值留到T52通过生产网关日志核对**，本任务不读取生产日志。
- catalog只导入纯probeinfo。统一浏览、原电影服务、原集层级服务，对已完成可见性/分页/版本上限筛选的当前页资产，恰好一次`native.probe.page`批量SQL读取成功快照；库/剧/季没有媒体源时不查探测；父级/Similar存在性预检跳过探测，只为实际返回页读取快照。最多200条目×100版本=20000资产，SQL限制20000行，单页快照总JSON传输预算32MiB，超预算整次失败（统一浏览映射503），不悄悄漏版本或传输无界JSON。
- 快照按cloud_file_id与资产关联。读取仅要求probed_at及非空成功result，不要求尝试state=ready，不要求当前file_fingerprint等于result_fingerprint：这是T23允许旧成功快照在新指纹尝试pending/probing/failed期间继续可用的语义。非法/未知版本的快照按未探测降级，绝不将未经校验的内容透传DTO；每版只用自己的成功结果，不借用另一版。
- Movie和Episode的各MediaSource填充Container、RunTimeTicks、Size、Bitrate、Width/Height及MediaStreams，保留原版本顺序/名称及固定直播放能力。条目级技术字段采用第一版本；没有快照时维持原有输出。探测duration优先，缺失或0时保持原runtime/TMDB兜底；旧版本0的duration-only仍可用。Series/Season聚合节点不伪造媒体源。不修改Path或播放URL构造（仍由T31处理）。
- 一次性版本补探不增加列或迁移：补漏扫描把`state=ready AND coalesce(result.schema_version,0)=0`加入候选；Enqueue在相同行锁/事务中仅将这类ready转pending，保留result/probed_at及result_fingerprint。转换之后已非ready，重复扫描/入队不再清零attempts或修改next_attempt_at，probing不被替换lease。失败后的pending继续原退避；failed/needs_action继续T23终态/人工恢复规则，不能因版本旧自动清除。成功写回版本1后退出候选。文件指纹变化、显式管理员Retry仍按T23原规则处理。
- 契约：新增`movie-probed.json`与`episode-probed.json`，包含DOVI兼容型、多版本各自结果、旧duration和无快照版本；真实登录HTTP链新增items-probed.json响应golden；旧的54份无探测golden保持字节不变。T24-A探测专用HDR golden和DTO扩展golden按上述新语义更新，不改其它既有契约。

- T24-B专项性能（本机media_tool_test，1万电影全部有快照，ID已预分配，首次浏览200项；非重启数据库/清OS缓存的物理冷测）：两次首次浏览367.8ms、417.8ms；后一次末页237.5ms、200个Ids批量354.7ms、Counts104.9ms。对比T17-B历史314ms，首次浏览增加约54–104ms（17.1%–33.0%）；包含新增快照读取/解析和技术字段装配，不宣称跨次测量可精确归因。200个Ids共7次SQL，其中探测快照恰好1次，浏览ID写入为0，均在原预算内。

## T25-A 单集截图状态与仓库（执行已由 T25-C/D 接入）

### 数据模型（Schema 60）

- `media_stills`仍以episode类型的item_id为主键，一集一行。尝试字段为`asset_id/fingerprint/duration_ticks/state/attempts/next_attempt_at/grabbing_started_at/lease_token/lease_until/last_error_class/last_error_reason`；成功快照字段为`result_asset_id/result_fingerprint/relative_path/digest/width/height/published_at/last_access_at`。失败/重试不清掉成功快照，Current读取不限制state=ready。
- 两个资产FK使用ON DELETE SET NULL，避免缓存阻断资产清理；快照文件元数据保留，后续B/D根据引用与可暴露性回收。原item类型复合FK保留，只允许episode。主来源和成功来源正常ready时一致，但FK置空墓碑合法。
- 新增state=`needs_action`；全部状态、错误class/reason的Go常量与迁移CHECK由D2测试逐项比对。自由文本error列删除，不保留原上游错误。lease/token/started与grabbing绑定，数据库硬限制`0 < lease_until-grabbing_started_at <= 5分钟`。成功文件信息必须成组出现，digest/指纹为64位小写hex，新成功图片宽度≤1280且路径严格匹配来源资产和指纹。
- 00060先归一旧数据：旧grabbing回pending并记录lease_expired；旧ready转pending以重新选来源，但完整且安全的旧图片信息保留（不伪造资产/指纹）。坏/不完整图片元数据清空，不删除或移动任何文件。Down保留完整图片为旧ready，needs_action降failed，丢失新增尝试/来源/访问诊断字段；Up→Down→Up及7种脏数据组合有回归。
- `native_server_settings.stills_enabled boolean NOT NULL DEFAULT true`。生成有效条件为enabled、probe_enabled、stills_enabled同时true；读取开关失败不会放行。尚未注册worker、探测成功触发或周期截图任务，默认关闭的原生服务不受影响。

### 接口与调度契约

- `stillqueue.New(db, Config)`、`Limits/Enabled/Enqueue(itemID)/Retry(itemID)/Claim(itemID)/Succeed(lease,image)/Fail(lease,failure)/RecoverExpired/Current(itemID)`。这是内部Go接口，不新增HTTP、管理页面或可直接返回客户端的Snapshot JSON。源itemID是稳定episode ID；T25-C再从探测资产映射至该ID。
- Enqueue在单个5秒事务内锁该集native_item_ids行，按`created_at DESC, asset_id ASC`从所有未隐藏TV库的可暴露、有效成功探测版本选一个来源（当前成功指纹必须匹配cloud_files和probe期望指纹，duration为有效正整数）。这是全局统一浏览媒体源顺序，不随用户当前库变化。多集文件只取首集，不为后续集生成截图条目；电影拒绝。无来源或TMDB season/episode成功快照已有still时置skipped并使旧lease失效，保留已有文件信息供B/D判断回收。
- jobs只唤醒：kind=native.still，queue=still，确定性key=`still:<主assetId>:<result_fingerprint>`，max_attempts=1；同键running只请求rerun，绝不使用jobs通用退避。执行时间唯一来源是media_stills.next_attempt_at及lease。重复入队不清零attempts或修改退避；只有来源/指纹变化或显式Retry重置。Retry同事务写native.still.retry审计，只含itemId/assetId/fingerprint。
- Claim使用SKIP LOCKED和一次DB时钟采样，在真正开始执行时建立lease；T25-C必须先拿真实ffx和linksource额度。成功写回需item/asset/fingerprint/token/attempt一致且未过期，重查三开关、主来源及TMDB still；迟到写回0行即放弃。元数据Succeed不写/检查文件，B/C必须先不可变发布文件，fence成功才建立引用，失败产物等待GC宽限期。Fail只存固定码，retryable指数退避1m/5m/30m/2h/12h/24h封顶，建议间隔优先；permanent→failed，manual→needs_action。RecoverExpired每批≤200，仅DB操作，关闭时也可恢复。

### 预算与后续边界

- 构造配置默认取链30秒、每帧45秒、三帧独立预算加收尾30秒，总需求255秒；lease默认300秒且硬上限300秒。预算不足或配置越界构造失败，不允许C启动。GCGrace默认15分钟，必须严格大于固定lease上限的2倍（>10分钟），不是只比较当前配置lease。模型与CHECK同时防超长lease；测试以数据库时间差断言，避免应用/DB时钟偏差造成错误判断。
- 本任务只提供已校验的构造入口，不接运行时启动；T25-C必须在启动时调用New并传播校验错误。T25-B实现不可变文件发布、持久卷只读核对、容量/LRU/GC；B的GC也必须复用这条宽限期规则。T25-C负责独立票据、三帧算法、背压和运行接线；T25-D负责图片三级回退、指纹ImageTag与浏览批量投影，不能直接输出当前Snapshot中的相对文件路径。


## T25-B1 截图文件契约（已接运行时，生产使用 Cache.Publish）

- `mediaprobe/stillstore` 只负责本地文件，接口为 `Open(root)`、`Publish(ctx, assetID, fingerprint, jpeg)`、`Read(ctx, relativePath)`、`Close()`。不获取链接、不调用ffmpeg、不读写数据库、不注册worker或HTTP；容量准入及LRU/GC留给B2，B2完成之前不得启用本包生成生产截图。
- 默认根目录 `/data/native-media/stills`；测试/后续配置也必须使用绝对、规范化且末两级为 `native-media/stills` 的目录。文件路径严格为 `<assetID前2位>/<assetID>-<fingerprint前16位>.jpg`，沿用A的 `ImagePath`；`Read` 不接受URL、任意文件名或客户端路径。错误均为固定短文本，不拼接路径、JPEG内容或底层错误。
- JPEG输入最多4MiB、宽不超过1280、高不超过16384且像素总数不超过16Mi。先校验DecodeConfig边界，再完整解码；成功receipt含相对路径、SHA-256、宽高，不含临时链接。C必须使用固定ffmpeg白名单输出并剥离上游metadata；本包不是任意图片上传服务。
- 从 `/` 开始按目录fd逐段 `O_NOFOLLOW` 打开，连指向根内的符号链接也拒绝；只创建末两级和两位十六进制分片目录。本包新建目录0700、文件0600；叶子和锁只允许单硬链接的普通文件，拒绝FIFO/设备等特殊文件。已有目录不会被自动chmod/chown。Linux/Darwin受支持，其它平台fail closed，不退回存在覆盖竞态的普通rename。
- 发布顺序为独占新建随机tmp、完整写入、文件fsync、关闭、原子不可覆盖rename（Linux `RENAME_NOREPLACE` / Darwin `RENAME_EXCL`）、目录fsync，然后才返回receipt。最终文件名由业务资产ID和指纹决定，随机数只用于tmp。已有同内容文件返回同一receipt且不修改mtime；不同内容返回冲突，绝不覆盖。C在此之后才能用lease fence建立数据库引用；fence失败的文件不得立即删除，应等待B2宽限期GC。
- 发布使用根目录 `.lock` 的跨进程flock（每次独立打开），非阻塞重试且等待受ctx约束；操作预算10秒，调用者更短ctx优先。图片Read不拿flock，RWMutex只协调Close和活跃操作；没有DB事务。ctx无法强制中断内核中的本地fsync，故此预算是协作式文件操作预算，不是操作系统I/O硬截止。
- 普通失败仅清理本次tmp并同步目录；rename后失败/取消不返回receipt，保留完整无引用文件。测试用真实子进程中途退出验证：崩溃前tmp不会被Read看见，rename后最多残留完整孤儿，重试能幂等收敛；B1不提前清理这些崩溃残留。B2必须保留 `.lock`，通过已批准的GC宽限期处理孤儿/tmp，不能跟随符号链接或越出根目录，也不能依赖文件系统atime判断LRU。
- Docker只在runtime现有mkdir/chown中增加此目录，属主app、仍以UID/GID 65532运行，build阶段不动。生产挂载已由协调者只读确认：`/root/docker/mediatool/data` → `/data`，是持久存储；本任务未再次访问或修改生产。bind mount会覆盖镜像目录，R2仍需验证app用户可写，不自动修复宿主卷权限。
- B2已将索引与 `cache_evicted` 合入未发布00060（不建00061），并提供淘汰超过24小时冷却期的条件重新入队仓库方法；T25-D已在图片请求中接入，冷却期直接回退季/剧海报。B1的底层文件职责不改变，运行入口和ImageTags由C/D负责。

- B1验证：全仓build/vet、68个有测试包的无数据库race回归与新包errcheck通过；新包macOS race连续10轮，Linux ARM64非root沙箱连续10轮、AMD64模拟执行1轮通过。路径fuzz 15秒/218925次，语句覆盖率85.3%，58份既有golden SHA-256不变。真实子进程退出、并发/符号链接替换、FIFO/硬链接拒绝均覆盖；Linux实测还校正了对合法发布抢占空文件名窗口的测试预期。本轮未改数据库/迁移，Up→Down→Up留给B2；完整镜像构建与发布留给R2。


## T25-B2-A 截图容量仓库（文件准入/GC 已接入）

- 直接补充未发布的00060，不新增00061，Schema仍为60。新增`size_bytes`（成功快照大小）、`reserved_bytes`（当前lease容量预留）、`evicted_at`，范围CHECK与部分索引配套；`skipped/cache_evicted`是缓存淘汰状态，不作为ffmpeg worker失败原因。旧完整快照保留、大小以0表示未知，仍可Current读取；未知大小不当作免费容量，Admit返回ErrUnknownSize，待B2-B测量并以快照fence校正。
- `Config.MaxBytes`默认2GiB，可配置1字节至1TiB（小值用于合成测试/限制，单图仍≤4MiB）。`Image.SizeBytes`由B1验证实际JPEG字节数后提供，不接受调用者宣称的零/超限大小；B1不可变路径和文件发布顺序不变。
- `Usage(ctx)`读取数据库成功快照size合计、未过期lease的有效预留合计及未知大小数，不读取目录/atime。容量准入`Admit(ctx,lease,size)`通过数据库非阻塞事务级advisory锁串行化；2秒操作预算、200ms行锁等待、1.5秒SQL预算，清理rollback另有最多1秒预算。事务内没有外部服务调用或文件I/O。
- 超限时LRU按`last_access_at,item_id`稳定排序且SKIP LOCKED，每次最多200个候选，不淘汰当前请求自身；先使既有用量≤min(90%上限,上限−新图大小)再预留。仍未达到目标时可提交已完成的淘汰进度，但`Admission.Reserved=false`，不得发布，后续有界重试续做。单图超过配置容量直接不准入，不为不可能容纳的图片清空缓存。
- 预留依附同一行的asset/fingerprint/token/attempt；重复Admit不会重复累计。Succeed必须匹配预留大小及有效fence，单条UPDATE把reserved_bytes清零并写成功size；失败保留旧快照size。Fail/Retry/来源切换/TMDB跳过/lease过期RecoverExpired同时释放预留；过期但尚未被每批≤200恢复扫描清理的预留，也立即不再计入有效额度。旧worker不能占用/消费新lease的预留。
- `EvictSnapshot(ctx,observed)`及Admit的LRU清理使用item/path/digest/published_at作为引用fence，事务中先清全部成功引用、使旧lease失效、释放预留并记录skipped/cache_evicted。只有明确提交成功才返回可处理的淘汰信息；错误或不确定提交不授权文件删除，最多留下孤儿。**本任务完全不删除文件**；B2-B必须在提交之后持发布锁并重查引用，再做fd相对路径删除，失败交GC。
- `TouchBatch(ctx,[]Access)`每批≤200，校验item/path/digest并用数据库时间条件更新，跨并发进程保证每张当前图片每小时最多更新一次；不改updated_at。B2-B才接有界内存合并/刷出，不在本任务增加图片热路径写库。
- `RequeueEvicted(ctx,itemID)`供T25-D图片请求调用：不足24小时不变更；满24小时且仍为cache_evicted时，在集级行锁事务中只重排一次并用既有jobs合并唤醒，不产生管理员审计。普通周期Enqueue不会恢复同来源的淘汰图片（即使已过24小时），避免缓存淘汰循环；显式管理员Retry可以覆盖冷却期并照常审计，新的来源/指纹可产生新工作。
- B2-A只承担数据库元数据；发布流程、内存触达合并、文件删除和GC已由B2-B完成，并由C/D接入。生产准入入口是Cache.Publish，不直接使用B1原始文件接口。三开关与GCGrace>2×MaxLease（5分钟）约束不变。

- B2-A验证：仅media_tool_test/Redis DB15、使用现有串行测试锁；migrations全量数据库回归通过，最终截图定向16项（本轮新增9项）通过，包含有效预留下Up→Down→Up、脏旧数据、D2常量/CHECK匹配、泄漏恢复、并发额度、LRU批次续做、每小时写入节流及24小时重排合并。延迟约束模拟commit失败，验证不会返回可删路径，原引用与额度写入回滚。全仓build/vet/race（68个有测试包）、新包errcheck和专门D2复跑通过；未改浏览golden，56份留存基线SHA-256不变。


## T25-B2-B 截图准入与维护组合（worker/图片端点已接入）

- 唯一运行时文件入口为`stillstore.NewCache(store, repository)`的`Publish(ctx, lease, jpeg)`；构造时再次Normalize预算，确认GCGrace严格大于2×MaxLease。原始Store.Publish仍供底层测试，不得作为生产绕过准入的入口。没有自动启动goroutine/定时器；C负责周期调用Flush/GC并在receipt后执行Succeed，D负责图片端点接入。
- 发布持同一根目录flock，先对≤200个未知大小引用逐个安全stat；可校正则用item/path/digest/published_at fence更新size，任何不可校正（缺失、权限、符号链接、非受控路径、大小异常）均按文件不存在清引用，而不是永久保留未知size。只有确认提交后才考虑删文件。未知集处理后缩小，不因一个坏文件永久拒绝全局；超过一批或预算、数据库临时故障、额度不足只返回`RetryError`，固定原因码及1分钟建议，由C按独立状态机退避。底层Admit的未知大小保护仍保留，不能绕过组合入口。
- 新增`Calibrate`把“旧成功引用”与“正在执行的新尝试”区分：校正失败时，若当前有有效grabbing lease，只清旧引用、记录evicted_at，保留新token/attempt/reservation；否则按skipped/cache_evicted淘汰。原始LRU EvictSnapshot仍会使旧lease失效。这样同一集缺失的旧图不会让当前修复尝试陷入冷却循环。所有写回仍受快照fence保护。
- 限额统计仍是数据库成功size及有效预留，不为每次发布扫描目录。Admit提交的LRU淘汰结果先清引用，再在文件锁内临删除重查`Referenced`（当前引用或有效尝试的预期路径），确认无引用才fd相对unlink并fsync目录。删除失败只递增CleanupFailures、留下GC孤儿，不把已清引用的坏文件变成新发布的永久阻塞。逻辑限额不包含尚处宽限期/删除失败的孤儿，磁盘实际占用可能暂时高于逻辑限额。
- `Read(ctx, snapshot)`校验当前文件digest，缺失/损坏/不安全文件统一ErrNotFound供D回退，不返回内部文件错误；成功只更新内存触达表，不写数据库。采样表最多4096条，溢出丢触达采样但不阻塞读图；Flush每批≤200，失败保留脏采样、小时内合并后续命中，DB条件UPDATE仍跨进程确保最多每小时一次访问写入。持久LRU按采样到的last_access_at/item_id排序，不依赖atime。
- `GC(ctx, limit)`上限200、默认调用预算2秒（调用方更短ctx优先），计数包括引用与目录探测，稀疏目录也有界。优先处理未知size，按ID分页stat校正其它引用/清缺失引用；确认数据库用量超限时按LRU有限回收至90%（不足一轮则下轮继续）。已知size偏差在扫描中修正，随后的维护轮/下一次准入采用真值，不允许新发布绕过限额。
- GC保留数据库ID、当前分片目录fd及待处理叶子游标，跨轮续扫；取消时保留未完成的叶子，单个目录读取失败则释放fd并向后推进，不阻塞其余分片。进程重启安全重新扫描，不声称游标持久化跨进程。宽限期按文件mtime只判断孤儿/tmp年龄，默认15分钟、严格>2×5分钟lease上限；它不参与LRU访问时间判断。所有本地FS预算均为协作式，无法强行中断内核中挂起的stat/fsync。
- 目录扫描仅固定00–ff分片，叶子仅接受canonical JPEG或`.tmp-<32位hex>`；.lock及其它文件名永不删除，所有路径逐段O_NOFOLLOW、叶子普通文件/单硬链接校验沿用B1。新孤儿宽限期内保留，有当前引用或有效尝试的文件即使很旧也不删。00060增加引用路径与未知size部分索引，Schema仍60、不建00061。
- B2-B不接TMDB优先级切换和图片路由。D切换后应按既有fence清掉不再使用的本地引用，GC才会把对应文件作为孤儿回收；没有引用清除证据，不会擅自删除仍被数据库引用的图片。

- B2-B验证：media_tool_test/Redis DB15现有串行测试锁下，最终22项截图集成（本轮新增6项）通过；含坏stat不阻塞其它新帧、同集修复保留有效lease、提交后删除、有限退避、size/限额校正、索引Up→Down→Up与失败修复提交后重试。全仓build/vet/race（68个有测试包）、D2、errcheck、两包连续5轮race通过；GC白名单fuzz 264373次通过。Linux ARM64非root连续3轮通过。
- **R2发布前风险**：本地ARM64 Linux上的AMD64模拟执行，多线程整包两次在标准库JPEG并发解码中SIGSEGV；本轮前存储实现的基线压力测试也出现Go运行时free-object错误。当前隔离并发测试10轮及GOMAXPROCS=1整包通过，但根因未确认，不能据此宣称原生AMD64已验证。发布前必须在原生AMD64 CI/主机复核；未修改生产逻辑来掩盖这一问题。


## T25-C/D 截图执行与图片三级回退

### 阶段一：选帧与原生 CI
- 选帧严格15%、12%、18%，每帧取得独立linksource票据并Claim一次，Ticket UA原样传ffx；每帧取链/截图分别应用已验证的独立预算。全局Still额度跨三帧串行持有，未持有帧额度和首帧取链额度前不领取media_stills lease。
- JPEG在Go中检查≤4MiB、宽≤1280、像素≤16Mi；亮度采用Rec.709加权RGB均值，均值≤8或≥247视为接近全黑/白；Welford计算方差。先遇正常帧就停止；三帧均不合格时选择方差最大者（同值保留首帧）。小于10ms或无法在ffx合法范围取非零偏移的异常时长拒绝，不在0秒截图。输出固定等比框1280×1080、JPEG quality=3，并以-map_metadata -1剥离上游metadata，不开放任何调用方argv。
- verify在Go测试前安装ffmpeg/ffprobe，设置MEDIA_TOOL_REQUIRE_FFMPEG=1；真实H264/AAC、轨道/HDR等和黑场样片缺工具/编码器时失败，不允许静默Skip；verbose日志能看到真实用例PASS和REAL_FFMPEG统计。额外原生ubuntu AMD64 go test -race -count=3 -v ./internal/modules/mediaprobe/...必须通过，publish仍依赖verify。本轮只修改门禁，未运行GitHub Actions，R2后由协调者检查原生结果。

### 阶段二：独立后台执行
- stillrunner.Service构造通过stillqueue.New执行T25-A预算校验，失败直接传播启动；不在构造时访问链接或打开目录。独立still worker仅一个、使用原有jobs和app进程，不新增容器；jobs始终只唤醒，执行错误由media_stills独立退避，Cache.RetryError映射retryable及建议间隔。
- 帧和首帧取链真实额度均在Claim之前取得，开始后以DB时钟建立≤5分钟lease；期间250ms检查enabled/probe_enabled/stills_enabled，关闭任一即取消。成功仅Cache.Publish后fenced Succeed，迟到结果放弃；日志仅assetId及固定结果码，不记录临时URL、JPEG或stderr。
- probe成功且fence提交后EnqueueAsset（电影不生成截图）；失败/丢唤醒由Episode ID游标每批≤200的周期扫描补齐，过期lease同时回收预留。维护周期30秒，Flush/GC仍使用B2预算；被TMDB/资格显式跳过的旧引用限批清理，文件由GC处理。server-only启动维护但不启动截图worker；all共享同一文件缓存与图片读取服务；worker-still支持独立队列角色。
- 默认原生服务关闭，不入队/执行，也不在/data创建截图目录；启用生成或读取已有快照时才安全打开目录，打开失败可重试而不锁存失败。

| 配置 | 默认 | 约束 |
| --- | --- | --- |
| NATIVE_STILLS_ROOT | /data/native-media/stills | clean绝对路径且末两级固定；运行时逐段拒绝符号链接 |
| NATIVE_STILLS_MAX_BYTES | 2147483648 | 1..1099511627776，逻辑快照/预留限额 |
| NATIVE_STILLS_FRAME_TIMEOUT | 45s | 正数≤2m，且与取链预算组合必须通过lease校验 |
| PLAYBACK_RESOLVE_TIMEOUT | 30s | 沿用现有5s..1m；例如60s取链需单帧≤30s才满足3帧+30s收尾≤300s |

### 阶段三：端点、Tag及投影
- Episode Primary优先TMDB still，再本地截图，再季/剧海报。来源均以服务端条目ID与当前可暴露资产决定；未暴露/隐藏库404，不因本地文件存在而放行。匿名IP限速和认证用户豁免规则不变；Backdrop/Logo等不会误返回本地Primary截图。
- 本地ImageTag为SHA256("native-still:"+完整result_fingerprint+":"+digest)，浏览与端点使用同一函数；只有请求tag等于当前tag才immutable，无tag或不匹配为3600秒。HEAD不含body，If-None-Match支持304。
- 资产信息中的旧TMDB still也按全局来源选取：最新可暴露版本优先、同时间按asset UUID升序，最多100版本；以EXISTS判断非隐藏库，避免跨库重复行消耗版本上限。同集跨库使用同一公开图片和Tag，不由当前用户库内版本单独决定；隐藏/缺失资产不会参与。
- 浏览只对当前页Episode IDs做一次native.still.page批量查询（≤200），纯数据映射，不stat、不取链、不入队。episode快照→season快照→全局可暴露版本的资产持久化TMDB still的统一优先级，本地成功快照不要求尝试state=ready，因此重探/失败期间保留旧成功图；无本地时Primary tag对应季/剧海报。新增两份合成本地/海报回退golden，已有纯映射golden不变。
- 图片请求发现引用文件缺失或损坏，清坏引用后回退海报，不返回500；被淘汰后24小时内只回退，满24小时通过RequeueEvicted原子重排一次。新的probe/来源变化仍按A规则可产生新工作。TMDB补齐后立即优先并清本地引用，文件保留宽限期后由GC回收。
- 本轮浏览测试夹具升级到schema60；有意测试旧格式迁移的夹具仍保持对应版本，不在生产浏览增加旧schema兼容分支。

### 本轮本地验证（2026-10-17）
- darwin/arm64、Go 1.26.9、ffmpeg 8.1。全仓 `MEDIA_TOOL_REQUIRE_FFMPEG=1 bash scripts/test-go.sh` 串行race通过：69个测试包、820个顶层PASS；仅显式生产导出fixture和子进程入口helper默认Skip，真实媒体用例均执行。
- 最后统一episode/season/资产still优先级后，再串行复跑26项探测/截图/图片专项，全部PASS（含两条投影路径的Tag一致性、00060 Up→Down→Up及脏数据）；既有88份golden哈希不变，新增本地/海报两份golden。
- 本机严格真实媒体三轮 `go test -race -count=3 -v ./internal/modules/mediaprobe/...` 通过；黑场样片三次均使用3帧，选中18%处正常帧。build/vet、mediaprobe和nativelibrary的errcheck、22项Python检查均通过。原生AMD64 Actions结果仍待R2推送后确认，未部署或访问生产。


- 追加跨库旧still差异回归后，最终对migrations、httpapi、mediaprobe及nativelibrary的全部测试再次串行race复跑通过（12个包、245个顶层PASS，其中Native数据库集成79项；仅子进程helper入口Skip，真实ffmpeg均执行）；全局资产图片查询改为物化同页剧/季的集键后批量排名，避免逐集重复扫描。3,000集的剧信息及首/末200项页面合计2.530秒（原6秒预算未放宽）；10,000电影带探测快照冷首页683ms、末页335ms、Counts144ms，200 ID装配7次SQL。最终构建/vet/errcheck/Python检查均通过；本地仍不代表原生AMD64门禁通过。


## T26 管理界面与 P2 收尾

P2（T21–T26）实现完成：受控 ffx、可信取链、探测状态机与执行、白名单 MediaStreams、截图独立状态机/存储/GC/执行/图片回退，以及管理 API/UI 均已接入。上面 T21–T25 小节保留子任务交付时的分层设计和历史范围；当前接入状态以本节与阶段表为准，不表示已经发布到生产。

### 管理契约与边界
- 沿用 `/media-server` 页面，不新增顶级路由。三个开关保存时必须提供全部布尔值和 revision；原生服务关闭时另外两个配置仍保留，但不执行。截图还依赖 probe_enabled 与成功时长。revision 不匹配返回409，更新与审计同事务。
- GET summary 返回 settings、probe/still 状态数组和 capacity（usedBytes/reservedBytes/limitBytes/unknownSizes）；容量上限沿用 NATIVE_STILLS_MAX_BYTES，默认2GiB。统计是当前尝试状态；重探失败时仍可用的旧成功快照不额外计入 ready。probe 没有 skipped 状态，只有 still 才有，不伪造新状态。
- GET tasks 参数 kind=probe/still、state 可为空（全部）、page 默认1、pageSize 默认50/上限200，offset≤100万。一次SQL使用相同过滤条件计算总数和页面，稳定按 next_attempt_at/key 排序；名称只在本页读取。返回 id/name/state/errorReason/attempts/nextAttemptAt，不返回文件路径、临时URL或 Probe 原文。原因码在页面译成中文，未知码仍保留可辨认文本。
- POST retry 请求 `{kind,id,state,bulk,confirmed}`。单条 id 为资产UUID或单集稳定数字ID；批量 id 为空且 confirmed=true，界面必须二次确认。按选取时的筛选快照最多执行500项（多取1项仅判断 limitReached），25秒总预算；不持有整批长事务。返回 selected/queued/skipped/failed/limitReached，部分失败不宣称全部成功。先持久化批量/单条意图审计，成功重排仍走原仓库逐项事务审计；审计失败不会开始重试。
- 手动 GC 不取链、不生成截图，允许管理员在服务关闭时清理缓存。先审计请求，再调用同一缓存实例/锁/游标的一轮 GC；返回 scanned（逻辑扫描项，含引用校正/目录探测）、reclaimed、reclaimedBytes、done、partial。只统计确认删除的文件。到预算且有进度或清理出现失败时返回 partial，未开始或不可用返回503；沿用前缀/符号链接/宽限期防护。
- 所有接口只注册管理端口：管理员会话、写请求CSRF、维护模式保护；媒体端口所有方法404。GET读取不写审计，开关/重试/手动GC写审计。共享样例位于 `internal/platform/httpapi/testdata/contracts/native-processing.json`，Go严格解码，Vue API及组件测试复用。
- 管理端与媒体图片端共享同一stillrunner.Service；e2e使用物理规范化的临时NATIVE_STILLS_ROOT并强制原生服务关闭，不接触生产截图目录或115。
- 界面区分加载、错误、真实空列表、部分成功和分页；批量确认期间冻结目标筛选，过时响应不覆盖新筛选，读取失败清除旧行。沿用既有组件、配色和移动端表格滚动，不引入字体/组件依赖。

### 电影冷首页性能核对
- 同一1万电影合成夹具、ID预分配、带探测快照、race模式连续三轮：615/445/493ms（中位493ms）。SQL分页阶段分别375/253/281ms；探测快照读取25/20/21ms，媒体源21/17/17ms，TMDB快照6/5/5ms，其他SQL约5–8ms；其余约145–180ms为解码/装配/校验等Go路径与调度开销。
- 断言电影页 `native.still.page` 查询次数为0：没有运行截图批量SQL或集键计算。历史418ms与683ms缺少同轮分段记录，不能把其265ms差值伪归因于某个新增查询；此次可复现的主要开销是通用分页/计数SQL及机器冷热/调度波动，而非截图接线。未增加索引或扩展，也未放宽超时；T53再评估该分页/计数结果的Redis缓存，不承诺一次性延迟差值已被消除。

### P2 遗留清单（不阻断代码阶段完成，不等于发布验收完成）
| 事项 | 后续任务 |
| --- | --- |
| 原生 ubuntu AMD64 的真实 ffmpeg 与三轮race门禁结果；失败阻止发布 | R2 |
| 验证app用户可写生产 `/data/native-media/stills`；不自动chmod/chown宿主业务卷 | R2 |
| Emby 4.8 网关日志实际 VideoRange/VideoRangeType（HDR/DOVI）值 | T52 |
| Hints/Genres/Filters在搜索词过滤前的5万候选上限；大库搜索投影/索引/缓存评估 | T53/T54 |
| `strm.go` 两处未检查的清理错误 | T54 |
| T22 grants数据库故障由403修正为503，发布说明需记录；镜像实际体积核对 | R2 |

本轮不访问生产、不触发真实115请求、不改变既有STRM或媒体文件；不开始P3。


### T26 最终验证记录（2026-10-17，本机 darwin/arm64）
- 冻结源码后的全仓 `MEDIA_TOOL_REQUIRE_FFMPEG=1 bash scripts/test-go.sh`：70个测试包、829个顶层PASS、零失败。仅生产导出fixture与子进程helper入口按设计Skip，真实ffmpeg用例实际执行；既有Emby golden、D9边界、00060 Up→Down→Up/D2均通过。最终同夹具电影冷首页513ms（分页296ms），继续断言截图SQL为0。
- 新增真实测试库HTTP链：注册管理员、Cookie会话、无认证401/缺CSRF403、开关revision冲突409、禁用503、媒体端全部方法404；500上限/501项分批、审计失败回滚与阻止重试、孤儿GC实际回收且保留引用图片，均通过。
- Vue 29个文件/95项测试通过；vue-tsc与Vite构建通过；Playwright烟测2项通过（真实管理员/实际进程GC，UI队列交互用共享契约夹具，队列真实写入由Go集成测试覆盖）。桌面及390px移动端视觉检查通过，无页面横向溢出；宽表格在容器内滚动。
- `go build ./...`、`go vet ./...`、mediaprobe全部errcheck、22项Python检查与`git diff --check`通过。未运行原生AMD64 Actions，未提交/推送/部署；P2代码完成不替代R2发布验收。


## T31/T32 PlaybackInfo 与原始流 302（已必审通过）

### 协议、地址及数据
- GET/POST `/Items/{id}/PlaybackInfo` 已接入，含无前缀、`/emby`/`/mediabrowser` 与路径/参数名大小写兼容。Movie/Episode 才可播放；库、剧、季及不可见条目404。跨用户UserId403；可浏览但无播放/下载权403，根ErrorCode及ResponseStatus.ErrorCode均为NotAllowed，Message为固定中文原因。
- 接受DeviceProfile对象/null、MediaSourceId、UserId、StartTimeTicks、MaxStreamingBitrate、AudioStreamIndex、SubtitleStreamIndex、EnableDirectPlay/DirectStream/Transcoding；POST可空，显式null按未指定处理。已知整数溢出/负值（流索引允许-1）或布尔非法400，正文≤256KiB，标量大小有界，冲突值拒绝。未知兼容字段忽略。
- 只有MediaSourceId用于选择版本、UserId用于绑定当前用户；其余字段解析后忽略，不执行服务器seek/音轨切换/字幕转换/限码率/转码。DeviceProfile和EnableTranscoding不能开启能力。每个允许播放的可见版本保留已有UUID媒体源Id和版本名，技术字段仅取当前成功探测快照（重探/失败期间仍可用）；缺快照保留原降级，MediaStreams空数组。跨库版本分别检查库级权限，不因同片另一版本可播放而放行当前版本。
- Path绝对URL和DirectStreamUrl相对URL均由唯一StreamURLs构造：`/Videos/{itemId}/stream.{ext}?Static=true&MediaSourceId=...&api_key=<当前用户token>`（参数排序不影响语义）。不输出云路径、pickCode、STRM正文或全局key。ext优先探测/既有容器，未知用mkv；只是原始文件的URL提示，不承诺封装转换。
- 公开origin统一供Public Info与PlaybackInfo使用：后台memberUrl > MEMBER_PUBLIC_URL > 可信直接代理的单值X-Forwarded-Host/Proto > 请求TLS/Host。未配置会记录一次警告；不信任非配置代理的转发头，也不从客户端DeviceProfile获取地址。
- DTO能力不可写：Protocol=Http、IsRemote=true、DirectPlay/DirectStream=true、Transcoding/Open/Close=false；没有TranscodingUrl。三个新增合成golden位于nativeplayback/testdata（单/多版本及无快照），既有浏览golden不变。

### 会话与槽位
- 沿用Schema60已有native_playback_sessions和playing/paused/stopped常量，无新迁移/状态。PlaybackInfo创建/复用未激活记录：state=stopped且ended_at=started_at，带user/device/item/source，不计入并发；同键已有活跃播放时只复用Id，不停止或靠metadata请求延长其TTL。
- 真正取流按顺序：T41用户状态/令牌 → T42可见性和库级播放/下载权 → playeraccess白名单 → 当前条目的可暴露MediaSourceId → 原子槽位 → 取链。未知/外部媒体源ID在UA检查之后统一404；已知可见但无权的版本先拒绝权限。
- 槽位只在短事务中锁定该user行，重查禁用/过期/credential_version、device归属与max_streams，按(user,device,media_source)去重；跨进程并发不会超售。TTL固定5分钟，与T42一致；取流/seek刷新，过期活动记录收敛stopped。客户端崩溃不会永久占满。
- 取链失败释放本次新占槽，last_progress_at时间fence防止迟到失败覆盖更新的seek；已有活动流的失败seek不主动误停原流，最多保留TTL。未激活旧记录按用户限批清理，不删除真实播放历史。
- T31/T32验收时尚未接Playing/Progress/Stopped；本组T34已接续约，并按同user/device/source合并PlaySessionId，防止重复槽位。

### 取链、安全与预算
- GET/HEAD stream、stream.{ext}、original.{ext}及Download均为302，显式Content-Length=0，body为空，不使用默认会生成HTML的http.Redirect。Static=false、m3u8/mpd及明确转码参数拒绝；不下载CDN媒体字节、不代理、不HLS、不转码。
- 新实现放nativeplayback/runtime，协议层仍仅依赖纯契约/DTO。新增Service/Actor携带T41设备身份与T42请求权限；旧StreamResolver签名缺少设备上下文，保留兼容的失败闭合默认实现，不拿不可信DeviceId补齐它。实际端点只调用新契约。复用只读STRM读取器、ParseStable和playbackgrants：文件必须在配置根内、普通文件、≤8KiB、UTF-8且与owned artifact摘要一致；校验正文/DB路径与asset/pickCode关联，不创建、不修复、不重写原STRM。取链后再次检查可暴露性、资产revision、路径/pickCode/大小和STRM摘要，避免发出已失效资产的地址。
- 使用实际播放器UA调用DynamicStorage → 现有官方115客户端，保留授权刷新、身份/大小核对和R1-F“路径查询无响应超时至多一次、预算足够才重试”的策略；不额外包一层重试。探测可信key/固定UA不参与此路径。
- 沿用UA/IP/provider版本维度的Redis短缓存；缓存未命中取链并发≤4（无Redis也有界），115限流置60秒冷却并带Retry-After返回429。冷却放在缓存下层，已有有效缓存仍能播放。授权失效/上游不可用/超时503，不可暴露/文件缺失/错源404。目标必须通过原playback.Policy的HTTP(S)/主机/DNS防私网检查，不接受额外headers或UA不匹配链接，Location≤8KiB。
- 监听器30秒WriteTimeout不变。PlaybackInfo总预算6秒；串流含认证/权限/取链总预算28秒，实际取链还受配置requestbudget（默认30秒、上限60秒）与剩余HTTP预算共同约束，预留响应时间，不靠放宽WriteTimeout掩盖问题。
- 私有管理日志保留完整302 CDN Location；请求URL保留业务查询但脱敏所有token/apiKey别名，不收集认证头或正文，不记用户token原文。R1-F阶段日志仅固定结果/数值计时，不存原Probe JSON或临时链接到会话表。临时CDN链接只按现有网关缓存规则存在Redis/内存和授权管理员私有日志中。
- D9回归检查播放运行包的传递依赖：禁止ffx、probequeue、stillrunner、linksource等执行包与os/exec，仅允许纯probeinfo结果包；协议包继续禁止pgx/redis/exec。

### 本组验证范围
- 全部115调用通过假Transport，CDN DNS用注入的合成公共地址，不访问真实115或CDN。完整Authenticate→Items→PlaybackInfo→302、真实POST/null客户端字段、探测/降级多版本、GET/HEAD/前缀/大小写、令牌各来源、全局key、跨用户/禁用/过期/权限/UA、错源、并发去重/TTL、缓存UA/IP隔离/冷却、失败释放、取链期间missing二次fence均有测试。
- 1万电影、ID已预分配的真实HTTP PlaybackInfo（包括T41认证、T42权限、SQL投影及未激活会话写入）：首轮195ms，复跑162ms，低于6秒预算；最后全量复跑数据在本任务汇报中记录。
- 新包及新共享读取器errcheck通过；strm.go原有两处defer清理错误仍明确留T54，本组未顺带改写原写入/清理行为。无新增依赖/扩展，不访问生产，不修改既有媒体/STRM，不开始T33/T34。


### T31/T32 最终本地验证（2026-10-17）
- 全仓串行race：72个测试包、842个顶层用例PASS、零失败；真实ffmpeg用例实际执行，唯一按设计跳过的是生产导出fixture与子进程helper入口。本组115及CDN始终使用假Transport/合成DNS，无真实115访问。
- 最新播放专项与1万夹具共7项顶层PASS，包含POST探测快照及null兼容、并发seek/不同设备限流、UA/IP缓存隔离和冷却缓存命中、HEAD零body、权限与missing二次fence。完整认证HTTP PlaybackInfo全仓174ms，最后专项188ms，均含会话写入且不占槽。
- 新增3份播放golden；67份既有JSON/golden哈希未改变。build/vet、协议/播放D9、Go契约/JSON安全/全局key拒绝、22项Python检查通过。nativeplayback全部、linksource及共享只读strm/read文件errcheck通过；strm.go旧第97/111行两处defer错误未改、仍留T54。
- 无新迁移/扩展/依赖；既有Schema60迁移往返与D2通过。本机为darwin/arm64，不代替R2原生AMD64门禁；未提交/推送/部署。T33/T34尚未开始。


## T33/T34 字幕与播放上报（2026-10-17，待本组审核）

### 字幕只读核对与交付
- 只读核对本机运行库 cloud_files、strm_entries（数据库会话设置 default_transaction_read_only）和本地STRM输出目录，五类外挂字幕 srt/ass/ssa/vtt/sup 均为0；没有访问远程生产，也没有导出媒体文件名/正文。
- 本组按“没有外挂字幕”分支交付：探测字幕保持原始stream Index，DeliveryMethod=Embed、IsExternal=false，无DeliveryUrl。客户端从原文件CDN直链自行读取。两种字幕端点通过T41认证后统一404，HEAD也无正文；不抽取、不转换、不取链、不占槽位。没有新增字幕关联或修改STRM生成流程。后续若实际发现外挂字幕，先单独审核只读关联/取链方案，不能将其伪装成已实现能力。
- 仅movie-probed/episode-probed及items-probed三份已有golden增加字幕交付字段，缺探测快照及非字幕流不变。

### 上报协议与会话
- 已核对Emby官方PlaystateService：现代JSON端点Playing/Progress/Stopped，以及以PlaySessionId查询参数调用的Ping；旧POST /Users/{UserId}/PlayingItems/{Id}、DELETE同路径、POST同路径/Progress使用查询参数（PositionTicks、IsPaused、MediaSourceId、PlaySessionId），Progress的EventName也可从JSON正文传入。资料：https://dev.emby.media/reference/RestAPI/PlaystateService.html 及其各操作文档。
- 兼容前缀/大小写、正文与查询参数；冲突值400，路径/正文UserId与令牌不一致403。DeviceId仅来自认证，不接受客户端替换设备归属。成功204空正文。
- 优先PlaySessionId（需属于当前用户和设备），否则按用户/设备/条目/可选版本取最近会话；未知会话加合法条目可自动创建；没有可解析会话且缺ItemId时400。由T31产生的stopped+ended_at=started_at草稿可升级playing；真正stopped为终态，迟到Progress/Ping/Stopped不复活。无显式旧ID的新Playing可开启重播。
- 与T32共用短user行锁，在事务内防御性重查账号/设备/max_streams，按用户+设备+媒体源合并槽位。PlaySessionId可在同一条目内切换至另一个有播放权的版本，合并旧/新槽位并保留原ID；跨条目的版本404，不因切换超售或重复计数。不会在事务内调用外部服务。普通Progress/Ping每10秒至多落库一次（请求仍进行即时认证权限检查），因此心跳持久化最多延迟10秒，远低于5分钟TTL；暂停/继续/停止及位置跳变>30秒立即写入。
- Schema61新增play_counted布尔值：单次会话超过90%只计数一次，重复报告/并发Stopped不重复增加PlayCount；新重播可再次计数。playing/paused/stopped常量和CHECK不变。
- 用户进度只使用所选版本成功探测快照durationTicks：<5%或时长<5分钟不记续播，>90%标已看、次数+1并清续播，其余保存位置；未知时长仅保存位置不自动标已看。每次实际进度写入更新LastPlayedDate。Ping不修改条目数据；Episode使用单集稳定ID，不写Series聚合节点。
- app启动及每分钟按≤200行、SKIP LOCKED收敛过期会话，不修改任何用户观看数据。只读管理列表不返回token、云路径、媒体源路径或STRM正文。

- 缓存清理：删除 /private/tmp/media-tool-t3334-cache.LZKBlaln、对应cache-path/本组临时脚本与测试日志；执行全局 /Users/dingding/Library/Caches/go-build 的 go clean -cache；实际释放 2412.73 MiB，未清理GOMODCACHE。


### T33/T34 最终本地验证（2026-10-17）
- 全仓 `MEDIA_TOOL_REQUIRE_FFMPEG=1 bash scripts/test-go.sh` 串行race：72个测试包、856项顶层PASS，零失败；真实ffmpeg Probe/Still/黑场回退旧回归实际执行。仅按设计Skip生产导出fixture和子进程helper入口。
- 最后最新播放/字幕专项19项顶层PASS、0 FAIL、0 SKIP（11项本组新增集成、6项T31/T32回归、D2及Schema镜像标签检查）。覆盖完整时序、三段阈值/未知时长、节流/Ping、旧接口、重复/并发Stopped、跨用户/设备、续约保槽、201条过期会话分批收敛、单集数据、无位置不清续播、过期完成标记隔离、同PlaySessionId多版本切换（max_streams=1与已有双槽位）、跨条目版本拒绝。
- 00061完成Up→Down→Up，Go Schema与Docker label同步61；三状态枚举/CHECK对照通过。首次全仓拦截到旧Docker label=60，已修复后复跑通过；首次扩展专项未设置TEST_REDIS_URL导致1项Skip，补齐DB15环境后19项均实际执行通过，不将Skip计为通过。
- build/vet、播放与协议包errcheck、D9、22项Python通过。新增2份HTTP/API契约；3份已有探测响应golden仅增加字幕Embed/IsExternal=false，其余golden通过。未改STRM生成/正文/文件，不抽取字幕，不访问真实115或远程生产；不开始T35/T36，提交/推送/发布留R2。

- 补验缓存清理：删除 /private/tmp/media-tool-t3334-cache.JNqPq1iN 及cache-path/临时脚本/日志，执行全局 /Users/dingding/Library/Caches/go-build 的go clean -cache；本轮释放 700.60 MiB（本组首次已释放2412.73 MiB，此轮为补验重新生成的缓存），未清理GOMODCACHE。


## T35/T36 用户数据与客户端回放（待审核）
- 所有实际浏览/详情/Latest/Resume/NextUp的BaseItemDto均填充UserData：Key为稳定数字条目ID；Played、PlaybackPositionTicks、PlayCount、IsFavorite、PlayedPercentage明确填充。日期未发生时LastPlayedDate省略，绝不伪造时间或输出null。剧/季额外UnplayedItemCount，Played/百分比/播放次数按当前可见可暴露逻辑集推导（次数为集播放次数之和），收藏为父条目自身属性。纯投影亦有完整的未观看默认值，统一浏览再覆盖当前用户数据。
- UserData POST只采纳可写字段PlaybackPositionTicks、PlayCount、Played、IsFavorite、LastPlayedDate；Key、PlayedPercentage、UnplayedItemCount为只读兼容字段，不影响目标或范围。显式false/0保留，非法数值/日期/冲突参数400。PlayedItems支持DatePlayed；重复标已看不重复加次数，取消已看清位置/次数/历史日期，收藏保持独立。
- 剧/季已看操作仅批量写可见集（多版本去重），最多5000；5001明确400且不做部分写入。个人字段写入与T34用同一用户行短锁防止覆盖，事务内不做网络或目录详情查询，结果读取只访问变更的用户行。
- IsPlayed、IsFavorite、IsResumable以及DatePlayed/PlayCount排序在SQL分页前处理，TotalRecordCount与筛选一致；Filters别名IsPlayed/IsUnplayed/IsFavorite/IsResumable可用。UserData始终输出，EnableUserData只作为兼容参数接受。
- Resume仅Movie/Episode中位置>0且Played=false的项，按LastPlayedDate降序、同值稳定ID，支持MediaTypes/ParentId/Limit/StartIndex；本服务仅Video，Audio/Photo/Book返回空。NextUp支持SeriesId、ParentId、Limit/StartIndex、Fields，默认最近90天、最多500部可见剧；超限400，可用SeriesId或ParentId缩小范围。最高已看季/集号为前沿，取之后首个未看；无已看则取首个未看。季0默认排除，EnableSpecials（别名IncludeSpecials/EnableSpecialEpisodes）可开启。
- NextUp只对轻量可见集键及用户历史做SQL排名/分页，再批量装配本页详情，避免对长剧全部集重复读取JSON。旧T06 UserData骨架因缺请求级PermissionChecker仍失败闭合；实际路由使用PersonalService/BrowseRepository/DiscoveryService，绝不从客户端接受可见库集合。
- 媒体端口全部真实认证与权限校验；不可见条目UserData读写404，跨用户403，服务禁用/不可用503。没有新迁移/索引/扩展/依赖，Schema仍61。外挂字幕需云盘索引先收录字幕文件后另行设计；仅收录视频不等于远程字幕不存在。
- 合成协议回放与端点调用矩阵见[NATIVE_CLIENT_SEQUENCE_MATRIX.md](NATIVE_CLIENT_SEQUENCE_MATRIX.md)，不替代实机验证。


### T35/T36 最终验证（2026-10-17）
- 全仓非数据库71包通过；首次全仓回归发现3份golden不一致（旧探测HTTP缺新增UserData字段，以及回放夹具DateCreated未固定），已修正并无更新标志复跑。最终migrations全量108项顶层PASS、0 FAIL、0 SKIP，合并覆盖72包/868项顶层用例。本组8项新增集成（2条回放+6项个人数据/边界测试）全部通过。生产导出fixture与子进程helper仍是非数据库套件按设计Skip。
- Infuse风格33次、SenPlayer风格36次真实HTTP请求；逐响应体/头D8扫描通过，115始终假的Transport、CDN不跟随。仅合成回放，不声称真实客户端抓包或生产实测。两份回放golden稳定，修复时只比较字段路径、固定合成创建日期，未输出生产策略/内容。
- 普通浏览用户数据SQL次数one=7/batch=7，NextUp one=9/batch=9；5000集成功、5001集明确400且零写入，多版本不重复计数；1MiB剧集快照场景通过。5000集NextUp分轮284–558ms；一万电影冷首页分轮308–683ms（同机负载不同，不作硬件性能承诺），200ID批量仍7次SQL。
- build/vet、目录/协议包errcheck、相关包race、D9、22项Python、diff-check通过。已有迁移往返/枚举检查通过；无新迁移/索引/依赖/扩展，Schema仍61。UserData相关纯投影与HTTP golden已按字段扩展更新，缺日期仍按T05省略。原strm.go两处清理错误继续留T54。
- 已写客户端端点矩阵供T52核对。外挂字幕列为未来事项，先扩展云盘字幕索引再设计关联。没有访问真实115或生产、没有提交/推送/部署、没有开始P4。

- 缓存清理：删除 /private/tmp/media-tool-t3536-cache.wm3hZm9d、cache-path及本组临时脚本/日志/诊断JSON，执行全局 /Users/dingding/Library/Caches/go-build 的go clean -cache；实际释放 2626.43 MiB，未清理GOMODCACHE。


## P4 / T43–T45 设备、用户与媒体库管理

### 阶段一：管理端数据与安全边界
- 复用 Schema 61，无新迁移或依赖。所有新增管理端点使用管理员 cookie、`protected` 中间件、写操作同源/CSRF 与维护窗口检查；媒体端口全部保持404。
- 设备按最近访问倒序、UUID升序分页；历史按 `last_played_at DESC,item_id`，均≤200、offset≤1000000、3秒预算。每页固定2次SQL（用户存在性+计数/分页/页内元数据装配），不逐条调用数据库或外部服务。
- 观看历史定义为 `media_user_item_data` 中每条目的最近观看状态（包括人工标记观看），不是每次播放事件日志；管理员可看已移除或不再授权条目的历史标题，不返回媒体路径。
- 单设备撤销按Login相同顺序锁定用户/设备行（3秒预算），撤销该设备所有令牌并停止其会话，事务内写审计；保留设备、历史和其他设备令牌，不递增用户credential_version。重试幂等；再次使用密码登录会创建新令牌，不恢复旧令牌。
- 设备DTO仅ID、名称、客户端、版本、创建/访问时间、有效令牌数；不返回设备key、IP、UA、令牌摘要/原文。分类映射DTO仅库/分类标识与名称、状态和数量，不返回云盘/本地目录、pickCode或STRM。

### 阶段二：现有用户页面
- 增加下载授权（依赖播放）、并发1–20、分级0–100/null、到期时间/null、设备/历史入口。隐藏库的既存授权会保留，不自动授予新增库。
- 编辑时未改变的 `maxStreams/maxParentalRating/expiresAt` 不写入PUT；null显式清除可空限制。日期控件以本地时间显示、UTC ISO8601提交；清空分级默认隐藏策略只在设置有限分级时启用。
- 原密码/强制改密功能保留；撤销设备有二次确认、分页与错误反馈，不显示令牌。在线会话复用T34管理API，手动刷新，不新增高频轮询。

### 阶段三：现有媒体库页面
- 显示ServerName、原生服务总开关、播放器地址和监听端口；另保留探测/截图依赖说明与独立开关。总开关关闭不删除任何媒体或授权。
- 公开地址只从后台 `access.memberUrl` 或 `MEMBER_PUBLIC_URL` 读取，不使用管理请求Host猜测媒体地址；未配置时明确提示配置，而非显示错误管理地址。禁止带userinfo/query/fragment的地址出现在新DTO中。监听端口只读，修改 `MEMBER_ADDR` 后须重启；不声称支持热切换。
- ServerName（1–100字符，与既有CHECK一致）与enabled持久化至native_server_settings，带revision并写审计。设置仓库沿用最多250ms生效窗口；页面两处总开关保存后互相刷新避免revision失配。
- `/System/Info` 返回真实持久化配置、认证后可见，`HasUpdateAvailable=false/CanSelfRestart=false`。公共发现/Ping仍是停用时的既有例外。共享golden固定字段名与类型。
- Infuse/SenPlayer指引使用Emby源和媒体用户密码，客户端版本可能改变入口名称；不需要全局key。不对真实客户端完成兼容认证作额外声称，生产时序仍留T52核对。
- 页面沿用Element Plus/现有主题，不新增顶级路由；小屏表格局部滚动、按钮≥44px、字段有显式标签。

### P4验证及遗留
- 共享契约：`native-management.json`、`native-sessions.json`、`media-user-permissions.json`（含省略/清空示例）；Go严格解码、Vue请求体断言、真实管理认证/CSRF/撤销集成与浏览器冒烟。
- 保留P5事项：AMD64 CI复核、app生产目录权限、T52 HDR/客户端真实时序核对、5万候选搜索上限、strm.go已有清理错误。本组不访问生产，不提交、推送或部署，等待P4审核。
- D8补充：设备名称、客户端、版本、历史/会话标题和分类显示名经过显示标签校验；客户端不能通过名称夹带解析URL、云路径、查询凭据或256bit令牌。仅隐藏显示值，不修改持久化数据或既有私有日志政策。
- errcheck范围说明：本组新/修改的执行路径零新增告警；全包`-ignoretests`仍有42条既存生产代码告警（旧rollback、HTTP写出/关闭等），未借本组越界修改，留T54统一加固，不能将全包结果称作全绿。

### P4 最终验证记录（2026-10-17）
- Go race分段覆盖完整106个包（72个测试包），合并去重875个顶层通过入口；migrations包110项集成全部实际执行，包含既有Up→Down→Up与D2对照。仅2个设计性辅助入口Skip：需单独授权的消费者fixture导出、供崩溃子进程使用的测试helper；真实ffmpeg测试没有Skip。
- P4设备/历史每页均2次SQL，205条设备/历史数据验证200上限、末页、计数、排序与跨用户隔离；真实HTTP验证管理员认证、CSRF、媒体端口404、令牌即时撤销、其他设备保持有效、ServerName持久化和禁用503。
- 前端vue-tsc/Vite build通过；32个测试文件104项单测通过；4条相关Playwright用例全部通过，P4真实链路约5秒。375/1440截图已核对，稳定等待数据与开关加载完成后截图，不使用force-click或放宽超时。
- E2E发现并修复Tabs未局部注册导致历史入口不可见的问题；开关使用关联label进行可访问点击。最终浏览器链路验证设备撤销、其他设备仍可认证，以及PUT清空限制后的真实数据库读回。
- build/vet与增量errcheck零告警；全包errcheck的42条既存告警如上明确保留，不宣称全包errcheck通过。Python脚本22项通过。所有可变数据库/E2E串行，只使用media_tool_test与Redis DB15。


## P5 / T51–T55 兼容与加固

### 启动端点
- 新增System/Endpoint、两种空CSS、DisplayPreferences/{id}、本人Configuration、Sessions；Localization返回四个已知目录的空数组，未知目录404。启动默认值不包含管理员配置、文件路径、任意远程资源。DisplayPreferences仅只读默认值，POST返回405，不伪造偏好持久化。
- socket认证后501，绝不101/Upgrade/Hijack，不建立后台会话；后续REST继续可用。是否自动降级须以真机为准，不能仅凭合成序列宣称Infuse/SenPlayer已验证。
- 所有已知请求并发最多64，超载503/Retry-After=1；公网启动发现每IP每分钟600次，超限429/Retry-After=60。图片维持独立匿名600次限流及已认证豁免；登录维持T41的IP+用户名失败限制。
- 请求头32KiB、头读取5秒、正文读取20秒、通用正文1MiB（超限413），登录JSON64KiB、协议字段自身上限和单请求6秒预算继续生效。兼容204端点同样受正文长度限制。未知URI只记录脱敏后的path/UA，每分钟最多60条诊断，URL查询值和认证头/正文不写日志。

### Redis与搜索
- 浏览、Views、Counts、发现结果使用Redis短缓存，TTL10秒、单条≤2MiB；Lua先核对类型/长度再GET，不从Redis读取任意大小正文。Redis I/O/取池遵守75ms上下文，故障退回SQL，不把缓存故障当媒体不存在。
- 键为摘要，包含用户ID/credential_version、每请求新加载的可见库集合和分级集合、查询、数据库/模式身份及MVCC快照。每次命中仍重新读取权限，检查库隐藏、禁用/过期；不缓存授权决定或用户token，不缓存PlaybackInfo、会话创建、临时链接。
- 写入失效使用PostgreSQL `pg_current_snapshot()`栅栏：任何已提交DB写入都会换键，包括分类/资产/绑定/STRM、TMDB、探测、截图、授权、个人进度以及直接SQL。栅栏保留xmax与活动写事务集合，忽略仅因只读事务结束而前移的xmin回收边界，减少无写入的假失效。并发晚提交也能区分；填充期间快照改变则不写缓存。无需迁移/触发器、Redis事件或不可靠跨存储事务。此方案**保守地多失效**，不声称按单库精确失效；其他表/数据库的事务也可能降低命中率，正确性优先，旧键依赖有界TTL回收。
- 相同查询miss合并，最多4个不同miss装配并发；singleflight只允许一个等待者取得原DTO图，其余反序列化独立图，避免字段裁剪改写共享对象。无Redis/未启用上下文超时的注入客户端直接使用原SQL路径。
- 电影SQL提前排除非电影分支，去掉非必要的大JSON CTE物化，选择性电影搜索在聚合前预筛选，最终权限/分级/全文条件仍在分页前验证。未新增索引或扩展，仍保持总数、跨库去重与稳定排序。
- 50000候选限制现在在Hints的完整搜索条件之后计数，精确词不再因为库总量超过50000被拒；正好50000允许，50001仍ErrDiscoveryTooBroad。Genres/Filters亦以条件过滤后的集合计数。无任意全库内存筛选或静默截断。
- 冷进程Redis命中与首次Redis未命中分开计时，不能将预热后的命中冒称首次未命中。性能在media_tool_test的10000电影夹具上实际记录；持续写入时保守栅栏会回到SQL成本。

### 安全与错误处理
- 详细逐项记录见NATIVE_ERROR_AUDIT.md：产品目录原428条均已显式处理；原42条是相关包子集。未改动本地data中的私有诊断脚本，不能宣称这些未跟踪脚本也通过产品errcheck门禁。
- 关键SQL/Scan/JSON解析和备份/恢复文件Close错误传播；安全释放有固定理由，禁止拿cleanup.Ignore吞持久化错误。解锁失败丢弃物理连接；日志清理不重入日志锁；STRM临时清理非NotExist错误加入主返回值。
- 继续只允许用户token，逐请求校验；禁止管理路由/目录穿越/任意取链/用户越权，空body302、UA真实取链、D9导入边界不变。所有测试使用假115 Transport，不访问真实115/CDN。

### 兼容证据与发布边界
- 本地现有报告可核对4.10.1.0合成consumer及网关Http/远程源/UA/401/302摘要，但没有4.8原始HDR枚举或真机启动抓包。T52差异与未核实项见NATIVE_CLIENT_SEQUENCE_MATRIX.md；未凭缺失证据修改已批准的probeinfo语义。
- 部署步骤、端口、65532权限、ffmpeg体积和R2门禁见NATIVE_DEPLOYMENT.md。本组不访问生产、不提交/推送/部署，无新增迁移/依赖/数据库扩展，Schema仍61。

### P5 最终验证记录（2026-10-17）
- 分段race回归覆盖完整107包，74个测试包、884个顶层测试入口最终通过；迁移113项全部实际执行（既有Up→Down→Up/D2通过，无新迁移）。仅消费者fixture导出和崩溃子进程helper两个设计性入口Skip，真实ffmpeg测试未Skip。
- 产品全范围errcheck、build、vet最终零告警；Python22项通过。后端改动未修改Vue源码，本轮仍复跑4条相关真实管理端/播放器认证E2E，全部通过（17.1秒）。数据库/E2E始终由同一测试锁串行，仅media_tool_test和Redis DB15。
- 最终同轮10000电影：原SQL首次页466.75ms、末页308.40ms、Counts159.86ms；Redis首次miss582.29ms、新repository实例hit115.48ms（3次SQL），约比450–680ms参考低74–83%。首次Redis为空的请求仍要承担SQL和缓存编码成本，**没有达到与命中相同的提速，不据此宣称完全冷缓存已优化到115ms**。
- 10000电影真实认证PlaybackInfo280.45ms；混合4并发8请求（浏览4、PlaybackInfo4）均200，最大1.118秒，未出现超时或D8泄露。该数据是隔离测试库/假115路径，不是生产吞吐或真实CDN性能。
- 50001电影选择性Hints/Genres/Filters、正好50000允许/50001明确拒绝、直接SQL授权/分级/隐藏/缺失/探测/UserData写入失效、晚提交旧xid、并发返回对象独立、Redis超限/故障降级均通过。
- 已发现并修复保守MVCC栅栏把只读xmin推进误当内容变化的问题；最后测试保留xmax/活动写事务集合并验证旧事务晚提交，未放宽权限或TTL。
- T52只有本地摘要证据：4.8实际HDR枚举、Infuse/SenPlayer真机启动/WebSocket降级仍未核实；4.10.1.0合成consumer不能替代。这些限制在组末报告和R2验收清单中继续保留。
