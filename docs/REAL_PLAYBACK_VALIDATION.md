# 真实115固定STRM服务端验证

2026-10-09。用户已启用CloudDrive2直链并人工确认媒体后，资产1、STRM1、候选confirmed、Emby刷新成功、输出journal无待修复。

## HTTPS修正

真实CloudDrive2配置useHttpDownload原为true，返回HTTP直链被安全策略拒绝。使用现有权限仅将该选项设false，其余直链开关/线程/IPv4/代理/账号/套餐不变；没有放宽TLS/URL/DNS策略。应用连接revision同步失效旧协议缓存，未更换访问凭据。新版维护工具尊重权限拒绝并有单项设置/脱敏回归。

## 实测

- CloudDrive2提供HTTPS直链，允许根目录读取成功，原视频仅读取最多1024字节，不下载整个文件。
- 现有文件级固定STRM包含assetId、当前公网origin和有效可撤销Key；测试不输出Key或签名临时URL。
- 符合提供方UA的固定STRM HEAD请求返回302，跳转目标经安全策略验证。
- 提供方要求特定UA；普通测试UA按预期走受控代理，不强制不可用302。GET/HEAD均206，Range获取1024视频字节，HEAD无body。
- 无效文件Key401，未签发跳转。
- Emby实际查询找到对应托管STRM（itemId=9）；无效播放用户Token通过网关被401拒绝。
- 已保存报告/root/docker/mediatool/reports/playback-server-check.json（0600），本地data/remote-deploy副本不含凭据或临时URL。

## 尚未证明

未使用后台API Key冒充普通播放用户，也未重置账号或创建额外Emby测试用户。普通用户成功授权的Emby网关路径及真实客户端解码/拖动/音轨/字幕/暂停恢复仍需用户从http://185.99.135.205:25015登录播放验证。

本文件的302是固定STRM兼容UA路径通过，不等于所有播放器都应该302；UA不匹配时代理是安全预期。公共HTTP管理/网关尚未配置TLS，媒体直链本身HTTPS不代替管理入口HTTPS。

## 2026-10-09：Emby网关真实授权播放修复（取代上述授权路径待验项）

使用现有真实Emby用户会话定位用户报告的问题，不使用后台API Key冒充用户：

1. Emby 4.10.1.0的 `/Users/Me` 实际返回500。现在从上游成功的 `/Users/AuthenticateByName` 响应学习权威Token/User.Id关联，Redis只保存上游origin+Token的SHA256摘要键及用户ID，TTL30天，所有动态Gateway共享。每次播放仍用调用者Token重新校验 `/Users/{Id}` 的播放策略及用户scoped item；不信任客户端UserId/DeviceId。登出/401清除绑定；未知身份或缓存故障不能签发云直链。
2. 用户item可缺失 `PlayAccess`。缺失时额外调用同一用户的PlaybackInfo，要求无ErrorCode、所选source的ID/Path一致；显式拒绝、无播放权限、撤销Token及冲突凭据均不得绕过。
3. 静态请求为 `stream.mpegts?Static=true`，云扩展为 `.ts`。补充实际容器别名，不同容器/codec转换/非静态/HLS仍交原Emby。
4. 原重写逻辑覆盖HLS的DirectStreamUrl。现在保留Emby选定的remux/transcoding路径、参数和支持能力，仅将相对/同上游地址归网关，不伪造播放器能力。

### 已部署及实测

- 更新 `/root/docker/mediatool` 的应用镜像/源码，仅重建本项目应用/Worker及迁移检查。数据库仍迁移15，无新增表，没有修改账号/密码/密钥/数据卷/其他项目。
- 一次性只读Emby本地认证库核验既有Token/localUserId/GUID关系，导入3条会话摘要。原Token仅在远端内存和维护命令stdin使用，不打印/落盘，不把认证库复制到本地；新登录会自动捕获。
- **真实有效用户、托管item9/source mediasource_9，兼容提供方UA的静态mpegts请求：网关302，X-Media-Gateway=emby-static，随后HTTPS CDN Range206/1024字节通过。**这次证明的是Emby授权网关，不再只有固定STRM成功。
- 同用户普通UA：经过网关鉴权，受控代理206/1024字节；无效用户Token401且无Location。
- 合成浏览器HLS能力配置的真实PlaybackInfo：原Emby及网关200，DirectStreamUrl/TranscodingUrl路径和参数保持、均归网关，能力不变，文件Key已隐藏。
- HLS播放列表200/有效EXTM3U，首个媒体分片200读取1024字节通过；只停止本次独立DeviceId/PlaySessionId的诊断编码，不停止其他用户播放。
- 本地全量Go `-p 1 -race -count=1`、Go vet、隔离Redis绑定回归及11项隔离Chrome E2E通过。回归覆盖旧接口500、PlayAccess缺失、TS别名、撤销/登出、伪造UserId、冲突Token、拒绝item/source及HLS参数保留。

### 仍需客户端复测

服务器已证明302后的媒体字节、代理与HLS字节读取，**不代表真实终端已解码、拖动、切换音轨/字幕通过**；报告仍为 `realPlayerDecoded=false`。请从 `http://185.99.135.205:25015` 网关（非8096）刷新/重新登录后播放；既有密码不变，不需注册或重置。

302并非所有客户端/请求的硬性成功指标：当前CD2直链带提供方UA要求，不兼容UA走代理；浏览器需要remux/HLS时保留Emby处理。未为了302放宽HTTPS/URL/DNS/header安全策略。管理/网关当前仍公网HTTP，需后续HTTPS。

新增报告（0600，无Token/文件Key/签名URL）：
- `/root/docker/mediatool/reports/gateway-user-session-check.json`
- `/root/docker/mediatool/reports/hls-metadata-check.json`
- `/root/docker/mediatool/reports/hls-byte-check.json`

维护工具：`media-tool diagnose session`（stdin 接收已独立核验的既有会话；旧 server/SQLite 包装已撤销）。导入关联必须由管理员独立核验，不向HTTP提供导入接口，不作为常规鉴权替代。普通诊断可查看不含秘密的X-Media-Gateway/Reason及X-Media-Delivery响应头；不要分享完整播放URL、HAR或Token。

上游接口参考：Emby官方User Authentication及GET/POST Items/{Id}/PlaybackInfo文档。

## 2026-10-09：SenPlayer original及真实Chrome检查（重要边界更新）

已读取用户提供的embyExternalUrl源码，并保留CloudDrive2架构进行机制对照，见REFERENCE_EMBY_EXTERNAL_URL.md。

- 真实SenPlayer/6.2.1使用original.mkv，现已接管original GET/HEAD且不要求Static=true，仍先验证用户、source和grant。兼容提供方UA：302到cdnfhnfile.115cdn.net，CDN206/1024字节；无效Token401。
- 该CDN真实绑定UA。服务器及操作者Mac网络上，同一签名链接正确UA206、Sen默认UA403。Mac测试正常验证TLS并固定已经核验的公网地址（本地DNS使用Fake-IP）；不保存URL，不导出Emby用户Token。原生PlaybackInfo返回RequiredHttpHeaders.User-Agent=CloudDrive2/1.1.2 (Linux-X64)，未泄露其他提供方headers。
- 对CD2customUserAgent的显式单字段对齐验证不成立，已经恢复原字段；没有保留配置变更，不降低UA/TLS策略。Sen是否采用新的播放header仍需其实际客户端重新获取PlaybackInfo验证；服务端模拟302不等于其现在已经不走服务器流量。
- 真正Chrome错误根因是绝对HLS字段被ApiClient再次拼接，生成/http:/host/... 404，现保持server-relative语义。用户当前item11网页实际出画面、readyState4/video.error=null、时间前进、+10秒、暂停及一次重新进入恢复通过，期间无账号/密码变更。
- **再次在约8分30秒冷恢复出现HLS首分片170.ts 404/network，FFmpeg部分记录0帧/无分片，故深seek/多次冷恢复仍未通过。**此前初始播放通过不应被理解成全操作稳定。独立原始流1MiB、256MiB偏移Range206/1024字节通过；还需定位Emby/FFmpeg和文件seek/会话状态。
- 新增不含URL/Token的deliveryReason和requiredUserAgent供管理检查及固定枚举访问日志使用。浏览器需要Emby HLS/转码时仍走服务器流量，不强制整个文件302。

报告新增browser-real-playback-check.json（必须同时记录冷恢复失败）、native-client-network-check.json。gateway-user-session-check.json仍只证明服务器请求/有限字节，realPlayerDecoded=false；真实Chrome证据与失败边界分开记录。所有媒体样本诊断不落盘影片。参考项目的强制direct-play和私有115预览Cookie接口未复制。

## 2026-10-10：115官方授权已配置后的302实测

官方连接启用/ready，Sen默认UA6.2.1通过官方取链：原始流original.mkv及静态流302到cdnfhnfile.115cdn.net，Direct-Verified=true，CDN206/1024字节。普通测试UA分别取链亦302；1MiB/256MiB偏移分别302→206/1024通过；无效Emby Token401。原生Metadata不再要求CloudDrive2固定UA。

Mac客户端网络同一官方链接使用Sen默认UA直接从CDN206读取，正常TLS及安全公网IP验证；不是通过mediatool代理。报告official115-gateway-check.json及official115-client-network-check.json不含原始Token/签名URL，真实播放器解码仍未冒充通过。用户下一步停止旧Sen播放、重新打开，保持默认UA。

## 2026-10-10：官方模式后的网页启动复测（未通过稳定门槛）

Chrome实际HLS请求仍有初始失败。普通播放用户的同一组无凭据HLS参数可读master/main/首TS200，只证明稍后可读；新安全访问日志捕获0.ts约10027ms后502、随即404，8.ts一次约678ms后502，随后多个分片200。不能据此盲调30秒网关超时，需继续定位Emby分片生成、上游响应/连接中断及客户端重试。

后来DOM显示readyState4、640x360、时间从约7秒到24秒、缓冲有数据/error为空；Mac锁屏下截图仍黑，未完成可见画面与完整操作。该轮不标为浏览器稳定通过，也不外推Sen解码/8分30秒冷恢复。已结束本次测试播放并保留用户详情页。脱敏报告browser-hls-initial-failures.json、browser-hls-route-check.json和browser-locked-playback-state.json在data/remote-deploy。

已修复中间件错误吞掉http.ErrAbortHandler并可能给截断媒体追加JSON的问题，保持原媒体前缀、正确交由net/http中止连接；普通panic仍500。该标准流中断修复有回归，不把它当作上述502/404根因已解决。


## 2026-10-10：取消请求不再误记502，冷seek仍未通过

独立普通用户诊断首TS10秒内断开，现按request_cancelled中止，不生成502错误体；同device/session/URI续请求约0.938秒200。实际HLS默认10秒首字节与约10.573秒冷启动冲突已核验，已知脚本SHA/MIME守卫的60秒budget在服务器公开GET确认。

正确按EXTINF选择510秒片段85.ts，而非初版总选0.ts：上游500，FFmpeg实际-ss 00:08:30.000/无帧，冷续播仍失败。当前Chrome旧缓存未证刷新（本轮无该JS新请求），其networkError不能作为新策略在页面生效后仍失败或成功的证据。后续在正常设备、新加载策略验证后继续首播/冷恢复/字幕音轨；不为通过而强制direct/禁转码。


## 2026-10-10：最新302复核与HLS诊断范围纠偏

Sen原生UA的original/Static再次验证302→cdnfhnfile.115cdn.net、CDN206/1KiB，错误Token401无Location；真实客户端解码仍未验收。此次明确使用管理员登录会话，不能代表非管理员权限。旧HLS工具仅检查播放许可，误将管理员会话标作普通用户；已改为默认拒绝管理员、显式管理员诊断且role准确标注，不补建账号或使用其他项目凭据。历史报告不覆盖；此前ordinaryUserScopedSourceVerified=true及“同URI就是同编码”的说法不再用于验收。

独立FFmpeg seek/null读取约15.402秒成功；直接Emby也出现一次85.ts 200与一次500，网关仍有500，网页冷恢复保持未通过，不能假定只是代理超时、固定15秒上限或只需创建目录。工具增加限定loopback的直接上游对照、总会话查找预算、禁止5xx立即重试与连接关闭回归。本轮仅诊断脚本/测试/文档变更，15项Python/Node回归通过，没有新的运行镜像/迁移。具体证据与下一步见HLS_STARTUP_RELIABILITY.md。


## 2026-10-10：网页启动缓存修复已发布，播放仍有限通过

固定SHA的index/apploader新增独立缓存键，不伪造Emby4.10.1.0版本；真实Chrome已加载改写的3100/22264字节脚本。实际HLS构造配置离线核验首字节60秒/总加载120秒/原全局默认不变。网页时间轴约8分30秒有一次真实出画面，随后恢复仍有网络/暂停/黑屏/重新加载，**不是稳定冷恢复或全部客户端通过**。独立无重试的510秒仍15.124秒上游500，异常为FfRunException/StartFfMpeg；不误推目录/固定15秒预算根因。

Go/race/vet、13E2E和17脚本回归通过；真实非root只读无网络amd64镜像验证后仅更新本程序和三个Worker，无迁移/账号/授权/原媒体修改，其他23容器保持。原生302→115CDN206/错误Token401再次保持；真实PT/订阅/通知与持续网页/客户端矩阵门槛继续开放。完整证据/回退见HLS_STARTUP_RELIABILITY.md本轮追加。


## 2026-10-10 V2启动保护已发布：首播有限通过，seek保持未通过

严格原版本hash/唯一锚点的90秒启动guard仅让HLS已明确选择、仍有次数的非fatal瞬时重试继续；权限/fatal/取消/耗尽立即终止，不手动重试或改换session。35状态场景、原回调函数离线、真实SDK字段/自建XHR、全量Go/race/vet、18脚本和13E2E通过。实际镜像验证后更新本程序/3Worker，无迁移或账号/授权/媒体修改，其他23容器保留。

Chrome按标准入口加载revision2；首播真实经历两次500/guard等待后实际影片画面与currentTime35→71→158秒连续播放，UI暂停/恢复有效。拖动510秒失败，退出后恢复四次有限等待仍约90秒终止，**不是seek或全部客户端矩阵通过**。PT业务/订阅/通知、非管理员、音轨/字幕/正常设备矩阵继续开放。精确证据与回退见HLS_STARTUP_RELIABILITY.md V2节。
