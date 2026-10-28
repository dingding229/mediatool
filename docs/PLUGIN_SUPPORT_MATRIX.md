# 插件能力表

- contractVersion：2。
- 契约摘要：`310d7eb2d8a96cdd2a3b9dd8f765a0fd24afe759b766b9b93f937ebcdfbc748d`。
- 读取/生成：`go run ./cmd/media-tool-dev plugins matrix [--check]`；读取编译期元数据，不执行工厂。
- `registered` 只表示存在窄门面，不保证任意环境可用；actor、adapter、owner、selector、revision 和实时 scope 不满足时 fail-closed。
- 表中的 supported=false 为固定 unsupported；其他条件性 unsupported 见下文。

## 业务 Host RPC（154 项）

| RPC | scope | 级别 | 注册能力 |
|---|---|---|---|
| `workflows.run` | `workflows.run` | D | registered |
| `workflows.status` | `workflows.run` | D | registered |
| `workflows.cancel` | `workflows.run` | D | registered |
| `downloads.list` | `downloads.read` | R | registered |
| `downloads.get` | `downloads.read` | R | registered |
| `downloads.progress` | `downloads.read` | R | registered |
| `downloads.files` | `downloads.read` | R | registered |
| `downloads.trackers` | `downloads.read` | R | registered |
| `downloads.manualList` | `downloads.read` | R | registered |
| `downloads.downloader.allHashes` | `downloads.read` | R | registered |
| `downloads.downloader.inspect` | `downloads.read` | R | registered |
| `downloads.cleanup.preview` | `downloads.read` | R | registered |
| `downloads.addResource` | `downloads.add` | D | registered |
| `downloads.manualCreate` | `downloads.add` | D | registered |
| `downloads.pause` | `downloads.control` | W | registered |
| `downloads.resume` | `downloads.control` | W | registered |
| `downloads.retry` | `downloads.control` | W | unsupported |
| `downloads.uploadControl` | `downloads.control` | W | registered |
| `downloads.manualRetry` | `downloads.control` | W | registered |
| `downloads.downloader.setPaused` | `downloads.control` | W | registered |
| `downloads.cleanup.approve` | `downloads.delete` | D | registered |
| `downloads.downloader.deleteOwned` | `downloads.delete` | D | registered |
| `downloads.downloader.configure` | `downloads.control` | D | registered |
| `metadata.wishlist` | `metadata.read` | R | registered |
| `subscriptions.ensure` | `subscriptions.write` | D | registered |
| `metadata.torrent.parse` | `metadata.read` | R | registered |
| `search.list` | `search.read` | R | registered |
| `search.get` | `search.read` | R | registered |
| `search.resources` | `search.read` | R | registered |
| `search.start` | `search.run` | W | registered |
| `sites.list` | `sites.read` | R | registered |
| `sites.get` | `sites.read` | R | registered |
| `sites.metrics` | `sites.read` | R | registered |
| `sites.rss.discover` | `sites.read` | R | registered |
| `sites.request` | `sites.request` | D | registered |
| `sites.checkin` | `sites.request` | D | registered |
| `sites.metrics.collect` | `sites.refresh` | W | registered |
| `sites.rss.refresh` | `sites.refresh` | W | registered |
| `sites.templates.catalog` | `sites.templates.read` | R | registered |
| `sites.templates.loaded` | `sites.templates.read` | R | registered |
| `sites.templates.load` | `sites.templates.select` | D | registered |
| `subscriptions.list` | `subscriptions.read` | R | registered |
| `subscriptions.get` | `subscriptions.read` | R | registered |
| `subscriptions.history` | `subscriptions.read` | R | registered |
| `subscriptions.create` | `subscriptions.write` | D | registered |
| `subscriptions.update` | `subscriptions.write` | D | registered |
| `subscriptions.queue` | `subscriptions.write` | D | registered |
| `subscriptions.delete` | `subscriptions.write` | D | registered |
| `subscriptions.evaluateRules` | `subscriptions.rules` | R | registered |
| `subscriptions.coverage` | `subscriptions.rules` | R | registered |
| `downloads.recheck` | `downloads.control` | D | registered |
| `subscriptions.completion` | `subscriptions.read` | R | registered |
| `library.assets` | `library.read` | R | registered |
| `library.candidates` | `library.read` | R | registered |
| `library.history` | `library.read` | R | registered |
| `library.scans` | `library.read` | R | registered |
| `library.recognize` | `library.recognize` | R | registered |
| `library.suggest` | `library.recognize` | R | registered |
| `library.organize.preview` | `library.organize` | D | registered |
| `library.correct.preview` | `library.organize` | D | registered |
| `library.scan.start` | `library.scan` | W | registered |
| `library.scan.control` | `library.scan` | W | registered |
| `library.organize.confirm` | `library.organize` | D | registered |
| `library.correct.apply` | `library.organize` | D | registered |
| `library.refresh` | `library.refresh` | W | registered |
| `library.history.hide` | `library.history.write` | W | registered |
| `fs.media.roots` | `fs.media:read` | R | registered |
| `fs.media.list` | `fs.media:read` | R | registered |
| `fs.media.stat` | `fs.media:read` | R | registered |
| `fs.media.get` | `fs.media:read` | R | registered |
| `fs.media.preview` | `fs.media:write` | D | registered |
| `fs.media.apply` | `fs.media:write` | D | registered |
| `fs.media.previewBatch` | `fs.media:write` | W | registered |
| `fs.media.approveBatch` | `fs.media:write` | D | registered |
| `fs.media.readText` | `fs.media:read` | R | unsupported |
| `storage.roots` | `storage.read` | R | registered |
| `storage.list` | `storage.read` | R | registered |
| `storage.stat` | `storage.read` | R | registered |
| `storage.get` | `storage.read` | R | registered |
| `storage.preview` | `storage.write` | D | registered |
| `storage.apply` | `storage.write` | D | registered |
| `storage.previewBatch` | `storage.write` | W | registered |
| `storage.approveBatch` | `storage.write` | D | registered |
| `storage.mkdir` | `storage.write` | D | registered |
| `storage.relocate` | `storage.write` | D | registered |
| `strm.entries` | `strm.read` | R | registered |
| `strm.diagnostics` | `strm.read` | R | registered |
| `strm.repair.preview` | `strm.repair` | D | registered |
| `strm.repair.approve` | `strm.repair` | D | registered |
| `strm.repair.retry` | `strm.repair` | D | registered |
| `metadata.parse` | `metadata.read` | R | registered |
| `metadata.attributes.parse` | `metadata.read` | R | registered |
| `metadata.attributes.validate` | `metadata.read` | R | registered |
| `metadata.name.render` | `metadata.read` | R | registered |
| `metadata.episodes` | `metadata.read` | R | registered |
| `metadata.normalize` | `metadata.read` | R | registered |
| `metadata.search` | `metadata.read` | R | registered |
| `metadata.details` | `metadata.read` | R | registered |
| `metadata.storedDetails` | `metadata.read` | R | registered |
| `metadata.refresh` | `metadata.refresh` | W | registered |
| `classification.get` | `classification.read` | R | registered |
| `classification.match` | `classification.read` | R | registered |
| `classification.render` | `classification.read` | R | registered |
| `classification.update` | `classification.write` | D | registered |
| `mediaserver.providers` | `mediaserver.read` | R | registered |
| `mediaserver.libraries` | `mediaserver.read` | R | registered |
| `mediaserver.items` | `mediaserver.read` | R | registered |
| `mediaserver.settings.get` | `mediaserver.read` | R | registered |
| `mediaserver.refresh` | `mediaserver.refresh` | W | registered |
| `mediaserver.settings.update` | `mediaserver.settings.write` | D | registered |
| `catalog.list` | `mediaserver.read` | R | registered |
| `catalog.item` | `mediaserver.read` | R | registered |
| `catalog.home` | `mediaserver.read` | R | unsupported |
| `catalog.discover` | `mediaserver.read` | R | unsupported |
| `catalog.nextUp` | `mediaserver.read` | R | unsupported |
| `catalog.userData` | `mediaserver.read` | R | unsupported |
| `mediausers.list` | `mediaserver.users.read` | R | registered |
| `mediausers.get` | `mediaserver.users.read` | R | registered |
| `mediausers.devices` | `mediaserver.users.read` | R | registered |
| `mediausers.history` | `mediaserver.users.read` | R | registered |
| `mediausers.update` | `mediaserver.users.write` | D | registered |
| `mediausers.revokeDevice` | `mediaserver.users.write` | D | registered |
| `playback.sessions` | `mediaserver.read` | R | registered |
| `playback.summary` | `mediaserver.read` | R | registered |
| `playeraccess.summary` | `mediaserver.read` | R | registered |
| `mediaserver.sessions` | `mediaserver.read` | R | registered |
| `mediaprobe.status` | `mediaprobe.read` | R | registered |
| `mediaprobe.list` | `mediaprobe.read` | R | registered |
| `mediaprobe.retry` | `mediaprobe.retry` | W | registered |
| `mediaprobe.stills.retry` | `mediaprobe.retry` | W | registered |
| `mediaprobe.stills.gc` | `mediaprobe.retry` | D | registered |
| `metadata.nativeDetails` | `metadata.read` | R | registered |
| `metadata.refreshDetails` | `metadata.refresh` | W | registered |
| `integrations.list` | `integrations.read` | R | registered |
| `integrations.status` | `integrations.read` | R | registered |
| `integrations.test` | `integrations.read` | W | registered |
| `settings.get` | `settings.read` | R | registered |
| `settings.patch` | `settings.write` | D | registered |
| `cache.status` | `cache.read` | R | registered |
| `cache.list` | `cache.read` | R | registered |
| `cache.invalidate` | `cache.invalidate` | W | registered |
| `jobs.list` | `jobs.read` | R | registered |
| `jobs.get` | `jobs.read` | R | registered |
| `jobs.cancel` | `jobs.cancel` | D | registered |
| `system.info` | `system.read` | R | registered |
| `system.health` | `system.read` | R | registered |
| `system.modules` | `system.read` | R | registered |
| `maintenance.health` | `maintenance.read` | R | registered |
| `maintenance.alerts` | `maintenance.read` | R | registered |
| `notifications.publish` | `notifications.send` | W | registered |
| `notifications.deliveries` | `notifications.read` | R | registered |
| `notifications.retry` | `notifications.retry` | D | registered |
| `networkproxy.status` | `integrations.read` | R | registered |
| `network.http` | `network.proxy.use` | D | registered |

## 核心 Host RPC（11 项）

| RPC | scope | 级别 |
|---|---|---|
| `plugin.config.get` | `plugin.config.read` | R |
| `plugin.config.update` | `plugin.config.write` | W |
| `plugin.data.get` | `plugin.data.read` | R |
| `plugin.data.list` | `plugin.data.read` | R |
| `plugin.data.put` | `plugin.data.write` | W |
| `plugin.data.delete` | `plugin.data.write` | W |
| `plugin.data.path` | `fs.data` | R |
| `plugin.state.get` | `plugin.state.read` | R |
| `runtime.capabilities` | `plugin.state.read` | R |
| `system.summary` | `system.read` | R |
| `notifications.send` | `notifications.send` | W |

## 广播与链

SDK 提供51个 typed 标识（32广播/19链），其中 N/A 项不接受注册。

| 类型 | 稳定标识 | 精确 scope / 边界 |
|---|---|---|
| 链 | `agent.llm.provider` | **N/A：契约不接受注册** |
| 链 | `auth.intercept` | `events.chain.auth.intercept` |
| 链 | `auth.verification` | `events.chain.auth.verification` |
| 链 | `command.register` | `events.chain.command.register` |
| 链 | `discover.source` | `events.chain.discover.source` |
| 链 | `media.recognize.convert` | `events.chain.media.recognize.convert` |
| 链 | `name.recognize` | `events.chain.name.recognize` |
| 链 | `plugin.data.reset` | `events.chain.plugin.data.reset` |
| 链 | `recommend.source` | `events.chain.recommend.source` |
| 链 | `resource.download` | `events.chain.resource.download` |
| 链 | `resource.selection` | `events.chain.resource.selection` |
| 链 | `storage.operation` | `events.chain.storage.operation` |
| 链 | `subscribe.completion.check` | `events.chain.subscribe.completion.check` |
| 链 | `subscribe.episodes.refresh` | `events.chain.subscribe.episodes.refresh` |
| 链 | `transfer.intercept` | `events.chain.transfer.intercept` |
| 链 | `transfer.overwrite.check` | `events.chain.transfer.overwrite.check` |
| 链 | `transfer.rename` | `events.chain.transfer.rename` |
| 链 | `transfer.rename.build` | `events.chain.transfer.rename.build` |
| 链 | `workflow.execution` | `events.chain.workflow.execution` |
| 广播 | `agent.tokens.usage` | **N/A：契约不接受注册** |
| 广播 | `command.executed` | `events.subscribe.command.executed` |
| 广播 | `download.added` | `events.subscribe.download.added` |
| 广播 | `download.files.deleted` | `events.subscribe.download.files.deleted` |
| 广播 | `download.task.deleted` | `events.subscribe.download.task.deleted` |
| 广播 | `library.history.hidden` | `events.subscribe.library.history.hidden` |
| 广播 | `library.organize.failed` | `events.subscribe.library.organize.failed` |
| 广播 | `library.organized` | `events.subscribe.library.organized` |
| 广播 | `library.subtitles.copied` | `events.subscribe.library.subtitles.copied` |
| 广播 | `library.subtitles.failed` | `events.subscribe.library.subtitles.failed` |
| 广播 | `messaging.action.received` | `events.subscribe.messaging.action.received` |
| 广播 | `messaging.inbound` | `events.subscribe.messaging.inbound` |
| 广播 | `metadata.scraped` | `events.subscribe.metadata.scraped` |
| 广播 | `module.registry.changed` | `events.subscribe.module.registry.changed` |
| 广播 | `notification.queued` | `events.subscribe.notification.queued` |
| 广播 | `plugin.action.invoked` | `events.subscribe.plugin.action.invoked` |
| 广播 | `plugin.config.changed` | `events.subscribe.plugin.config.changed` |
| 广播 | `plugin.custom.triggered` | `events.subscribe.plugin.custom.triggered` |
| 广播 | `plugin.reloaded` | `events.subscribe.plugin.reloaded` |
| 广播 | `plugin.webhook.received` | `events.subscribe.plugin.webhook.received` |
| 广播 | `settings.changed` | `events.subscribe.settings.changed` |
| 广播 | `site.account.deleted` | `events.subscribe.site.account.deleted` |
| 广播 | `site.account.updated` | `events.subscribe.site.account.updated` |
| 广播 | `site.metrics.refreshed` | `events.subscribe.site.metrics.refreshed` |
| 广播 | `subscription.coverage.completed` | `events.subscribe.subscription.coverage.completed` |
| 广播 | `subscription.created` | `events.subscribe.subscription.created` |
| 广播 | `subscription.deleted` | `events.subscribe.subscription.deleted` |
| 广播 | `subscription.updated` | `events.subscribe.subscription.updated` |
| 广播 | `system.operation.failed` | `events.subscribe.system.operation.failed` |
| 广播 | `transfer.audio.complete` | **N/A：契约不接受注册** |
| 广播 | `transfer.audio.failed` | **N/A：契约不接受注册** |
| 广播 | `workflow.started` | `events.subscribe.workflow.started` |

## 调用与授权边界

- 业务 RPC 使用当前实例和授权范围；缺少权限、调用者身份或所需适配器时明确拒绝，不把接口存在当成业务可用。
- 配置、KV 和状态写入受修订号、实时授权及执行身份约束；撤权或保存配置后旧身份不能继续写入。
- 广播只接收已声明且授权的精确事件。排队、请求已受理和结果未知不等于完成。
- 策略链每个处理器最多200毫秒、整链2秒、最多8个处理器；输入输出经过校验，不能提升权限。
- 插件不能替换固定播放解析、取得115授权或临时媒体地址，也不能通过媒体端口访问插件管理 API。
- 管理 API 要求管理员会话与写请求来源检查；固定结果记录不收集配置、正文或操作密钥。

## 不支持的能力

- catalog.home/discover/nextUp/userData、downloads.retry、fs.media.readText 为固定 unsupported。其他目录能力还要求真实调用者和媒体适配器，不能采用插件传入的用户权限替代。
- CloudDrive2 缺少可靠修改时间时，不退化为仅依靠文件大小判断静默。
- 不支持私有 TMDB 收藏/OAuth、外部账号直接登录、LLM 对话或自动工具循环；AgentLLMProvider 为 N/A。
- 不开放任意网页正文、Cookie/认证头、共享 TMDB 或代理凭据、115 授权/直链和媒体中转。sites.request 只返回允许范围内的有界信息。
- 发现/推荐只读取插件候选的后台缓存，不是通用推荐引擎。收集结束不代表追剧完成，身份或资产未知时不发送完成事件。
- 工作流只执行管理员批准的有界定义，工具只由管理员人工调用；未知外部副作用不自动重放。

## 内置插件与运行范围

| ID | 版本 | 安全配置/使用范围 |
|---|---|---|
| `hello-go` | 1.9.0 | 人工调用示例；不自动运行业务 |
| `library-refresh-notify` | 1.2.0 | active=false；7日原事实重投/8日完成账本滚动 |
| `seeding-cleanup` | 1.0.0 | active=false / execute=false / deleteFiles=false |
| `site-checkin` | 1.0.0 | active=false；宿主持有凭据 |
| `torrent-assistant` | 1.0.0 | active=false / write=false / recheck=false；活跃监控即使不写也可能通知 |
| `watch-organize` | 1.1.0 | active=false；默认深度3/上限5，叶文件size+mtime静默，总项≤200/轮提交≤10 |
| `wishlist-subscribe` | 1.0.0 | active=false / create=false；公开来源，唯一匹配只创建停用订阅 |

安装或确认权限后仍保持停用，只有管理员明确启用才运行。JSON 配置可在停用时保存；动态表单和页面需要启用的实例，不会为了渲染自动启动。

不可用实例的配置与数据保留。插件修改通过固定模块与同源界面资源分发，不接受运行期来源、提交、版本选择或上传安装。具体操作见[插件说明](PLUGINS.md)。
