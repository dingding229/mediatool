# 115 官方开放平台直链播放

更新：2026-10-10（Asia/Shanghai）。依据用户提供的115官方文档开发，不复用CloudDrive2的AppID、115授权或Cookie，不引入私有115接口。

## 为什么新增

现有CloudDrive2返回的115链接要求固定User-Agent。真实SenPlayer默认UA会被CDN403拒绝，故受控代理产生服务器媒体流量。给客户端提示header没有证明Sen实际采用。官方模块让程序在取链时发送实际播放器UA，选中官方连接后禁止静默回退为CloudDrive2代理。

## 官方接口（已逐项读取公开Markdown文档）

- `POST https://proapi.115.com/open/ufile/downurl`：multipart `pick_code`，Bearer access_token；返回按文件ID分组的url.url。
- `POST https://proapi.115.com/open/folder/get_info`：multipart绝对 `path`，取得文件ID、pick_code、名称、大小；不需要重新生成STRM。
- `GET https://proapi.115.com/open/user/info`：只读验证授权。
- `POST https://passportapi.115.com/open/authDeviceCode`：client_id、PKCE challenge、sha256。
- `GET https://qrcodeapi.115.com/get/status/`：长轮询uid/time/sign。
- `POST https://passportapi.115.com/open/deviceCodeToToken`：uid和原始code_verifier。
- `POST https://passportapi.115.com/open/refreshToken`：轮换access/refresh凭据。

PKCE使用43–128位随机verifier和二进制SHA256的URL-safe Base64（无padding）。当前默认64字符。只需要自己的AppID；该扫码模式不需要AppSecret。应用必须是自己申请、符合平台审核和权限要求的应用。

官方刷新文档要求：响应expires_in决定访问Token有效期；同一授权60秒内重复刷新受限；access/refresh需成对原子保存；40140116/19/20/37为终态，不能无限重试。

## 配置步骤

1. 管理页面「服务连接」添加「115 官方直链（独立播放）」。
2. 填写自己的115开放平台AppID。挂载前缀一般为`/115`，不是媒体根目录：`/115/Mediatool/film.mkv`映射到115账号内`/Mediatool/film.mkv`。
3. 先保存为**未启用**。点击「115扫码授权」，在115 App中核对应用和授权范围后手动确认。
4. 扫码成功后Access/Refresh Token仅加密保存在PostgreSQL，不回显浏览器，不写日志。也可在密码输入框手动录入自己合法授权的Token，不要贴到聊天中。
5. 点击「验证」，通过后编辑并显式启用官方连接。授权成功本身不会自动切换提供方。
6. 停止SenPlayer旧播放，重新打开同一个STRM媒体。旧文件级grant/assetId不变；无需重建STRM或改Emby库路径。

CloudDrive2继续负责扫描、上传和路径数据。`open115`只改变播放取链；禁用官方连接才恢复CloudDrive2作为播放来源。启用官方但授权失效、路径不匹配或CDN验证失败时明确报错，不能把失败藏在服务器媒体中转后面。

## 播放安全和流量

- 请求时读取真实HTTP User-Agent，传入官方downurl请求。
- 根据已确认资产的当前路径映射；验证官方名称/大小及pick_code相符，错误挂载前缀或账号不会随意播放同名其他文件。
- 现有Emby用户/item/source/grant授权继续在302之前执行，不以115账号Token代替Emby播放用户权限。
- 官方模式为direct-only。返回Location之前用同UA请求HTTPS CDN `Range: bytes=0-1023`，最多读取1KiB，必须是有效206和媒体内容。失败返回502，不强制发一个不能播放的302；强制代理配置冲突返回409。
- 因此「不经过服务器媒体流量」指整部媒体不由服务器转发；鉴权、取链和最多1KiB验证仍有少量接口/探测流量。
- 无临时URL返回管理诊断或公开日志。缓存按UA/IP/连接版本/文件版本隔离，官方地址短缓存45秒，这是本程序缓存上限，不是编造的115地址有效期。
- 服务器成功验证同UA不代表客户端网络也一定可读，最终仍需真实Sen播放器验证CDN连接和播放，真实官方取链的现有证据与客户端边界见本文2026-10-10记录。

## Token与二维码状态

`integrations.credentials_ciphertext`保存凭据。刷新持有PostgreSQL连接行锁，多进程也只执行一个刷新；更新access/refresh成对提交，限制重复刷新60秒。终态持久标记后停止尝试，重新授权才能恢复。刷新网络响应不确定时停止自动使用可能已失效的旧Refresh Token。

迁移16新增`open115_authorizations`，UID/sign/verifier加密保存，绑定管理员账号和credential_version及连接revision，有效期5分钟。迁移17增加一次性兑换claim：扫码长轮询不持锁；确认后先提交exchanging并清除verifier，再请求兑换，完成时重新核对会话/账号版本/连接revision。新会话或取消优先；进程崩溃后的孤立claim不重放，5分钟到期或重新生成二维码。取消、成功、失败和过期清除加密payload。二维码本地生成，不交第三方图像服务；前端关窗/离开页面停止轮询，移除二维码并请求取消。

管理入口当前公网HTTP，敏感配置和扫码控制应尽快放在HTTPS或SSH隧道后使用；115官方API及CDN均保留HTTPS与证书/DNS校验。

## 测试与待完成

已经以官方协议fixture验证multipart下载、Bearer隔离、实际UA传递、PKCE、拒绝跨域认证重定向、文件路径映射、direct-only不代理及403不伪装302。真实隔离PG验证并发刷新只调用一次、原子凭据、终态不重试、扫码会话所有权与凭据保密。

用户自己的授权及默认Sen UA下的网关302/CDN206已在2026-10-10完成有限验证；仍需要真实Sen客户端解码/拖动/音轨/字幕/零服务器媒体中转验收。没有借用他人/CloudDrive2应用授权实现演示成功。

## 2026-10-10：用户真实授权后的检查

用户已完成自己的115官方授权，连接已启用且只读验证ready。已有STRM保持原文件grant，不创建或重置账号。

使用现有Emby真实用户会话、item11/source mediasource_11和SenPlayer/6.2.1默认UA检查：官方提供方已选中，original.mkv及Static流均302到cdnfhnfile.115cdn.net；同UA CDN206/1024字节，网关Direct-Verified=true。1MiB和256MiB偏移也是网关302后CDN206/1024字节，错误Emby Token401无Location。原生播放信息不再注入CloudDrive2固定UA，普通测试UA也按其自身UA取链并302。

在操作者Mac网络独立使用Sen默认UA读取该官方链接，正常TLS验证、经核验的公网DNS地址固定（本地DNS为Fake-IP）后206/1024字节通过，未经mediatool转发媒体。签名URL仅通过私密内存管道使用，不保存；未导出Emby/115 Token。

以上证明真实授权的默认UA取链、网关302、客户端网络CDN字节与非零Range，不等于真实Sen应用已解码或其所有操作通过。请停止旧播放并重新进入同一影片；无需修改UA。网页需要HLS/转码时仍可有Emby流量，和原生直放分开验收。

## 2026-10-10：授权故障恢复回归

真实隔离PostgreSQL测试覆盖长轮询时读取播放Token/取消不阻塞、两Manager共享兑换claim、兑换前清verifier、请求断开后持久失败、重启孤立claim到期、取消/重新生成/编辑连接/管理员凭据版本改变优先于旧成功响应。Vue明确显示“正在加密保存”，清理二维码，继续轮询；关闭仍发送取消。测试使用自有HTTP协议fixture，不再次借用或更改用户115授权。
