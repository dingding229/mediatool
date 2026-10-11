# 远端CloudDrive2 / Emby测试部署

日期：2026-10-09（Asia/Shanghai）。经用户授权，在指定服务器/root/docker/mediatool部署，Compose项目名mediatool-test；没有调整其他既有项目、Docker守护进程或全局挂载传播。

## 已验证

- AMD64实际远端容器运行：应用、PG18、Redis8、控制/上传/库扫描Worker、CloudDrive2、Emby和私有回环适配器。
- 主库迁移15，管理员0，由用户首次注册；密码6–20。应用readyz通过，服务端管理接口无Cookie为401。
- 官方CloudDrive2镜像digest为sha256:3fb20d1d891299ce54102a369cb8662dc82dadadcc1cc4650624165766f13df2；网页200、官方gRPC GetSystemInfo成功、SystemReady=true、IsLogin=false。仅证明服务可达，不代表115读取授权。
- 官方Emby镜像digest为sha256:997e8c1ea1fde7d303eb455143e7be77dc4f20cb06acb11ffdcbcfaa87186ec2；实际System/Info/Public版本4.10.1.0；独立网关公共路由通过。
- 本程序/Emby共享STRM目录，应用可写、Docker mount给Emby为RW=false；原下载共享目录只读给应用。
- Emby同网络命名空间的回环适配器将固定地址localhost:25014转到server:8080，解决SSH隧道测试中Emby容器localhost指向自身的问题；只绑定容器回环，不新增公网入口。
- 自有5秒320x180 H.264/AAC合成视频生成并ffprobe校验，放在test-media/Owned-Test-5s.mp4；不使用第三方版权样片，也不代表真实Emby用户播放通过。
- .env是0600，远端单独生成应用/会话/播放/数据库/缓存密钥，未复制本地.env或业务数据。新公开端口全部绑定127.0.0.1，数据库/Redis无宿主发布端口。

## 待用户完成

通过SSH隧道访问25014（本程序）、25015（Emby网关）、19798（CloudDrive2）、8096（Emby初始化）。注册本程序管理员、初始化Emby、登录CloudDrive2并添加115，在服务连接保存对应访问凭据。容器内部CloudDrive2地址http://clouddrive2:19798，Emby地址http://emby:8096，Emby媒体目录/strm；云盘根目录以真实路径为准。

尚未验证真实115文件读取、临时URL、STRM字节、真实播放器解码/拖动/恢复及PT下载入库。不会把提供方未登录当作已通过。

## 远端操作文件

- /root/docker/mediatool/README.md：SSH隧道和初始化步骤。
- /root/docker/mediatool/compose.yaml及compose.remote-test.yaml：独立测试部署。
- /root/docker/mediatool/verify-deployment.py及reports/deployment.json：不含密钥的只读诊断。
- /root/docker/mediatool/source：无本地凭据的源码快照。

SSH私钥只用于本次授权连接，未存储到仓库、镜像或远端配置；使用完清理本地临时文件，不自动修改服务器authorized_keys。聊天已披露的私钥应由操作者轮换。

## 用户要求的公网访问更新（2026-10-09）

按用户明确要求，将本项目四个端口改为0.0.0.0监听：25014（应用）、25015（网关）、19798（CloudDrive2）、8096（Emby）。访问主机185.99.135.205对应端口，不再需要SSH隧道；APP_PUBLIC_URL=http://185.99.135.205:25014、EMBY_PUBLIC_URL=http://185.99.135.205:25015同步更新。当前是HTTP，尚未配置TLS，用户已提示首次初始化抢注风险和密码/Token明文传输风险，后续建议HTTPS反代。

更新前.env/Compose/诊断说明在远端backups/before-public-*私密备份；只改变本项目网络绑定及地址，未修改密钥、未删除卷、未开放PostgreSQL/Redis、未改变全局防火墙或23个其他运行容器。变更时管理员0、STRM记录0，无旧输出需要改写。

实际本机客户端从公网分别请求四个服务均200；使用正确公网Origin的无效账号登录为401而非CSRF错误；Emby容器网络命名空间可请求新的公网固定地址。远端只读诊断报告记录networkMode=public-http-user-requested，预计端口和数据库不发布检查均通过。报告副本在data/remote-deploy/deployment-public.json，不包含密钥。

此前“仅回环/必须隧道”段落是历史部署记录，已被本节取代。首次注册仍开放，应尽快由用户完成本程序、CloudDrive2与Emby初始化；真实115播放仍未验收。

## 当前配置及播放修复状态（2026-10-09）

此前“管理员0/CD2未登录/需隧道”是历史快照。用户已完成管理员、CloudDrive2/115及Emby配置，人工确认、资产/STRM生成和专用库刷新完成。固定入口及有效Emby用户授权静态网关302/代理读取已通过；合成浏览器HLS配置的播放列表和分片字节也通过，真实终端解码仍需复测。

此次只更新自己的应用镜像/源码和Worker，迁移仍15，账号/密码/云授权/密钥/媒体/数据卷不变。Redis绑定仅保存Token摘要/User.Id；后续经网关登录自动记录。已有会话的一次性只读维护导入不能作为日常HTTP鉴权替代。回退镜像media-tool:before-gateway-fix-20261009，旧网关源码private/source-before-gateway-fix-20261009.tgz；不得为回退删除数据卷。

当前25014管理、25015网关、19798 CD2、8096原Emby。播放器用25015；新增三个脱敏报告和验收边界见REAL_PLAYBACK_VALIDATION.md。仍为用户要求的公网HTTP，尚未配置HTTPS。

## 115官方独立播放能力部署

已部署官方open115适配器、PKCE扫码配置界面及迁移16。升级前本项目数据库保存到private/database-before-open115.sql.gz（0600），原镜像media-tool:before-open115和源码备份保留。没有删除卷、重置账号、修改Emby/CD2授权或自动切换播放来源。

目前官方连接启用数量0：需要用户自己的AppID及115 App确认授权，随后显式启用。未授权前原CD2链路仍可代理，不应把本轮上线等同于真实官方CDN播放已完成。迁移回退不自动删除官方配置/授权数据，必须先审查已有记录。

最新真实日志确认原Sen请求original.mkv/stream.mpegts已进入授权网关，但delivery=proxy、reason=provider_user_agent_required；不是只看固定STRM兼容UA的模拟302。新增官方模式的验收与配置步骤见OFFICIAL_115_PLAYBACK.md。

## 2026-10-10：授权可靠性修复部署与复测（当前状态）

本项目已升级到源码迁移17，server及三Worker运行正常。用户自己的open115保持enabled/ready，1个authorized会话、2个资产、0订阅；TMDB/CD2/Emby连接仍ready。PT/qBittorrent/Telegram尚未配置，不借用其他项目账号或容器。

变更前数据库0600备份为private/database-before-auth-hardening-20261010.sql.gz；旧镜像media-tool:before-auth-hardening-20261010，旧源码保存在private/source-before-auth-hardening-20261010。未修改账号密码/主密钥/Emby或CD2授权，未删除卷。23个其他运行容器ID摘要在变更前后相同。

实际应用/迁移已成功后，源码解包遇到服务器旧Python不支持TarFile.filter；未重放迁移/部署或覆盖备份。已核对源归档SHA256，验证仅普通文件/无链接/无路径穿越/无私密.env，手动安全复制到已有空stage并完成源码切换。此恢复不影响用户运行数据；部署报告明确记录恢复原因。

发布后用既有真实Emby用户会话复测item11/source mediasource_11：默认Sen UA的original/Static均302到cdnfhnfile.115cdn.net，CDN206/1024字节，1MiB/256MiB偏移亦通过；普通测试UA按自身取链302，错误Token401无Location，无新会话摘要导入。仍realPlayerDecoded=false，不外推真实Sen解码/音轨/字幕/长拖动或网页冷恢复。

新增迁移17后的实际数据库备份private/database-after-auth-hardening-20261010.sql.gz（0600），恢复到本项目新建的独立临时库，迁移版本/管理员/连接/资产/官方会话与加密字节事实相符，临时库已清理，未覆盖业务库。该测试验证逻辑恢复，不宣称已对恢复副本执行密文解密或主密钥轮换。报告auth-hardening-restore-check.json明确ciphertextDecryptionVerified=false。

报告：reports/auth-hardening-deployment.json、gateway-user-session-check.json、auth-hardening-restore-check.json、reliability-current.json，均0600。本地脱敏副本在data/remote-deploy。管理仍25014、Emby网关25015，用户要求的公网HTTP未自动改为HTTPS；PG/Redis未公开。

## 2026-10-10：真实自有上传、访问指标与流中断修复

已发布维护工具cloud-upload-smoke、安全status/responseBytes/duration指标以及http.ErrAbortHandler正确透传。数据库仍17，无新迁移、未重放迁移。变更前0600数据库备份为private/database-before-upload-metrics-20261010.sql.gz及private/database-before-stream-abort-20261010.sql.gz；回退镜像分别media-tool:before-upload-metrics-20261010、media-tool:before-stream-abort-20261010，旧源码私密保留。

自有49,982字节小视频经真实CD2上传，云大小/SHA-1一致、Range206读取整个文件SHA-256一致；镜像内维护入口再验证已有持久session及同一目的文件通过，不重新Start、不创造PT/下载/影视身份业务记录。诊断目录private/owned-upload-check-20261010持续保留，云端测试文件未自动删除；原始test-media文件保留。不能据文件大小推定普通上传吞吐，秒传/去重可能发生。

新日志揭示HLS初始0.ts约10秒502及一次404，后来分片200；后来Chrome DOM有缓冲/readyState4/640x360/时间前进，但Mac锁屏截图黑，不标真实画面或完整冷恢复通过。已结束本次浏览器测试，未停止其他客户端。修复流中断不能往媒体尾部追加JSON，不冒称已解决所有启动失败。

最终只读核验：server/三Worker就绪，迁移17、资产仍2、官方authorized会话仍1、其他23容器ID摘要相同。原始流默认Sen UA的官方302/CDN206与错误Token401复测通过，仍realPlayerDecoded=false。详情与报告见OWNED_UPLOAD_VALIDATION.md、REAL_PLAYBACK_VALIDATION.md，data/remote-deploy/{owned-upload-*,stream-abort-deployment.json,browser-hls-initial-failures.json}。公网HTTP保持用户选择，不修改云授权/账号密码/主密钥或删除卷。


## 2026-10-10：HLS启动预算与取消分型

已部署取消请求不伪502、固定上游错误分类、已知Emby4.10.1.0网页模块的60秒HLS首字节预算，以及仅媒体片段75秒响应头预算（普通API30秒/共享池）。原Emby文件不改，未知hash/格式原样透传；application/x-javascript实测兼容修正后公开GET22,264字节/policy/no-store/去旧ETag已确认。

第一版hls-startup因旧MIME未应用规则，未冒称通过；其备份/回退仍保留。最终hls-mime镜像/源码与备份分别private/database-before-hls-mime-20261010.sql.gz、media-tool:before-hls-mime-20261010、private/source-before-hls-mime-20261010，PG仍17，不重放迁移、不改账号/授权/媒体/卷。

独立诊断0秒：首TS10秒断开，记录request_cancelled而不是502；同会话约0.938秒后200/TS，45秒单请求约10.573秒后200。正确510秒timeline85.ts仍500且FFmpeg真实-ss/无帧，未解决冷恢复。旧Chrome本轮未出现加载basehtmlplayer的新服务器请求，因此缓存模块是否采用新规则不证；锁屏状态下DOM后来有缓冲/时间但不算正常画面。

所有诊断仅停止独立device/session，Chrome测试已结束保留详情页。全量Go race/vet、12项E2E、9项Python/Node回归通过；最终3Worker/2资产/1官方授权及其他23容器ID摘要保持。详见HLS_STARTUP_RELIABILITY.md，真实PT/qBittorrent/通知/完整客户端矩阵仍未通过。


## 2026-10-10：最新302复核与HLS诊断范围纠偏

Sen原生UA的original/Static再次验证302→cdnfhnfile.115cdn.net、CDN206/1KiB，错误Token401无Location；真实客户端解码仍未验收。此次明确使用管理员登录会话，不能代表非管理员权限。旧HLS工具仅检查播放许可，误将管理员会话标作普通用户；已改为默认拒绝管理员、显式管理员诊断且role准确标注，不补建账号或使用其他项目凭据。历史报告不覆盖；此前ordinaryUserScopedSourceVerified=true及“同URI就是同编码”的说法不再用于验收。

独立FFmpeg seek/null读取约15.402秒成功；直接Emby也出现一次85.ts 200与一次500，网关仍有500，网页冷恢复保持未通过，不能假定只是代理超时、固定15秒上限或只需创建目录。工具增加限定loopback的直接上游对照、总会话查找预算、禁止5xx立即重试与连接关闭回归。本轮仅诊断脚本/测试/文档变更，15项Python/Node回归通过，没有新的运行镜像/迁移。具体证据与下一步见HLS_STARTUP_RELIABILITY.md。


## 2026-10-10 PT配置兼容发布

最终镜像media-tool:sites-compat-20261010已在本项目更新server/三个Worker。前置真实x86_64 Docker审计：非root、只读、无网络，193静态配置加载/159核心支持/34部分支持；不是PT账号验收。本地原生arm64同样通过，amd64模拟异常仍未解决，不改变生产CPU配置。无迁移重放、自动配置导入或账号/授权变更；Emby/CD2与其他23个容器未更新。

发布保留回退镜像media-tool:before-sites-compat-20261010、private/database-before-sites-compat-20261010.sql.gz和private/source-before-sites-compat-20261010。公开Vue资源可提供同提交恢复入口；原有默认Sen UA的original/Static302→cdnfhnfile.115cdn.net/206再验证通过，错误Token401无直链。真实客户端解码/非管理员权限与网页续播未声称完成。报告sites-compat-deployment/after/official115-check及sites-image-physical-check.json。


## 2026-10-10：网页启动缓存修复已发布，播放仍有限通过

固定SHA的index/apploader新增独立缓存键，不伪造Emby4.10.1.0版本；真实Chrome已加载改写的3100/22264字节脚本。实际HLS构造配置离线核验首字节60秒/总加载120秒/原全局默认不变。网页时间轴约8分30秒有一次真实出画面，随后恢复仍有网络/暂停/黑屏/重新加载，**不是稳定冷恢复或全部客户端通过**。独立无重试的510秒仍15.124秒上游500，异常为FfRunException/StartFfMpeg；不误推目录/固定15秒预算根因。

Go/race/vet、13E2E和17脚本回归通过；真实非root只读无网络amd64镜像验证后仅更新本程序和三个Worker，无迁移/账号/授权/原媒体修改，其他23容器保持。原生302→115CDN206/错误Token401再次保持；真实PT/订阅/通知与持续网页/客户端矩阵门槛继续开放。完整证据/回退见HLS_STARTUP_RELIABILITY.md本轮追加。


## 2026-10-10 V2启动保护已发布：首播有限通过，seek保持未通过

严格原版本hash/唯一锚点的90秒启动guard仅让HLS已明确选择、仍有次数的非fatal瞬时重试继续；权限/fatal/取消/耗尽立即终止，不手动重试或改换session。35状态场景、原回调函数离线、真实SDK字段/自建XHR、全量Go/race/vet、18脚本和13E2E通过。实际镜像验证后更新本程序/3Worker，无迁移或账号/授权/媒体修改，其他23容器保留。

Chrome按标准入口加载revision2；首播真实经历两次500/guard等待后实际影片画面与currentTime35→71→158秒连续播放，UI暂停/恢复有效。拖动510秒失败，退出后恢复四次有限等待仍约90秒终止，**不是seek或全部客户端矩阵通过**。PT业务/订阅/通知、非管理员、音轨/字幕/正常设备矩阵继续开放。精确证据与回退见HLS_STARTUP_RELIABILITY.md V2节。
