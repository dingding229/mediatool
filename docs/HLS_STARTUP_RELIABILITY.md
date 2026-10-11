# Emby Web HLS 冷启动与取消请求

更新：2026-10-10。此文只记录已经观察/验证的范围，不替代完整客户端矩阵。

## 已定位的时间预算冲突

- 用户Emby版本4.10.1.0，当前实际网页HLS模块为1.6.0-beta.2；其fragLoadPolicy.default的maxTimeToFirstByteMs为10,000ms，总片段加载预算120,000ms。
- basehtmlplayer在第一次播放的非致命NETWORK_ERROR也会reject当前play promise并销毁HLS实例，因此不能假设库内默认重试一定能救回第一次超时。
- 独立诊断device/session、已登录用户可见的item11/source mediasource_11：master/main均200；首TS在10秒客户端期限内超时，随后请求同一URI约0.7秒后200/有效TS同步字节。当时未核验管理员角色或进程连续性，不能据此宣称普通用户权限、同一编码进程通过；见下方纠偏。
- 此前应用把取消后ErrorHandler的写入尝试也记成502，不能据此把所有502认作客户端收到的上游故障。

## 修复边界

1. 反代错误只输出固定分类：request_cancelled/upstream_timeout/upstream_closed/upstream_unavailable。输入请求context已取消时交由net/http中止，不再生成虚假的502错误体；仅上游返回取消且输入仍活跃不能归咎用户。
2. 管理媒体网关只对已核验的basehtmlplayer.js SHA-256 `8c61400307b6a13a668731bbc086b5673538f9aaee261e7b8d2bb49403a6e531`改写HLS选项。原始Emby文件不改，未知版本/压缩/非JS/非200/不唯一匹配原样透传。
3. 首字节预算60秒；用运行时已有fragLoadPolicy克隆，仅改maxTimeToFirstByteMs，保持总加载预算、重试策略、编解码能力、授权和许可证检查不变。不存在该配置的旧HLS模块保留原选项。
4. 仅GET/HEAD、规范的HLS片段路径使用75秒上游响应头预算；普通API/master/原始流仍30秒。客户端取消随时生效，无无限等待。传输池跨Gateway工厂共享，避免每片段新建连接池。
5. 已修改脚本去原ETag/Last-Modified并no-store；条件重载获取完整原始脚本再校验。已经在浏览器内存/长期缓存的旧模块仍需硬刷新或新的浏览器会话，不能说部署即代表旧页面已使用新规则。
6. TLS/DNS/Emby user/item/source权限、官方direct-only、必要HLS/转码保留，不为了302伪造浏览器能力。

## 诊断工具与防止假验收

scripts/remote-hls-check.py仅运行在授权项目主机，以只读认证库建立会话关联，并复查账号policy与scoped item/source。现在默认要求IsAdministrator=false、IsDisabled=false、EnableMediaPlayback=true；管理员诊断必须显式--allow-admin-session，报告userRole=admin且普通用户门槛为false。Token仅内存，不导出、不写报告。每轮随机独立诊断device/PlaySession，只停止自己创建的编码，不停止实际Chrome或Sen用户。

master/main/fragment读取有上限，只取1KiB TS样本，记录状态/耗时/同步字节，不宣称真实播放器解码。StartTimeTicks并不保证服务端马上seek：正确诊断应从虚拟playlist的EXTINF时间轴选择覆盖指定位置的片段，而不是总请求第一条0.ts。

此前名为baseline-510的报告仍选择0.ts，FFmpeg没有-ss证据，**不能作为8分30秒续播通过**。已修正segment_at解析/选择并单独生成selected报告；还需核验服务端实际seek与真实正常设备播放。原报告保留，不能掩盖无效测试。

## 回归

Go覆盖取消不伪502、固定分类/无原错误URL泄漏、hash/唯一匹配/不重复patch、未知脚本字节与缓存原样保持、已知改写长度/缓存正确、条件请求和仅媒体预算分流。Python/Node执行本程序自己的选项表达式，验证60秒首字节、120秒总预算/重试保持、全局默认对象不被修改及旧模块fallback；不执行整个第三方JS。

仍待：实际正常设备网页首播、8分30秒多次冷恢复、拖动/音轨/字幕与完整客户端矩阵；PT/qBittorrent/通知全链路。不能把TS样本或预算修改作为整体首轮完成。

## 2026-10-10当前实际发布与验收边界

- 已发布MIME修正后的脚本（Emby实为application/x-javascript），公开GET确认X-Media-Browser-Policy=hls-first-byte-60s、no-store、无旧ETag、正文22,264字节且实际包含60秒规则。第一版因严格MIME未改写，没有把那一版声称为上线通过。
- 输入10秒断开的独立诊断，应用现记录HTTP stream aborted/request_cancelled而非虚假502；同一片段随后约0.938秒200，诊断只停止自己device/session且204成功。
- 未取消、45秒预算的从头首TS约10.573秒后200/有效同步字节。此限量字节证据不是全部网页通过。
- 正确按playlist时间轴510秒选择85.ts时上游500；FFmpeg出现实际-ss 00:08:30.000/无帧以及部分output header/No such file错误。结束后目录被清理不是首失败的充分解释。冷seek问题仍开放。
- 已有Chrome页面即使调用reload/尝试硬刷新，本次服务端未观察到它重新请求basehtmlplayer（已验证的22,264字节请求来自独立公开检查）。页面仍可能沿用旧模块并出现10秒networkError；之后DOM有缓冲/时间，但不伪称新策略已在该页生效或锁屏黑图是正常画面。正常设备/新浏览器会话加载验证仍需继续。

全量Go race/vet、12项E2E、9项Python/Node/路由隔离回归通过，amd64镜像构建；PG仍17、3Worker/2资产/1官方授权保持、其他23容器未操作。发布保留备份/回退，未修改原Emby文件、账号密码或云授权。


## 本轮新增证据与验收纠偏（2026-10-10）

- **诊断角色**：原工具只检查EnableMediaPlayback，没有检查IsAdministrator；本轮只读核验实际选中的是管理员的登录会话。它不是后台服务API Key，但也不是非管理员权限证据。原报告保留，不再将其中ordinaryUserScopedSourceVerified=true作为可信验收项；官方302历史报告的ordinary*字段指普通测试UA，不代表非管理员账号。
- **诊断守卫**：默认未选到合格的活动非管理员会话，不降级选管理员，不代建账号。身份查找总预算30秒；禁用/缺字段/非布尔/身份不匹配均拒绝。HTTP错误响应及时关闭。
- **对照与重试**：新增--delivery upstream，限定127.0.0.1:8096的根路径，和网关127.0.0.1:25015对照；不接受任意URL，不是线上播放回退。只重试客户端中断，不立即重放上游500；sameEncodingOnRetryProven固定false，因为相同URI/device/session不证明服务端没有重建进程。
- **原生302复核**：明确标注管理员会话的SenPlayer/6.2.1 UA；original与Static均302/Direct-Verified，目标cdnfhnfile.115cdn.net，CDN均206/1KiB，错误Token401且无Location。realPlayerDecoded=false；没有把这个样本当作实际Sen应用解码或非管理员验收。
- **隔离输入检查**：指定-ss 510秒、noaccurate_seek、视频流复制/null输出，独立FFmpeg约15.402秒退出0并取得媒体包。没有动Emby输出目录，不代表精确帧定位或播放器解码，也不说明全部网络冷启动都快于Emby等待时间。
- **续播仍不稳定**：无网关的直接Emby对照曾约16.089秒200/正确85.ts同步字节，随后独立直接Emby对照又约15.108秒500；以最终工具根路径（和代理实际剥离挂载前缀后的路径一致）再对照约15.248秒200/1KiB。前两次直接对照使用Emby的/emby别名，报告保留，不混作完全相同wire路径。网关单次约15.130秒500。因此既不能归咎网关独有，也不能断言Emby存在不可越过的15秒固定超时，更不能把一次200标成冷恢复已解决。
- **生命周期观察**：限定新日志、相同选定输入和510秒seek，观察输出目录/playlist/片段状态。部分日志到进程结束才可见；记录不是完整进程生命周期证据。不同实验/立即重试可能涉及多个编码候选，报告ambiguity，不用最终清理后的目录不存在反推首个错误。只停止自己device/session，204已确认。

脱敏证据在data/remote-deploy/official115-current-role-aware-check.json、hls-default-role-guard-check.json、hls-seek-lifecycle-510.json、ffmpeg-null-seek-510.json、hls-seek-upstream-control-510.json、hls-seek-lifecycle-upstream-510.json、hls-seek-gateway-role-aware-510.json、hls-seek-upstream-root-control-510.json。旧报告未覆盖。当前脚本回归15项Python/Node测试通过；本轮未修改Go运行程序、数据库迁移或播放策略，不能宣称本轮重发镜像或重新做完整Go/E2E。

下一步：定位Emby首次seek失败时的具体异常/输入读取/编码等待时序，以新会话核验网页实际加载已发布的60秒策略，再在正常设备做首播、冷续播、音轨与字幕。非管理员设备权限与PT业务全链路仍是独立门槛。


## 2026-10-10：启动缓存命名空间修复与实际 Chrome 验证

### 已发布的缓存修复

旧页面的apploader/模块缓存键仍只包含Emby版本，不能从一次curl成功推出旧浏览器已加载60秒策略。本轮核验当前公开index/apploader的精确SHA，增加独立`mt_hls_startup=1`缓存键：index只改apploader脚本URL，loader只改模块urlCacheParam，不改data-appversion/dashboardVersion、登录、能力、许可证或原Emby文件。三个资产必须200/正确MIME/无压缩、完整SHA与唯一锚点匹配；未知版本原样透传。已改写内容no-store/正确Content-Length，移除旧验证和完整性头；有条件请求先取得完整原始字节以核验。

实际公开GET已确认：index14956字节/SHA95110fd993c760b2bf284a20ffba3094fd5b02e7f5cb2258860b340433a25c1c；apploader3100字节/SHA62125cf6da465f9313e3dbfe8e071af42449de90a0cdb083da3cefb21ffb4548；basehtmlplayer22264字节/SHA67e638be1abb6ca4440a7069733a1dbbf1ca806479c47a933ab3e819016fd425。真实Chrome的DOM脚本src含该缓存键、仍v4.10.1.0，服务器记录相应200/正确字节数；这次是实际网页证据，不是只curl。

另外，当前实际HLS库1.6.0-beta.2在离线Node VM中构造Hls实例，验证选中首字节60000ms、总加载120000ms、原全局10000ms默认不变。最初AMD exports工厂包装不完整导致离线检查失败，补真实exports模型后通过。此检查不调用loadSource，不访问账号或网络；它不同于此前只执行自有选项表达式，也不能替代真实播放。

### 首次失败时序与有限客户端证据

独立device/session的510秒首次请求（无立即重试）约15.124秒500，error body1300字节；ffmpeg候选日志当时没有Input #0/输出/片段，约16.792秒目录消失。只在约5秒观察后DELETE自己的编码、204确认。日志可能延迟写入，不能单凭此推出具体等待上限或目录就是初始根因。当前上游异常形状为FfRunException/Error starting ffmpeg，调用位于StartFfMpeg/GetDynamicSegment，不是网关自造502。未更改Emby编码设置或删除用户媒体。

SSH曾在握手/传输中断；没有凭marker当进程活着、没有盲目重建未知编码。可追踪runner的既有PID/终态receipt读回证明已结束，并保存脱敏时序。首版诊断最终Path拼接错误修正后，早先未确认结果不用于验收。

Chrome：新缓存键实际加载后，初次仍出现network与AbortError、首TS中断/404；后来DOM有缓冲。通过真实时间轴控件（步长0.012，6.35拒绝，6.348有效）跳到约510秒，可取得510–581秒缓冲、currentTime到511.246，并保存实际影片画面截图`data/remote-deploy/hls-bootstrap-browser-osd.png`。这证明一次真实出画面，**不是反复冷恢复/持续播放全部通过**。结束后点击恢复播放能取得512秒附近缓冲，但仍有暂停/重新加载/黑屏，最新DOM曾回到0/readyState0；保留两份截图和chronology，不用较早帧掩盖后续失败。每次仅通过自己标签页的Escape结束，未停止Sen或别人的编码。

### 发布与继续门槛

全量隔离Go/race/vet、13项E2E、17项Python/Node通过；最终amd64实际镜像以非root/只读/无网络在服务器通过193配置静态审计后才发布。PG仍17、3Worker/2资产/1官方授权、其他23容器摘要一致。备份/回退为`private/database-before-hls-bootstrap-20261010.sql.gz`、`media-tool:before-hls-bootstrap-20261010`与`private/source-before-hls-bootstrap-20261010`。原生Sen UA的original/Static仍302→cdnfhnfile.115cdn.net→206/1KiB，错误Token401无Location，管理员会话字节证据非实际Sen解码。

下一优先项：核验basehtmlplayer首次非fatal网络事件即reject当前promise、HLS自身有限重试与编码初次读取慢之间的影响；如修改，必须保持fatal/取消终止、明确启动总预算，不做无限重试或强制直放。继续实际输入读取/seek失败根因、网页持续播放/冷恢复/音轨字幕与非管理员客户端；PT/qBittorrent/通知全链路门槛不删。

证据：`hls-bootstrap-{public-verification,deployment,image-check,first-browser-requests,browser-request-evidence,final-state,native115-check}.json`，`hls-observer-state-readback.json`，`hls-server-error-shape.json`，`hls-actual-library-policy.json`与浏览器截图。所有签名URL/Token不进入输出；官方直链验证只读至多1KiB，网页HLS仍经过必要的Emby转码链路，不承诺零服务器流量。


## 2026-10-10：V2 有限启动等待，首播连续播放通过、seek仍失败

### 修复边界

当前已核验4.10.1.0的绑定回调在reject仍有效时，将首次非fatal网络错误也销毁播放器；这会打断HLS已经选择的有限重试。V2插入自有启动guard，**仅**在启动尚未成功、signal未取消、Hls版本1.6.0-beta.2、明确nonfatal网络加载错误、HLS errorAction.action=5（RetryRequest）、有效剩余retryCount/maxNumRetry时，交给HLS原重试继续。只允许0/408/500/502/503/504等瞬时响应；400/401/403/404/409/429、致命错误、缺失/耗尽retry配置、错误类别/未知SDK不延期。

不调用startLoad/loadSource、不增加重试、不替换播放session、不修改HLS的重试次数/延迟、用户权限、许可或格式能力。启动总体上限90秒；成功/失败/abort后清timer/事件；destroy采用microtask检查，保证vendor先destroy再reject的原错误分类不被误改成AbortError。用户取消/外部销毁会及时结束；已播放状态保持原vendor错误语义，不将所有运行错误都延迟。

仍严格hash/三个唯一锚点全部核验后才原子改写，未知版本/压缩/MIME/状态不改。模块命名空间更新revision2，Emby真实版本仍4.10.1.0；basehtmlplayer26371字节/SHA8c24634eb19b8d89cbce89fff74dd4ba782b284e2f2cb80428029a0d022e50e5，index14956与apploader3100实测策略/sha/no-store正确。

### 验证

35项自建状态场景与**当前原始回调函数**离线执行均通过：瞬时错误、有/无重试、switch/do-nothing动作、权限拒绝、致命/media错误、90秒deadline、已取消/中途取消、外部destroy、成功后错误、autoplay拒绝和清timer/listener。Go验证三个锚点缺失/重复不能局部改写、headers与长度、未知版本不碰；全量Go/race/vet、18项Python/Node和13项E2E通过。

实际HLS库使用自建XHR（不出网络、不访问媒体/账号）先503再200可沿原XHR重试取得manifest；这一路不发公共ERROR，**不把它当guard救回fragment**。另用真实ErrorController/已解析level验证fragment错误动作5、retryCount0/max6，和guard字段匹配。初次离线包装缺window.status、误读hls.errorController（实际位于networkControllers）失败后核对源再修正，不改真实库代码。

### 真实客户端与剩余问题

新Chrome标签页最初三个资源加载失败（approuter/gesture/basehtmlplayer），server记录HTTP200/正确字节，未从它推定代码语法成功或新增自动页重试。按正常用户入口/web/index.html后主页成功；不能单凭该顺序认定测试参数或网络就是根因。

真实页面模块src含revision2与原Emby版本。首播记录guard等待于07:30:16.895Z、07:30:27.193Z，对应首0.ts先15128ms500、后9016ms500；随后得到实际影片画面，DOMcurrentTime35.298→71.541→158.785、readyState4/pausedfalse/无media error；证明一次两分多钟持续播放与启动瞬时恢复，而不是只缓存样本。UI暂停有效，显示播放158.785秒；原生索引/隐藏按钮失败后用可见video区域恢复，没有通过evaluate调用play或强改状态。

**seek仍失败**：真实时间轴6.348请求510.046秒，之后无法播放。退出后新的恢复启动可看到四次guard等待，但约90秒后错误/ready0/缓冲空并停止；日志170.ts多次约15秒500，deadline后连接中止/404。没有将这个失败隐藏或无限继续。当前guard只解决启动时过早结束，不改变已播放seek错误处理，上游FFmpeg seek/读取/启动仍需继续定位。

整体播放、非管理员、音轨/字幕、重复冷恢复与实际Sen矩阵仍未完成；真实PT→qBittorrent→CD2→输出→Emby→通知和订阅门槛保持。V2不意味着网页所有播放都零服务器流量，必要HLS/转码仍经Emby。

### 发布

实际非root/只读/无网络amd64镜像审计通过后才发布server/3Worker；schema仍17，原账号/主密钥/授权/媒体未改，其他23容器不变。原生Sen UA的original/Static仍302→cdnfhnfile.115cdn.net→206/1KiB，坏Token401无Location（管理员会话字节证据，非Sen实际解码）。回退media-tool:before-hls-startup-guard-20261010、private/database-before-hls-startup-guard-20261010.sql.gz、private/source-before-hls-startup-guard-20261010。

证据：data/diagnostics/hls-startup-guard-{original-callback,actual-library,expected-assets}.json，data/remote-deploy/hls-startup-guard-{deployment,image-check,public-assets,client-chronology,playback-requests,final-state,native115-check}.json及首次真实画面/冷恢复失败截图。成功和失败分开，目标保持active。

## 2026-10-10：MKV Range 重开与有限成功验证缓存（本轮候选）

实际官方授权仍有效：选定 item11 / mediasource_11，固定STRM302到cdnfhnfile.115cdn.net；同一显式UA读取文件首1KiB和末1KiB均206，Content-Range总大小5,483,098,130字节一致。这不是整片解码/非管理员验收。

隔离FFmpeg null、510秒定位对照：固定STRM输入约19.089秒完成、CDN直链约16.149秒完成；随后trace对照约17.078/10.938秒完成，两者都exit0/73帧copy。trace确认同一次MKV定位需要5次seek/6次HTTP Range（首部、尾部、前部、索引、回读、350,225,868字节处）；固定输入各Range都再次请求STRM并302，旧逻辑每次额外探测CDN首1KiB。独立Emby正确85.ts仍约15.123秒500，输入尚未打开、0帧，间隔日志FfRunException；不能仅据此声称所有15秒失败均为固定内部超时。

候选修复：Engine级内存成功验证缓存，完整签名URL/精确UA/来源IP的SHA-256键，最多128项；有效期从探测开始算最多20秒，且不超过CacheUntil/ExpiresAt减5秒。未知寿命不缓存，命中不续期，失败不缓存；同键singleflight合并，取消的请求立即退出而共享探测最多10秒继续，排队与网络共用10秒总预算，最多4个同时CDN探测。仅保存摘要键/期限，无凭据/URL/媒体正文。每次请求仍重新解析授权、选源、resolve及DNS/URL policy，配置停用/过期/地址变更不可被该观察缓存放行。X-Media-Direct-Verified表明当前链接有近期有效的1KiB观测，不是每次重新读字节或整个媒体通过。

固定STRM、官方direct-only、客户端UA、必要转码以及V2网页总预算不变；不改Emby账号/设置/源文件，不跳过鉴权，不缓存负面状态。回归增加正向命中、URL/UA/IP隔离、固定期限、寿命边界、取消/并发、容量和网络上限、DNS变化/过期不可绕过、无效MIME/溢出Range/短正文不进入缓存。

发布/真实播放器结论以data/remote-deploy/direct-verification-*和后续本节记录为准。本段只说明候选与发布前证据，不自动将拖动、恢复或全项目门槛改为通过。
