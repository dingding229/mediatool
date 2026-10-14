# 播放器UA白名单与访问记录

2026-10-14 · 0.22.0-dev · 数据库迁移45

## 使用

1. 在「媒体浏览 → 播放器访问」查看策略。升级默认**限制关闭**，但已开始记录通过本程序播放入口的UA。
2. 用真实播放器播放一次现有STRM，刷新「UA访问记录」。相同完整UA聚合访问次数、通过/拒绝次数、入口和首次/最近时间。
3. 点击「查看UA」核对完整原值；「加入白名单」设置备注并明确确认。此操作默认增加完整匹配规则，不自动启用全局限制。
4. 如播放器版本常变，可手动编辑为明确的前缀匹配，例如 `SenPlayer/`（仅示例，须以实际记录为准）。所有匹配**区分大小写**，不使用正则、通配符或模糊包含。
5. 确认需要的真实UA都已加入后，点击「启用UA限制」。未匹配或空UA返回403，无CDN Location；规则变更对后续请求立即生效。
6. 可关闭UA限制恢复仅记录模式。清理UA记录不删除白名单、媒体、程序原始日志或文件播放授权。

## 作用边界

- 固定STRM解析的GET/HEAD。
- Emby网关的PlaybackInfo、Videos/Audio媒体输出、Items下载/文件、LiveTV和Sync文件入口；独立根监听器与嵌套/大小写Emby挂载采用同一识别。
- 后续内置媒体库member播放接口。当前仍返回预留501，UA命中不会启动未实现的播放能力。
- 不限制管理后台、Emby登录/目录/海报、字幕/附件及播放状态上报。后台管理仅管理员可访问，媒体用户无权编辑白名单。
- 不保护绕过本程序直接访问原Emby地址的请求，也不追溯撤销已发出的115 CDN临时URL。
- **UA可由客户端声明和伪装**：这是一层播放器使用策略，不是强身份认证、设备证明或DRM；原有管理员/member/Emby身份、文件Key、资产在库授权保持独立。
- 通过计数仅表示「通过UA检查」，后续可能仍因文件密钥、上游权限、媒体缺失或解析错误失败。真实响应码和完整请求/302URL见程序日志。

## 实现与限制

- Go模块 `playeraccess`，PG保存全局策略、规则及UA聚合记录，不启用缓存策略，避免修改后缓存仍允许。每次检查及记录使用有锁超时的短事务，**在任何上游/缓存取链之前结束事务**，不持有锁或数据库连接播放视频。
- 默认全局关闭。配置数据库不可用或策略/记录无法确认时503、关闭放行；不以“读取失败”推断关闭策略。仅无DB的development纯单元测试环境没有持久化策略。
- 原始请求UA未经白名单文本替换，继续传入115解析及CDN验证，仍302输出原始临时播放地址；不改变STRM，不引入中转或转码。
- 规则最多128条；UA最多4096字节，禁止控制字符、重复User-Agent头、无效UTF8或超长值，不截断后加入白名单。空UA可被记录，不可加入规则。
- 聚合记录最多2000种UA，插入新UA时淘汰最久未访问项；记录按SHA256摘要区分完整原值，前缀不会合并记录。分页最多100、支持UA关键词与当前白名单匹配状态筛选。
- 请求来源IP使用实际连接地址，不信任Forwarded/X-Forwarded-For。应用位于反代之后时可能显示反代地址，不能据此推断真实终端IP。
- 策略更新/从记录添加使用revision冲突保护；管理员修改和清理写入审计。UA聚合表不复制URL中的永久播放密钥；管理员原始日志和数据库展示策略不改变。
- 无新增定时任务或容器；仅app/postgres/redis，限额淘汰与访问事务完成，无非cron调度。

## API

所有管理接口需要管理员会话；写请求需 `X-Requested-With: media-tool` 并通过Origin检查。

- `GET /api/v1/player-access/policy`：enabled/revision/updatedAt/rules。
- `PUT /api/v1/player-access/policy`：完整规则快照及revision，乐观替换；新规则省略id，现有规则保持id。启用需至少一条启用规则。
- `GET /api/v1/player-access/observations?page=1&pageSize=25&q=&filter=`：filter为空、whitelisted或unlisted，按当前规则判断状态。
- `POST /api/v1/player-access/observations/{id}/allow`：`{revision,label,confirm:true}`，使用记录完整UA创建/启用exact规则。
- `POST /api/v1/player-access/observations/clear`：`{revision,confirm:true}`，仅清理聚合记录。

管理错误：400输入无效，409修订冲突，404原记录已清理/淘汰，503数据库/策略不可用。
播放入口：403 `player_ua_denied`，400 `invalid_player_ua`，503 `player_ua_unavailable`；没有CDN跳转。UA通过后仍由原业务入口继续身份、文件授权和解析。
