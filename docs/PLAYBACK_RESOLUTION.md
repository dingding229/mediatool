# 固定 STRM 的冷取链预算、连接复用与阶段诊断

日期：2026-10-11（北京时间）。resolve-budget已发布到指定项目；数据库保持迁移20。准确部署和验收证据见data/remote-deploy/resolve-budget-*报告。

## 本轮证据与范围

上阶段真实访问日志中，固定STRM入口出现约10002–10010ms后返回502；稍后同资产/UA又能302到115 CDN并读取206。该现象证明取链仍有间歇失败，**不能仅凭耗时认定115限流、令牌刷新、DNS或具体API是根因**。

源码审查确认两处独立限制：

1. 播放器Engine的总取链预算为10秒，缓存singleflight源任务预算为12秒；官方路径核对和取下载地址是两个串行API，每次HTTP调用自身可等12秒，总预算可能先于正常慢调用截断。
2. 默认115 Client每次API调用新建Transport，路径查询/取链/刷新无法复用连接，且短期内创建多个空闲连接池。连接池缺陷已经通过隔离TLS服务的实际socket计数与请求凭据隔离测试修正，**不据此宣称它就是上一轮全部超时的根因**。

## 当前策略

- `PLAYBACK_RESOLVE_TIMEOUT=30s`为默认总取链预算，配置范围5秒到1分钟。Engine和缓存singleflight源任务使用同一默认/配置值，不再隐藏10/12秒夹层；更早的播放器取消/父请求期限仍优先。
- 官方每次API请求保留12秒HTTP上限、10秒响应头/TLS握手上限；新增5秒拨号上限。默认Transport跨调用复用，连接/空闲连接数有界，无环境代理、无共享Cookie Jar。Bearer与实际播放器UA仍逐次写入独立请求，115来源固定HTTPS、API重定向不跟随。
- 路径/文件大小/pickCode/取链文件身份核对仍执行；不为了缩短延迟跳过校验，不预绑定元数据请求的UA，不把临时CDN地址写入STRM。
- 缓存保留原有短期有效期、完整来源修订/UA/IP分区、单flight与四个源任务槽位。排队计入源任务总预算；一个取消的等待者不取消其他等待者共享的有限任务，已经取消的请求不能新启后台源请求。不存在无限重试。
- 令牌刷新仍受数据库跨进程锁、一次过期重试、冷却和结果不确定时停止重用规则约束。本轮不重新授权、不更换账号/应用ID、不借用其他项目Cookie。
- 总取链超时/网络超时明确返回504、`cloud_resolution_timeout`，无Location、不回退媒体中转；用户取消使用net/http的中断语义，不向断开的客户端追加误导错误正文。其他提供方失败继续按权限/文件/安全策略分类。
- 管理监听默认WriteTimeout为30秒；固定STRM入口仅为本次响应设置**有限**`总取链预算+17秒`写期限，容纳5秒目标检查、10秒CDN采样和2秒输出余量，不清除其写期限。诊断Find与Inspect使用独立阶段预算，响应写期限有限为总取链预算+22秒；不让诊断提前15秒夹断真实播放预算。
- HLS/转码/中转拒绝策略不变。CDN验证仍至多1KiB，失败不能变成relay；本轮30秒只调整取链，并非将所有网络/客户端超时统一放宽。

现有部署未声明新环境变量时自动采用30秒默认，不需要修改既有.env/密钥或live compose。使用仓库最新Docker compose时可显式设置新变量；自定义compose必须声明透传该变量才能覆盖默认值。

## 可诊断阶段

日志消息为`Playback resolution stage`，与HTTP访问日志共用程序生成的requestId，只包含固定stage/outcome和耗时/数字提供方代码。缓存源任务完成时间可能晚于取消等待者的HTTP响应；仍属于同一有限任务，不应误认为启动了另一媒体传输。

阶段包括：

- cache_version、cache_lookup、cache_hit/cache_miss、cache_joined、cache_wait
- source_queue、source_resolve、cloud_file_lookup、cloud_profile_lookup
- official_token、official_refresh
- official_http_path/official_path_lookup
- official_http_download/official_download
- file_identity_check、download_identity_check
- resolve、target_policy、cdn_check

outcome为ok、timeout、cancelled、rate_limited、authorization、forbidden、invalid_response、unavailable、provider_rejected或failed。HTTP阶段和业务阶段分别保留，因此能区分令牌锁等待、API等待、身份核对和CDN验证；未知错误只记录failed，不打印error.Error()。

禁止记录STRM查询参数、文件路径、UA/IP正文、Bearer、Token、Cookie、临时URL、提供方message和请求body。providerCode/HTTPStatus只取115 APIError的数字字段；requestId不使用客户端输入。系统状态/preflight返回有效playbackResolveTimeoutMs，不返回凭据。

## 验收边界

本地隔离测试包含：真实时间10.2秒成功取链（超出旧10秒）、短预算明确504无中转、客户端取消、共享等待者取消隔离、队列超时不执行源请求、已取消请求不启动后台、令牌/路径/API限流阶段、敏感输入/错误日志过滤、TLS连接复用且Bearer/UA不串用，以及管理WriteTimeout较短时有限固定入口仍成功。

真实部署验收应继续确认：

1. 原条目11/17的媒体详情与PlaybackInfo逐字等于磁盘STRM，未提供Videos/.../stream替代。
2. 不带EmbyToken、按实际UA请求原固定链接，302→cdnfhnfile.115cdn.net→206；相同UA/IP立即复读观察缓存，改变UA观察独立冷取链。
3. 实际日志出现上述冷/热阶段且没有敏感字段；原HLS/转码接口仍拒绝。
4. .env/账号/主密钥/媒体/现有输出/授权及Emby、CloudDrive2、其他项目容器身份保持。没有数据库迁移或自动归档清理。

成功字节链、增加预算和连接复用均不能代替真实设备解码、长seek、非管理员矩阵或持续压力下稳定性验收，也不能据此标记整个PT下载/订阅/通知项目完成。


## 实际发布与本轮有限验证

2026-10-11（北京时间）已发布media-tool:resolve-budget-20261011，只重建本项目server及三个Worker，无新迁移。真实条目11/17分别按原生/浏览器UA执行首次与立即重复请求，八组均302→cdnfhnfile.115cdn.net→206；额外复核条目11的两次请求也均302，立即重复命中缓存。全部直接使用原STRM链接、没有EmbyToken。

首个请求正常完成一次官方Token刷新（约882ms），因此连接revision变化后下一次请求重新核对/取链，而不是使用旧修订分区的缓存；这是预期失效，临时CDN地址不要求永久相等。刷新后额外复读命中缓存，并验证当前文件Key、EmbyToken、选中网盘路径和临时URL均未出现在本程序最近日志中。没有重新扫码授权，收到的新Token对按既有规则加密持久化，未借用其他项目身份。

冷取链真实日志显示缓存版本/队列、Token/刷新、官方路径、下载地址、身份核对、目标和CDN检查的固定阶段；本轮八组成功HTTP耗时46–4736ms，不外推长期稳定性。先前约10秒502的历史记录保留，没有声称确定其所有上游原因。10.2秒慢取链成功及504/取消/连接复用是隔离和原生amd64测试证据，并非制造了真实115故障后宣称修复。

原5个输出文件、资产/绑定/播放授权/扫描计划、管理员密码记录、.env/主密钥/live compose以及Emby/CD2和其他23容器身份均由发布前后摘要核对保持。HLS/转码/错误容器请求仍明确拒绝。真实设备解码/长seek/普通用户矩阵、HTTPS及用户自己的PT/qBittorrent/外部通知链路仍未全部验收。

本次发布后的文档状态更新使用独立docs-supplement证明，保留最初的源码包/镜像/发布manifest，不替换历史构建证据。当前源码由原manifest与文档补充SHA共同核对；运行代码未做第二次修改或重启。
