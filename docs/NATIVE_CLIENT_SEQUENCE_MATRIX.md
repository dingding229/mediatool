# T36 客户端协议回放与 T52 生产日志调用矩阵

日期：2026-10-17。测试：`migrations/native_client_sequence_integration_test.go`。

## 证据与边界

这是两条合成协议回放，不是 Infuse 或 SenPlayer 实机抓包，也不代表已覆盖闭源客户端全部版本。实际使用 HTTP socket（httptest.Server + http.Client），真实认证/权限/SQL/会话/用户数据；115 使用假的 Transport，CDN 302 不跟随。没有访问真实 115 或生产。

参考的第一方资料：
- [Firecore：连接信息](https://support.firecore.com/hc/en-us/articles/21072505575319-Connection-Info-for-Emby-Jellyfin-and-Plex)：Infuse-Direct 连接类型及引号形式的 X-Emby-Authorization。
- [Jellyfin Swift SDK](https://github.com/jellyfin/jellyfin-sdk-swift/blob/main/Sources/PassthroughAPIClientDelegate.swift)：Authorization 中的 MediaBrowser 字段及无引号格式。SenPlayer 风格回放借此覆盖通用兼容变体，不声称该 SDK 被 SenPlayer 使用。
- [Emby 用户库 API](https://dev.emby.media/reference/RestAPI/UserLibraryService.html)、[播放状态 API](https://dev.emby.media/reference/RestAPI/PlaystateService.html)、[NextUp API](https://dev.emby.media/reference/RestAPI/TvShowsService/getShowsNextup.html)。

## 头部与路径

| 项目 | Infuse 风格 | SenPlayer 风格 |
|---|---|---|
| User-Agent | 合成 Infuse-Direct/8.0 | 合成 SenPlayer/1.0 |
| 登录身份头 | X-Emby-Authorization，MediaBrowser，引号字段 | Authorization，混合大小写前缀/键、无引号字段与多余空格 |
| 声明字段 | Client、Device、DeviceId、Version | 同左；Device 的无引号值含空格 |
| 登录正文 | Username/Pw | username/Password |
| 后续 token | 轮换 MediaBrowser Token、X-Emby-Token、查询 ApiKey | 同左，Authorization 使用无引号 Token |
| 路径 | /emby，混合大小写，公开端点及流地址无前缀 | /mediabrowser，混合大小写，公开端点及流地址无前缀 |

GET/HEAD 流地址中的 api_key 是当前用户 token，不是 STRM 全局 key。

## 调用矩阵

路径省略前缀；两列是两条新增序列中的 phase 标识。兼容别名的既有单测/集成测试继续保留。

| 方法与端点 | Infuse phase | SenPlayer phase | 核对点 |
|---|---|---|---|
| GET /System/Info/Public | public | public | 产品、版本、公开地址，无认证 |
| POST /Users/AuthenticateByName | authenticate | authenticate | 真实密码校验，返回用户 token |
| GET /Users/{userId} | user | user | 当前用户、最小策略 |
| GET /Users/{userId}/Views | views | views | 可见库、UserData |
| GET /Users/{userId}/Items/Latest | latest | latest | 最近资产、完整用户状态 |
| GET /Users/{userId}/Items/Resume | resume-before / progress / stopped | 同左 | 空→单集续播→完成后为空 |
| GET /Shows/NextUp | next-before / progress / stopped | 同左 | 空→首集→下一集，SeriesId/UserId |
| GET /Users/{userId}/Items | items | items | 库父级、分页、排序 |
| GET /Shows/{id}/Seasons | seasons | seasons | 可见季、未看集数 |
| GET /Shows/{id}/Episodes | episodes | episodes | SeasonId、集顺序、用户状态 |
| GET /Items/{id} | series-detail / season-detail | 同左 | 父级聚合与 Key |
| GET /Items/Counts | counts | counts | 仅可见范围计数 |
| GET /Users/{userId}/Items/{itemId} | detail | detail | 单集详情、Embed 字幕、用户状态 |
| POST /Items/{id}/PlaybackInfo | playback-info | playback-info | 所选版本、用户作用域绝对/相对流 URL |
| HEAD /Videos/{id}/stream.{ext} | stream-head | stream-head | 302、零正文、不跟随 CDN |
| GET /Videos/{id}/stream.{ext} | stream-get | stream-get | 与 HEAD 相同 Location，播放器 UA 取链 |
| POST /Sessions/Playing | playing | playing | 激活 PlaybackInfo 会话 |
| POST /Sessions/Playing/Progress | progress | progress | 20% 位置与续播 |
| POST /Sessions/Playing/Ping | ping | ping | 同会话心跳 |
| POST /Sessions/Playing/Stopped | stopped | stopped | 95% 完成，次数+1，清续播 |
| GET /Users/{userId}/Items/{itemId}/UserData | userdata-progress / stopped | 同左 | 位置、百分比、Played、PlayCount、Key、时间 |
| POST /Users/{userId}/Items/{itemId}/UserData | userdata-update | userdata-update | 显式位置更新 |
| POST /Users/{userId}/FavoriteItems/{itemId} | favorite-add | favorite-add | 收藏独立属性 |
| DELETE /Users/{userId}/FavoriteItems/{itemId} | favorite-delete | favorite-delete | 取消收藏 |
| POST /Users/{userId}/PlayedItems/{itemId} | played-add | played-add | DatePlayed 参数 |
| DELETE /Users/{userId}/PlayedItems/{itemId} | played-delete | played-delete | 取消已看 |
| POST /Users/{userId}/PlayingItems/{itemId} | 既有 T34 测试 | legacy-playing | 无 PlaySessionId 匹配/重播 |
| POST 上述路径/Progress | 既有 T34 测试 | legacy-progress | 查询位置/暂停与正文 EventName |
| DELETE 上述路径 | 既有 T34 测试 | legacy-stopped | 幂等停止路径 |
| GET 其他用户的 UserData | cross-user | cross-user | 403，不返回对方数据 |

## Golden 与 D8 检查

- 每条序列的全部响应按 phase 保存为 golden：`internal/modules/embyapi/testdata/sequences/{infuse,senplayer}.json`。
- 仅归一化临时本地端口、合成 ID、用户 token 和易变活动日期；字段名、字段类型、数组、状态码、计数、用户状态及 302 Location 保留。bytes 为归一化正文的字节数；204/302 另断言实际正文为零。
- 对每个响应体、全部响应头逐层检查 URL 解码后的内容，拒绝云路径、pickCode 名称及值、稳定 STRM 端点/正文、全局 STRM key。允许协议规定的登录 AccessToken 和用户作用域流 URL；golden 中仅有合成 token。
- 图片、字幕 404、Download、GET PlaybackInfo、Latest/Items 的其他别名及 UA/并发拒绝等由既有 T16–T34 测试覆盖，并非本矩阵新增实机证据。

## T52 对照方法

在拥有管理员权限的私有日志中，按设备/客户端和时间窗口对照上表，核对路径、方法、参数与状态码，尤其是登录后的 Resume/NextUp、PlaySessionId、HEAD/GET 302 及最终 UserData。不得把生产 token、完整生产日志或媒体内容复制进公开测试夹具/Actions。真实客户端若有新端点/参数变体，先记录差异，再添加合成回放。HDR/VideoRange 真实值仍按既有 T52 事项核验。


## T52 本地兼容证据核对（2026-10-17，只读）

### 可用证据及限制

本组只读取仓库内既有 `data/remote-deploy` 与诊断报告；未执行其中脚本、未访问生产、未导出完整原始报告。34份相关JSON验收报告，以及12份本地JSONL（38行）均未包含原始MediaStreams或VideoRange/VideoRangeType，广义本地JSON/log搜索也未找到这些字段。两份consumer matrix明确记录Emby **4.10.1.0** 和合成fixture，不是Emby 4.8或Infuse/SenPlayer真机启动抓包。不能据此把当前HDR映射标成“已由4.8实测证实”。

| 本地摘要事实 | 原生实现/差异 | 结论 |
|---|---|---|
| 网关验证nativeSourceProtocol=Http、nativeSourceIsRemote=true | PlaybackInfo同为Http/远程源，但Path指向本端口用户作用域302端点 | 形状一致；不照搬网关临时CDN地址 |
| nativeMetadataHeaderCount=1、UA匹配provider；原始/Static302通过 | 原生使用播放器真实UA在进程内取链，不把115元数据header/临时链接下发为浏览信息 | 有意的安全边界差异，不引入中转 |
| invalidTokenStatus=401 | 原生仅接受媒体用户token，全局STRM key不能登录或取流 | 拒绝语义一致，身份系统不同 |
| consumer版本4.10.1.0、fictionalFixtures=true | 本协议兼容版本4.8.11.1；T36两条客户端风格序列为合成回放 | 不能将4.10消费成功当作4.8/真机证明 |
| 无原始启动URI/参数/正文 | 新增System/Endpoint、Branding/Css别名、DisplayPreferences/{id}、用户Configuration、Sessions默认响应 | 补齐计划清单并以golden与REST回放验证；不是生产发现 |
| 无HDR10/HLG/DOVI枚举原文 | 保持T24-B批准映射与schema_version=1，不凭摘要改动语义 | **Emby4.8实际取值仍未核实**；需R2另行提供脱敏枚举样本 |

### HDR字段当前契约（非4.8运行时证据）

- `VideoRange`仅SDR/HDR；`VideoRangeType`为SDR、HDR10、HLG、DOVI、DOVIWithHDR10、DOVIWithHLG、DOVIWithSDR。
- PQ即HDR/HDR10，HLG即HDR/HLG，不要求primaries同时存在；DOVI兼容ID 1/4/2分别表示HDR10/HLG/SDR基底，0或缺失为DOVI。DisplayTitle使用HDR10、HLG、Dolby Vision，不输出Jellyfin式枚举名作为人类标题。
- 本地报告没有可证实的映射差异，故未擅自改为DolbyVision等其他wire值；现有全矩阵/异常组合测试继续固定这些语义。R2只需补充版本、两字段枚举及DOVI兼容ID，勿复制token、真实标题/路径或完整PlaybackInfo。

### 新启动调用矩阵

| 端点 | 默认行为与安全边界 | 验证 |
|---|---|---|
| GET System/Endpoint | 匿名、IsLocal/IsInNetwork均false，不把代理私网IP误当授权 | startup golden |
| GET Branding/Configuration、Css、Css.css | 本地空品牌/CSS，不获取外部资源 | GET/HEAD、三个前缀 |
| GET DisplayPreferences、DisplayPreferences/{id} | 已认证用户只读默认值；不伪造POST持久化 | golden、跨用户403、ID长度限制 |
| GET Localization/{Countries,Cultures,ParentalRatings,Options} | 合法空数组，不是QueryResult；不支持的目录404 | golden |
| GET Users/{userId}/Configuration | 本人最小配置 | golden、T41身份门禁 |
| GET Sessions | 合法空列表，不公开他人的活跃会话 | golden |
| GET socket + Upgrade:websocket | 501、无101/Upgrade，不伪造WebSocket；随后REST仍可用 | 拒绝后继续启动序列 |

真实客户端是否对501自动降级仍需真机确认；合成回放只证明服务器REST没有被禁用或占用资源。

本地摘要证据指纹（仅校验文件，不包含内容；未新增原始报告到仓库）：

| 本地文件 | SHA-256 |
|---|---|
| `data/remote-deploy/emby-consumer-matrix.json` | `fb260040207f635e2748f147e113b29d6593f2b0b3f010f07594973de38e1d73` |
| `data/remote-deploy/emby-projected-consumer-matrix.json` | `b344c2cee8801429f9a9d5fef86274139f239a1d37e30f32857dfed89310fe06` |
| `data/remote-deploy/gateway-user-session-check.json` | `04ef1d90cefb3dea0bb5395b18b01b693a8541477e738ec774a97e839b264dff` |
| `data/remote-deploy/owned-upload-final-gateway-check.json` | `4717e760c4cad61eb849b14194f0c283c5a512bbe65e494585a4eecf66528ca2` |
