# 架构说明

更新：2026-10-10，0.7.0-dev，源码迁移17。本文描述当前实现；旧M0/M1进展保存在CLI_REFERENCE.md，不再把已实现的下载/上传/通知标为占位接口。代码完成和真实业务验收分开，见CLI_REFERENCE.md。

## 运行入口与依赖

- `cmd/media-tool -role server`：管理HTTP/Vue、REST API/SSE、固定STRM解析，以及独立Emby根网关。
- `cmd/media-tool -role worker -queue default`：搜索、订阅评估、下载创建/对账、Emby刷新/确认、Outbox通知投递、扫描计划和输出恢复协调。
- `-role worker -queue transfer`：独立长时上传/校验；`-queue library`：独立云盘流式扫描，避免占用普通任务处理。
- `-role strm`：受约束的STRM写入；`-role healthcheck`：容器就绪探测。
- `media-tool migrate`：版本化迁移；测试必须`-test-only`，实际库名不是media_tool_test便拒绝。down为破坏性运维，不用于日常回退。
- `media-tool secrets rekey`：应用和全部Worker停止后的九张加密表原子轮换（含search-run作用域快照），默认dry-run。
- `media-tool admin reset`：操作者本地恢复管理员访问；不生成默认账号或自动重置用户密码。
- `cmd/*-smoke`：只读/限量验证工具，不等同客户端解码验收。

PostgreSQL保存账号、配置、资源、下载/传输、索引、文件凭据、订阅、任务/租约、Outbox、输出恢复journal和审计事实。Redis只保存可重建缓存/失效提示和Emby Token摘要→用户ID映射，不能取代持久队列或独立授权校验。生产业务必须有主库；缺依赖明确503，不用内存假数据冒充可用。

## 模块与扩展边界

```text
cmd / platform.httpapi / Vue
           ↓
accounts · integrations · sites/search · subscriptions
           ↓
acquisition → downloads → storage → library/strm → emby
           ↓                        ↓
       jobs / Outbox            playbackgrants / playback
           ↓
     messaging / notifications

adapters实现各领域端口；platform提供DB/缓存/安全HTTP/加密/静态嵌入
```

- PT站点配置取固定pre-dessert-sites快照并记录解析支持/拒绝，不保证该仓库全部规则已经兼容。Cookie/API Key为加密配置，不默认借用账号。
- qBittorrent适配器创建/查询下载；下载源通过明确共享路径只读访问，校验元数据/文件和v1/v2/hybrid身份，保留做种源。
- CloudDrive2官方gRPC负责扫描、读取、上传、校验、传输控制和受约束恢复；上传进度100%不是完成事实，未知启动不能盲重试。
- `open115`是独立播放解析适配器，不替代CD2的扫描/上传，不借用CD2/其他应用115授权。
- TMDB用于身份确认；影视输出不会以未知身份自动猜集数或覆盖另一剧集的共享NFO。
- 通知有站内和Telegram渠道；明确拒绝/限流/未知结果分开处理，主业务成功不因通知失败回滚。

## 账号与服务凭据

首次注册唯一管理员成功后公开注册关闭，之后账号密码登录，密码6–20字符。会话为HttpOnly/SameSite Cookie、8小时有效、数据库可撤销；改密提升credential_version并撤销旧会话。旧ADMIN_API_KEY不再授权管理请求。不把管理账号当作Emby播放用户。

变更请求检查X-Requested-With以及Origin；固定APP_PUBLIC_URL/EMBY_PUBLIC_URL不信任任意转发头。生产HTTPS Cookie策略和用户明确要求的远端公网HTTP测试部署分别记录，媒体CDN的HTTPS不能替代管理入口TLS。

服务凭据用APP_SECRET_KEY的AES-GCM加密、按记录ID绑定AAD，配置修订避免旧凭据发往新目标。官方扫码会话额外使用`open115-auth:<sessionID>`命名空间AAD；rekey保留ID/AAD且任一解密失败回滚全部表。主密钥必须独立备份，不能只替换环境变量。

## 官方115授权与动态取链

自己的AppID → 本地PKCE/二维码 → 加密、管理员版本/连接revision绑定的5分钟会话。二维码状态长轮询不持数据库锁。确认后先持久化exchanging并清verifier，再执行一次性兑换；完成时重新检查会话、账号版本、连接revision。取消/新二维码/编辑优先于旧结果，崩溃或未知响应不重放旧code，孤立claim到期后重授权。授权成功不自动启用连接。

Access/Refresh按官方expires_in成对加密保存，PG行锁串行跨进程刷新，60秒冷却；终态或不确定刷新结果持久阻断旧Refresh重试。播放使用实际HTTP UA取downurl，核对当前路径、文件名称/大小和pick_code；临时地址仅短缓存45秒，不编造官方URL真实有效期。

## 持久任务与恢复

业务变更和任务/Outbox在同一PG事务建立，SKIP LOCKED领取、UUID租约、心跳/完成均检查token与到期。至少一次运行不代表外部副作用恰好一次：下载唯一决策/外部关联、传输会话、控制revision/上传RPC互斥及对账负责避免盲重复。

输出先写加密journal再stage/落盘，文件摘要/归属保护人工修改和共享NFO，重启自动恢复可恢复项，冲突保留给操作者。扫描完整性独立计票，两次完整扫描才确认缺失；失败/部分扫描不触发删除，缺失默认不删已有STRM。周期子目录对账已实现，CD2 PushMessage实时消费仍未实现。

SSE只是失效提示，断线后重新GET持久数据；重启不能把任务/通知/上传恢复事实丢在内存。

## STRM、Emby与流量路径

STRM写固定HTTP解析地址、assetId及可撤销文件级Key；path仅兼容提示，临时115/CDN链接不入文件。移动后用资产当前路径重新解析，输出迁移/旧输出清理仍保守，不盲覆盖/删除。

```text
客户端 → Emby根网关 → 调用者Token的user/item/source授权
                          ↓ 托管原始流，源/路径/权限均匹配
                       动态取链（实际UA）
                          ↓ 同UA HTTPS Range最多1KiB验证
                     HTTP 302 → 客户端直接访问115 CDN

不支持原始直放/需转换/HLS → 原Emby remux/转码（保留服务器媒体流量）
```

网关不使用后台API Key冒充用户。仅接受固定托管origin和有效资产归属，元数据隐藏文件Key。成功登录学习权威Token摘要/User.Id绑定，每次播放仍复查用户policy及scoped item，不能信任客户端UserId/DeviceId。未知源/多源/不支持/变换保持上游能力，DirectStream/Transcoding使用server-relative URL，避免网页重复拼接。

官方连接启用时direct-only：CDN验证失败明确502、强制代理冲突409，不偷偷退回CD2媒体代理。CD2播放模式可对固定UA/必须附加头情况受控代理。URL/DNS/拨号/TLS/重定向和跨域去凭据检查都保留；缓存按UA/IP/连接版本/文件版本隔离。

公开日志仅固定诊断枚举，不记录query、Cookie、Token、签名Location或完整PlaybackInfo。302模式仍有少量鉴权/API/最多1KiB探测流量，不宣称服务器绝对零字节。

## 前端和部署

Vue 3/TypeScript/Vite/Element Plus，构建产物由Go embed；SPA回退、指纹资源缓存、管理响应不缓存。界面只显示真实API状态；空配置、未知结果、暂停/失败都与成功区分。扫码取消移除二维码、停止轮询，新状态明确异步保存进度。

Compose包含应用、三个Worker、迁移、PG/Redis；持久卷不随重建删除。基础管理8080/网关8091默认回环，NAS共享目录通过compose.shared.yaml明确映射，下载只读/STRM可写。用户远端测试例外允许公网HTTP，PG/Redis仍不公开。Docker支持arm64/amd64构建；模拟amd64不等于物理x86 NAS验证。

## 已知边界

真实默认Sen UA的302/CDN206和非零Range样本已通过，但真实Sen解码/完整客户端矩阵未通过。网页约8分30秒冷恢复出现HLS404/FFmpeg0帧，尚未宣称解决。真实PT搜索→下载做种→上传→输出→Emby→通知、订阅及容量/HTTPS/恢复门槛继续按证据矩阵验收。


## 来源恢复发布收尾：2026-10-11

已发布media-tool:source-reconfirm-20261011，schema20→22，仅停止/更新server、worker、transfer-worker、library-worker；账号/主密钥/115授权/媒体身份不改，无PT账户或规则自动导入。全量本地race/vet、Vue9项/构建、18脚本、23E2E；原生x86_64五包30项顶层测试无跳过，使用新内部网络/独立PostgreSQL，非生产库/非真实PT资源。原生跨编译fixture不带race，本地race证据另列。ARM64原生只读无网络配置审计通过，193/159/34/0仍只是静态支持，不提高accountVerified。

补验证qB接受但返回503的未知结果按已落库Hash核对、无第二次Add；旧receipt不恢复新暂停代，来源revision/同run提示在网络读取中变化被拒绝；读取无效task详情后不以空结构覆盖原数据。初始候选不包含最后这一读取失败保护，故保留初始包并生成validated独立源码/镜像/fixture清单；正式切换的是validated版本，不替换或冒充历史证据。

发布前备份已在另一个独立PG恢复，升级22并执行9类密文dry-run回滚，再结构降20/升22验证。第一次SSH克隆派发失败后读取证实无marker/process/目录/容器/网络/卷；第二次执行输出丢失，后续发现服务器已完成的passed回执，采用回执而没有第三次执行。发布亦以持久化receipt核对，不盲目重放。临时资源、含密钥目录与SSH材料在收尾清理。

发布后12项原STRM媒体详情/GET/POST PlaybackInfo逐字一致，四组代表性播放器UA头请求302→cdnfhnfile.115cdn.net→206，HLS/转码拒绝，验证窗口无新增FFmpeg日志；不是实际设备解码或非管理员权限验收。原5份磁盘输出、密码记录/.env/compose、资产绑定/播放授权/扫描计划与Emby/CD2/其他23容器由摘要核对保持。API仍要求管理员登录，首次注册已关闭，密码新设置6–20位。

回退镜像media-tool:before-source-reconfirm-20261011；备份/root/docker/mediatool/private/database-before-source-reconfirm-20261011.sql.gz；旧源码/root/docker/mediatool/private/source-before-source-reconfirm-20261011。发布/原生/克隆/保护证据为data/remote-deploy/source-reconfirm-*报告与validated-release-manifest。文档收尾另用docs-supplement，不修改运行代码或再次重启。

真实PT/qB/外部通知连接与订阅当前仍0；合法资源搜索→下载→做种保留→CD2→STRM/NFO→Emby→通知、真实缺集/促销与设备/权限/HTTPS/容量稳定性仍未完成。下一步继续pre-dessert请求体/分页/框架等更宽契约，以及上述实际验收，整体目标active。
