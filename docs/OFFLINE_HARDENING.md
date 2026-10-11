# 无账号开发与验收记录

更新：2026-10-09，0.6.0-dev，迁移13/14。

## 本轮新增

- 实际生效配置的preflight，不回传服务密钥；验收脚本无需本地保存CloudDrive2/Emby提供方凭据。
- 文件级asset STRM显式下载、当前索引路径读取；独立Emby网关参数；报告始终保留realPlayerVerified=false。
- Emby链接重写使用本程序origin/资产归属，而不是只匹配strm302路径。撤销/轮换后的托管Key也不向客户端泄露。关闭旧全局Key同时作用于网关。
- 元数据超过16MiB时明确502，不能以未经脱敏原文回退，也不能截断后冒充200。
- STRM/NFO加密恢复日志；意图与媒体绑定/播放grant同事务提交。文件fsync、rename与父目录fsync后提交摘要；重启遇到已落盘目标版本可幂等确认，遇到手工内容则保留并标冲突。
- 凭据轮换同事务stage全部STRM，避免更换Key后丢失文件重写意图；旧版输出必须验证归属才能接管。
- 上传每个变更RPC与持久化确认受PG锁、控制revision、Worker租约保护。暂停/取消不再被当作后台自动重试失败。源文件恢复前重新计算SHA1。
- 上传字节按已确认区间合并，不按最高offset或哈希计算冒充数据传输；进度100%不意味着校验完成。
- 会话列举、人工绑定：路径/大小匹配、明确选择ID、远端暂停确认后才绑定；未知列表/未知控制结果不自动重建。
- 候选/资产分页、最多20项逐条确认、身份/季集修正、定时子目录扫描，默认关闭。
- 主密钥离线轮换覆盖集成凭据、资源下载hint、播放grant、待执行artifact日志及保留旧表的ciphertext。任一解密失败整事务回滚，默认只检查。
- CloudDrive2缓存不再保留所有配置revision；旧连接等待活动操作排空后关闭。
- 固定EMBY_PUBLIC_URL作为反代返回链接origin；忽略客户端伪造X-Forwarded-Host/Proto。

## 执行验证

```bash
# 必须指向隔离media_tool_test，按包顺序避免TRUNCATE互相干扰
bash scripts/load-env.sh bash -c 'set -a; source .env.test; set +a; export TEST_REDIS_URL="$REDIS_URL"; go test -p 1 -race -count=1 ./...'
go vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
npm run test --prefix web
npm run build --prefix web
E2E_BASE_URL=http://localhost:18080 npm run test:e2e --prefix web
python3 scripts/load-check.py --requests 500 --concurrency 16
python3 scripts/backup-restore-check.py
```

包含journal落盘中断/回滚/手工修改保护、共享NFO、主密钥dry-run及失败回滚、上传RPC锁/旧revision拒绝、字节去重、数据库配置优先/停用、分页上限、定时调度、固定公网origin、超大JSON拒绝、480次并发元数据重写与IPv6上游测试。

实际运行结果在本文件末尾及 WORK_LOG.md更新。压力脚本只请求readyz，不是视频/CDN吞吐测试；模拟amd64运行不是物理x86设备验收。

## 操作入口

- 云盘索引：分页、明确身份批量确认、修正身份/季集、定时扫描、输出待恢复与重试。
- 下载与传输：暂停/恢复/取消、核对、结果未知时显式绑定会话。
- 服务连接：配置真实服务，不在聊天提交凭据。

## 离线主密钥轮换

**不要在线执行，也不要只修改APP_SECRET_KEY。先备份数据库并单独保留旧密钥，停止server与全部Worker，只保留PostgreSQL/Redis。** 新密钥用本地0600文件保存，不放在命令参数、日志或数据库备份中。

原生环境检查：

```bash
# APP_SECRET_KEY_NEXT_FILE是本地新密钥文件路径；文件只含新32字节hex密钥
APP_SECRET_KEY_NEXT_FILE=/private/path/next.key bash scripts/load-env.sh go run ./cmd/rekey --offline-confirmed
# 检查成功后才提交（不会自动改.env）
APP_SECRET_KEY_NEXT_FILE=/private/path/next.key bash scripts/load-env.sh go run ./cmd/rekey --offline-confirmed --apply
```

容器镜像内包含/app/rekey；通过现有Compose的数据库网络执行，并只读挂载新密钥文件。dry-run不提交。apply成功后安全更新本地.env的APP_SECRET_KEY，再启动全部服务。结果未知时保留两把密钥，核对数据库，不盲目重复轮换。旧密钥与新备份保留周期由操作者决定。

## 播放验收

```bash
# 通过账号密码建立管理员会话；M0 Emby播放用户令牌/媒体ID用于测试，提供方连接来自数据库
python3 scripts/verify-m0.py --username '<已注册管理员账号>' --base-url http://localhost:18080 --gateway-url http://localhost:18091 --asset-id '<已有资产UUID>'
```

非本机访问要求HTTPS；公网APP_PUBLIC_URL与EMBY_PUBLIC_URL分别对应管理/解析服务与Emby网关。验证实例目前仅绑定回环地址；远程Emby或电视需要显式配置网络/HTTPS反代及共享目录。


## 2026-10-09安全依赖更新

检查发现Go1.26.8标准库与golang.org/x/net0.58.0的已知可达漏洞。官方Go1.26.9于2026-10-08发布安全补丁，本轮保持1.26系列升级至1.26.9，并升级x/net至0.60.0（对应最低x/sync0.23.0、x/text0.42.0）。Docker Go镜像固定官方manifest digest。修复后govulncheck重新检查，不沿用旧“无漏洞”结果。

主密钥轮换工具已交付和在隔离数据库测试，未替用户执行业务库轮换。跨Movies/TV媒体类型修改或不同剧集共享NFO归属冲突不会自动搬移输出；先重新导入到正确目录，完整输出迁移仍属后续工作。托管目录不得由其他程序同时写入；摘要保护不是跨进程文件系统的条件写入协议。

## Docker主密钥轮换命令（操作者手动执行）

先将新密钥保存为本机0600文件，并确认Colima/NAS容器能只读挂载该路径。下列命令**未在业务库执行**：

```bash
# 先备份并停应用/全部Worker，数据库和Redis继续运行
docker compose --env-file .env -f docker-compose.yml stop app
# NEXT_KEY_FILE只包含路径，不在参数中传密钥值
NEXT_KEY_FILE=/private/path/next.key
docker compose --env-file .env -f docker-compose.yml run --rm --no-deps \
  --user "$(id -u):$(id -g)" \
  -v "$NEXT_KEY_FILE:/run/next.key:ro" \
  -e APP_SECRET_KEY_NEXT_FILE=/run/next.key \
  --entrypoint /app/rekey app --offline-confirmed
# 默认dry-run；确认检查成功后在同一命令末尾增加--apply才提交。
```

提交成功后安全更新.env的APP_SECRET_KEY，再用原部署组合启动server和全部Worker。不要把新旧密钥打印出来；UID/GID要保证能读取私密文件，不要为运行工具而改成世界可读。


## 实测结果摘要

- 全量Go race/Go vet、2项Vitest、2项Python脚本测试和8项Chrome E2E通过。
- 修复后govulncheck无已知可达漏洞，npm生产依赖audit为0。
- ARM64原生/AMD64模拟容器启动与rekey帮助入口通过；报告记录具体imageId。
- readyz 500请求、16并发：500成功/0失败，P95约62.93ms。仅本机就绪接口，不作生产视频容量承诺。
- 备份临时恢复核验迁移14及43张表通过，未覆盖业务库。
- 真实播放仍needs_configuration，未验收。

补充：受控媒体代理使用自有TLS fixture执行96个1MiB Range请求、16并发，逐响应验证206/长度/SHA256，以及客户端凭据和上游Cookie隔离，race模式通过。这是96MiB本机代理字节验证，不是编码解码、真实115/CDN或NAS吞吐验收。日志见data/diagnostics/owned-proxy-load.log。

## 2026-10-10：官方授权加密表与测试入口

源码迁移17。主密钥轮换覆盖8张加密表，新增open115_authorizations使用open115-auth:<sessionID>作为AAD；已经清空的payload（终态或兑换claim）保持为空。隔离PG dry-run/apply/坏密文全事务回滚通过；没有对业务库执行rekey。

全量Go集成测试使用`bash scripts/test-go.sh`；指定包可追加包路径，不接受覆盖-p的参数。该入口先显式替换DATABASE_URL为TEST_DATABASE_URL，迁移还使用-test-only查询实际库名，缓存仅Redis DB15。不要仅source .env.test后用普通迁移命令，因为.env.test不覆盖业务DATABASE_URL。E2E与Go集成测试不能同时运行。旧实测结果按其日期保留，最新需求矩阵见FIRST_ROUND_ACCEPTANCE.md。


## 数据库22的轮换与回退

当前cmd/rekey覆盖9类密文，新增search_run_resources.download_hint_ciphertext，AAD为search-run:<snapshotID>。历史版本17/20的8表工具不覆盖新快照，不能用于升级后的库。22的官方备份克隆已升级、9类dry-run回滚、降至20再升22，旧密文/资产/授权保持；生产未更换主密钥。

降版本会删除新快照与控制字段。只有维护窗中尚未接纳新业务、确认新快照/订阅/任务为空时才可进行已验证的结构回退；已有用户新业务必须先备份和人工确认，不能因为“旧镜像能启动”就自动down或恢复数据库覆盖用户数据。


## 来源恢复发布收尾：2026-10-11

已发布media-tool:source-reconfirm-20261011，schema20→22，仅停止/更新server、worker、transfer-worker、library-worker；账号/主密钥/115授权/媒体身份不改，无PT账户或规则自动导入。全量本地race/vet、Vue9项/构建、18脚本、23E2E；原生x86_64五包30项顶层测试无跳过，使用新内部网络/独立PostgreSQL，非生产库/非真实PT资源。原生跨编译fixture不带race，本地race证据另列。ARM64原生只读无网络配置审计通过，193/159/34/0仍只是静态支持，不提高accountVerified。

补验证qB接受但返回503的未知结果按已落库Hash核对、无第二次Add；旧receipt不恢复新暂停代，来源revision/同run提示在网络读取中变化被拒绝；读取无效task详情后不以空结构覆盖原数据。初始候选不包含最后这一读取失败保护，故保留初始包并生成validated独立源码/镜像/fixture清单；正式切换的是validated版本，不替换或冒充历史证据。

发布前备份已在另一个独立PG恢复，升级22并执行9类密文dry-run回滚，再结构降20/升22验证。第一次SSH克隆派发失败后读取证实无marker/process/目录/容器/网络/卷；第二次执行输出丢失，后续发现服务器已完成的passed回执，采用回执而没有第三次执行。发布亦以持久化receipt核对，不盲目重放。临时资源、含密钥目录与SSH材料在收尾清理。

发布后12项原STRM媒体详情/GET/POST PlaybackInfo逐字一致，四组代表性播放器UA头请求302→cdnfhnfile.115cdn.net→206，HLS/转码拒绝，验证窗口无新增FFmpeg日志；不是实际设备解码或非管理员权限验收。原5份磁盘输出、密码记录/.env/compose、资产绑定/播放授权/扫描计划与Emby/CD2/其他23容器由摘要核对保持。API仍要求管理员登录，首次注册已关闭，密码新设置6–20位。

回退镜像media-tool:before-source-reconfirm-20261011；备份/root/docker/mediatool/private/database-before-source-reconfirm-20261011.sql.gz；旧源码/root/docker/mediatool/private/source-before-source-reconfirm-20261011。发布/原生/克隆/保护证据为data/remote-deploy/source-reconfirm-*报告与validated-release-manifest。文档收尾另用docs-supplement，不修改运行代码或再次重启。

真实PT/qB/外部通知连接与订阅当前仍0；合法资源搜索→下载→做种保留→CD2→STRM/NFO→Emby→通知、真实缺集/促销与设备/权限/HTTPS/容量稳定性仍未完成。下一步继续pre-dessert请求体/分页/框架等更宽契约，以及上述实际验收，整体目标active。
