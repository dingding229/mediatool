# 前端接口与业务流程

> 字段、参数、状态码及监听端口以 [OpenAPI](../api/openapi.yaml) 和实际服务响应为准。

## 0. 使用方式、覆盖范围与维护

- [OpenAPI](../api/openapi.yaml) 使用 OpenAPI **3.1 / JSON-compatible YAML 1.2**，可直接 JSON/YAML 解析；没有给运行时引入 YAML/生成器依赖。每个 operation 有稳定且唯一的 `operationId`、业务 tags、安全方式、参数和具体 wire schema，不再使用通用 `OK`。
- **管理端 259 个实际注册 pattern**（包括 prefix/静态/兼容入口）；**原生媒体端 61 个实际注册项**。管理端 Go GET 隐含 HEAD，methodless mount 展开 OpenAPI 支持的八种标准方法；原生 GET 隐含 HEAD，已知 path 有 OPTIONS。展开后 **291 paths / 625 path-method operations**，其中管理端467、原生端158；这些不是625个独立业务功能。
- `x-listener=management` 只属于管理端；`native-media` 的 operation-level `servers` 指向单独媒体地址。管理端 `/emby/{path}` 是外部 Emby 兼容网关；不是把 `/Users` 暴露到管理员端口。原生 `/emby`、`/mediabrowser` 前缀/大小写别名和 mount 内动态路径见第9、10节，不伪装为静态管理路由。
- prefix 的 `{path}`、插件 `{path}` 与原生 `{wildcard}` 可覆盖多段路径；保留 `x-go-registration` 中的原始 pattern。methodless 的 `x-any-http-method` 表示 Go 无 method 限制，未被 OpenAPI 标准表达的 CONNECT 等方法不据此获得业务能力。未知 `/api/` 是404，旧 `/cloudStorage` prefix 是501；精确的 `strm302` 除外。
- [逐接口前端使用清单](FRONTEND_API_ROUTES.md) 列出全部 path/method、调用痕迹及源文件。这里的“未检出”不等于死代码：原生播放器、外部API、HEAD、下载/图片URL、运行中插件均可能没有 Vue `request()` 调用。

## 1. 所有业务共同遵守的约定

### Cookie、来源与响应

管理 JSON 请求使用 `credentials: 'same-origin'`，会话 Cookie 为 `media_tool_session`（HttpOnly、SameSite=Strict；生产 Secure）。HEAD只读（不能以HEAD body绕过PUT防护）。修改操作，包括登录/注册，需 `X-Requested-With: media-tool` 并通过同源 Origin/Fetch Metadata 检查。不能把自定义头当成身份，也不要把管理员 Cookie/token 放 query、localStorage、跨域URL或公开截图。

```http
POST /api/v1/auth/login
Content-Type: application/json
X-Requested-With: media-tool
Origin: https://admin.example.invalid

{"username":"example_admin","password":"ExampleOnly-2026"}
```

成功一般为 `{"data": ...}`；错误为 `{"error":{"code":"...","message":"...","flow":{...}}}`。`flow` 可省略，不是所有失败都能重试。二进制、STRM、SSE、Emby原生/兼容响应**不是**普通 JSON Envelope，不要交给 `web/src/api/client.ts` 的 `request()` 强行解析。`X-Request-ID` 可用于私有日志排错；不要把错误响应、日志/数据视图里的原值复制进公开产物。

### 状态与错误处理

| HTTP / code | 前端动作 |
|---|---|
| 401 / unauthorized | 清会话视图，登录后重新读取状态；不自动重发原修改请求 |
| 403 / csrf_rejected | 核对管理域名、Cookie、来源、自定义头；不循环登录重试 |
| 403 / password_change_required | 只允许改密/会话/退出流程；完成后重新登录 |
| 400 / invalid_json、invalid_pagination | 显示字段错误；保留草稿，不自动修剪危险路径/丢掉重复 query |
| 404 | 按具体 code 显示资源不存在；不要把所有503转成“未配置” |
| 409 / *_changed、*_conflict | 重读当前 revision/fingerprint/preview；重新确认，不覆盖他人修改 |
| 410 / event_retention_expired | 提示7天回放窗口已过；读取退休游标，不静默从新游标重放 |
| 423 / maintenance_paused | 保留草稿，业务写入暂停；运维核验/复核后显式恢复 |
| 429 | 使用 Retry-After；禁用连续提交，不改 operationKey 伪造新意图 |
| 502/503/504 或连接中断 | 结果可能未知，先GET/inspect/reconcile；不能按“失败”等同未发生 |

- 搜索/订阅、手动下载、文件批次、扫描、115授权、jobs、插件snapshot/journal等宿主状态枚举也列在schema；qB/外部provider状态是开放值，不应把未知值强转“失败”。
- `flow={phase,retryable,nextAction,reason}` 是**观察/指引**，不是操作授权。重试仍受服务端状态、lease、revision、digest、权限和确认约束。
- 常见 phase：download、organize、strm、emby_refresh、emby_verify、complete、inspect，以及维护 preview/approve/execute/verify。`nextAction=verify` / `reconcile_upload` 不可映射成通用 retry。
- 202/queued/approved 仅是排队/批准。请轮询原任务或读取 SSE 失效提示后GET；不要显示“已下载/已整理/已入库/已恢复”。SSE `/api/v1/jobs/events` 的 tick 不是持久事件总线，重连后GET对账。
- 一般分页 `page/pageSize`；用户/部分列表用 `startIndex/limit`；数据视图等另有游标/offset。**具体默认值/上限看每个 operation**，不能全局硬编码同一上限。坏编码、重复、空字符串、负数、越界要保留服务端拒绝，禁止客户端悄悄折叠重复参数。
- 标记 `revision` / `expectedRevision` / `fingerprint` / `digest` 的表单须保存读时基线；提交失败先读现状。确认参数默认false，不能在通用客户端自动补true。

以下示例均为人工合成、脱敏值。标注“响应节选”的例子只展示相关字段，完整字段、nullable/omitempty、枚举和各状态响应看 OpenAPI。

## 2. 登录、管理员账号与会话

1. GET `/api/v1/auth/setup` → 看 registrationOpen；只在开放时展示首次注册。注册成功即关闭公开注册，不能靠UI隐藏代替后端门。
2. POST `/auth/register` 或 `/auth/login` → 设置Cookie → GET `/auth/session` 对账，显示账号状态；刷新页面不能凭前端缓存认定已登录。
3. MustChangePassword 时限制工作台；POST `/auth/password` 提交当前/新/确认密码，成功后重新登录。退出POST `/auth/logout`；失联也清本地已登录视图，但不伪称服务器已撤销成功。
4. 登录 body 不另采集；一次性恢复密码只在 startup stdout，不做“从日志自动读取恢复口令”的页面。管理员账号与第9节媒体用户不是同一套会话。

```http
GET /api/v1/auth/setup
```
```json
{"data":{"registrationOpen":false,"passwordMinLength":6,"passwordMaxLength":20}}
```

界面仍须以 setup 实际返回的范围校验，不复制示例数字作为新限制。登录响应节选：`{"data":{"authenticated":true,"account":{"username":"example_admin"}}}`。

## 3. 系统设置、命名与网络代理

1. GET `/settings/application`、`/settings/access`、`/settings/network-proxy`，按各自 revision 编辑；PUT 后以回读为准。管理/成员/播放地址有不同用途，不能只改可见URL而破坏已有STRM。
2. POST `/settings/naming-preview` 是纯预览；没有写配置。分类 GET/PUT `/classification` 与命名/目录预览配合；不要因预览成功自动保存。
3. 代理URL是私有配置，TMDB/站点开关分开；保存/探测失败保留草稿。SSRF 校验和已验证目标IP pin由后端执行，不应向用户提供“关闭安全校验”选项。
4. 保存返回 `validation_record_failed` 等未知回执时，先GET revision与探测记录，不能再POST创建第二份连接。

```http
POST /api/v1/settings/naming-preview
Content-Type: application/json
X-Requested-With: media-tool

{"movieTemplate":"{title} ({year})/{title} ({year}) - {videoFormat}","seriesTemplate":"{title} ({year})/Season {season}/{title} - {season_episode} - 第{episode}集"}
```
```json
{"data":{"movie":"示例电影 (1997)/示例电影 (1997) - 1080p.mkv","series":"示例剧集 (2026)/Season 01/示例剧集 - S01E04 - 第04集.mkv"}}
```

输出为格式示意，具体占位符/格式是否合法由预览器裁决；前端不自行拼接媒体路径，也不把预览当真实资产名称。

## 4. 115 / CloudDrive2 / Emby 连接、授权与 readiness

1. GET `/integrations`；创建/编辑指定 kind 的连接，保留secretFields所表示的已有秘密。未重新输入不等于清除秘密，清除/替换须按请求契约明确处理。
2. 115：POST `/integrations/{id}/115/auth/start`（JSON `{}`）→ 显示该 sessionId 对应二维码 → poll（同sessionId）→ waiting/scanned → exchanging → authorized/expired/cancelled/failed；离开/取消只取消该会话。不要丢失session后不断start，更不要从其它项目借授权。
3. POST `/integrations/{id}/test` → 回读连接 readiness；授权、保存、连接probe和显式启用是不同步骤，授权成功不自动enable。
4. GET `/readiness` 汇总 open115 / clouddrive2 / strm / emby。not_configured → configure；authorization_required → authorize/configure_credentials；disabled → enable；check_required → test；check_failed → inspect；ready 仅指新鲜保存probe。STRM configured 的下一步仍是实际播放器302验证。
5. `revision`变化使旧probe过期；24小时陈旧probe需重新测试。不能把CD2根目录可读等同115取链可用，更不能把HTTP健康等同播放器实际播放和拖动成功。
6. 私有管理员 reveal 需明确confirm、revision和字段；仅展示给已认证管理员，不复制到公开截图、遥测、Actions。保留115授权、主密钥和现有账号。

```http
POST /api/v1/integrations/11111111-1111-4111-8111-111111111111/115/auth/poll
Content-Type: application/json
X-Requested-With: media-tool

{"sessionId":"22222222-2222-4222-8222-222222222222"}
```

readiness响应节选：
```json
{"data":{"scope":"configuration_and_last_probe","delivery":"original_strm_302","components":{"strm":{"state":"configured","nextAction":"verify_player_302","reason":"stable_resolver_configured_not_player_verified"}}}}
```

## 5. 原生站点模板与 PT 账号

1. GET `/pt/templates` 展示固定mediatool-site pin编译的目录；POST `/pt/templates/load` 只加载管理员选择的ids；启动/升级只刷新已选择定义并保留现有PT账号使用模板。
2. POST `/pt/accounts` 或PUT `/{id}` 绑定definitionId、values、userId/rssUrl等；保持已有凭据/账号身份。重发现rediscover有服务端定义，不能把更换模板当“删除后重建账号”。
3. GET `/pt/accounts` → 按站点连接/账号状态探测与刷新资料/指标；失败区分登录过期、网络、解析/定义变更，不循环保存cookie。
4. 没有远程source/commit/version/ZIP/动态Go编译入口，没有pre-dessert/common继承。模板选择与账号启用、RSS订阅是不同步骤。

```http
POST /api/v1/pt/templates/load
Content-Type: application/json
X-Requested-With: media-tool

{"ids":["synthetic-example"]}
```

若该合成ID不在当前编译目录，错误Envelope示例：
```json
{"error":{"code":"invalid_templates","message":"未知站点模板"}}
```

错误码以上需以具体 operation 的枚举为准；UI只能提交GET目录返回的实际ID，不能提供任意代码/地址输入框。

## 6. 搜索、媒体身份与订阅

1. 先选择可用PT账号/媒体身份，可GET `/search/media` 查询TMDB → POST `/search` 传keyword/accountIds/media → GET `/search/{id}` 追踪pending/running/succeeded/partial/failed。缓存结果也要展示站点错误，partial不等于无资源。
2. POST `/search/{id}/media` 修改关联身份；必须刷新后再选资源下载，不能让资源沿用旧TMDB/年份而无确认。接口是 `/search`，不是历史计划中的 `/search/runs`。
3. 保存 `/subscriptions` 后读取revision与enabled；POST `/{id}/evaluate` 使用preview=true先看决策，再由用户决定实际执行。读 `/subscription-runs/{id}` 和 `/{id}/history`。
4. 评估preview与live是不同意图，活跃run冲突 `subscription_run_mode_conflict` 时等原run完成，不换id绕过；重复点击不应生成新的并行下载意图。
5. 取消/归档需确认；规则覆盖、缺集、来源失效应显示真实reason，不承诺订阅保存后必定下载。

```http
POST /api/v1/subscriptions/11111111-1111-4111-8111-111111111111/evaluate
Content-Type: application/json
X-Requested-With: media-tool

{"preview":true}
```
```json
{"data":{"id":"33333333-3333-4333-8333-333333333333"}}
```

HTTP202仅表示已创建评估；轮询run，不能立即显示订阅已下载。

## 7. 下载 → 整理/上传 → STRM → Emby 链路

| 观察 | UI可提供动作 | 禁止推断 |
|---|---|---|
| pending/failed job，flow.retryable=true | POST `/pipeline/{kind}/{id}/retry` 当前安全阶段 | 不重建整个资源/任务 |
| downloading/added | 等待、POST `/downloads/sync` 后回读 | 不以qB请求成功当下载完成 |
| local_complete | 查看transfer、确认目录/身份 | 不以本地文件存在当云端已上传 |
| starting/uploading/uncertain | reconcile_upload，查看sessions绑定 | 不自动发起第二个上传 |
| needs_output_reconciliation | GET output-preview，人工核对controlRevision/planDigest，再resume-import | 不跳过新预览/确认 |
| strm_ready | emby_refresh阶段队列/失败可安全重试 | 不把STRM落盘当Emby可见 |
| refresh_requested | emby_verify对账 | 刷新API接受不等于媒体入库 |
| imported / complete | 显示已核验相应链路状态 | 不代表真实播放器解码/音轨/字幕验收 |

- 首次下载POST `/downloads` 使用searchRunId/resourceId和选定TMDB身份。年份/身份修正要显式confirm，来源过期走 `/{id}/source-preview` → `reconfirm-source`，不是重建任务。
- 手动torrent POST `/downloads/manual` 支持JSON url（magnet/允许的torrent URL）或multipart字段torrent（.torrent ≤10MiB）。任务inspect后未知提交不能盲目retry；明确acceptDuplicateRisk才允许相应危险重试。
- 新建管理员手动任务明确进入托管后处理：下载完成后唯一标题/年份/类型识别，复用现有整理→上传校验→STRM/入库链。设置与共享目录冻结在原任务加密选项中；历史任务和 SDK 的仅下载入口不自动回溯或扩权。
- POST `/downloads/manual/{id}/import`：管理员同源写接口，body 为 `{confirm:true,mediaType:"movie"|"series",tmdbId:正整数}`。明确选择原已完成任务的身份并排队，不重新添加种子；running/已托管状态不可覆盖。202 仅代表排队。季集不明、多媒体身份不一致、源映射变更仍阻止入库。
- 手动列表 `processingState` / `managedTaskId` 区分仅下载、等待完成、识别中、待人工及托管阶段。新增后台任务种类 `download.manual.import` 是幂等识别/登记；实际上传、入库和清理仍由原流水线承担。
- 清理不绕过既有校验、完整入库、原路径/hash/标签、共享文件或 H&R 保护；来源 H&R 未知时保留，用户可在现有清理确认流程中核对后明确批准。单纯打开列表或启用设置不会处理历史文件。

- 上传control的action/sessionId与输出恢复的digest/revision不可混用；收到未知提交结果先GET原task和sessions，不自行生成新operation身份。
- 下载清理需要confirm和HR证据；cleanup/reconcile只核对原操作。删除真实媒体、取消做种和删除业务卷不是普通测试/“修复重试”。
- STRM原文持有稳定resolver，不存临时CDN URL。preview/批量preview/repairs-preview是预览，approve明确digest+confirm后才执行。保留现有STRM原文，不能批量把临时URL写回。
- `/api/v1/cloudStorage/strm302` 仅GET/HEAD严格302；实际UA用于官方115取链及校验。provider需特殊headers/HTTP/不兼容UA时409，不relay、HLS或转码；CDN验证失败502，不能偷偷fallback。fetch默认跟随302会掩盖边界，诊断必须手动观察跳转且不传播管理员Cookie。

```http
POST /api/v1/pipeline/downloads/11111111-1111-4111-8111-111111111111/retry
X-Requested-With: media-tool
```
```json
{"data":{"queued":true,"flow":{"phase":"download","retryable":false,"nextAction":"wait","reason":"retry_queued"}}}
```

错误可有同结构 `error.flow`；只按retryable和nextAction+最新任务状态展示动作，不将message正则匹配成重试权限。

## 8. 文件工作台、扫描、媒体库与 STRM 修复

1. GET `/files/roots` → `/files` 受控浏览；文件操作走batch-preview →读batch →approve(digest+confirm) →轮询/reconcile。路径只编码一次，服务端根目录与符号链接保护不可被前端normalize替代。
2. 扫描POST `/library/scans` → GET扫描/候选列表 → recognize/confirm（支持明确批量确认）。目录变更、归档清理、STRM输出修复均先预览范围，再按digest审批。
3. GET `/media/library/sources` 选择managed/Emby/插件来源 → `/media/library` 列表 →条目详情；source错误是错误，不伪装空库。poster/STRM content走二进制专门请求。
4. `/library/outputs/{id}/content` 与资产STRM下载含稳定授权URL，仅私有管理员可查看，不能把原文放普通公共缩略图/统计埋点。
5. artifact repair、cleanup可能needs_reconcile/unknown，操作结果不明应inspect原batch；不得自动重放rename/move/delete。

```http
POST /api/v1/strm/repairs/11111111-1111-4111-8111-111111111111/approve
Content-Type: application/json
X-Requested-With: media-tool

{"digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","confirm":true}
```

过期/不匹配预览拒绝示例：
```json
{"error":{"code":"strm_repair_conflict","message":"预览已变化，请刷新并重新确认"}}
```

## 9. 原生媒体服务、媒体用户与 Emby 兼容端口

### 管理端流程

1. GET `/media-server/providers`、`/native/settings`、`/native/libraries`、library-mappings → 配置enabled/revision/serverName →明确启用。原生服务不是额外容器，仍在唯一app内。
2. `/media-server/users` 创建/编辑媒体用户与库权限，设置最终密码、UA/分级/流数限制。管理员临时密码需要改成最终密码后才能原生登录；后台管理员Cookie不能充当媒体token。
3. 查看用户devices/history/native sessions，撤销设备token需确认；用户禁用/权限撤销后重读，不依赖旧前端播放信息。
4. processing summary/tasks →按job kind与最新状态retry/受控GC；bulk需明确confirmed。probe/stills任务是媒体预处理，不是媒体relay/HLS/转码服务。

```http
POST /api/v1/media-server/users/11111111-1111-4111-8111-111111111111/devices/22222222-2222-4222-8222-222222222222/revoke
X-Requested-With: media-tool
```

响应 `{"data":{"revoked":true}}`；重读devices确认撤销，不拿管理session去测试媒体登录。

### 独立原生媒体协议（不包 Envelope）

- `/System/Info/Public` → `/Users/AuthenticateByName`（Username/Pw或Password）→返回AccessToken →Views/Items →PlaybackInfo →Videos/stream严格302 →Sessions/Playing/Progress/Stopped上报。详情/列表/进度等DTO为PascalCase，列表是QueryResult或指定数组，不能泛用管理data解包。
- token可从X-Emby-Token、X-MediaBrowser-Token、Emby/MediaBrowser Authorization字段或兼容query api_key读取；冲突token拒绝。优先header，不把媒体token放日志/公开URL。
- 原生服务只接受自己的路由；管理 `/api/v1/*`、PWA/admin路径在媒体端404。GET包含HEAD；已知path OPTIONS只返回静态Allow，不授予任意Origin/Credentials跨域权限。
- `/socket` 明确501（没有WebSocket实现）；字幕stream明确404（STRM原媒体由播放器从CDN处理）；媒体转码/HLS参数被拒绝。外部Emby网关可以代理元数据/图片/登录，媒体仍只允许受管原始源302，不重开fallback。

```http
POST /Users/AuthenticateByName
X-Emby-Authorization: MediaBrowser Client="Example", Device="Synthetic", DeviceId="synthetic-device", Version="1"
Content-Type: application/json

{"Username":"example_media_user","Pw":"ExampleOnly-2026"}
```

媒体地址响应节选：
```json
{"AccessToken":"example-token-not-valid","ServerId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","User":{"Name":"example_media_user"}}
```

错误是 `{"ResponseStatus":{"ErrorCode":"Unauthorized","Message":"Unauthorized"}}`，不是管理error Envelope。受控播放403可能增加顶层ErrorCode/Message。更多兼容边界见 [NATIVE_EMBY_SERVER](NATIVE_EMBY_SERVER.md)。

## 10. 插件目录、安装/启用、能力、UI 与动态 API

1. GET `/plugins/catalog` 与 `/plugins`；只有同pin编译目录定义。显示元数据/权限、fingerprint，管理员选择后才POST `/plugins/operations` kind=install。无source/version/commit/URL/ZIP/任意源代码上传入口。
2. 首次意图生成operationKey，pending/回复丢失/列表刷新失败都保留原key与输入；同意图重试使用原key。只有明确“新建另一实例”才能换key。安装/配置不等于enabled/running。
3. enable/configure/review/disable/remove共用expectedRevision/fingerprint和确认。升级新增权限进入待审核，不自动扩大scope；removed definition不可用但保留配置/KV/data。
4. GET `/{id}/capabilities`：unsupported（未声明/不可用）、authorization_required（待授权）、disabled、starting、runtime_failed（含unknown运行结果）、ready（本代lease与snapshot注册有效）。不要把“不支持”显示成“授权后一定可用”。
5. command/action/tool/chain/workflow按各自descriptor与权限，写/危险动作需明确确认。runs的pending/running/succeeded/failed/unknown/canceled是真实状态；unknown先inspect，不自动重发外部副作用。
6. UI读 `ui-surfaces`、`/{id}/ui/{form|page|dashboard|meta}`；优先JSON节点白名单；可信ESM来自同pin镜像构建时打包、plugin-ID命名空间。禁止运行时远程脚本/动态编译。页面加载失败与“没有插件”分开显示。
7. state snapshots create/restore/delete在disabled实例、revision/fingerprint/confirm约束下操作；没有读取跨实例snapshot或伪造所有权捷径。
8. journal retention的warning/blocked可恢复；Schedule是独立维护域。回放7天外410，X-Event-Retired-Cursor供人工选择范围，不能清空journal或换游标掩盖问题。

```http
POST /api/v1/plugins/operations
Content-Type: application/json
X-Requested-With: media-tool

{"operationKey":"example-install-01","kind":"install","pluginId":"synthetic-example","fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","expectedRevision":0,"confirm":true,"scopes":[],"config":{}}
```

实际pluginId/fingerprint/scopes必须来自catalog及用户确认。安装成功响应节选：
```json
{"data":{"id":"11111111-1111-4111-8111-111111111111","pluginId":"synthetic-example","desiredState":"disabled","state":"stopped","revision":1}}
```

### 动态挂载不是静态路由集合

`/api/v1/plugin/{instance}/{path...}` 的具体method/path/auth/body/响应由**当前同pin、已授权且lease有效**descriptor决定。OpenAPI列宿主mount和宿主边界错误；动态schema需读extensions/SDK契约，不能把任意路径宣称成功，也不能把宿主mount的所有方法当插件都实现。

- auth=admin：管理员Cookie+来源/CSRF；apikey：只接受X-API-Key；bearer：只接受Authorization Bearer；public：仅明确获准public descriptor。apikey与bearer不能混用；这也不是external API key。
- 超限/descriptor失效/权限不足/维护暂停都在宿主拒绝。动态JSON不是通用OK，流式或非JSON能力不能凭UI猜测。
- 外部API `/api/external/v1/*` 另用永久凭据+具体scope；不接受管理员Cookie或STRM key代替。`POST /api/v1/external-api/keys` 的 **201** 创建回执仅此一次返回完整 `token`，请立即安全保存；公开示例不使用真key。
- `GET /api/v1/external-api/keys` 返回 `KeySummary[]`：`id/name/prefix/scopes/expiresAt/revokedAt/lastUsedAt/createdAt`。`prefix` 仅为原令牌前15字符的脱敏提示，不含完整 `token`、哈希或密文；列表不读取/解密密文，旧凭据和撤销凭据同样适用。前端列表不依赖明文，也不会补显或持久化创建回执。创建结果未知时只读列表核对，禁止自动重复创建；明文丢失则撤销并显式创建新凭据。

## 11. 运维、备份、日志与数据视图

1. GET `/ops` 获取policy/backups/drills/releases/requests/mode/schema/toolsReady →备份/演练排队需confirm →轮询原ID；不能把202显示成已完成。备份下载是私有tar，不按JSON解析。
2. restore/rollback：POST requests/preview →展示保护范围、digest、confirmText →approve →**宿主受控CLI execute** →读取结果 →POST verify →人工复核 →resume。网页没有任意shell/容器执行入口。
3. flow：preview/approve/execute/verify/complete。approved不是已执行，completed但verifiedAt缺失仍需verify；failed/未知不得自动重复切库。verify是核对receipt/版本/schema/受保护摘要，不重新执行、不自动解除维护。
4. 使用正式 Docker Hub 不可变 AMD64 镜像执行维护，不随意逆向修改数据库；回滚要完整旧数据库+密钥/STRM+匹配旧镜像备份。
5. GET `/logs/query`、`/logs/summary`；原始URL/数据是管理员私有原值，不能自动打码造成诊断失真，也不能上传公开Actions。
6. 仅允许白名单表/列/固定排序映射；界面不要提供SQL片段/where拼接。日志导出必须显式用户触发并提示私有范围；登录body/Cookie/Authorization不额外收集，恢复口令不入web日志。
7. ACK容量告警不等于问题解决；blocked恢复要检查GC/订阅ACK/未解决run，禁止清库、截断幂等身份或绕过迟提交保护。

```http
POST /api/v1/ops/requests/11111111-1111-4111-8111-111111111111/verify
Content-Type: application/json
X-Requested-With: media-tool

{"confirm":true}
```

核验后响应节选：
```json
{"data":{"state":"completed","verifiedAt":"2026-10-20T00:00:00Z","flow":{"phase":"complete","retryable":false,"nextAction":"resume_after_review","reason":"protected_data_verified"}}}
```

若无有效执行收据或摘要不符必须拒绝，不应把“本页面显示版本号正确”当作verify成功。

## 12. 通知、投递与未知结果

1. 配置并测试Telegram等integration →GET `/notifications`；业务Outbox与插件journal维护域独立，通知失败不回滚媒体入库。
2. 每次投递区分pending/failed/unknown等状态。只有明确失败且可安全重试才提供普通retry；unknown先核对外部是否已送达，明确接受重复风险后方可再次投递。
3. 不再使用已退役notification_channels/channel_id；按当前integration/投递记录操作，凭据不进普通事件列表。

```http
POST /api/v1/notification-deliveries/11111111-1111-4111-8111-111111111111/retry
Content-Type: application/json
X-Requested-With: media-tool

{"acceptDuplicateRisk":false}
```
```json
{"data":{"queued":true}}
```

这是重新排队，不是外部送达证明；409 notification_retry_blocked应保留原记录，提示inspect或重复风险确认，不能改成“发送成功”。

## 13. 界面调用约定

- 以schema字段/枚举和真实source区分empty/loading/error/unsupported/disabled/authorization_required/unknown；不要把catch统一变空数组。
- 所有修改入口防重复点击并保留原意图；取消请求不等于撤销已提交操作，重连先回读/对账。
- 时间按date-time处理，原生UTCDateTime固定7位小数Z；数值ID可能是字符串，不强制转JS Number丢精度。
- revision冲突保留用户草稿供对比，不自动覆盖；删除/危险动作显示预览范围和明确确认文案。
- 不支持的HLS/relay/转码控件不能重新激活；禁用的直链策略开关也不能通过旧API/环境绕过后端策略。

## 工作区呈现与交互规范

页面归属、标签和统一组件规则见 [前端设计规范](FRONTEND_DESIGN_MASTER.md)。刷新仅读取当前标签，下载器同步为显式业务操作；统一分页为上一页/第N页/下一页，子分区统一分段控件。空、错、不支持、未授权须分别呈现。配置预填当前生效值而不自动写回默认值，未保存草稿在刷新和修订冲突时保留。日志与数据浏览按管理员权限显示原始值及完整请求/跳转URL。

## B2 管理端会话、任务控制与115健康检测（schema73）

- 管理员登录位于 PostgreSQL `administrator_sessions`，不同于媒体服务器用户会话。`GET /api/v1/auth/sessions?page=1` 返回 `{items,hasNext}`，每页50条，含无凭据意义的会话 UUID、账号、创建/最后活动/到期时间、原始连接IP及UA、`current`。所有管理端账号目前均为完整管理员，可分页读取全部有效管理端登录；不返回cookie、摘要或密码。IP不信任任意转发头。升级前没有活动元数据的旧会话以其创建时间作为最早可确认活动，IP/UA保持空值至实际请求更新，不伪造迁移时登录。
- `POST /api/v1/auth/sessions/{id}/revoke` 必须 `{confirm:true}`；幂等。撤销当前登录返回 `loginRequired:true` 并清cookie，下一次请求401；UI明确跳转登录页。撤销单个管理端登录不影响其他管理端登录或媒体服务器用户登录。
- `GET /api/v1/tasks/{kind}/{id}` 返回 `ControlResult{id,state,revision,queued,stopPending,externalEffects,reason}`。类型为 `jobs/downloads/transfers/scans/repairs/organize/archives/ops`；业务ID解析为当前阶段原job，不新建另一套业务任务。`POST /api/v1/tasks/{kind}/{id}/{action}` 的 action 仅为 `cancel/retry`，要求 `{confirm:true,revision:<刚读取的修订>}`。
- 202仅表示排队或请求协作停止，不代表完成。pending取消立即阻止认领；running请求持久化，执行器200ms检查并取消context，HTTP有界等待2秒，尚未退出返回202。已发送的外部副作用不保证可撤回；写任务中断保留unknown并禁止租约过期自动重放。读取核对不触发新的外部提交。
- retry以行锁+修订CAS进入持久化checking，在锁外核对业务真实状态，只有确认未完成才重新排队；已完成直接收敛，失败保留unknown与原因。并发/重复同修订请求最多执行一次核对；失联checking两分钟后回unknown，不自动执行。下载核对原下载器/哈希/路径，上传核对原端点/目标大小哈希及活动会话，修复核对原STRM正文与归属；扫描重建新generation，不复用不完整缺失票据。文件整理核对路径后若有剩余项必须重新预览，不能重放旧计划。宿主维护仍必须沿原维护单核验；通用层明确拒绝不可证明安全的重试。
- `archives`是已有人工批准的同步清理账本：取消仅限未执行的approved；retry按原批准范围重新检查归属、保留期、隔离文件和当前输出，原子路径步骤幂等。已发生的物理删除不能撤回。`ops`仅取消preview/approved，executing不能从应用撤回，retry要求原宿主核验。
- 旧 `/pipeline/{kind}/{id}/retry`、上传control、扫描pause/resume/cancel、STRM批次retry、归档retry、文件核对、ops取消/核验接口**全部保留**，未删除调用点。原预览/批准/digest/来源重确认边界不被通用层替代。
- 115健康检测由现有default调度器调用，PG持久化租约、配置修订、下次时间与失败次数。新建/配置变化后延迟5–10分钟，正常6–7小时，失败30分钟指数退避至6小时并加抖动；多实例和重启共享截止时间。只调用已有`/open/user/info` Probe，必要时复用原串行令牌刷新；不列目录、不取下载链接。回写已有lastCheckState/lastCheckMessage/lastCheckedAt，修订变化不写入旧检测结果，保留原授权。连接页只读展示检测时间/结果；成功不是实际播放验收证明。
- 手动下载的“未见目标”仍有延迟接受风险，通用层仅核对，不绕过原页面的重复风险确认。已取消归档操作仅可通过新的预览与批准重新开始，旧取消批准不能由retry恢复。

## B3：内容身份、层级媒体与外挂字幕（schema74）

- 管理端登录仍每请求读取 PG；活动时间同 IP/UA 下最多每 60 秒写入一次，IP/UA
  变化立即记录。UA 以 UTF-8 边界限制为 512 字节；活动更新失败只记日志，不否定已通过的认证。
- 插件定义指纹为 `content-v1:<sha256>`：规范 JSON 描述符、权限/schema/处理器声明，
  同 pin 的插件源码、SDK 和捆绑 ESM 内容参与计算；主程序 BuildID 不参与。
  旧 BuildID 身份只有在构建期对 go.sum 已锁定历史 pin 的源码/ESM 哈希证明内容等价时才静默重绑；不能仅凭同一 manifest 猜测处理器相同。无法证明等价则正常增修订并换代，但不误判为权限变化、不改变已启用意图。权限变化仍待确认。
- 插件写权限错误返回固定中文原因和下一步（停用、只读、权限待确认），不透传远端任意错误文本。
  `sites.request`/`sites.checkin` 返回 `status/state/matched/body/headers/truncated/encoding`。
  默认正文上限 256 KiB，`maxResponseBytes` 可设置更低正整数；同时受现有业务回执
  JSON 80 KiB 安全预算约束，必要时更早截断并标记。只接受 UTF-8 文本，二进制拒绝；
  仅保留 Content-Type、Content-Language、Retry-After，清除账号凭据，不跟随重定向。
- GET `/api/v1/media/library?source=managed` 返回作品级列表（Series/Movie），不再按当前文件页分组。
  `parentId=series_<作品UUID>` 返回按季号排序的季列表；
  `parentId=season_<作品UUID>_<季号>` 返回季内按集号排序的物理文件列表。
  各层独立使用 `page/pageSize/total`。季/作品没有原始 STRM 文件链接。
  Emby 来源使用真实 ParentId，非递归读取 Season/Episode。
- 原生 Emby GET `/Items` 同时校验 ParentId、SeriesId、SeasonId；
  `/Shows/{id}/Seasons` 和 `/Shows/{id}/Episodes?SeasonId=...` 保留分页和跨作品校验。
- 原生 PlaybackInfo 中，外挂字幕 MediaStreams 返回 `IsExternal=true`、
  `DeliveryMethod=External` 和用户鉴权的 `DeliveryUrl`。字幕流路由沿用
  GET/HEAD `/Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{format}` 及已有短路由；
  `SubtitleId` 绑定云盘文件身份，`MediaSourceId` 绑定当前可播放版本。
  仅原格式零正文 302，实际请求 UA 同样通过播放器白名单并用于官方 115 取链；
  格式转换明确 400，嵌入字幕提取不支持。字幕正文不落地、不代理、不转码。

## 网页恢复（schema 77）

`GET /api/v1/ops/restores` 返回最近50条持久化执行记录。schema77新增 `tableChanges:[{name,currentRows,backupRows,currentMissing}]`，仅表名/行数，不返回内部摘要、凭据或密文。预览响应为具名 `WebRestorePreview`，新增固定 `warnings[]`（管理员密码回退、115令牌回退/可能需授权、会话失效），原 `restore/confirmText/includes/excludes/message` 保留。

`POST /api/v1/ops/restores/preview` 接收 `{backupId}`。全部public业务表随备份回退，不比较当前保护表是否与备份一致；当前业务数据为空/逻辑损坏也允许。候选校验完整性、当前master key下全部备份密文及向上迁移，高schema拒绝。`tableChanges`给出每张将回退表的当前/原备份行数差异；相同行数不表示不回退。

`POST /api/v1/ops/restores/{id}/execute` 仍接收 `{digest,text}`，精确短语 `恢复 <备份UUID>`；CSRF及10分钟内新近登录不变。202仅排队，重复同ID不重复执行。执行先暂停/排空任务和写入，强制恢复前快照，单事务按外键顺序DELETE/COPY全部业务表。导入期间认证/进度SELECT可读；提交时账号、密码与会话一起切换，旧会话失效，401不证明失败或未执行。前端只记回执UUID，提示备份账号重新登录后进入运维恢复页GET结果，禁止自动重放POST。

`POST /api/v1/ops/restores/{id}/reconcile` 接收 `{confirm:true}`，仅核对中断提交标记及已应用备份的数据，不重跑。失败自动补偿，仍不明则unknown并保持维护。CLI使用同一执行器。API路径无增删；保留的协调账本、运行态安全归一化、磁盘不覆盖边界详见[部署恢复说明](deploy/backup.md)。

状态仍为preview/pending/running/unknown/completed/rolled_back/failed，返回backupSchema/programSchema、snapshotId/appliedBackup及阶段message。重复execute对pending/running返回202，对历史终态/unknown返回200原状态，不新建执行。

前端传输仍统一由`web/src/api/client.ts`管理：SSE subscribeJobEvents、外部令牌testExternalToken（credentials:omit）、originalSTRM（仅同源既有媒体路径）。普通JSON API保持same-origin；index.html不注入业务数据，传输归属与调用路径守卫继续生效。

### F5a 插件配置展示边界

- 插件设置表单在构建时从同一固定模块版本导出到编译资源；停用实例使用静态表单，不调用 UI RPC，不启动 worker。该资源只含控件、约束和默认值，不含实例配置或凭据，也不向 index.html 注入业务数据。
- 实例详情的受保护 GET `/plugins/{id}` 只读实例、配置修订和定义标识，不读取或返回 KV。内部受管数据、配置、快照及 SDK 能力保留；不再提供通用实例数据查看器或配置 JSON 编辑器。
- 已删除原调试面板独占的 `/plugins/{id}/tools`、`/tools/call`、`/auth/status`、`/state-snapshots` 以及 `/plugins/auth/bindings|begin` 管理路由。保留 `/plugins/workflows`，因为插件工作区的独立工作流管理仍在使用；保留共享 `/plugins/{id}/ui/{kind}`，因为已授权的插件页面、仪表盘和 SDK UI 契约仍使用它。
- 数值控件按完整 min/max/step 范围设置，不以常用值枚举截断合法配置。保存仍使用修订校验、幂等操作与原有权限边界；仅更新表单不扩大执行权限，也不把已启用实例改为停用或待确认。

### 日志保留与限时 Debug（F5b）
日志仍为私有轮转 JSONL（最多 50 MiB），不是 PostgreSQL 日志表。`/logs/query` 默认每页 200 条；`before` 固定检索快照，后续页复用同一快照，检索窗口最多 20,000 条且通过 `truncated` 明示，不把窗口计数说成全量历史。返回管理员原值和完整 URL。

应用设置 `logging.retentionDays` 为 1–90，默认 7；`logging.debugUntil` 是自动关闭时间，过去/零时间关闭，未来最多 24 小时。每天按 1000 条/批清理文件中的过期记录；后台按秒关闭过期 Debug，不采集登录请求体。一次性恢复密码仍只写启动 stdout。
