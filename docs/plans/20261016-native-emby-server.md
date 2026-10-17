# 内置媒体库（Emby 兼容）总体计划与任务拆分

> 本文由总体计划与任务拆分合并而成。它是内置媒体库开发阶段的工作边界、审核标准和任务顺序；按协调者当前任务组连续执行，组内自行分阶段并报告进度，整组完成后统一审核。

## 第一部分：总体计划

# 内置媒体库（Emby 兼容）开发计划

制定：Claude Code（协调/审核） 执行：Codex 会话 01a11bd2  日期：2026-10-16

## 用户原始需求（逐条）
1. 使用 Go 开发
2. 使用独立的端口访问
3. 兼容 Emby 接口，Infuse、SenPlayer 等播放器可按 “Emby 服务器” 添加
4. 媒体库分类与「目录分类」中的分类保持一致
5. 访问 STRM 中的播放链接做预处理，获取媒体信息
6. 完善媒体库用户功能
7. 媒体内容全部为 STRM，按此处理，不保留网页播放等不必要功能
8. 海报、刮削信息直接引用程序中的 TMDB
9. 其他必要功能
要求：先搭代码框架，再按需求做细节；完成后推送 GitHub 并更新服务器。

补充指令（2026-10-16，用户原话）：“你可以自行创建媒体用户账号测试，预留ffmpeg/ffprobe，TMDB没有单集封面时，需要使用ffmpeg/ffprobe获取截图补充”。

## 总体原则（审核标准）
- 遵守 AGENTS.md：只在 app 内运行；不新增容器/服务；播放只允许 302 到 CDN，绝不中转、HLS、转码；STRM 正文不改；用实际播放器 UA 获取 115 临时链接；UA 白名单对本端口同样生效。
- 镜像预留 ffmpeg/ffprobe（runtime 镜像 `apk add ffmpeg`）。用途只有两个：ffprobe 媒体信息预处理（P2）、ffmpeg 抓取单集截图（P1）。严禁用于播放链路（不得转码、不得生成 HLS、不得经 ffmpeg 输出给客户端）；在代码里用单一受控封装调用，并加测试断言播放路径不会调用它。
- 内置媒体库与现有 Emby 并存：不改动现有 Emby 网关、Emby 服务器和 STRM 输出。
- 不兼容旧格式：现有 member 网页、/watch、member 专用 PWA 行为、`native_media_libraries` 的固定 movies/series 行可以直接替换；但媒体用户账号（media_users）要保留并迁移权限。后台工作台 PWA 是独立的既有需求，必须保留，只由管理端口提供。
- 公开仓库不得提交真实媒体标题、真实 Emby 响应或任何凭据；契约测试用合成数据。

## 架构
```
独立端口（沿用 MEMBER_ADDR 监听，服务器公网 25018；如需改名，配置同时接受旧名）
└─ internal/modules/embyapi        协议层：路由（大小写不敏感、支持 /emby 前缀与无前缀）、
                                   X-Emby-Authorization/X-Emby-Token/api_key 解析、Emby DTO
                                   （SystemInfo、UserDto、BaseItemDto、MediaSourceInfo、
                                   MediaStream、QueryResult）、错误映射、分页/排序/过滤参数解析
└─ internal/modules/nativelibrary  领域层：库=目录分类、条目投影（电影/剧/季/集）、稳定 ID、
                                   查询、用户数据（进度/已看/收藏）、搜索
└─ internal/modules/mediaprobe     STRM 链接预处理：ffprobe 获取媒体信息；ffmpeg 抓单集截图；结果入库
└─ internal/modules/mediausers     扩展：Emby 访问令牌、设备、会话、权限、并发、家长控制
└─ 复用：playback（STRM 解析 + 302）、playeraccess（UA 白名单）、tmdb（元数据/图片）、
   classification（分类规则）、library（资产/STRM/剧集绑定）、jobs（任务队列）
```

## 阶段与验收（每阶段完成后停下，等协调者审核再进入下一阶段）

### P0 代码框架（先做，单独审核）
- 新建上述包与接口（Go interface + 构造函数），端口监听接入 `cmd/media-tool`，替换 MemberHandler 为 embyapi 路由。
- 删除 member 网页端：Vue 中 member 视图/路由、member 专用 PWA realm/manifest 改写、`member 专用网页 API` 网页接口、相关 e2e；保留后台「自研媒体库」「媒体用户」管理页及后台工作台 PWA。
- 新迁移（按顺序编号）：
  - `native_libraries`：由目录分类同步（见 P1），替换旧固定表；`media_user_library_grants` 迁移外键（旧 movies/series 授权映射到对应媒体类型的所有新库）。
  - `native_item_ids`：内部实体 → 稳定 32 位十六进制 Emby ID（电影/剧/季/集/库/用户）。
  - `media_user_tokens`（摘要存储、设备、客户端、版本、最后活动）、`media_user_devices`、`media_user_item_data`（进度 ticks、已看、收藏、播放次数、最后播放）、`media_probe_results`（cloud_file 维度，含版本/尺寸/指纹，失败原因与重试时间）、`native_playback_sessions`。
- 所有端点先注册为可工作的空实现：返回结构正确的 Emby JSON（空列表/空结果），未实现的写端点返回 Emby 风格错误；不得 panic。
- 契约测试骨架：用合成数据断言 JSON 字段名/类型（PascalCase、Id 为字符串、RunTimeTicks 为 int64 等）。
- 文档：`docs/NATIVE_EMBY_SERVER.md`（架构、端点清单、数据流），旧 `docs/NATIVE_MEDIA_SERVER.md` 更新或替换。
- 验收：`go build`、`go vet`、单元测试通过；在测试库启动后，`/System/Info/Public` 返回合法 JSON。

### P1 浏览与元数据
- 库 = 目录分类：每个启用分组下的启用子规则对应一个 Emby 库（CollectionFolder）；没有子规则的分组本身作为一个库。CollectionType 按分组媒体类型取 movies/tvshows。分类策略修改（revision 变化）后同步库表，已删除分类的库标记为隐藏，授权保留不丢。
- 资产归属：资产没有持久化分类结果，需要新增资产→库的归属（按已绑定 TMDB 信息调用 classification 的匹配逻辑计算，并与 STRM 实际输出目录一致；不一致时以实际输出目录为准）。分类策略或资产绑定变化时重新计算。
- 条目投影：电影、剧集→季→集，只暴露已确认绑定且云端文件存在的资产。字段来自程序已有 TMDB 数据：Name、OriginalTitle、Overview、ProductionYear、PremiereDate、Genres、CommunityRating、OfficialRating、ProviderIds{Tmdb,Imdb}、People（演职员，可后补）、IndexNumber/ParentIndexNumber、SeriesId/SeasonId。
- 图片：`/Items/{id}/Images/{Primary|Backdrop|Thumb|Logo}` 引用程序内 TMDB 图片（复用 tmdb_image 现有缓存/代理逻辑），ImageTags 用 TMDB 路径摘要，支持 maxWidth/maxHeight/quality 参数映射到 TMDB 尺寸；集的 Primary 优先用 TMDB still。
- TMDB 没有单集 still 时：由 P2 的后台任务用 ffmpeg 从该集 STRM 解析得到的链接截取一帧（时间点取时长的约 10%–20%，避开片头黑屏，失败再试另一时间点），缩放后存为 JPEG，放到 app 数据卷下专用目录（如 /data/native-media/stills/，属于可重建缓存，不纳入备份），ImageTag 用截图摘要；TMDB 之后补上 still 时优先切换回 TMDB。截图未生成前 Primary 回落到剧集海报或季海报。
- 端点：Users/{id}/Views、Users/{id}/Items（ParentId、IncludeItemTypes、Recursive、SortBy/SortOrder、StartIndex/Limit、Filters、Genres、Years、SearchTerm、Fields、IsPlayed/IsFavorite）、Users/{id}/Items/{id}、Items/Latest、Shows/{id}/Seasons、Shows/{id}/Episodes、Search/Hints、Genres、Items/Filters。
- 验收：合成数据契约测试；查询必须分页且有上限，不做全表无界查询。

### P2 STRM 预处理（媒体信息，已完成并审核通过）
- 后台任务按资产读取 STRM 中的稳定解析地址 → 走现有解析获得 115 临时链接（使用固定的探测 UA 获取并通过 `-user_agent` 传给 ffprobe/ffmpeg，链接只用于本次探测，不缓存给播放器）→ `ffprobe -show_format -show_streams -of json`，必须设置 `-probesize`、`-analyzeduration`、`-rw_timeout`/总超时，并在进程级强制 kill 超时任务。
  - 解析并入库：容器、时长、码率、视频编码/分辨率/帧率/位深/HDR 与杜比视界标志、音轨（编码/声道/语言/默认）、内嵌字幕轨（编码/语言/强制/默认）、章节。
  - 失败：只给容器扩展名与文件大小，标记 probe_failed 并记录原因，不阻塞浏览。
- 全局并发 1–2，接入现有 requestbudget/115 限流，失败指数退避；文件 size/指纹变化后重新探测；后台可手动重探、查看进度和失败列表。
- 截图任务（见 P1）与探测共用同一并发/限流预算，先探测后截图（截图需要时长）。
- 结果用于 PlaybackInfo 的 MediaSources/MediaStreams、条目的 RunTimeTicks、视频分辨率等。
- 验收：用合成小样本（测试时用 ffmpeg 现场生成的几秒测试视频，不提交大文件）做探测与截图单测；对超时 kill、并发上限、115 限流做单测；断言播放路径不调用 ffmpeg。

### P3 播放与用户数据
- `Items/{id}/PlaybackInfo`（GET/POST）返回 MediaSources：Protocol=Http、SupportsDirectPlay=true、SupportsDirectStream=true、SupportsTranscoding=false、TranscodingUrl 为空；Path/DirectStreamUrl 指向本端口的 `/Videos/{id}/stream.{container}?static=true&MediaSourceId=...&api_key=...`。
- `/Videos/{id}/stream[.ext]`、`/Videos/{id}/original.ext`、`/Items/{id}/Download`（受用户下载权限控制）：校验令牌、库权限、canPlay、UA 白名单、并发限制 → 用实际请求 UA 走现有 115 解析 → 302 到 CDN。任何转码/HLS 参数（如 master.m3u8、VideoCodec 要求转码）返回明确错误，不降级中转。
- 外部字幕：若输出目录有字幕边车文件，作为 External MediaStream 暴露，下载同样 302/直读受控来源。
- `Sessions/Playing`、`/Progress`、`/Stopped`、`Users/{id}/PlayedItems/{id}`、`Users/{id}/FavoriteItems/{id}`、`Users/{id}/Items/{id}/UserData`、`Users/{id}/Items/Resume`、`Shows/NextUp` 更新/读取 media_user_item_data。
- 验收：模拟 Infuse/SenPlayer 请求序列的集成测试（测试库）；验证 302 Location、UA 透传、权限拒绝。

### P4 媒体用户功能
- Emby 认证：`Users/AuthenticateByName`、`Users/Public`（默认不公开用户列表，可配置）、`Users/{id}`、`Users/{id}/Password`（本人改密）、`Sessions/Logout`；令牌按设备签发、可撤销。
- 后台「媒体用户」完善：库级浏览/播放/下载权限、最大同时播放数、家长分级上限（基于 TMDB 分级）、启用/禁用、过期时间、设备列表与撤销、在线会话与正在播放、观看历史、重置密码（沿用现有规则）。
- 后台「自研媒体库」：服务地址/端口与客户端添加说明（Infuse、SenPlayer 填写方式）、库与分类对应关系、预处理进度与失败列表、手动重探、启用/停用内置媒体库。
- 验收：权限矩阵测试（无权限库不可见、不可播放、禁用/撤销即时生效）。

### P5 其他必要功能与加固
- 兼容性：`System/Info`、`System/Info/Public`、`System/Ping`、`Branding/Configuration`、`DisplayPreferences`、`Localization/*` 等客户端启动常用端点给合法默认值；未知端点记录日志（含 UA/路径）便于补齐；WebSocket `/socket` 不实现时明确拒绝。
- 缓存：条目/库查询用 Redis 短缓存，分类/资产变化时失效。
- 安全：登录限流（复用媒体用户限流）、本端口不暴露任何后台接口和云盘路径/pickCode/STRM 密钥；日志记录完整请求（符合 AGENTS.md 管理员可见原值规则）。
- 兼容样本：可以只读查看生产 Emby 网关日志中 Infuse/SenPlayer 的真实请求路径与参数，以确定端点覆盖范围；只在私有环境使用，不把真实数据写进仓库。

### P6 发布
- 全量测试（Go race、vet、Vue、Python、数据库集成测试串行，仅 media_tool_test/Redis DB15）→ 提交推送 → 等 Actions 发布 AMD64 Docker Hub 不可变镜像 → 更新 /root/docker/mediatool → 验收：端口可达、/System/Info/Public 正常、旧 Emby 及数据不变。
- 生产验收：用户已授权你自行在生产创建媒体用户账号测试。账号名必须明显为测试用途（如 `codex_test_<日期>`），只授予测试所需权限，验收后删除（或至少禁用并撤销会话），在交付报告中写明创建与清理情况。不得修改其他已有媒体用户或管理员账号。
- 真实 Infuse/SenPlayer 客户端添加与播放必须由用户实测确认，交付报告中如实写明。


---

## 第二部分：任务拆分与防缺陷清单

# 内置媒体库（Emby 兼容）任务拆分

配合本合并文档第一部分“总体计划”（原则与架构）使用。执行：Codex 会话 01a11bd2；审核：Claude Code。

## 一、执行流程（每个任务都一样）
> **2026-10-16 顺序调整（协调者已锁定）**：T16 之后先完成 T41 认证、T42 权限，再实现 T17–T19 浏览端点，以真实用户/权限做集成测试。原任务编号与其余阶段内容保留，不采用假认证过渡。

1. 协调者按任务组下发，组内连续完成、按阶段输出一行进度，组末统一汇报后停下；不要提前开始后续任务组。**2026-10-17 新规则立即生效**：T26/P2 已审核完成；T31/T32 已必审通过；T33/T34 已审核；T35+T36/P3 已审核；T43+T44+T45/P4 已审核；当前 T51–T55/P5，完成后等审核再由协调者安排R2。
2. 每组约 **2000 行非测试代码**（2026-10-17 P4起）（测试与文档不计）；超出时内部拆成若干阶段连续完成，不再申请拆分。只有需求歧义、必须改变已批准设计、需要新数据库扩展/依赖或访问生产环境时，才停下询问协调者。
3. 汇报格式固定：①完成内容 ②修改文件列表 ③运行的命令与结果 ④「防缺陷清单」逐条打勾或说明不适用 ⑤遗留问题；另加一行「缓存清理：实际路径、释放空间」。
4. 任务内发现前置设计有问题：先汇报，不要自行改动已审核的接口或表结构。
5. 阶段内不提交、不部署；R1 和 R2 是统一的发布任务。
6. **自T24起，每个任务汇报前清理Go编译缓存**：等待并确认所有Go构建/测试进程退出；若其他任务仍在使用，先汇报并延后，不终止其进程。删除本任务专属GOCACHE目录及对应cache-path文件；遇只读文件先`chmod -R u+w`。然后执行`go clean -cache`清理全局`~/Library/Caches/go-build`，绝不执行`go clean -modcache`或删除GOMODCACHE。仅清理本项目任务产生的临时目录和测试`.log`/`.json`，不动其他程序缓存；保留必要结论到汇报并记录释放空间。T23-B的1.7G专属缓存已由协调者清理，不重复计入本任务释放量。

## 二、防缺陷清单（每个任务必须逐条自检）
本轮审计中实际出现过的缺陷类型，逐条对应：
- D1 契约一致：每个前后端接口，都在 `testdata/contracts/<名称>.json` 放一份请求/响应样例；Go handler 测试用 DisallowUnknownFields 解码这份样例，前端 vitest 断言实际请求体与样例字段一致。Emby 接口的响应同样有 golden 样例，测试断言字段名、大小写和类型（Id 为字符串、Ticks 为 int64、日期为 ISO8601）。
- D2 状态一致：每个状态列都定义为 Go 常量，迁移里的 CHECK 约束必须列出同一组值；加一个单元测试，读取迁移 SQL，断言常量集合等于 CHECK 集合。
- D3 不忽略错误：新代码没有 `_ =` 或丢弃的 error（确实应该忽略的，写注释说明原因）；对新包运行 `go run github.com/kisielk/errcheck@v1.9.0 ./internal/modules/<新包>/...` 并保证通过。所有 DB 写入都检查 err；需要命中行的更新还要检查 RowsAffected。
- D4 中间状态可恢复：probing、running、deleting 这类中间状态，都必须有超时收敛逻辑（启动时或周期任务里），并有测试模拟“进程在中途退出”。
- D5 幂等：幂等键只能由业务字段组成（资产 ID + 版本等），不得包含随机值；重复调用要有测试。
- D6 有界：所有列表查询都有 LIMIT 和分页上限；所有外部调用（115、TMDB、ffmpeg/ffprobe、HTTP）都有超时；并发有上限；读取量有上限。
- D7 锁的范围：播放与浏览的热路径不得持有全局锁或长事务；需要加锁时说明锁的范围和超时。
- D8 安全：本端口任何响应都不得出现云盘路径、pickCode、STRM 全局 apiKey、管理员凭据；每个返回媒体的端点都有“无权限用户被拒绝”的测试。
- D9 不转码：除 mediaprobe 包以外，任何地方都不能 import ffmpeg 调用封装；用 `go list -deps` 或测试断言 embyapi/playback 不依赖它。
- D10 只改范围内的文件：不修改任务范围以外的文件；需要修改时先说明。

## 三、关键设计决定（审核后锁定，后续任务不得擅自更改）
- **ID**：条目 ID 仿照 Emby，使用数字字符串（`native_item_ids` 的 bigint 序列，例如 "100234"），按（实体类型, 内部键）唯一，永不复用；用户 ID 与 ServerId 为 32 位小写十六进制。
- **服务标识**：ProductName "Emby Server"，Version 默认 "4.8.10.0"，从 `native_server_settings` 配置；ServerName 同样可配置，禁止在代码多处写死，ServerId 首次启动时生成并持久化。
- **路由**：路径大小写不敏感，同时支持 `/emby/` 前缀和无前缀；查询参数名大小写不敏感。
- **认证**：从 `X-Emby-Authorization`、`Authorization: MediaBrowser ...`、`X-Emby-Token`、`X-MediaBrowser-Token`、`api_key` 查询参数中解析 Token/Client/Device/DeviceId/Version。令牌为随机 256bit，数据库只存 SHA256 摘要，按设备签发。用户禁用、改密或权限变更时，令牌立即失效（沿用 credential_version）。
- **播放地址（T06 前协调者裁决锁定）**：`BaseItemDto.Path` 若输出，只能是无任何 `/` 或 `\` 的合成文件名。`MediaSourceInfo.Path` 允许且应当使用本端口的绝对播放地址：`<内置媒体库公开地址>/Videos/{id}/stream.{ext}?Static=true&MediaSourceId=<id>&api_key=<该用户令牌>`；`DirectStreamUrl` 使用同一地址的相对形式（`/Videos/...`）。只能由 T31 唯一构造函数生成二者。DTO 只做格式/敏感前缀校验，公开地址归属与用户令牌作用域由 T31 校验。**绝不能把 STRM 正文（内含全局 apiKey 的解析地址）、pickCode、云盘路径或临时 CDN 地址下发到这些字段。**
- **库**：每个启用分组下的每条启用子规则对应一个库；没有子规则的分组本身作为一个库。库 ID 由（分组 ID, 规则 ID）确定。分类删除后库隐藏，但保留授权。
- **ffmpeg/ffprobe**：只能在 `internal/modules/mediaprobe/ffx` 子包里调用，统一封装：超时、kill、并发信号量、stderr 截断、参数白名单。
- **T01 锁定 ID 规则**：库、电影、剧集、季、集共用一个全局 bigint 序列；`(entity_type, internal_key)` 唯一；ID 转为数字字符串后永不复用。T36/T52 用真实请求序列验证客户端兼容性。
- **T01 锁定服务标识配置**：默认 `Version` 为 `4.8.10.0`，但必须从 `native_server_settings` 读取；`ServerName` 同样可配置，禁止在多个代码位置写死。
- **T01 锁定播放地址兼容性**：T32 同时接受 `/Videos/{id}/stream`、`/Videos/{id}/stream.{ext}` 和 `/Videos/{id}/original.{ext}`，支持 GET/HEAD；查询参数名大小写不敏感，包括 `Static/static`、`MediaSourceId/mediaSourceId`、`api_key/ApiKey`。
- **T01 锁定端口规则**：继续使用 `MEMBER_ADDR` 配置名，不修改服务器 Compose；T07 只读核对内部监听端口与公网 `25018` 映射，并更正旧文档中 `8092` 的描述。
- **T01 锁定用户入口迁移**：整套 member 专用网页登录/API 入口移除，包括登录、会话、改密、退出、目录、海报和预留播放；媒体用户以后只通过本端口 Emby 认证（T41）。后台 `/api/v1/media-server/*` 保留。
- **T01 锁定授权迁移**：T03 中旧 `movies` 授权复制到所有电影分组对应库，旧 `series` 授权复制到所有剧集分组对应库；`can_play` 原样复制；分类删除时库隐藏但授权保留。

## 四、任务列表

### R1 发布审计修复（在全部新功能之前）
前提：审计 A/B 7 项与备份返工都已通过审核。全量测试（DB 测试串行，只用 media_tool_test 和 Redis DB15）→ 提交（按主题拆成 2–4 个 commit）→ 推送 → 等 Actions 发布 AMD64 镜像 → 用不可变镜像更新服务器 → 验收：STRM 全局修复预览不再返回 400、插件事件投递、三个服务健康、数据保持不变。

### 阶段 P0：框架（每个任务只搭结构，不实现业务）
- **T01 文档与边界**：把总体计划和任务拆分合并为本文件 `docs/plans/20261016-native-emby-server.md`；新建 `docs/NATIVE_EMBY_SERVER.md` 的骨架（端点清单表格，状态都写“未实现”）。不改代码。
- **T02 移除网页端**：删除 member Vue 视图、路由、member 专用 PWA 行为、`member 专用网页 API` 网页接口及相关 e2e/单测；MemberHandler 暂时只返回 404 JSON。保留后台「自研媒体库」「媒体用户」页面及后台工作台 PWA（Manifest、Service Worker、安装入口、移动端设置）。验收：typecheck 通过，所有测试通过，后台页面与 PWA 正常，member 端口包括 PWA 路径在内均返回 404 JSON。
- **T03 数据库迁移**：一次性新增 P0 所需的全部表：native_libraries（替换旧固定表，迁移授权）、native_item_ids、media_user_tokens、media_user_devices、media_user_item_data、media_probe_results、media_stills、native_playback_sessions、native_server_settings。每张表的 CHECK 约束、索引和外键都在本任务确定；同时提交 D2 的常量/约束对照测试。迁移 Down 必须可执行。先只建表，不写业务代码。
- **T04 embyapi 协议骨架**：新建包：路由器（大小写不敏感、/emby 前缀）、认证头解析器、请求上下文（用户、设备、令牌）、Emby 错误响应、通用 Query 参数解析（StartIndex/Limit 上限 200、SortBy、Fields、Ids 等）。只做单元测试，不接数据库。
- **T05 Emby DTO 与 golden 契约**：定义 SystemInfo、PublicSystemInfo、UserDto、UserPolicy、AuthenticationResult、BaseItemDto、UserItemDataDto、MediaSourceInfo、MediaStream、QueryResult、PlaybackInfoResponse、SessionInfo 的 Go 结构体；建立 golden 测试框架（D1）。字段参照 Emby 公开 API 文档，只写客户端会用到的字段。
- **T06 领域接口**：nativelibrary（LibraryService、ItemService、UserDataService）、mediaprobe（Prober、StillGrabber 接口与 ffx 封装骨架）、播放适配接口（复用 playback 解析）的 interface、构造函数和空实现；embyapi 依赖这些接口，不直接访问数据库。
- **T07 监听接入**：把 MEMBER_ADDR 监听切换到 embyapi 路由；所有计划中的端点都注册为结构正确的空实现（空列表、空 QueryResult；未实现的写端点返回 501 风格的 Emby 错误）；`/System/Info/Public` 和 `/System/Ping` 返回真实值。验收：启动测试实例，curl 端点清单里每个端点都返回合法 JSON 且没有 panic。
- **P0 审核点**：协调者审核包结构、接口、表结构，锁定设计后再进入 P1。

### 阶段 P1：库与浏览
- **T11 分类→库同步**：根据目录分类策略同步 native_libraries（新增、隐藏、改名），在分类策略 revision 变化时触发；授权迁移规则落地。`library_granularity` 默认 `rule`，可切换 `group`；分类禁用/删除只隐藏库，授权与 item_id 保留；首次同步展开 legacy 授权，后续新库默认无授权。测试：增删改分组/规则、同名消歧、粒度切换、legacy 展开、重复同步与 ID 稳定；管理端只读库列表使用 D1 样例。**行为变更：启用分组但没有启用子规则时，新的分类渲染使用该分组目录，不再落入电影/电视剧类型兜底；只影响今后新生成或显式重新渲染的输出。已有 STRM 正文、`strm_entries.output_path` 和文件位置不会被同步任务自动移动或改写。**
- **T12-R3 解码兼容记录**：`classification.DecodeRecord` 保持线上旧语义：顶层 JSON `null`、顶层缺失 `groups` 或 `groups:null` 使用默认分组；对象内分组/规则省略字段可能继承 Defaults 的对应元素字段，这是已知兼容问题，本任务不修正。只读生产核对显示当前 `directory_classification.value` 为顶层 JSON `null`，未输出策略内容；旧/新比较的 policy、Match、Render 均一致。
- **T12 资产归属**：计算资产所属库（用实际 STRM 输出目录的最长路径段前缀；classification 只写诊断），持久化，并在绑定、STRM 输出、云文件状态变化后通过 `native.assign` jobs 增量重算。只暴露已确认绑定、云端文件 present 且存在有效 STRM 的资产；兜底库 `f:movie/f:series` 无匹配资产时隐藏。全量重算分批≤500、可中断恢复。测试覆盖：路径段边界、最长匹配、兜底、无效资产过滤、诊断、任务幂等。
- **T13 ID 分配**：native_item_ids 的分配与查询（库、电影、剧、季、集），保证并发安全（ON CONFLICT），ID 稳定。锁定内部键：movie/series=`tmdb:<tmdbId>`（按 entity_type 区分），season=`tmdb:<seriesTmdbId>:s<季号>`，episode=`tmdb:<seriesTmdbId>:s<季号>e<首集号>`，library 沿用 T11/T12 的 g:/r:/f: ID。多集文件只生成首集条目，IndexNumberEnd 表示末集，其余集不单独分配条目（实际投影在 T15）。媒体 ID 只按可暴露资产分配，不删除、不复用；全局无可暴露版本才退休，重新出现清除退休标记。EnsureIDs 每批≤500，批量 INSERT 冲突忽略后独立 SELECT 回读，避免并发快照不可见；default worker 复用现有 cron 按 ID 游标每轮≤500、3 秒预算收敛生命周期，重启可从 0 幂等重扫。
- **T14 电影投影**：电影和 CollectionFolder 的 BaseItemDto 纯函数投影（TMDB 字段、ProviderIds、Genres、评分、分级、年份、图片路径摘要）。只读 media_items 与 metadata 中已持久化字段，缺失值留空，探测时长 T24 接入，不在浏览热路径请求 TMDB。内部 MovieRepository 按可见库与 T12 共享可暴露证据查询、多版本聚合、当前库 ParentId/媒体源/最早 DateCreated；支持四种双向稳定排序、分页≤200、每电影源≤100、查询 4 秒预算。T17 再授权并接 HTTP，本任务不改变路由或认证边界。新增投影 golden、电影查询/ID 生命周期集成测试。
- **T14b TMDB 元数据补全**：先盘点现有 `media_items.metadata`、资产 `media_identity.episodeDetails` 与 `docs/EPISODE_METADATA.md` 的字段边界；新增迁移 00058 `native_tmdb_details`（类型/身份/语言唯一键、≤4MiB规范化payload、pending/fetching/ready/failed、fetching超时字段与到期索引）。复用 integrations 的 TMDB Token、中文语言和 TMDB 范围代理，仅为 T12 可暴露资产涉及的 movie/series/season 排队；全局并发≤2、每请求超时、429有界指数退避、失败不阻塞浏览。电影 release_dates（CN→US）、vote_average、external_ids、runtime、genres、credits、images；剧集首播/分级/外部ID/主创/图片；季一次取得集名、简介、播出日期、still和时长。ready快照30天刷新，running合并 rerun，stale fetching 收敛。T14 投影读取快照，探测runtime优先、TMDB runtime兜底；不在浏览热路径外呼 TMDB。新增 httptest、迁移、队列幂等/恢复和投影 golden 测试。
- **T15 剧集投影**：剧→季→集服务层投影，不接 HTTP。剧按 TMDB ID 聚合且按当前库至少一集可暴露才显示；季 0 为特别篇且保留 `IndexNumber=0`；集使用资产 `media_identity.episodeDetails`，缺失季快照时按季号回退，缺失集快照时使用绑定 season/episode 最小身份。多版本集合并多个 MediaSource，多集文件仅首集条目并设置 `IndexNumberEnd`，跨库共享稳定 ID。`HierarchyService` 接收可见库集合，列表分页≤200；集查询先按逻辑集键分页，再读取本页物理资产，每集最多最新100个版本，不因长篇剧总资产数失败；series/season TMDB快照按请求/季去重读取。剧支持 SortName/DateCreated/ProductionYear/CommunityRating/DatePlayed（T35 前回退 DateCreated），集按季号/集号稳定排序。补充 hierarchy golden 和 media_tool_test 集成测试，包括3000集压力场景。
- **T16 图片端点**：实现 `GET/HEAD /Items/{id}/Images/{type}[/{index}]`，只接受服务端根据已暴露条目 ID解析出的 TMDB 路径，禁止客户端远程 URL。Movie/Series poster/backdrop/logo，Season 季海报回退剧海报，Episode still 回退季/剧海报；CollectionFolder 统一 404。匿名访问仅返回已暴露条目的公共图片字节，不返回元数据/路径/播放信息，兼容无认证 `<img>` 请求；后续 T41 可接入用户授权。复用 TMDB 范围代理、Redis 缓存和 singleflight；尺寸映射到 TMDB w92/w154/w185/w342/w500/w780/w1280/original，不做服务端缩放；ETag 使用 ImageTag，支持 304/HEAD。`media_stills` ready 读取接口预留给 T25。测试覆盖尺寸、304、HEAD、未暴露404、并发合并、超时、大小上限和 URL 注入。
- **T17 Views 与 Items 查询**：在 T41/T42 完成后实现 Users/{id}/Views、Users/{id}/Items（ParentId、IncludeItemTypes、Recursive、排序、分页、Genres、Years、IsPlayed、IsFavorite、Ids、SearchTerm），只返回用户有权限的库。
  - 协调者已同意拆为 T17-A / T17-B。T17-A 仅完成统一分页查询、权限/分级 SQL 过滤、跨库去重、混合 ID 批量、Counts 与数据库性能测试；不接 HTTP。T17-B 再接 Views/MediaFolders/Items/详情/Counts 路由、真实认证请求链、请求总预算与响应 golden。
  - T17-A 约定：库父级在聚合前限定库；全局实体去重；同值按数值ID升序。T17-B 裁决：由native.assign在归属事务内预分配完整实体ID家族，周期任务每轮≤500补齐缺失ID；正常浏览只读，仅≤200当前页及必要祖先缺失时兜底，并按服务实例警告一次，不在浏览中扫描分配全库ID。Random 不支持并忽略，剩余排序为空时回退 SortName 升序。无用户数据时 IsPlayed/IsFavorite=true 返回空，DatePlayed回退DateCreated。
  - 查询与 Counts 共用同一过滤计划；Counts可接可选过滤条件但忽略分页。所有服务5秒预算，页≤200、单条版本≤100，所有元数据只读持久化快照。T17-A已审核；T17-B接入通用HTTP（含认证/权限总预算6秒、合成golden与真实登录请求链），完成后停下等待T18。
- **T18 详情与层级端点**：Users/{id}/Items/{id}、Shows/{id}/Seasons、Shows/{id}/Episodes。
  - T18/T19合并执行：专用Seasons/Episodes走T17统一SQL与T15投影，跨可见库去重；SeasonId校验归属，StartItemId含自身起始，不生成缺失项；IsMissing只兼容解析。
- **T19 发现类端点**：Items/Latest、Search/Hints、Genres、Items/Filters。
  - Latest返回数组，默认按剧聚合、按最新可暴露资产排序；Hints在SQL中NFKC/大小写不敏感匹配标题与原始标题；无新增索引或迁移，以50000逻辑候选/5秒服务预算限界，溢出明确400。Genre使用genre:+规范化名完整SHA256，不占五类数字ID序列；所有发现结果统一权限与分级。完成后停下等P1审核，不提前做P2。
- **P1 审核点**。

### 阶段 P2：预处理与截图
- **T21 ffx 封装**：ffprobe/ffmpeg 调用封装（参数白名单、超时、kill、并发信号量、stderr 截断）；Dockerfile 的 runtime 阶段 `apk add ffmpeg`；单测里用 ffmpeg 现场生成几秒的测试视频。
  - T21实现边界：移除任意Run(args)占位，只暴露Probe/Still固定argv。Probe默认30秒、Still45秒，上限2分钟；全局Probe2/Still1（各可设1–2），队列等待同样受ctx约束。Linux/macOS独立进程组取消，100ms TERM回收后KILL，stdout默认8MiB、stderr尾4096字节脱敏；缺二进制ErrUnavailable，New不阻止启动。本地文件仅测试显式开启，生产默认关闭。
  - Docker仅runtime增加ffmpeg；当前AMD64 APK依赖元数据估算下载+47.12MiB、安装+126.47MiB，不等于实测OCI镜像增量，R2发布时再核对实物。保持build阶段、Schema和官方部署路径不变。
  - 成功Probe原始JSON仍可能含filename临时URL；T24只提取白名单结果，不直接记录/持久化原JSON。T21不接STRM解析、任务队列或MediaStreams，不提前开始T22。
- **T22 探测链接获取**：从 STRM 正文读取稳定解析地址 → 用固定探测 UA 走现有解析得到临时链接；接入 requestbudget/115 限流；链接只用于本次探测。
  - T22实现：linksource按ExposableAssetsSQL读取实际受管STRM，核对所有权/SHA256及assetId/path/pickCode，复用网关严格解析和单文件grant；可信后台标记只在包内设置，不修改外部UA白名单，不HTTP回调，不用Redis直链缓存。固定可配置探测UA随一次性内存Ticket交给ffx。
  - 取链预算默认30秒/最高60秒；独立全进程并发默认1（可设1–2）；115限流返回retryable及60秒建议冷却。只记录资产/阶段/固定结果与R1-F数字耗时，临时URL不持久化/不写日志或错误；不可暴露/文件缺失permanent，授权/证据异常manual，限流/超时retryable，T23再落库。取链后重验DB与正文，异常不自动修复STRM。T22不建任务、不接ffprobe、不开始T23。
- **T23 探测任务**：jobs 队列唤醒（幂等键 = probe:资产ID:文件指纹）、状态机（pending/probing/ready/failed/needs_action，D2/D4）、指数退避、文件变化后重新探测、全局并发 1–2。
  - **协调者批准拆分与锁定约束**：T23-A先做Schema59与持久状态机/仓库，T23-B再做执行、调度和管理HTTP。只有media_probe_results的next_attempt_at/lease决定执行；jobs只唤醒，失败探测的job成功结束，不能触发jobs通用重试。写回带token/attempt/指纹fence，影响0行放弃；只有新目标指纹下的有效成功可以替换旧快照，失败绝不覆盖旧成功。
  - T23-A实现：目标file_fingerprint与成功result_fingerprint/result/probed_at分离；只接收强类型DurationTicks快照，不持久化原始JSON。SKIP LOCKED领取、每次新UUID lease、过期恢复每批≤200；相同指纹入队不清除终态/退避，手动Retry与审计原子提交。新指纹重置attempts并使旧lease失效。Current读取成功快照不依赖attempt state。
  - 指纹使用size/provider_file_id/实际metadata哈希map（2=SHA1、1=MD5）及存在时的mtime，忽略扫描updated_at/目录；无哈希/mtime时无法识别同providerID同大小的内容替换。退避无建议时1m/5m/30m/2h/12h/24h封顶，有建议优先且同样封顶24h。默认lease=30s取链+30sffx+15s余量；B先取执行槽再领取，避免空占lease。
  - T23-B已审核：native.assign后入队、每批≤200游标补漏/恢复、真实ffx(2)/linksource(1)许可在Claim前获取（私有key/一次消费），两个唤醒worker，不新增服务。双开关enabled默认false、probe_enabled默认true，入队/领取/成功提交均按二者同时true；500ms监测关闭并取消执行。资源等待不占探测lease。
  - 未发布的00059修正：坏ready先降pending并立即到期，残留开始时间/不完整成功凭据归一化；有效failed/pending旧成功快照保留。9种脏数据迁移回归。原始Probe JSON只在内存校验并取duration，错误只存固定码，job总是成功结束；完整解析留T24。
  - 管理POST /api/v1/media-server/native/probe/{assetId}/retry已注册，管理员/CSRF/维护窗口、双开关、原子审计与Go/Vue共享契约；媒体端404，完整页面留T26。阶段内不提交、不部署。
- **T24 探测结果解析**：ffprobe JSON → media_probe_results（容器、时长、码率、视频/音频/字幕轨、HDR/DV、章节）→ MediaStreams 映射函数。
  - **拆分已批准**：T24-A仅实现纯结果包/白名单解析/DisplayTitle、可选DTO字段、队列成功快照写入切换及合成fixture/golden；T24-B再做浏览投影与版本补探。本次A不改变现有浏览响应，不修改Path/URL构造，不开始T25。
  - **T24-A实现（基本审核通过；三项语义修正见B）**：`mediaprobe/probeinfo`不依赖执行器或数据库；catalog仅可依赖该纯包，源码扫描覆盖其它平台文件，另查传递依赖。成功快照`schema_version=1`；未知字段、原始JSON/URL/filename/任意tags不落库。旧duration-only快照仍可读取（版本0），不在A重排。
  - **T24-B锁定要求**：旧版本补探只转换一次，不重复重置pending退避或正在执行的lease；重探期间保留旧成功快照。按页批量加载各版本结果并映射MediaStreams，缺探测时维持既有golden，T31仍独占播放URL构造。
  - **T24-B实现（已审核）**：未发布schema_version保持1，修正VideoRange=SDR/HDR与VideoRangeType=DOVI兼容型；PQ/HLG不依赖primaries，DisplayTitle为Dolby Vision等可读名；T52核对Emby4.8生产网关日志实际值。统一浏览与电影/集服务只对当前页批量读取一次快照，≤20000资产且总传输≤32MiB；探测优先、TMDB兜底，旧成功快照在重探期间继续可用。版本0仅ready自动转pending一次，pending退避/probing lease/failed与needs_action恢复边界不变，不增迁移、不接T25。
  - **A边界**：≤8MiB输入、≤64条原始流、每条≤64KiB；未知/非法可选数值省略，非法duration/index与结构拒绝；字符串限长并丢弃控制字符、URL/路径/敏感标记。attached_pic/data/字体附件过滤但不重编号；字幕未知codec不输出。HDR按DOVI配置优先、HLG其次、PQ→HDR10，否则SDR。真实媒体仅测试时临时合成，不提交二进制。

- **T25 单集截图**：TMDB 没有 still 的集才截图：先探测拿到时长，取约 10%–20% 处截一帧，失败换时间点；缩放后存 JPEG 到 /data/native-media/stills/；写入 media_stills；TMDB 有 still 后切回。
  - **拆分已批准**：A迁移/独立状态仓库；B安全文件存储+LRU/GC及生产持久卷只读核对；C选帧/独立票据/执行背压/三开关与worker接线；D图片端三级回退及ImageTags/契约。逐段≤约600行（测试另算），审核后才进入下一段。
  - **T25-A实现（已审核）**：00060、stills_enabled默认true、独立stillqueue；一集一行、全局created_at DESC/asset_id ASC选可暴露且有成功探测的主版本；成功图片与尝试来源分离、FK墓碑不阻断资产清理；lease/token/attempt fence、独立退避/needs_action、每批≤200恢复、同事务审计。jobs仍只唤醒，key=still:assetId:result_fingerprint，不做通用job重试。不接ffx/文件读写/触发/worker/HTTP。
  - **预算已锁定**：lease硬上限5分钟；默认三次(30s取链+45s截图)+30s收尾=255s，配置lease默认300s，构造时拒绝不覆盖最坏预算的组合。GCGrace默认15分钟且必须>2×5分钟。B/C使用同一配置校验契约；C启动必须校验并传播失败。不可变发布先于fence引用，孤儿GC必须遵守宽限期。

  - **B进一步拆分已批准**：B1仅做JPEG验证、安全目录访问、不可变原子发布、Docker runtime目录与属主、文件测试；B2做容量准入/LRU/孤儿和tmp清理、访问时间节流及仓库配套。B1不激活运行时，不能在B2容量控制完成前用于生产截图。
  - **T25-B1实现（已审核）**：stillstore提供Open/Publish/Read/Close，默认/data/native-media/stills，逐段目录fd+O_NOFOLLOW且拒绝硬链接/非普通文件；JPEG≤4MiB、宽≤1280、总像素≤16Mi；tmp→fsync→no-replace rename→目录fsync后返回receipt，随后C再执行fenced引用。Linux RENAME_NOREPLACE、Darwin RENAME_EXCL，其它平台关闭功能；同内容幂等、不同内容报冲突。故障仅清理本次tmp，进程崩溃残留由B2按宽限期清理。Docker只在runtime增加目录并沿用app属主，不改build阶段。
  - **B2配套约束已批准**：索引及skipped/cache_evicted原因码直接合并未发布00060，不另开00061，并重跑Up→Down→Up与D2；仓库提供淘汰超过24小时才可条件重新入队的方法，由D图片请求接入，冷却期返回季/剧海报。LRU不得依赖atime，使用数据库或内存访问时间，每图每小时最多一次写入；GC只在根fd下操作、不跟随符号链接，保留.lock并遵守>2×lease上限的宽限期。
  - **生产持久卷结论（协调者已核对）**：宿主/root/docker/mediatool/data绑定至容器/data，属于持久存储；B1不重复SSH、不修改生产配置。绑定挂载会覆盖镜像内目录权限，R2还需以app身份验证可写，不能自动chown宿主业务卷。

  - **B2再拆分已批准**：B2-A仅数据库配套（size/lease容量预留、LRU fenced清引用、每小时触达写入、24小时条件重入队、迁移往返）；B2-B才做发布准入组合、内存访问合并、提交后文件删除及有界可续扫GC。不得把任一半当作运行时完整缓存。
  - **T25-B2-A实现（已审核）**：00060直接增加size_bytes/reserved_bytes/evicted_at及容量/预留/淘汰索引，新增skipped/cache_evicted原因；Schema仍60，不建00061。默认容量2GiB，可通过stillqueue.Config.MaxBytes配置；JPEG receipt补真实SizeBytes。Admit按数据库成功快照size合计+有效lease预留判定，在2秒上下文内用非阻塞事务级advisory锁串行准入；LRU每批≤200、按last_access_at/item_id，超限先降至90%（同时保证新图可容纳）才授权发布，不足则提交有限淘汰进度但不授权发布。事务中无文件I/O，只有确认提交的清引用结果才能交B2-B删除。
  - **预留回收已锁定**：reservation跟随现有lease token/attempt，重复准入不重复累计，Succeed要求receipt大小等于预留并原子转为成功size；Fail、来源切换、重试、TMDB跳过及RecoverExpired均清零，过期lease在用量统计中立即排除，迟到worker不得消费新预留。未知旧快照size=0仍可读，但准入拒绝，留B2-B安全校正。
  - **淘汰恢复与触达**：EvictSnapshot用item/path/digest/published_at作fence，清引用并使旧lease失效；不碰磁盘。TouchBatch≤200，通过数据库条件UPDATE跨进程保证同图每小时最多写一次，B2-B再接内存合并。普通Enqueue不恢复同来源的cache_evicted，RequeueEvicted满24小时后在行锁事务中只重排一次，不冒用管理员审计；显式Retry可人工覆盖冷却期，来源/指纹变化属于新工作。

  - **T25-B2-B实现（已审核）**：新增stillstore.Cache组合入口，发布在同一跨进程文件锁内先限批校正未知size、刷触达、Admit，再处理已提交LRU清引用结果后删除，获准才写tmp。拒绝返回RetryError（固定storage_full/storage_unavailable、建议1分钟），交C的独立退避处理，不走jobs通用重试；单次未知校正≤200，无法stat按缺失引用清理，不留下全局拒绝锁存。修复旧引用保留独立有效的新lease。
  - **B2-B维护边界**：GC每轮≤200逻辑条目（含目录探测）、2秒协作预算；持有目录fd/待处理条目和数据库ID游标跨轮续扫，重启安全从头开始。先校正未知size，数据库引用分页校正/缺失清理，超限按LRU回收至90%，再扫描256个固定hex分片的canonical JPEG/受控tmp；宽限期默认15分钟、严格>2×5分钟lease上限，LRU不读atime。临删除重查当前引用/有效尝试，所有unlink在提交之后且fd相对执行；失败计数并留孤儿，不卡住其它发布。
  - **触达与配套**：读图仅内存合并（上限4096），Flush每批≤200，数据库小时节流继续兜底；满采样表丢触达采样而不阻塞读取，过期清样本可释放容量。00060再补relative_path/未知size部分索引，不增Schema/迁移号。C/D已接Flush/GC周期调度、fenced Succeed、图片三级回退/指纹Tag（见下，待本轮审核），原始Store.Publish保留为测试底层接口，生产仅用Cache.Publish。

  - **T25-C/D实现（已审核）**：流程预算按2026-10-17协调者新规则为约1200行非测试代码，超过时内部连续分阶段，不因拆分停顿。stillrunner实现15%→12%→18%独立票据/预算，Go JPEG亮度与方差，纯黑/白时选最大方差；ffx.ReserveStill跨三帧持真实额度，先帧/取链额度后Claim，独立still worker与三开关取消、fenced Succeed、独立退避、30秒补漏/Flush/GC。
  - **运行接线**：probe成功提交后EnqueueAsset，周期Episode ID游标≤200补漏；启动构造调用stillqueue.New预算校验，错误传播；关闭功能不打开缓存目录。all/server/worker-still模式共享适当的维护生命周期；服务角色图片访问使用同一Cache。NATIVE_STILLS_ROOT、NATIVE_STILLS_MAX_BYTES、NATIVE_STILLS_FRAME_TIMEOUT分别配置目录、逻辑容量、单帧时限；resolve预算沿用PLAYBACK_RESOLVE_TIMEOUT，组合必须满足5分钟lease上限。
  - **图片与投影**：Primary优先TMDB单集still（episode快照→season快照→资产已持久化信息，与端点/投影一致），再本地成功快照，再季/剧海报；本地Tag统一SHA256(native-still:fingerprint:digest)，每页最多一次批量读取，不读文件/不入队；坏文件请求降级，淘汰满24小时图片请求原子重排。TMDB到达清本地引用，GC按宽限期回收。
  - **CI原生门禁**：verify Go测试前apt安装ffmpeg，MEDIA_TOOL_REQUIRE_FFMPEG=1将缺工具/编码器的真实媒体Skip改为失败；test-go.sh输出详细测试结果；额外go test -race -count=3 -v ./internal/modules/mediaprobe/...。publish依赖verify，不通过不得发布。实际原生AMD64结果留R2推送后检查，不以本地模拟代替。

- **T26 后台管理（已审核）**：既有自研媒体库页内加入三个开关及依赖提示、探测/截图状态统计、容量/预留、按状态分页、原因中文解释、单条与确认后的≤500批量重试、手动有界GC及实际回收量。管理API沿用管理员会话/CSRF/维护保护，开关revision事务审计，重试意图+逐项审计，GC前后审计；Go/Vue共享契约与组件/e2e回归。不增路由、迁移、索引、扩展或依赖。
  - **P2收尾**：T21–T26已审核完成；阶段代码完成不代表发布完成。R2仍须核对原生AMD64门禁、app用户对生产截图目录的写权限；T52核对HDR实际取值；T53/T54评估搜索词过滤前5万候选上限；T54处理strm.go两处未检查清理错误。T22 grants数据库故障403→503及实际镜像体积写入R2发布核对。
  - **冷首页核对**：1万电影三轮615/445/493ms，中位493ms；电影页截图SQL为0。主要耗时为通用分页SQL253–375ms及Go装配/校验，不能将历史两个单次值的265ms差异归因于图片或集键查询；不增索引/放宽超时，T53评估分页结果缓存。
  - **最终验证**：冻结源码后全仓race 70包/829项通过；Vue 95项、e2e烟测2项、构建/vet/errcheck、22项Python及00060迁移往返/D2通过。真实ffmpeg执行；新增测试均隔离到media_tool_test/Redis15并串行。P2实现与审核已完成；原生AMD64等发布验证仍留R2。
- **P2 审核点**。

### 阶段 P3：播放与用户数据
- **T31 PlaybackInfo（已审核）**：GET/POST含前缀/大小写与nullable客户端字段兼容；固定直接播放能力。唯一StreamURLs生成绝对Path/相对DirectStreamUrl，绑定当前用户token与公开origin；未知容器用mkv提示，不转换格式。MediaSource沿用T24快照与降级，按版本检查可见库播放权。未激活会话使用现有stopped+ended_at=started_at，Info不占槽；无新迁移或状态。1万电影认证HTTPInfo首测195ms、复跑162ms，最终数据见汇报。
- **T32 流端点 302（已审核）**：GET/HEAD原始流/下载，用户状态→T42权限→UA→可暴露媒体源→按user/device/source去重的原子槽位→真实UA进程内取链→零body302。用户行短锁、五分钟TTL、失败新槽位时间fence释放；不持锁做外部调用。复用STRM只读验证、DynamicStorage、UA/IP/版本缓存、官方限流/刷新/R1-F单次有界路径重试；429/Retry-After、授权失效503、不可暴露404。全局STRM key显式拒绝，日志URL脱敏用户token但保留完整CDN Location。协议与播放依赖图禁止执行包，仅可用纯probeinfo。
  - **T31/T32边界**：Info预算6秒、取流总预算28秒，保持30秒WriteTimeout；MaxStreamingBitrate、DeviceProfile、播放能力/索引等只解析不转码。共有只读STRM读取器从探测侧抽出，不改原正文或写入行为；strm.go两处旧清理错误留T54。T34须补Playing/Progress续约、PlaySessionId同键归并与停止上报，不在本组提前实现。
  - **T31/T32验证**：全仓race 72包/842项通过，最终7项播放专项通过；1万电影真实HTTPInfo含认证/权限/会话全仓174ms、末次188ms。3份新增golden、67份既有JSON未变；build/vet、新包/新读取器errcheck、D9、22项Python通过。旧STRM两处defer错误仍留T54，不提前接T33/T34，不部署。
- **T33 字幕（已审核）**：本机只读核对没有外挂字幕，内嵌流明确Embed且无DeliveryUrl，字幕端点404；外挂字幕关联/生成流程另行审核后再实现，禁止抽取内嵌字幕。
- **T34 播放上报（已审核）**：Sessions/Playing、/Progress、/Stopped、/Ping及旧PlayingItems已接入，写入native_playback_sessions和media_user_item_data；短用户行锁，不跨外部调用，普通Progress/Ping按10秒节流。
  - **已锁定 D4 要求**：`native_playback_sessions` 长时间未收到 Progress（建议超过 5 分钟）必须收敛为 `stopped` 并填写结束时间，释放 `max_streams` 名额；补充中断/超时恢复测试。
- **T35 用户数据端点**：PlayedItems、FavoriteItems、UserData、Items/Resume、Shows/NextUp。
- **T36 客户端序列测试**：模拟 Infuse 与 SenPlayer 典型请求序列的集成测试（登录 → Views → Items → 详情 → PlaybackInfo → stream 302 → 进度上报）。
- **P3 审核点**。

### 阶段 P4：用户功能
- **T41 Emby 认证**：AuthenticateByName、Users/Public（默认不公开）、Users/{id}、Users/{id}/Password、Sessions/Logout、令牌按设备签发与撤销；复用媒体用户限流。
  - **T41 实现边界**：`nativeauth.Store` 每个受保护请求都查 DB，不使用令牌短缓存；登录返回 256 bit 随机令牌，仅持久化 SHA-256 摘要，按 `(user,DeviceId)` 复用设备并撤销该设备旧令牌；365 天有效期、每 10 分钟节流滑动续期。禁用、过期、必须改密、凭据修订或管理员撤销立即生效。本人改密保留当前令牌并撤销其他设备。
  - **初始密码选择**：后台创建/重置默认“不要求改密”，保留显式 `mustChangePassword` 选项；勾选时播放器登录 401，并提示管理员设置最终密码。不自动清除已有用户标记，不修改生产密码。
  - **D8 用户令牌边界**：成功登录的 `AuthenticationResult.AccessToken`，以及T31唯一构造器生成的本端口Path/DirectStreamUrl可按协议携带当前用户令牌；不能混入全局STRM key。其他响应、数据库、审计和日志不返回/保存令牌明文，诊断日志只保留摘要前缀，请求URL必须脱敏令牌。
  - **图片补充**：匿名图片每 IP 每分钟 600 次；只有已验证令牌绕过此限制，内存计数器最多 10000 个 IP，过期回收。仅 tag 与当前 ImageTag 相等时使用一年 immutable；缺失/旧 tag 缓存一小时，不使用 immutable。
  - **已锁定续期要求**：令牌采用长有效期（建议 365 天），每次成功使用时滑动续期并节流持久化，避免 Infuse/SenPlayer 频繁掉登录；禁用账号、改密、权限变更及撤销仍须立即失效，不能以续期缓存绕过凭据版本检查。
- **T42 权限模型**：库级浏览/播放/下载、最大同时播放数、家长分级上限（TMDB 分级映射）、账号过期时间；所有端点统一走一个权限判定函数。
  - T42 实现边界：每请求创建独立 PermissionChecker，VisibleLibraries/ItemAllowed 与原 CanBrowse/CanPlay/CanDownload/ParentalAllowed/StreamSlots 统一加载一次账号和授权快照，不跨请求缓存；电影查询补齐 VisibleLibraries 必填校验，T15 直接使用同一集合。
  - 授权行=浏览，can_download 必须同时 can_play；隐藏库保留授权。设置分级限制时空/未知分级默认隐藏；映射表集中在 nativepermissions/ratings.go（本程序年龄等价值 0–100，不宣称大陆存在官方评级制度）。
  - 普通拒绝404；播放显式映射403及固定原因。StreamSlots 只统计5分钟内活动会话，T34仍负责原子预留及超时收敛。
  - 管理 GET/PUT 与列表补 canDownload/maxStreams/maxParentalRating/expiresAt，共享JSON契约；旧页面保留新字段，完整控件留给T44。PUT省略nullable字段保留，null清除；不改变表结构，不提前接入浏览端点。
- **T43 设备与会话（已实现，待组末审核）**：设备列表、单设备撤销、在线会话、正在播放、最近条目观看历史；复用既有表，分页≤200，固定2次SQL，无迁移。
- **T44 后台用户页面（已实现，待组末审核）**：媒体用户管理的 Vue 页面补齐上述功能（D1）。
- **T45 后台媒体库页面（已实现，待组末审核）**：服务地址与端口、Infuse/SenPlayer 添加说明、库与分类对应关系、启用/停用内置媒体库、ServerName 设置。
- **P4 审核点**。

### 阶段 P5：兼容与加固
- **T51 启动类端点（已实现，待组末审核）**：System/Info、Branding/Configuration、DisplayPreferences、Localization/*，以及客户端常用的其他启动端点返回合法默认值；未知端点记录日志（UA/路径）并返回 404 JSON；/socket 明确拒绝。
- **T52 真实请求对照（本地摘要已核对，4.8 HDR/真机证据缺失）**：只读查看生产 Emby 网关日志中 Infuse/SenPlayer 的请求路径和参数，列出缺失的端点和参数并补齐（真实数据不入仓库）。
- **T53 缓存（已实现，待组末审核）**：库和条目查询使用 Redis 短缓存，分类/资产/探测结果变化时精确失效。
  - P1审核遗留：Hints/Genres/Filters的50000候选上限在搜索词过滤前计算；库超过上限时精确词仍可能被拒绝。T53/T54再评估搜索投影或索引，P2不顺带修改。
- **T54 安全与负载复查（已实现，待全量验证）**：D6/D7/D8 全面复查，并对浏览与 PlaybackInfo 做并发压测（测试库）。
  - T22审核遗留：检查 `internal/modules/strm/strm.go` 中既有 `root.Close()` / `root.Remove(tmp)` 的清理错误处理；T22/T23不顺带修改。
- **T55 文档（已同步，待组末审核）**：完成 `docs/NATIVE_EMBY_SERVER.md`（端点状态、添加方法、限制说明）。
- **P5 审核点**。

### R2 发布内置媒体库
- **T25-B2-B平台复核项**：原生AMD64必须复跑stillstore并发/JPEG测试。本地AMD64模拟多线程出现标准库解码/运行时内存错误，旧存储基线压力也可出现；ARM64原生、AMD64单P整包及隔离并发通过，但未确认根因，模拟测试不替代原生发布验收。
- **T22提交说明必记**：`playbackgrants.Authorize` 将数据库故障与撤销/无权限区分开；数据库不可用由原先403调整为503，取消/超时保留相应错误语义。网关提取共享STRM解析器保持原有origin/key/grant校验，Emby回归已通过。
全量测试 → 提交（按阶段拆 commit）→ 推送 → Actions 发布 AMD64 镜像 → 更新服务器 → 验收：
- 端口可达，/System/Info/Public 正常；
- 新建测试媒体用户 `codex_test_<日期>`，用模拟的 Infuse/SenPlayer 请求跑完整序列（登录 → 浏览 → PlaybackInfo → 302 到 CDN → 进度上报），验证完删除该账号；
- 预处理任务在生产正常运行，115 限流没有被突破；
- 原 Emby、STRM、115 授权、账号和数据卷都不变。
最后提交交付报告，写明需要用户在 Infuse、SenPlayer 真机上确认的事项。


### T33/T34 本组落实（待审核）
- T33只读核对本机库/输出没有外挂字幕（五种扩展均0），未访问远程生产。内嵌字幕Embed+显式IsExternal=false，保留Index且无DeliveryUrl；两种字幕GET/HEAD统一404，不抽取/取链、不占槽。外挂字幕关联及STRM生成流程未变更。
- T34现代及旧PlayingItems接口接入真实认证/权限；PlaySessionId绑定用户设备，草稿激活、同媒体源槽位合并，跨用户/设备拒绝；普通心跳10秒持久化节流，状态/seek立即写入。按所选版本成功探测时长写单集/电影用户数据，Ping与过期收敛不改用户数据。
- 00061只增加会话play_counted以实现每次播放计数幂等；Schema61，原三状态常量/CHECK不变。app启动及每分钟≤200行收敛；管理端只读/api/v1/media-server/native/sessions，分页≤200，媒体端口不注册。完整管理UI留T43，UserData/Resume/NextUp留T35/T36。

- 缓存清理：删除 /private/tmp/media-tool-t3334-cache.LZKBlaln、对应cache-path/本组临时脚本与测试日志；执行全局 /Users/dingding/Library/Caches/go-build 的 go clean -cache；实际释放 2412.73 MiB，未清理GOMODCACHE。

- **本组最终验证**：全仓72包/856项顶层PASS；最新播放专项19项（含11项新增集成）PASS且无Skip。Schema61 Up→Down→Up、D2、build/vet/errcheck/D9、22项Python及golden通过。无外挂字幕关联/抽取、无新依赖/扩展、无生产写入；等待T33/T34审核，不提前做T35/T36。

- 补验缓存清理：删除 /private/tmp/media-tool-t3334-cache.JNqPq1iN 及cache-path/临时脚本/日志，执行全局 /Users/dingding/Library/Caches/go-build 的go clean -cache；本轮释放 700.60 MiB（本组首次已释放2412.73 MiB，此轮为补验重新生成的缓存），未清理GOMODCACHE。


### T35/T36 本组实现（待审核）
- 外挂字幕未来事项：协调者只读生产确认cloud_files只收录视频（2个mkv、1个ts）；不能据此推断远程没有外挂字幕。目前仅支持Embed。后续必须先让云盘索引收录srt/ass/ssa/vtt/sup，再审核稳定关联、语言/Index和受控302方案，不能擅改STRM流程或抽取内嵌字幕。
- UserData读写、已看/收藏、Resume/NextUp接入真实T41/T42上下文。Key沿用稳定数字条目ID；剧/季Played、未看数、百分比和总播放次数由可见且可暴露的逻辑集聚合；收藏独立存于该条目。无播放记录的LastPlayedDate按T05省略，其他要求的标量填零/false。
- IsPlayed/IsFavorite/IsResumable、DatePlayed/PlayCount排序均在SQL分页前完成；支持Filters=IsResumable等常见别名。普通浏览无新增逐条SQL。剧/季标记作用于可见集，最大5000（版本不重复计数），超过时400说明上限，零写入。写回与T34共用短用户行锁，事务内无外部调用，不在事务内重装配目录元数据。
- Resume按位置>0且未看完、最近播放降序；NextUp默认90天内有播放记录的剧，按可见已看集最高季/集号之后的第一未看集取一条，无已看时取首个未看集。默认跳过季0，可用EnableSpecials=true（亦兼容IncludeSpecials/EnableSpecialEpisodes）。近期剧最多500，超限要求SeriesId/ParentId缩小范围；SQL先排名轻量键，再按当前页批量装配详情。无新迁移/索引/扩展/依赖，Schema仍61。
- 两条真实HTTP集成序列仅为依据Firecore官方头部及Jellyfin Swift/Emby文档的合成回放，不是Infuse/SenPlayer实机抓包；调用矩阵见docs/NATIVE_CLIENT_SEQUENCE_MATRIX.md，待T52生产日志核对。未开始P4或发布。

- **本组最终验证**：非数据库71包通过，修正golden后migrations全量108项PASS且无Skip；合并72包/868项。新增8项集成通过；两客户端合成回放33/36次真实HTTP，SQL次数7/7、NextUp 9/9，5000/5001和大快照边界通过。build/vet/errcheck/D9/race、22项Python与golden通过；Schema61无新增迁移/扩展。等待本组审核，不开始P4/发布。

- 缓存清理：删除 /private/tmp/media-tool-t3536-cache.wm3hZm9d、cache-path及本组临时脚本/日志/诊断JSON，执行全局 /Users/dingding/Library/Caches/go-build 的go clean -cache；实际释放 2626.43 MiB，未清理GOMODCACHE。

## P4 / T43–T45 组末记录（待协调者审核）
- T43：设备分页/撤销、最近条目观看历史、在线会话展示；不新增迁移，Schema61保持不变。单设备撤销只处理该设备令牌/槽位，历史保留，其他设备不受影响。
- T44：下载/播放/浏览授权、同时播放、家长分级、到期设置和设备/历史入口；共享契约固定省略保留/null清除。
- T45：现有页面内展示安全媒体地址/监听端口与两种客户端指引、分类库对应、ServerName和总开关，设置持久化并供System/Info读取。
- 最终验证：106包完整集合中72测试包race通过，875顶层通过入口；110迁移集成通过；Vue104单测/32文件、typecheck/build、4条相关E2E、Python22项通过。2个设计性辅助/导出入口Skip，真实ffmpeg无Skip；新执行路径errcheck零告警，全包42条既存告警留T54。
- 本组无生产访问、提交、推送或部署；P4实现结束，等待审核。P5/R2遗留继续保留：原生AMD64 CI、生产目录app权限、真实HDR与客户端时序核对、5万候选上限、strm.go既有清理错误及旧errcheck告警。

- 缓存已清理：本组专属GOCACHE、cache-path、临时日志/辅助脚本和本轮E2E last-run JSON；确认Go进程退出后执行全局go clean -cache，合计释放2367.94 MiB（2.312 GiB），GOMODCACHE未动。

## P5 / T51–T55 组末记录（2026-10-17，待审核）
1. T51启动默认端点、CSS/路径别名、只读偏好、本人配置、合法空本地化/Sessions与WebSocket501，golden、HEAD/前缀、限流/正文上限/过载测试通过。
2. T52只读核对34份Emby/网关JSON报告及12份JSONL；可核对4.10.1.0合成consumer和Http/远程/UA/401/302摘要，无4.8原始HDR枚举/真机启动证据，保持已批准probeinfo语义并明确未核实，不能写成实测完成。
3. T53 Redis短缓存（10秒/2MiB/75msI/O）带新权限/分级及MVCC栅栏；写入提交换键、晚提交xid测试通过。保守全库事务失效不是按单库精确失效；写入频繁时命中率降低。10000电影新服务实例hit115.48ms/3SQL，首次Redis miss582.29ms，两个场景分别报告。
4. T54产品目录原428条errcheck均显式处理；原42条是相关包子集。失败解锁丢弃物理连接，STRM清理/关键Close/SQL/Scan/JSON错误处理，原生端口D8、限流、64并发与正文限额复查。4并发8次混合真实HTTP全部200，最大1.118秒。
5. T55端点表、OpenAPI、兼容矩阵、逐项错误审计、NATIVE_DEPLOYMENT.md已更新；部署参数/权限/ffmpeg约+126MiB为发布前说明，不是本轮生产验收。
- 最终验证集合：107包（74测试包）、884项顶层通过，113迁移集成，4条相关E2E，22 Python；build/vet/产品errcheck零告警。两个设计性辅助入口Skip，真实ffmpeg未Skip。
- 无新增迁移/依赖/扩展；Schema61；无前端源码改动；不访问生产、不提交/推送/部署。P5实现与本地可证实核对结束，等待审核；R2须补原生AMD64、Hub不可变镜像、实际目录权限/镜像大小、4.8 HDR与真机时序证据。

- P5缓存清理：删除/private/tmp/media-tool-t5155-cache.Ag6bO9eW、cache-path及本任务临时脚本/日志/JSON和本轮E2E last-run JSON；确认Go进程退出后执行全局go clean -cache，释放3696.29 MiB（3.610 GiB），GOMODCACHE未动。
