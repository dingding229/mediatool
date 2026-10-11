# 仅直链播放：STRM入口与Emby网关

日期：2026-10-10。用户要求：不要转码，媒体读取直接使用115 CDN。

## 当前用户要求：使用STRM原链接

2026-10-10用户明确要求不再使用25015/Videos/.../stream作为媒体信息中的播放地址。严格直链模式因此保留媒体来源Path中的STRM固定链接原文，并声明远程HTTP来源；移除DirectStreamUrl/TranscodingUrl等流代理或转码入口，不为客户端预先绑定115临时链接/UA，也不携带Emby用户Token到STRM链接。文件级Key将向已通过Emby元数据权限检查的用户提供，持有链接者可直接访问对应文件，直至该Key被撤销。用户已明确选择这一行为；日志/诊断不输出Key，管理凭据与115 Token仍不暴露。

### 固定播放契约（2026-10-11复核）

```text
Emby媒体详情 / PlaybackInfo
  → MediaSources[].Path = STRM文件里的固定链接
  → 客户端请求 /api/v1/cloudStorage/strm302（文件级Key、pickCode/path）
  → 按实际媒体请求UA取得115临时直链
  → 302 Location: https://cdnfhnfile.115cdn.net/...
  → 客户端从115 CDN读取媒体
```

- 严格模式中原链接的查询顺序与编码保持原样，不重建为`25015/Videos/{item}/stream`，不附加Emby Token，不在返回媒体详情时提前取115链接。
- `25015`继续提供Emby元数据/鉴权/API代理；它不是返回给客户端的媒体播放地址。历史Static/original入口只作为受权限检查的兼容入口保留，严格模式不会把它们写入`Path`。
- `/emby`挂载入口和专用网关根入口都遵循该契约。来源保持`IsRemote=true`、`Protocol=Http`；不预绑定客户端UA或提供方秘密头。
- 清除流/转码URL并关闭DirectStream/Transcoding，不改变上游真实DirectPlay能力。无兼容解码能力时明确失败，不以服务器中转伪装直链成功。

## 历史行为：为什么原先两条链接不同

STRM文件中保存本程序8080（测试部署公网25014）的固定解析地址：`/api/v1/cloudStorage/strm302`。它携带文件级、可撤销的播放授权，播放时取得匹配实际UA的115临时地址，不把临时地址写进STRM。

Emby元数据发给客户端时，托管STRM地址改写为专用网关8091（测试部署公网25015）的`/Videos/{item}/stream?MediaSourceId={source}&Static=true`。网关先检查当前Emby用户对选定item/source的权限，再验证托管STRM授权并302到115。这样不会向Emby客户端直接泄漏可绕过Emby权限的文件Key。**显示这个入口不等于媒体字节走服务器**；判据是实际响应是否302及Location是否115 CDN。以上是之前的认证入口模式说明；当前严格模式按用户要求改用STRM原链接。不要在文档/日志复述用户提供的Key。

## 严格模式

Emby连接公开配置增加布尔`directPlayOnly`；环境默认开关`EMBY_DIRECT_PLAY_ONLY`（默认false，保持其他部署兼容）。已保存Emby连接显式布尔设置优先；管理界面显示有效策略。

- 开启后依然要求启用302网关、真实用户鉴权与托管文件授权。
- 原始文件/明确Static=true且容器一致的GET/HEAD才能302；不匹配格式、转码参数、非Static、来源不可确认均明确失败，禁止回退上游媒体。
- HLS、分片、DASH、动态流和重新封装请求不会被转发到Emby编码端点。匿名请求仍401，有权用户收到409 `direct_play_required`。
- 本程序STRM入口也执行只允许302的策略；CloudDrive2来源若需要特殊请求头/不匹配UA/HTTP/强制代理等则409，不偷偷relay。
- PlaybackInfo中托管来源的SupportsTranscoding/SupportsDirectStream为false，移除TranscodingUrl及HLS/封装DirectStreamUrl；保留原SupportsDirectPlay判定，不伪造浏览器支持TS/MKV、AC3或隔行H264。
- 不注入HLS启动修复，不改Emby账户权限、许可证或原媒体。普通元数据、图片、会话、字幕权限仍按Emby处理。
- 仅通过25015播放受此策略保护；直接连接原Emby8096不经过本程序，不能声称原Emby全局已禁转码。

## 客户端要求

严格直链不能把不支持的编解码格式变成支持。对于TS/H264/AC3或MKV/EAC3等，请使用能原生解码的播放器；网页不兼容时明确不能播放，不用启用转码来掩盖。配置检查、302/1KiB采样不能替代真实设备解码验收。

## 本轮验收范围

发布前已核对item17的上游Path为固定解析端点，网关Path为Static入口；Static实测302→cdnfhnfile.115cdn.net→206/1024字节，坏Token401无Location。此前诊断脚本误用original.mkv探测.ts来源，触发旧容器不匹配回退；该结果不能作为正确original.ts失败证据。严格模式明确拒绝这类错误路径，不再中转。

发布与最终正/负验证按data/remote-deploy/direct-only-*、item17-*记录，完成前不把候选设置等同实际已启用或整个播放器矩阵完成。

## 实际发布验证（2026-10-10）

严格策略已在指定项目启用。item17磁盘STRM与原始Emby Path完全一致，assetId吻合，没有把HTML中的amp;误当查询键。STRM、Static入口与original.ts在同UA/网络上下文下返回相同临时URL摘要，均302→cdnfhnfile.115cdn.net→206/1024字节，MPEG-TS同步字节有效。实际PlaybackInfo为SupportsDirectPlay=true（上游本身为true）、SupportsTranscoding=false、SupportsDirectStream=false，无TranscodingUrl/remux DirectStreamUrl或文件Key。

实际HLS manifest/segment、嵌套Emby别名、错误original.mkv和转码参数均409/direct_play_required，不带Location；这一限定测试窗口新增FFmpeg日志0，不能外推所有用户进程。错误Token401，测试仅为管理员会话的字节/策略验证，不宣称Sen实际解码或全客户端通过。最终加固还以拒绝默认方式覆盖视频/音频项下未知媒体端点，仅字幕/附件/附加部分元数据API例外；STRM入口也显式返回direct-only策略头。

只更新本项目Media Tool/三个Worker及用户要求的Emby连接播放策略；无新迁移、未改账号/密码/主密钥/115授权/媒体。Emby/CD2及23个其他容器身份由发布前后记录核对。部署、备份、回退和最终记录见data/remote-deploy/direct-only-*、direct-only-hardened-*及item17-*。恢复旧镜像会取消严格行为，不能把兼容旧版本回退当作保留无转码策略。

## 2026-10-11 STRM原链接再复核（不重复部署）

11:53（Asia/Shanghai）在指定服务器执行只读协议验证：条目11/17、代表性SenPlayer/浏览器UA、媒体详情及GET/POST PlaybackInfo共12项，`Path`均与磁盘STRM原链接逐字一致，无`/Videos/.../stream`、预绑定请求头或流/转码URL。四组直接使用原STRM且不附加Emby Token的请求均302→`cdnfhnfile.115cdn.net`→206，并返回1024字节Range样本。HLS/转码请求仍409，坏Token401。

已有正式版本已满足该播放契约，因此本轮没有重新构建/部署、重启或修改账号/115授权/STRM。配置与输出摘要、全部容器身份保持，验证窗口无新增FFmpeg日志。首次SSH主机预检未通过，未执行认证操作；再次只读扫描核对原主机指纹后执行验证，临时SSH材料随后删除。

本地补充根入口与`/emby`挂载入口的16组元数据回归，明确拒绝DirectStreamUrl/TranscodingUrl/预绑定头并保持真实能力。证据：`data/diagnostics/original-strm-live-recheck-20261011.json`、`original-strm-isolated-regression-20261011.log`及`original-strm-targeted-vet-20261011.log`。此结论只涵盖协议与有限字节检查，不是全项目测试、真实设备解码、长seek或非管理员权限矩阵验收；其余开发任务仍未全部完成。


## 2026-10-11 13:26 来源一致性版本后的播放复核

pt-mixed-proof正式版本、schema22未迁移。两条目/两类代表性UA/媒体详情与GET/POST PlaybackInfo共12项均与磁盘STRM原文一致；四组原STRM请求不携Emby Token，均302→cdnfhnfile.115cdn.net→206/1024字节。HLS/转码409、坏Token401、测试窗口无新增FFmpeg日志；原5份输出、有效严格设置和全部受保护服务保持。此为HTTP/有限字节检查，不是Sen实际解码、普通用户权限或长期拖动验收。原STRM无需重写，用户授权未重置。证据：pt-mixed-proof-original-strm-check.json与pt-mixed-proof-preservation.json。


## 2026-10-11 16:18 请求头版本后的播放保护复核

pt-request-headers/schema22更新仅PT协议与管理入口，未改115授权或重写STRM。两条目、原生/浏览器代表性UA及媒体详情/GET/POST PlaybackInfo共12项均保留磁盘STRM原文；四组原STRM请求不附加Emby Token，302→cdnfhnfile.115cdn.net→206/1024字节。HLS/转码409、坏Token401、窗口无新增FFmpeg日志、5输出及受保护容器/配置保持。这仍是HTTP/有限字节检查，真实设备解码、非管理员/长seek/字幕等另行验收。
