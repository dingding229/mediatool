# 插件支持矩阵 · R 源码冻结

- 源码候选：0.27.0；schema 65；contractVersion 2。
- SDK pin：`v0.0.0-20261019145526-4ad0d7c57697`。
- 契约摘要：`5fb1a7fcf7fbf87a232793570f5fda45616f1f36e2b6062f188cbd2012f3c90c`。
- 生成/校验：`go run ./cmd/media-tool-dev plugins matrix [--check]`；仅读取编译期元数据，不执行工厂。
- `registered` 只表示存在窄门面，不保证任意环境可用；actor、adapter、owner、selector、revision 和实时 scope 不满足时 fail-closed。
- 表中的 supported=false 为固定 unsupported；其他条件性 unsupported 见下文。

## 业务 Host RPC（157 项）

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
| `library.archive.list` | `library.archive.read` | R | registered |
| `library.archive.preview` | `library.archive.read` | R | registered |
| `library.scan.start` | `library.scan` | W | registered |
| `library.scan.control` | `library.scan` | W | registered |
| `library.organize.confirm` | `library.organize` | D | registered |
| `library.correct.apply` | `library.organize` | D | registered |
| `library.refresh` | `library.refresh` | W | registered |
| `library.history.hide` | `library.history.write` | W | registered |
| `library.archive.confirmCleanup` | `library.archive.delete` | D | registered |
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

原 MP 映射50项（含4项N/A）之外，宿主另有 `plugin.config.changed` 配置事实；所以当前SDK合计51个typed标识（32广播/19链），不是扩大MP能力声明。

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

## 拒绝矩阵与证据边界

- 每个业务 RPC 均经 `TestEveryBusinessMethodLiveScopeAndRevocation`（host_business_test）逐项走成功代理、缺 scope、撤权、unsupported；真实门面的成功/投影/保护另由 plugins/business 单测与集成测试证明，不能把代理成功算真实业务成功。
- 核心配置/KV/状态：concurrency_test、host_test、v2_test 覆盖 KEY SHARE 并发读取、续租、撤权和提交前写 fence；scope 从不缓存。
- 广播：acquisition/organizing/discovery/platform/workflow 对应 map/integration/真实 worker 测试覆盖 commit/rollback、精确 scope、撤权；版本化事实键，不把 requested/unknown 当成功。
- 链：生产每 handler 200ms、整链 2s、最多 8；SDK schema 和宿主再次校验，非法结果只收窄或回落，RPC 不跨写事务，提交前 fence。
- 严格 302：server_test、server_integration_test、media_integration_test 与 worker 接线用例保护 STRM 原文、合法 actor 和无播放/115 URL 投影；插件不得替换 Resolve。
- 管理 API：admin 会话/非 GET CSRF；媒体端口无插件 API。实例 key/public 网关另有精确权限与限流。失败记录只保留固定结果码/requestId，不记正文、配置、operationKey 或原始错误。

## 明确 unsupported / N/A（不因 R 扩范围）

- catalog.home/discover/nextUp/userData 为固定 unsupported；catalog/nativelibrary 等依赖宿主真实 actor，不能采用插件传入的 userId/visibleLibraries 替代。缺失 actor/媒体适配器时其他相关方法也返回 unsupported。
- downloads.retry、fs.media.readText 固定 unsupported。CloudDrive2 没有可靠 mtime 时不退化为仅 size 静默检测。
- 私有 TMDB 收藏/OAuth、真实 auth provider 登录适配器、LLM 对话/自动 Agent 循环不支持；AgentLLMProvider N/A。假 provider 测试不是生产登录能力。
- 不开放任意正文提取、Cookie/Set-Cookie/认证头、TMDB token、代理凭据、115 授权/直链、原始播放 API 或媒体中转。sites.request 只有有界脱敏布尔标记，不返回正文。
- 没有推荐引擎；discover/recommend 只接插件 typed 候选的后台缓存。收集结束不是追剧完成；unknown/缺 TMDB/缺资产不发完成事件。
- workflows 仅管理员批准的有界 DAG，工具仅人工调用；未知外部副作用不自动重放，不恢复旧代码。

## 内置插件与停用态安全验收

| ID | 版本 | 当前版本 history（来自 manifest） | 安全配置/当前范围 |
|---|---|---|---|
| `hello-go` | 1.9.0 | Bounded manual agent tools (no autonomous loop), readonly echo and own-KV write examples | 人工框架检查；不自动运行验收业务 |
| `library-refresh-notify` | 1.1.0 | F6-β：宿主强制七日重投窗口；仅淘汰完成八日且已过重投窗口的记录 | active=false；7日原事实重投/8日完成账本滚动 |
| `seeding-cleanup` | 1.0.0 | F6-β：默认停用；有界队列、持久收据、宿主实时授权与危险操作确认 | active=false / execute=false / deleteFiles=false |
| `site-checkin` | 1.0.0 | F6-α：有界、默认不执行业务、Host 实时授权与幂等收据 | active=false；宿主持有凭据 |
| `torrent-assistant` | 1.0.0 | F6-γ：默认停用，合成来源验收，有界输入/队列，危险写操作需管理员确认 | active=false / write=false / recheck=false；活跃监控即使不写也可能通知 |
| `watch-organize` | 1.1.0 | F6-γ：默认 3 / 最大 5 层、有界递归、叶文件静默和宿主链接证据保护 | active=false；默认深度3/上限5，叶文件size+mtime静默，总项≤200/轮提交≤10 |
| `wishlist-subscribe` | 1.0.0 | F6-γ：默认停用，合成来源验收，有界输入/队列，危险写操作需管理员确认 | active=false / create=false；公开来源，唯一匹配只创建停用订阅 |

安装/确认权限后仍停用；JSON 配置可在停用时保存。动态表单/状态页需要显式启用的 worker，不能为渲染自动启动。离线静态 schema 表单仅作后续设计建议，不在 R 实施。
安全模式浏览器验收只证明安装/权限/配置读回/停用隔离；六插件活跃业务分支由合成站点/downloader/TMDB/豆瓣/115 Transport/小文件的真实同 app worker E2E 证明，不能宣称在生产真实账户上签到/删种/整理成功。
保留的 absent 实例和测试快照由用户决定；R 不自动清理。新插件先在插件仓库测试推送并过 CI，再更新主程序精确 pin 和同源 UI 清单，最后经另行批准的备份部署。
