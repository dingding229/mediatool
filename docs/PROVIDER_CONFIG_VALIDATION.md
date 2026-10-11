# 真实提供方配置验证

2026-10-09。用户确认Emby/CloudDrive2配置完成后，只使用已保存的加密连接验证，不索要或输出密码、API Key、Cookie或签名临时URL。此前Emby忘记密码流程已由用户中止；只生成过恢复文件，未兑换PIN、未清空或修改账号密码。

## 已确认及补齐

- CloudDrive2已登录并就绪，现有API凭据可读取允许根目录/115/Mediatool，发现实际云端视频文件。
- Emby现有API Key调用受保护Users/Library接口200，不仅是公共Info接口可达。
- 原Emby连接缺少STRM路径和gatewayEnabled；已补/strm及302开关，原账号和凭据值不变。
- 创建专用电影库/strm/Movies、剧集库/strm/TV，不覆盖其他库或源文件；默认不立即扫描大量资源。
- 发起已授权根目录的流式索引，扫描成功，得到1个needs_confirmation候选。TMDB连接已存在；实际身份仍须用户确认，不自动冒充某电影或生成示例身份。
- 修复UI/API待确认筛选使用pending、而数据库默认为needs_confirmation造成候选不可见的问题；pending作为旧API别名保留，分页总数与列表一致。

## 真实直链门槛

官方1.1.2 Build 26-10-06 23:48:14实际RPC能力：文件是云端文件、ProviderSupportsDirectURL=true，但DirectLinkEnabled=false、DirectURLPresent=false、RelayPathPresent=true。因此没有获取到115原始临时直链，尚未验证字节或真实播放器，不将中转地址偷偷冒充302直链。

用户需在CloudDrive2的115设置启用直链；如果显示角色/权限不足，先确认对应权限。程序不代购买或修改套餐，不改为私有115 API，不放宽URL/TLS/DNS安全策略。

随后在本程序云盘索引对候选人工确认TMDB身份，生成文件级固定STRM/NFO，再做Emby可见性/用户授权/302/Range/真实播放验收。当前资产/STRM仍0。

诊断工具cmd/integration-smoke默认只读，视频检查只取最多1024字节并取消流，不下载整个文件；cmd/complete-emby-layout必须--apply、只补已知部署映射和专用库，保留凭据。只读能力诊断序列化布尔值/版本，禁止回传CloudDrive临时URL、账号标识和代理密码。
