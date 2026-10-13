# 定时任务与反代运行配置

站点数据刷新、订阅评估、云盘周期扫描和插件 services 使用五字段 cron
（分 时 日 月 周），统一时区 `Asia/Shanghai`。支持 `0 9,21 * * *` 等日历规则，
不接受 `@every`、秒字段或用户覆盖时区。站点刷新最少15分钟，订阅/云盘最少5分钟，
插件最少1分钟。下一次运行时间持久化；停用/修改有版本隔离，同类运行去重。

队列轮询、租约心跳、网络重试、文件变化合并，以及浏览器状态轮询是可靠性/交互机制，
不是独立业务计划。后台对账通过 cron 的秒字段驱动，每15秒处理到期计划。

反代域名直接编辑唯一 `docker-compose.yml` 的 app.environment：

```yaml
AUTH_ALLOWED_ORIGINS: https://media.example.com
HOST_PROC_DIR: /host/proc
TZ: Asia/Shanghai
```

多个域名使用逗号分隔，必须是完整 Origin（协议+域名+可选端口），不能有路径。
`APP_PUBLIC_URL` 始终保留原固定 STRM 入口；追加登录域名不会改变已有媒体链接。
系统不会把任意 `X-Forwarded-Host` 当作可信来源，不关闭 Origin/CSRF 校验。

总览通过 app 的只读的 `/proc/loadavg`、`/proc/meminfo`、`/proc/uptime` 挂载读取宿主机负载和内存；磁盘为数据目录
所在文件系统剩余空间。读取失败显示不可用，不伪造为0；不需要特权或额外监控容器。

搜索缓存页面展示 PostgreSQL 中真实搜索结果（包括部分结果/失败/运行中），Redis
仅负责可复用结果索引的 TTL。索引过期后历史结果仍可查看。清理只隐藏列表项、停止复用，
不删除下载依赖的资源和身份证明。海报使用已验证的 TMDB 身份，缺少唯一匹配时显示占位。
TMDB 缓存独立清理，不会清掉站点结果。

外部测试 qBittorrent 不是主程序的 Compose 服务。测试实例只暴露本机 WebUI，采用持久
配置和不可变镜像，开启 CSRF/Host 校验；凭证存放服务器私有目录，不能提交仓库。
