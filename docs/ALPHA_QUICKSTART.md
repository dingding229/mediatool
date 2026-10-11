> 当前管理员改用首次注册的账号密码登录，旧启动Key不再生效。操作与恢复见ADMIN_ACCOUNTS.md。

# 当前可操作版本使用说明

更新：2026-10-10，0.7.0-dev，源码迁移17。主要功能和真实官方取链已通过有限验证，但PT入库全链路/播放器完整验收尚未通过，不能当作完整1.0发布。

## 启动与保密

已有.env不要重复初始化，也不要重置APP_SECRET_KEY。

```bash
python3 scripts/init-env.py  # 仅首次、没有.env时
# 应用、控制/上传/库扫描Worker、PostgreSQL、Redis
docker compose --env-file .env -f docker-compose.yml up -d --build
```

首次打开管理界面注册唯一管理员账号，之后用账号密码登录。ADMIN_API_KEY不再用于登录或管理API。APP_SECRET_KEY用于加密保存的数据，必须独立安全备份；不要与数据库备份一起公开。当前Docker验证管理端18080、网关18091；基础部署管理8080、网关8091。宿主机8080预览也已更新，但与Docker验证端口不同。

## 配置顺序

在“服务连接”填入真实信息，不把凭据发到聊天：

1. CloudDrive2：原生http(s) gRPC地址、目录和API Token。上传另需目录写入、远程上传与校验读取权限。
2. Emby：地址、刷新用后台Key、Emby看到的STRM目录、网关开关。播放验收另需普通用户Token，不能用后台Key代替用户权限。
3. TMDB：API Read Access Token，用于媒体身份选择。
4. qBittorrent：Web API、账户、共享路径映射、分类。
5. PT：确认上游配置许可后导入固定快照，按站点配置Cookie/API Key等。
6. 115官方直链：自己的AppID，保存停用连接→扫码授权→验证→显式启用。按实际播放器UA取链，CD2继续负责扫描/上传；见OFFICIAL_115_PLAYBACK.md。
7. Telegram：Bot Token与Chat ID。连接检查不发送测试通知。

同类型启用一个主连接，PT可多个。115官方和CD2各一个连接可同时启用：前者播放、后者索引/上传。地址变化必须明确替换已有凭据，不把旧凭据自动发送给新端点。停用Emby不会偷偷回退旧环境地址。

## 共享目录与网络

```dotenv
DOWNLOADS_HOST_PATH=/已有下载器的宿主目录
STRM_HOST_PATH=/共享给Emby的STRM目录
APP_UID=1000
APP_GID=1000
```

```bash
docker compose --env-file .env -f docker-compose.yml -f deploy/compose.shared.yaml up -d --build
```

下载目录本程序只读，做种源文件不改名/移动。STRM目录本程序可写、Emby可读，UID/GID要匹配；不要盲目递归改权限。Mac/Colima还需宿主目录已挂载到VM。基础命名卷不自动等同NAS已有目录。

APP_PUBLIC_URL必须能被Emby和播放器访问。验证端口只绑定本机，其他设备不能使用localhost。HTTPS反代后的网关配置EMBY_PUBLIC_URL；程序不信任客户端自己填写的转发头。

## 推荐验收顺序

1. 先用已有合法115小视频：扫描→人工确认→固定STRM→Emby网关播放、拖动、恢复。
2. 再手动搜索一个合法小资源并确认TMDB：下载→本地校验→CloudDrive2上传→云端校验→STRM/NFO/字幕→Emby可见→通知。
3. 最后创建停用订阅，先预演，正确后再启用一个订阅。RSS首次默认建立基线，不自动抓全部历史。

新STRM包含assetId与文件级Key，path只是兼容提示。临时115/CDN URL不写入STRM，播放时解析。文件Key可撤销/轮换；已经签发的CDN临时链接不保证立即失效。

## 新管理入口

- 云盘索引：候选/资产分页、最多20项明确身份批量确认；剧集每文件填写季集，不根据合集标题猜。
- 已入库文件：身份/季集修正，保留assetId与固定链接。共享剧集NFO冲突会拒绝，不能影响其他集。
- 定时扫描：默认停用，最小5分钟，指定子目录周期对账。不是PushMessage实时监听；失败扫描不投票缺失，两次完整扫描才确认，默认不删STRM。
- 输出待恢复：加密journal自动恢复正常未完成写入；手工修改保留并提示冲突，核对后可重试。
- 上传：持久化会话暂停/恢复/取消/核对。未知启动不自动重建，人工绑定需ID/路径/大小/远端确认；源文件变化不能恢复旧会话。
- 通知：结果未知先核对，人工重发需接受重复风险，凭据或目标修订变化不能误发。

上传100%、STRM生成、Emby可见和真实客户端解码是不同门槛。

## 工具

```bash
python3 scripts/verify-m0.py --username '<已注册管理员账号>' --base-url http://localhost:18080 --gateway-url http://localhost:18091 --asset-id '<已有资产UUID>'
python3 scripts/backup-restore-check.py
```

播放脚本读取数据库连接，使用管理员账号密码建立临时Cookie会话；不要求.env存放CloudDrive2/Emby提供方凭据，仍需测试媒体ID和Emby用户Token。脚本只能验证服务链路与部分字节，不宣称真实播放器解码。更多测试、主密钥轮换及范围见OFFLINE_HARDENING.md。Go全量集成测试使用`bash scripts/test-go.sh`，不要在普通.env的DATABASE_URL下直接执行测试迁移。
