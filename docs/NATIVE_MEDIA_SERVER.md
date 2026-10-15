# 内置媒体库接口与媒体用户（v1）

本轮提供可运行的扩展骨架、独立账号和授权目录，**不是完整的 Emby 替代服务器**。现有 Emby、115 临时播放地址、固定 STRM 内容和302网关保持原样。

## 使用入口

- 后台「媒体浏览 → 自研媒体库」：查看 builtin / Emby 的真实能力与扩展状态。
- 后台「媒体浏览 → 媒体用户」：创建、编辑、禁用、删除用户，重置密码、撤销会话。
- 独立媒体监听端口（默认8092）的根路径 `/`：独立媒体用户登录、首次改密码、只读授权目录、本人修改密码、退出。无公开注册。
- 初始逻辑媒体库：`movies` 电影、`series` 剧集，以已确认 `media_asset_bindings.state=ready` 及 `cloud_files.state=present` 投影资产，不移动文件，不自动授权未确认资产。
- 管理员和 Emby 账号不能在媒体登录入口使用。初始密码 / 管理员重置后的密码须修改后才能浏览。6–20字符，支持中文和空格，不去除密码首尾空格。
- 新用户默认无目录权限；管理员显式授权浏览及预留播放。勾选播放不会在尚未实现的版本中启动播放。

## 内置运行与未来语言选择

- 推荐 Go：在 app 同一进程执行，复用领域模型、数据库、任务队列和115解析；前端继续 Vue/TypeScript。
- `internal/modules/mediaserver` 提供 Provider（Describe / Libraries / List / Playback）和显式注册表。适配器需实现同一 v1 DTO / 错误语义，不直接暴露内部存储对象。
- `EmbyAdapter` 保留现有目录集成的接口边界，不给 member 传递 Emby API Key；现有 Emby 网关与管理页面保持不变。
- 未来 TypeScript 模块通过受控适配器实现相同 JSON 契约；本轮**没有安装 Node 运行时、启动 TypeScript 服务或新增容器**。TS 实现的进程通信/打包方案需在实际选型时确定，不执行动态远程代码。

## HTTP API

均返回 `{data: ...}` 或 `{error:{code,message}}`。所有写请求携带 `X-Requested-With: media-tool` 并通过既有 Origin 白名单。后台写请求要求管理员会话；member 写请求只接受媒体会话。用户输入不能指定调用者 userId 或后台 Provider。

|身份|方法|接口|用途|
|---|---|---|---|
|管理员|GET|`/api/v1/media-server/providers`|能力和接口版本|
|管理员|GET|`/api/v1/media-server/libraries`|可授权的逻辑媒体库|
|管理员|GET|`/api/v1/media-server/users`|最多500用户的管理列表|
|管理员|POST|`/api/v1/media-server/users`|创建；username/displayName/password/enabled/grants|
|管理员|PUT|`/api/v1/media-server/users/{id}`|编辑；同创建但不含password，必须revision|
|管理员|POST|`/api/v1/media-server/users/{id}/password`|重置；revision/password|
|管理员|POST|`/api/v1/media-server/users/{id}/sessions/revoke`|撤销全部会话；revision|
|管理员|DELETE|`/api/v1/media-server/users/{id}`|删除身份及权限、会话，不删除文件；revision|
|公开|POST|`/api/v1/member/auth/login`|username/password；无注册接口|
|媒体用户|GET|`/api/v1/member/auth/session`|会话与本人基本信息|
|媒体用户|POST|`/api/v1/member/auth/password`|currentPassword/newPassword/confirmPassword，成功需重新登录|
|媒体用户|POST|`/api/v1/member/auth/logout`|仅退出当前媒体会话|
|媒体用户|GET|`/api/v1/member/libraries`|本人可浏览媒体库|
|媒体用户|GET|`/api/v1/member/libraries/{library}/items`|page/pageSize/q，目录不含文件路径或播放密钥|
|媒体用户|GET|`/api/v1/member/libraries/{library}/items/{item}/poster`|校验用户及资产后读取固定TMDB源图片|
|媒体用户|POST|`/api/v1/member/libraries/{library}/items/{item}/playback`|预留；授权在库资产返回501，没有权限403，不存在404|

创建示例（占位值仅说明格式，不会创建系统示例用户）：

```json
{"username":"your_media_user","displayName":"家庭成员","password":"自行设置","enabled":true,"grants":[{"libraryId":"movies","canPlay":false}]}
```

`grants` 中出现的库表示可浏览；`canPlay` 仅表示未来播放许可，不能越过浏览许可。编辑用乐观 `revision`；409后须刷新，不自动覆盖。

## 身份和保密边界

- 媒体用户名、Argon2id密码哈希、随机会话摘要保存在独立表。密码从不保存明文或回显；管理员正常日志/配置原值展示政策不改变。
- `media_tool_member_session` 独立随机256bit token，8小时，数据库仅存SHA256摘要；cookie HttpOnly / SameSite=Strict / Path=/api/v1/member，生产Secure。管理员cookie不用于member反向亦然。
- 会话读取实时核对用户启用状态、credential_version、过期和撤销。编辑权限/禁用/密码重置/全部会话撤销递增版本并撤销旧会话；密码修改条件更新避免并发失效账号继续改密码。
- 媒体登录限流使用独立PG计数器（每分钟总计120、实际来源20、账号10），忽略不可信Forwarded IP；密码修改每用户每分钟10次；与管理员限流隔离，但共享全进程两个密码计算槽。
- 后台保留完整可审计原值；媒体用户没有任何系统配置、站点、日志、数据库、授权管理或上游路径访问接口。
- member响应/海报为no-store；海报路径来自已授权资产、仅固定TMDB图片源，禁止客户端指定远程地址或API Key。

## 下一阶段与验收边界

播放票据必须独立于永久STRM/global/file grant，绑定用户、资源、设备/实际UA、短有效期与授权版本；支持撤销后，再接入现有115解析302到CDN。当前接口不签发票据，不偷用管理员播放权限，不代理视频流、不回退到转码。

完整目录层级/物理路径划分、媒体详情/收藏/进度、播放器/字幕/设备、Emby客户端协议替代及显式切换均未实现。真实客户端解码与真实媒体写操作不属于本轮只读上线验收。新增表通过迁移44接入既有备份，不需要新增加密密钥列。
