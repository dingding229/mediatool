# 当前入口说明（2026-10-09）

下方保留历史M0基线，不作为当前配置要求。0.6.0-dev支持数据库管理的CloudDrive2/Emby连接、文件级assetId和独立网关；请使用ALPHA_QUICKSTART.md与OFFLINE_HARDENING.md中的新版命令。不要求.env重复保存提供方凭据。

---

# M0：CloudDrive2 接入与播放验证

更新：2026-10-08。版本：0.2.0-m0。

**代码与本地协议测试已完成，真实 115/Emby 验收未完成。** 当前本地 .env 没有 CloudDrive2/Emby 地址、云盘读取凭据和测试媒体，因此没有执行真实服务调用，也没有宣称播放器解码成功。

## 1. 本轮交付

- 官方 CloudDrive2 protocol 1.1.2 固定快照及生成的 Go gRPC 客户端。来源/哈希/工具版本见 internal/adapters/clouddrive2/proto/source.json。
- 原生 gRPC http(s) 连接，HTTPS 校验证书，支持自定义 CA；不支持跳过证书校验。
- 优先使用受限 API Token；可选 CloudDrive2 账号密码 GetToken，按返回期限复用、并发合并刷新，Unauthenticated 最多重新认证一次。
- FindFileByPath、GetSubFiles、GetSystemInfo、GetDownloadUrlPath。PickCode 不冒充 CloudDrive2 file ID。
- GetDownloadUrlPath(get_direct_url=true)，保存 expiresIn/UA/附加头；每次播放查询当前地址，尚未使用 Redis 缓存直链。
- 固定 STRM 的 GET/HEAD 解析；可直连时 302，特殊 UA/头、强制代理模式时受控流代理。
- 受控代理透传 Range/If-Range，有限内存复制，取消联动；401/403 在返回响应头前最多刷新一次。
- URL/DNS/实际拨号的私网拦截及拒绝跳转回已配置的本程序/Emby/CloudDrive2 主机，可选主机允许列表；重定向重新检查，跨域不携带提供方凭据。
- 最小 Emby 静态流拦截：只使用调用者令牌，确认 /Users/Me、当前用户媒体权限、PlayAccess 和选中的媒体源，再映射本程序管理的 STRM。
- 只读连接/文件/Emby 诊断 API、Vue 操作入口、单文件 STRM 写入命令、可运行的真实环境检查脚本。

## 2. 配置 CloudDrive2

在本地 `.env` 填写，不把真实凭据发到聊天、日志或 Git：

```dotenv
# 必须是支持原生 HTTP/2 gRPC 的 CloudDrive2 地址，而非只代理网页的地址。
CLOUDDRIVE_ENDPOINT=http://192.168.1.10:19798
CLOUDDRIVE_TOKEN=<限定目录、允许文件查询和下载链接读取的API令牌>
STRM_ALLOWED_ROOTS=/115/MS_Media
```

- HTTP 仅用于信任的私网；公网请用 HTTPS，并确保反代支持原生 gRPC/HTTP2，不是仅 grpc-web。
- API Token 模式不会自动使用账号密码替换已过期/无权限的 token。
- 如果使用账号模式，CLOUDDRIVE_TOKEN 留空，填写 CLOUDDRIVE_USERNAME/CLOUDDRIVE_PASSWORD；这是 CloudDrive2 账号，不是 115 Cookie。启用 2FA 的账号优先使用已创建的 API Token。
- 私有 CA：CLOUDDRIVE_CA_FILE 指向 PEM，正常校验主机名和证书链。Docker 要额外只读挂载 CA 文件；不能把宿主机路径直接当成容器路径。
- 使用受限目录读取权限，避免管理令牌出现在 STRM。上传/删除/文件修改能力本轮不调用。
- 连接检查是只读的，但读取元数据/链接仍受服务端额度和权限限制。

参数：CLOUDDRIVE_REQUEST_TIMEOUT 默认 10s（500ms～1m），CLOUDDRIVE_MAX_LIST_FILES 默认 1000（1～10000）。目录超过限制明确报错，不将截断列表当全量结果；全库扫描/分片对账留到 M4。

启动或重启服务后，在“系统设置”点击“验证 CloudDrive2”。只有系统就绪、账号已登录、凭据能读取指定根目录才报告 ready。就绪不代表 CDN 或真实播放器已验证。

## 3. 固定 STRM 和播放策略

```text
https://<本程序域名>/api/v1/cloudStorage/strm302?apiKey=<播放Key>&pickCode=<可选真实值>&path=<编码路径>
```

配置：

```dotenv
APP_PUBLIC_URL=https://media.example.com
STRM_PROXY_FALLBACK=true
STRM_FORCE_PROXY=false
PLAYBACK_ALLOW_HTTP=false
PLAYBACK_ALLOWED_HOSTS=
```

PLAYBACK_ALLOWED_HOSTS 可填逗号分隔的精确域名或 `*.domain`；空值表示允许经 DNS 验证的公共 HTTPS 目标，不表示允许内网地址。通配符不自动包含域名根节点。实际 CDN 域名应以真实返回为准，不猜测硬编码。

- 无访问头限制、适合当前 UA、HTTPS 公共目标：返回 302 + no-store。
- 特定 UA/额外请求头：默认受控代理，不把这些头当 302 响应头要求播放器自动转发。
- IP 绑定或客户端不支持重定向：可显式设 STRM_FORCE_PROXY=true；该模式承载视频带宽。
- 如果服务确实只给 HTTP 公共源，可显式启用 PLAYBACK_ALLOW_HTTP=true，并以代理模式读取；不要在公网传输敏感上游头到明文源。
- 响应缺少 directUrl：明确报错。不把 CD2 管理 token 的 static URL 冒充成 115 直链，也不自动拼接静态签名。
- 当前每次解析都申请当前链接，故不因 CDN 地址到期重建 STRM。路径重命名仍需映射/重建，稳定 assetId 在后续阶段实现。
- 代理不转发调用者 Cookie、管理 Key 或 Emby token，拒绝提供方 Host/连接头注入和 HTML 媒体响应。
- 返回 302 后本服务无法获知播放器下一跳错误；不能承诺透明处理所有 CDN 403。

### 写入一份测试 STRM

不要使用未获授权媒体。APP_PUBLIC_URL 必须能被 Emby 服务器访问；localhost 在不同机器/容器上含义不同。

```bash
make build
bash scripts/load-env.sh ./bin/media-tool -role strm \
  -cloud-path '/115/MS_Media/电影/授权测试视频.mkv' \
  -strm-relative 'Movies/授权测试视频/授权测试视频.strm'
```

写入 STRM_OUTPUT_DIR 下指定路径，使用根目录约束和原子替换；命令不打印含密钥 URL。**只有单文件写入，不是批量自动入库。** 将此目录按只读方式共享给 Emby，在 Emby 中添加媒体库并扫描。

## 4. 最小 Emby 静态流网关

```dotenv
EMBY_UPSTREAM_URL=http://192.168.1.10:8096
EMBY_PROXY_ENABLED=true
EMBY_302_ENABLED=true
# Emby 看到的 STRM 根目录；不是云盘目录。
EMBY_STRM_PATH_PREFIX=/strm
# 仅在 STRM 使用额外的、明确受信任的内网解析 origin 时填写。
EMBY_STRM_RESOLVER_ORIGINS=http://media-tool:8080
```

本程序 `/emby/` 为网关挂载路径，例如：

```text
/emby/Videos/<ItemId>/stream.mkv?Static=true&MediaSourceId=<SourceId>
```

严格规则：

1. 原调用者认证 → /Users/Me 获取实际身份；忽略客户端伪造的 UserId。
2. /Users/<实际用户>/Items/<媒体> 查询权限，要求允许媒体播放、PlayAccess=Full。
3. 选择明确媒体源。多源且没指定 ID 时，不猜第一个。
4. 仅处理本程序固定解析 URL，或安全读取已配置输出目录中的 .strm。核对 origin、播放 Key、允许云盘目录；不读取任意 Emby 文件、不跟随指向目录外的 symlink。
5. Static=true（大小写兼容）且不涉及改码/尺寸/烧字幕/非零时间偏移，容器与原文件相符时才拦截。
6. HLS、转码、非托管媒体、未知权限状态走原 Emby。明确 401/403/404 不签发云端重定向。

响应 `X-Media-Gateway: emby-static` 表示实际拦截；`X-Media-Delivery: redirect|proxy` 表示交付方式。两者都不含密钥。

**边界：** 尚未重写所有 Emby Web 绝对 URL、PlaybackInfo、客户端直连源及下载路由；原生客户端必须支持配置带 `/emby` 基路径，或先通过固定播放接口验证。未完成全客户端兼容矩阵和生产发布验收，不以此替代完整 M5 网关。

## 5. 诊断接口

管理请求需会话或管理 Bearer Key，POST 带 X-Requested-With: media-tool。

| 接口 | 内容 | 不证明 |
| --- | --- | --- |
| POST /api/v1/cloudStorage/test | 服务就绪、认证、目录/文件可读 | CDN/播放器可播放 |
| POST /api/v1/cloudStorage/diagnose | 文件、实时链接获取、DNS/头要求、交付模式 | 未读取视频字节 |
| POST /api/v1/emby/test | 公共信息接口可达和版本 | 用户授权或视频解码 |
| POST /api/v1/emby/check | 用户权限和 STRM 映射 | 未读取视频字节 |

管理诊断不回传临时 URL、CloudDrive2 token 或账号身份。前端在配置缺失时显示明确错误，不生成模拟“成功”。

## 6. 真实环境验收脚本

填入本地 .env：

```dotenv
M0_MEDIA_PATH=/115/MS_Media/电影/授权测试视频.mkv
M0_EMBY_ITEM_ID=<Emby扫描后的媒体ID>
M0_EMBY_MEDIA_SOURCE_ID=<媒体源ID，单源可为空>
M0_EMBY_USER_TOKEN=<有权播放该媒体的Emby用户访问令牌>
```

使用正常用户令牌，而非绕过用户身份的管理服务 API Key。

```bash
python3 scripts/verify-m0.py --base-url http://localhost:8080
```

脚本只读：检查认证与映射，检查 HEAD，使用 Range 读取最多 1024 个采样字节，确认网关确实拦截。上游忽略 Range 时及时关闭连接，不下载整部影片。重定向到 CDN 时不转发管理/Emby 凭据，只允许公共 HTTPS 目标，并将解析后的地址固定到实际 TLS 拨号，防止检查后重新解析到私网。

报告在 data/diagnostics/m0-report.json（忽略提交，0600），不输出 raw URL/token。

- 退出 0：服务端链路读取验证通过；**仍不是播放器解码验收**。
- 退出 2：配置缺失或某项失败；报告明确未验收。
- serverChainVerified 和 realPlayerVerified 分开；脚本不会将 realPlayerVerified 写为 true。

当前本地执行结果：needs_configuration，serverChainVerified=false，realPlayerVerified=false。不要把协议 fixture 通过或代码可编译改写为真实播放成功。

## 7. 人工客户端矩阵

补充服务端/客户端版本、影片格式、音轨/字幕。逐项验证 Emby Web/桌面/Android-TV/iOS（有设备时）：启动、拖动、续播、长时间播放、音轨切换、字幕、转码、外网连通。标记 302 通过 / 需代理 / 需转码 / 不支持，记录失败原因。

只有真实环境检查和人工矩阵满足目标后，才可把 M0 真实播放标记为完成。
