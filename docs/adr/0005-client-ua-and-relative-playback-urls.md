# ADR 0005：客户端UA与Emby相对播放地址

日期：2026-10-09。状态：已采用；真实Sen客户端header应用、Emby深位置冷恢复仍待验证。

## 背景

用户参考embyExternalUrl。真实SenPlayer原始流original.mkv未进入网关；Chrome将绝对DirectStream/Transcoding地址再次拼接为/http:/host/...404。CDN直链绑定CloudDrive2要求的UA，默认Sen UA403，匹配UA可在服务器外网络206读取。

## 决策

1. stream（Static=true）及original（固有raw-file语义）共享用户/item/source/grant校验；真实变换参数及HLS留给Emby，授权未知不签发CDN链接。
2. DirectStreamUrl/TranscodingUrl保持server-relative契约，不改写为绝对网关URL；Path仅显示稳定网关入口，原STRM grant不回传客户端。
3. 不强制SupportsDirectPlay/DirectStream=true，也不移除必要转码。参考实现可配置的强制直播放不是通用媒体兼容方案。
4. CD2返回userAgent是播放前置条件，不把建议/限制混为一谈。为匹配的原生客户端通过标准RequiredHttpHeaders提示公开UA；不传Cookie/Authorization等提供方秘密，网页不注入无法设置的UA。没有公开按caller UA取链能力时，不伪造支持。
5. 固定入口、用户鉴权和直链缓存相互独立。链接缓存仍按UA/IP/连接revision/文件版本隔离；缓存命中不得跳过权限。
6. 管理诊断提供公开requiredUserAgent、固定deliveryReason；访问日志只记录固定标签，不记录完整URL/query/认证头/原始PlaybackInfo。

## 验收边界

服务器兼容UA302/有限字节、Mac网络CDN206与真实客户端解码分别记录。真实Chrome首播/快进/暂停/一次恢复通过，不覆盖后续8分30秒冷恢复404问题；原生Sen是否使用提示UA不凭模拟请求宣布通过。

## 不采用

未经用户同意引入AList、直接调用115私有预览HLS/Cookie、关闭HTTPS/DNS/UA验证、将HLS切片302到整个文件、绕过播放用户权限、记录签名URL及管理员/用户Token。
