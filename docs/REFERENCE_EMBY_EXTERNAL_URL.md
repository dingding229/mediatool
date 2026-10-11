# embyExternalUrl 参考实现对照与采用边界

> **当前策略（2026-10-11）：** 指定部署已使用严格直链模式，返回STRM原链接，不再生成`25015/Videos/.../stream`播放地址；媒体请求时取得115临时链接并302。以下对相对DirectStreamUrl、CloudDrive2固定UA及网页HLS的分析是历史排查记录，不是当前部署的播放方案。当前契约与验证范围见`DIRECT_PLAY_ONLY.md`和`PLAYBACK_RESOLUTION.md`。

日期：2026-10-09。用户明确提供 `bpking1/embyExternalUrl/emby2Alist/README.zh-Hans.md` 作为参考。读取README及播放、AList、STRM、转码相关源码，固定commit `91798cc2ce5e14646c6b939b9f5d1d8d137541e9`；只读快照在忽略的data/reference目录，不在Docker镜像中。

## 对照结论

| 参考实现机制 | 本项目处理 |
| --- | --- |
| `/videos/{id}/stream`、`original` 分流 | Go网关均接管；original无Static参数亦可进入授权直播放，不把HLS切片跳成整个文件 |
| PlaybackInfo生成相对DirectStreamUrl | DirectStreamUrl/TranscodingUrl保持Emby客户端server-relative约定，Path仅指向固定网关地址；避免网页ApiClient二次拼接产生`/http:/host/...`404 |
| AList取链接请求携带当前UA，115缓存加UA | 已有链接缓存包含UA/IP/连接revision/文件版本；CloudDrive2公共RPC当前请求字段没有caller UA，不伪装成AList拥有的取链能力 |
| 固定STRM/路径映射到临时地址 | 保留稳定文件级grant/assetId，播放时解析CD2直链；验证已注册origin、允许云根目录、当前grant及文件状态 |
| 直链/转码选择 | 保留上游设备支持能力、HLS地址和转码权限，不无条件改SupportsDirectPlay=true、SupportsTranscoding=false |
| 原链路回退 | UA不匹配/额外请求头/无法确认权限/无法映射来源时保守处理；401/403/404授权拒绝不得签发CDN地址 |
| 日志诊断 | 记录固定枚举的gateway/delivery/reason，不复制参考实现中完整PlaybackInfo、播放query、签名URL或认证header日志 |
| 115预览HLS/字幕扩展 | 不引入参考实现对115私有预览接口和Cookie的直接调用，不改变用户要求的CloudDrive2接入 |

参考仓库是MIT许可，快照保留其LICENSE。当前Go实现未复制整段NJS业务代码；后续若引入第三方代码必须保持原许可与署名。

## SenPlayer：必须区分路由和CDN访问条件

实际客户端UA为 `SenPlayer/6.2.1`，调用 `original.mkv`。旧路由未接管original导致原Emby中转；现已修复，并继续校验真实用户、item/source和文件权限。

当前CD2返回直链域名 `cdnfhnfile.115cdn.net`，要求 `CloudDrive2/1.1.2 (Linux-X64)`。不仅在服务器测试：在操作者Mac网络上，使用正常TLS验证、固定经校验的公网DNS地址读取1KiB，同一链接正确UA206、SenPlayer默认UA403。该测试不保存签名URL、不复制Emby用户Token到本地，证明此次目标并非只能由服务器IP读取。

原生PlaybackInfo已按Emby标准RequiredHttpHeaders给SenPlayer返回公开的User-Agent，并在捕获到调用者凭据且验证为云来源时显式标记Remote/Http，保持可鉴权的Path（不返回Cookie/Authorization/其他提供方秘密），只在原生播放信息接口提供，不在媒体列表或网页请求加入无法设置的UA。若客户端不采用该字段，用户需在客户端媒体请求头/UA设置使用上述值；必须停止旧播放、重新获取PlaybackInfo后复测。不能以服务端模拟兼容UA的302测试冒充SenPlayer实际已经改用CDN。

尝试CD2公开customUserAgent偏好以对齐Sen默认UA时，未验证得到匹配的直链，因此单字段恢复原值，不保留该变更。未修改账号、套餐或直链/TLS/线程/代理等其他配置。可逆维护命令 `cmd/align-cloud-download-ua` 仅显式apply可用，验证失败恢复该字段，重读当前配置保留其他并发设置；未知写入结果先读回，不假设未生效。

## 网页：初始播放修复，但深位置冷恢复未验收

实际Chrome失败请求曾是 `/http:/185.99.135.205:25015/videos/11/master.m3u8`，返回404。改为server-relative字段后，当前用户页面能出画面，readyState=4，无video.error，时间从6秒持续增加；+10秒、暂停、退出再恢复也曾通过，播放至约8分31秒。不是只有合成device profile/1KiB分片检查。

随后再次冷恢复约8分30秒时，出现原Emby `/videos/11/hls1/main/170.ts` 分片404/网络错误；FFmpeg记录初始分片Frames=0，部分进程未生成分片。独立代理1MiB及256MiB偏移Range均206/1KiB，故不能简单宣称代理未支持Range或把该问题归为302失败。仍需单独定位Emby/FFmpeg冷启动、文件seek及播放会话生命周期。没有关闭转码、强改设备支持或宣称所有seek/字幕/音轨已通过。

目前该MKV含EAC3 5.1，Chrome实际选择HLS/转码；这条链路仍使用服务器流量。实现原生客户端CDN直播放不能让所有浏览器自动解码不兼容的容器/编码。

## 可操作诊断

管理鉴权后的播放检查返回：

- `mode`: redirect/proxy/unsupported
- `deliveryReason`: `direct_conditions_met`、`provider_user_agent_required`、`provider_headers_required`、`forced_proxy`等固定枚举
- `requiredUserAgent`: 可配置到原生播放器的公开UA；不含临时URL或访问密钥
- `mediaBytesChecked=false`: Inspect不代表读取媒体或真实解码

实际请求头：`X-Media-Gateway`、`X-Media-Gateway-Reason`、`X-Media-Delivery`、`X-Media-Delivery-Reason`。固定枚举进入访问日志，不记录完整query/token/cookie/provider URL。

## 后续优先项

1. 用户停止旧SenPlayer播放并重新打开，确认标准RequiredHttpHeaders是否生效；不生效则设置上述公开UA，再验证客户端实际CDN连接及网关redirect日志。
2. 对约8分30秒位置的原Emby冷恢复做独立FFmpeg/文件seek及会话检查；补真实浏览器长seek回归，不能用正常初始播放代替。
3. 如果需要与参考实现一样按每个caller UA获取115链接，先验证CD2新的公开能力；没有能力前需用户明确同意增加其他提供方，而非悄悄加入AList/115 Cookie私有接口。
