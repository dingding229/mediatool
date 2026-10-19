# F6 六插件验收清单与安全模式边界

F6 已完成部署和浏览器安全模式/SSH复核；R-α 不部署、不连接生产。本清单不是执行真实站点、种子或媒体写入的授权。
六插件的活跃业务分支由合成 TMDB/豆瓣/qBittorrent/目录/115 Transport 的真实 app worker E2E 验证。
生产 Chrome 验收默认用下表安全模式；如需要完整活跃分支，须另行指定合成 Host 输入，
或由用户明确授权对应业务动作。**active=false 的 service succeeded 只证明调用/空操作，不能写成业务成功。**

## 通用步骤

1. 管理端内置目录中选择对应插件；只展示当前镜像内定义，不选择来源/commit/版本。
2. 用户亲自确认危险 scope，安装/确认后仍停用；检查默认配置全部关闭业务。
3. 配置 → 用户亲自确认启用 → 查看状态页、运行记录、按实例日志；完成后停用。
4. 记录完整 instance ID、revision、验收时间窗。SSH 仅按实例核对，不读取/回显密钥。
5. 停用后核对 desired=disabled、runtime=stopped，worker_pid=0，generation/owner/lease 为空；
   无进程组/子孙残留，网关拒绝，无 ERROR/panic。不可把 pending/requested 当已送达/已完成。
6. 保持 STRM 原文和严格 302；既有生产快照由用户决定，不在验收中代删。

| 插件 | Chrome 配置及操作 / 预期 | 用户需亲自确认的危险点 | SSH 只读核对 |
|---|---|---|---|
| site-checkin（站点自动签到） | 生产先保持 active=false，检查 accountId/origin/path/successMarker/cron 表单；手动 checkin 可验证空操作。完整合成验收：配置合成站点，成功只记宿主布尔 marker；验证码/失败进入 needs_action。 | sites.request、启用 active；真实站点签到未授权则跳过，不用真实账户填测试。 | service/event 终态、status/UTC 日 checkpoint、通知 outbox pending；无 Cookie、响应凭据进入 KV/日志；停用收割。 |
| seeding-cleanup（自动做种清理） | active=true、execute=false、deleteFiles=false，仅在已有安全合成下载任务时手动 cleanup，状态页显示 preview 清单；没有合成输入则不激活、不把空清单算筛选成功。 | downloads.delete；execute 和 deleteFiles 是独立危险开关，生产浏览器均保持 false。 | 无 downloader 删除请求/文件变化；合成完整测试核对 retired 或 pending 证据、HR/owned/共享/在途保护、重复 key 不重放。 |
| library-refresh-notify（媒体库刷新与入库通知） | 生产 active=false，检查 provider/intervalSeconds 和状态页；不人为制造真实入库。合成输入：提交 TransferComplete，重复事件只通知一次、同资产合并刷新。 | mediaserver.refresh；启用 active 后接受真实事件可能发通知/刷新，须明确批准后才做。 | 原事实时间、通知 pending、刷新 requested/queued；七日重投/八日已完成淘汰，pending 不丢；无 STRM 内容变化。 |
| watch-organize（目录监控实时整理 1.1.0） | 查看 rootId/directory/destination/quietSeconds/depth（默认3，最大5）；仅获准的合成根可 active=true。准备“片名/视频”及“剧名/Season/剧集”，叶文件稳定后只生成一次 core organize 计划。深度之外不扫；链接不进入。生产媒体根不得作测试输入。 | fs.media:write、active；所有真正移动必须用户明确授权。 | root-relative 身份/size/mtime、计划 digest/revision、成功读回、单轮≤10、总条目≤200；符号/硬链接未处理；CloudDrive2 mtime 不明为 unsupported；STRM 不变。 |
| wishlist-subscribe（TMDB/豆瓣想看订阅） | source 选 tmdb-list/douban-wish/douban-subject，只填 ID；accountIds 为宿主已有测试 PT 账户 UUID。默认 create=false。合成列表检查候选/退避；开启 create 后唯一匹配只创建**停用订阅**。歧义项从状态页复制 sourceId/candidates，resolve 动作 JSON 填 sourceId/tmdbId/mediaType，只能选已有候选。生产默认不拉真实外部页面。 | subscriptions.write、resolve、create；元数据公开读取与创建订阅分开核验；不启用订阅、不搜索、不下载。私有收藏/OAuth unsupported。 | 来源请求间隔/退避、pending≤200；Host 凭据未下发；并发/重复 identity 只有一条订阅、已入库跳过、无 subscription.run/download jobs；不把 ensured 计为“新增”。 |
| torrent-assistant（下载器助手） | active=true、write=false、recheck=false 的合成任务预览；查看 page/dashboard、downloader-status 命令、GET admin /status。合成任务可经明确配置 write=true 验证标签、分类、限速/做种限制；默认错误/卡住只通知。assist 动作遵守当前配置。生产无合成任务则 active=false。 | downloads.control、assist、write、recheck；recheck 是重新校验而非重启进程；绝不在生产种子上试写。 | owner 标签仍在；仅本实例 plugin: 标记；auto_tmm/HR/共享/在途/其他规则均受保护；分类读回与冻结一致，未知意图保留；一事件一次通知/校验，gateway 未登录拒绝，停用后拒绝。 |

## 完整功能证据与拒绝矩阵

- SDK：每个新 RPC 的成功、缺 scope、撤权、unsupported；真实 Go stdio golden。
- Host：fixed-origin/no-cookie、正文上限、解析不确定/限流退避、subscriptions.ensure 并发/库内去重；
  downloads.configure 每个远端写前 fence，读回失败/部分成功不重放，HR/auto_tmm/保护标记拒绝。
- 真 worker：两公开来源→唯一/人工选择→停用订阅、真实管理 HTTP status、合成 downloader 注释写、
  重复操作收据、撤权拒绝与停用收割；目录三层叶文件与 STRM 不变性。
- 生产全流程需 F6 收尾另行下发：备份→不可变 Hub digest→健康/schema/保护表/302→用户 Chrome→SSH 复核。

## 已完成的生产安全模式与 R 边界

2026-10-19：六插件各保留一个 disabled/stopped 实例，JSON安全配置读回一致、修订2；hello-go停用修订21。
六插件没有启用、没有运行/Host业务调用/业务任务；6条 plugin.config.changed 是配置保存事实，不是通知。
额外的已卸载测试实例和既有测试快照由用户决定，R不自动清理。
这不证明六插件活跃业务在真实生产账户上执行过；活跃分支证据仍来自合成输入的真实worker测试。
停用时动态表单/状态页不运行；可直接保存JSON。R仅改善说明，不实现离线静态schema或隐式启用。
