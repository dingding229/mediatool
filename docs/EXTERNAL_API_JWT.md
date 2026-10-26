# 外部 API JWT（schema89 / v0.42.2）

签名：APP_SECRET_KEY解码为32字节，经标准库HKDF-SHA256、nil salt、独立info `media-tool:external-api:jwt:hs256:v1` 得到32字节HS256密钥，仅驻内存。使用锁定的golang-jwt/jwt/v5 v5.3.1；WithValidMethods仅允许HS256，再显式检查算法对象。请求token最多2048字节，不接受none、其他算法、URL参数或管理员Cookie替代。

v1 claims序列化顺序固定为 jti/scopes/iat/iss/aud/ver，scopes字典序且无重复，iat固定为创建时间Unix秒，iss/aud/ver固定，不设exp。header固定alg/kid/typ。校验签名后只接受与v1规范重签相同的字节（拒绝额外/重复claims、exp或异常header）。再查jti状态、原claims及所需权限。状态不缓存，撤销提交后新请求拒绝，不取消已获准的在途任务。主密钥轮换走既有停机rekey流程，同事务撤销旧JWT授权；不保留旧签名密钥回退，不在本任务轮换生产主密钥。

创建、查看、复制均先在同事务提交披露审计后才返回token；审计不含token、签名、根密钥或派生密钥。查看/复制POST管理接口须有效管理员Cookie及CSRF，已撤销凭据禁止披露。列表不含token，前端仅在当前弹窗暂存，关闭/离页清空。每次复制也请求新的披露审计。若创建回执丢失，不自动重复创建；管理员可核对后查看该凭据。

数据库不保存token、digest、ciphertext、prefix或expiry。仅保存ID、固定claims所需元数据、状态及使用摘要。schema89清除旧外部凭据；迁移回执external_api_migration_receipt只记录被清数量，不记录内容。旧调用历史保留名称和原ID作为历史观察，不再是有效授权外键。Down只恢复旧结构且为空，不重构旧凭据；真正回滚仍按既有已验证备份流程。备份验证仅为此明确删除投影放行，其他业务/凭据照常严格校验。

调用IP采用已有clientip可信代理证据：直接或可信代理来源才有clientIP，伪造XFF无效；异常链记录未知，不用服务器出口替代。审计用途允许可信私网地址，不改变播放统计“仅公网IP”。现有鉴权/120次每分钟限流事务仍保留，但新增IP/最终状态写入采用2048项非阻塞队列、单协程、最多256项批次，每秒冲刷；队列满记计数告警，不阻塞业务。仅UPDATE既有调用/凭据，不因迟到记录重建已删除数据；按key→call一致加锁顺序，遵守维护写入围栏。

详情默认50条，上限100；保留最多30天且每凭据1000条，后台分批清理；最近一分钟的滚动限流窗口不会被清除。IP摘要超过30天清空，最后调用时间可保留。正式业务十条路由/六类scope均不变，专用验证表单和客户端函数删除，status.read继续存在。新管理端路径均登记路由表/OpenAPI并经web/src/api/调用。

## 运维验收（仅显式授权后使用）

`docker compose exec -T app /app/media-tool ops external-jwt-check -confirm`

仅创建一条带t15-check前缀的status.read临时凭据，调用同一业务Create/Reveal/Revoke/Delete服务；两次披露审计后比较token，但不输出token。只通过容器内127.0.0.1访问正式HTTP status接口，等待后台IP入库，再用同一token验证撤销后401/403，删除临时凭据和调用行并等两个批次周期确认不重建。失败也执行有界清理，安全审计保留；不获取/导出管理员会话、密码或其他现有凭据。该命令是本镜像内owner-only CLI，不是HTTP入口。管理端鉴权/CSRF/同事务审计另外由集成测试和E2E覆盖。
