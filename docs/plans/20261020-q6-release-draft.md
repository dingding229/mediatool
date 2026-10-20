# Q6 发布草案（待审核；本文件不表示已推送/发布/部署）

## 阻断前提

- Q5/Q5.5审核通过；工作区干净，确认本项目批准的Git凭据，不复制其他项目凭据。
- 只提交/推送dingding229/mediatool；插件pin未改变。如以后确需改插件，先在插件仓库测试推送，再更新pin。
- 等待该提交build.yml全部门禁，尤其ubuntu-latest原生Linux AMD64的test-plugin-kernel.sh；任何失败中止，不以Mac skip/cross-build替代。
- 从官方Docker Hub取得对应提交的linux/amd64 immutable digest；校验revision/schema71/arch/仓库。禁止本地镜像档或别的registry代替。

## 操作者私有只读预检

1. 核实当前owned server根为/root/docker/mediatool，唯一根Compose仍为app/postgres/redis，参数为私有直接值。记录旧镜像digest、schema65、卷映射；不回显凭据到公开终端产物/Actions。
2. 停止/暂停本项目写者，保留外部Emby/CD2原服务和数据；在不会有新写入的窗口执行[只读SQL](../sql/20261020-q6-preflight.sql)。以ON_ERROR_STOP执行并核对expected_database；错误即中止发布，不能DELETE修数据或手工跳版本。
3. 特别核对R4的1条state_tasks：revision>0、fingerprint为64位小写hex、result对象、snapshot_id存在且instance_id相同。仅报告总数/合法数，不导出行内容。SQL亦预检D1空表/NULL引用/空覆盖与migration67 JSON形状。
4. SQL是发布门，不替代迁移事务内的锁定断言；网络断开/超时/结果不完整都不能算通过。

## 备份、部署、验证

1. 保留并校验升级前完整PG备份、私有Compose、主密钥/账号/115授权/STRM原文与输出归属、旧镜像digest。备份仅在操作者私有路径，保留近期回滚副本；不要把初始化Compose、数据库或日志放公开Actions/Release。
2. 核验备份可读、摘要和永久凭据/AAD；跨schema回滚必须是完整旧数据库＋匹配旧二进制/Compose，不运行破坏性Down，不让旧镜像直接读schema71。
3. 在本项目根Compose以官方Hub immutable digest更新app；按标准维护命令与审批流程执行，不创建第二份正式Compose/sidecar。66/70索引非事务，其他迁移有锁/超时；中断后先核对版本与索引状态，再使用同一版本重跑，不squash历史。
4. 等待app健康、/readyz、build revision/schema71；核验Goose最终状态、FK/CHECK均validated、并发索引有效。失败时停止后续动作，保留私有诊断与原库/备份，不能误称部署成功。
5. 管理员会话/CSRF、115授权与显式启用、CD2只读连通、站点/插件仅已选实例、能力状态、journal容量与Schedule、维护验证状态只读复核。
6. 保持原STRM原文/稳定resolver；用实际播放器UA取官方115链接，仅接受302到CDN，不relay/HLS/转码；验证Emby受管原始源权限，unsupported/unknown不自动重放。真实设备/媒体写操作另需所有者明确允许，不拿生产媒体当夹具。
7. 如走维护单执行，执行完成仍是verify阶段：核对版本/schema/保护摘要写收据，人工复核后显式解除维护；批准、健康或核验均不是业务真实播放证明。
8. 汇报Actions run、Hub digest、服务器实际revision/schema、验证结果与仍有缺口。Q7前不做全局prune/业务卷清理；仅清理本项目路径/标签的构建残留，保留回滚备份。
