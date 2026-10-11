# Docker Hub 部署（单 Compose、三个服务）

唯一部署文件是根目录 `docker-compose.yml`，只有 `app`、`postgres`、`redis`。
主程序启动时先执行嵌入数据库迁移，然后在同一 Go 进程启动管理 HTTP、Emby 网关、default/transfer/library 三个消费者、扫描监听和调度。不再运行独立 Worker 或 migrate 容器。
Emby 和 CloudDrive2 由各自已有服务提供，在服务连接页面配置地址；不要删除它们的媒体、配置及用户。

## 首次安装

```sh
python3 scripts/init-env.py
# 编辑 .env：APP_PUBLIC_URL、APP_BIND、APP_PORT、EMBY_GATEWAY_PORT；保管好主密钥。
# 首次初始化前不公开注册入口；在可信网络完成注册后再设置 APP_BIND=0.0.0.0。
# .env 中 APP_IMAGE 设为已发布的 dingding229/mediatool:sha-<commit> 或摘要。
docker compose config --quiet
docker compose pull
docker compose up -d --wait --wait-timeout 180
```

应用镜像目前只有 linux/amd64；ARM64 主机需仿真，不宣称原生支持。
生产建议 HTTPS；APP_ENV=development 允许既有公网 HTTP，但密码及恢复码会受明文传输风险影响。
PostgreSQL / Redis 端口只绑定 127.0.0.1，不能对公网开放。

## 更新与回滚

GitHub Actions 测试通过后发布 `sha-<完整 commit>` 和 `latest`，部署固定 SHA / digest 而不是追逐 latest：

```sh
bash scripts/update-server.sh dingding229/mediatool:sha-<40位commit>
docker compose logs --tail=100 app
```

脚本先 pull、验证 AMD64、备份数据库和配置，再重启 app，不构建/上传本地镜像。
初始化旧部署需要先迁移现有 volume 名及 bind mount 到 .env；保留 .env 的账号/主密钥、115 授权及 STRM 文件。不能把旧 Compose 直接 down -v。
升级失败保留备份，不自动还原覆盖新写入；恢复旧镜像前核对迁移兼容性，禁止自动向下迁移。

## 存储与日志

`STRM_MOUNT`、`DOWNLOADS_MOUNT`、`LOCAL_FILES_MOUNT` 可指定绝对宿主目录；空值使用命名卷。下载来源默认只读；本地管理目录 `/data/files` 可写。管理员只应登记容器实际可访问的本地根目录。
默认 UID/GID 65532，可通过 APP_UID/APP_GID 调整。宿主 bind 目录需要事先设置权限。
内置 Go slog 输出 JSON，`LOG_LEVEL=debug|info|warn|error` 控制级别；改后重启 app。
Docker json-file 每个服务最多 5 个 10MB 日志文件，保留有限历史，无单独日志容器。凭据及任意错误对象脱敏；日志不写请求体、Cookie、RawQuery 或 CDN 签名。

## 密码找回

登录后在系统设置验证当前密码，生成并保存一次性恢复码（30 天有效）。找回入口在登录页；输入账号、恢复码和 6–20 位新密码，成功后全部会话撤销。
恢复码只显示一次，服务器仅保存摘要，轮换/修改密码会作废旧码。不自动生成/打印现有管理员的恢复码。
未保存有效恢复码时，服务器所有者可停止 app，通过镜像中的 `/app/admin-reset` 离线恢复，使用 0600 密码文件；不要将密码放到命令参数或重新开放公开注册。
