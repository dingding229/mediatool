# 原生媒体端口部署与R2验收

本文件是操作说明，不是部署记录。P5没有访问生产、提交/推送、构建发布镜像或改动生产文件；实际发布仍由协调者在R2安排。

## 唯一部署形态

- 只使用项目根`docker-compose.yml`，服务恰为app、postgres、redis；Emby/CloudDrive2是既有外部依赖，不创建替代容器，不移除其配置、媒体或卷。
- 所有功能与后台任务在app内执行。生产初始化Compose中的参数使用直接值，不依赖`.env`插值；不得把初始化文件、凭据、日志/数据库或私有验收报告提交到仓库/Actions产物。
- R2顺序：本地全量测试→审核→按阶段提交/推送dingding229/mediatool→等待Actions原生AMD64验证及Docker Hub发布→使用已核实的不可变Hub digest更新本项目→复核/保留回退备份。禁止用本地镜像档案或其他registry替代。

## 端口与公开地址

| 配置 | 默认/建议 | 说明 |
|---|---|---|
| APP_ADDR | 127.0.0.1:8080（容器内部按既有Compose） | 管理端；不得当作原生Emby地址 |
| EMBY_GATEWAY_ADDR | 127.0.0.1:8091 | 既有外部Emby网关，保持现状 |
| MEMBER_ADDR | 默认127.0.0.1:8092 | 原生媒体端口；空值不监听，修改后需重启 |
| MEMBER_PUBLIC_URL | 独立HTTP(S) origin | 客户端可达媒体地址；不能带账号、query、fragment或云路径 |
| 后台access.memberUrl | 优先于MEMBER_PUBLIC_URL | 与PlaybackInfo/Public Info使用同一地址规则 |

容器若需端口映射/反向代理，应在app内部监听`0.0.0.0:8092`，再由本项目现有代理/防火墙控制宿主暴露；不要把postgres/redis公开。地址配置示例可使用`https://media.example.invalid`，实际操作必须替换成用户自己的直接值，不能把示例当作已部署地址。勿信任任意X-Forwarded-*；仅既有trustedProxyCIDRs中的直接代理可影响origin。

后台开启“内置媒体库”，设置ServerName、媒体用户及库授权；另外两个处理开关依赖总开关，截图还依赖探测。默认总开关false，probe/stills默认true但不会单独生效。管理员为播放器设置最终密码，勾选强制改密时播放器登录会被401拒绝；不用全局STRM key登录。

## 持久卷与截图权限

- 默认`NATIVE_STILLS_ROOT=/data/native-media/stills`，容量`NATIVE_STILLS_MAX_BYTES=2147483648`（2GiB）；默认单帧45秒，启动时严格校验三帧预算、lease上限与GC宽限期。
- runtime镜像USER为**65532:65532**，Dockerfile已创建stills目录并设置app属主。宿主bind mount会遮蔽镜像内的属主，不能仅凭Dockerfile就宣称生产可写。
- 前序只读确认的生产挂载为`/root/docker/mediatool/data → /data`，属于持久存储；本组没有再次读取/访问生产。R2应核对这一既有挂载及目标目录实际权限。
- 只针对本项目`data/native-media/stills`建立目录并授予65532写权限（例如目录0750、文件0640）；不得递归chown整个`/data`或修改媒体、115授权、STRM/数据库/其他项目目录。若宿主ACL/userns映射不同，先核实，不盲目套用UID。
- R2可在发布镜像中以app用户对stills目录做单个临时文件写入/rename/删除验证，不能用真实媒体作为测试夹具。截图文件由不可覆盖发布、lease/fence与GC引用管理；勿在运行中手工rm缓存树或跟随符号链接。

## ffmpeg与镜像体积

- runtime阶段安装Alpine ffmpeg（包含ffprobe），不用独立容器；Go/Node build阶段不安装、不执行生产转码。D9只允许mediaprobe导入ffx，原始STRM和播放器302链路不经ffmpeg。
- 既有测量的镜像增量约**+126MiB**；此数值是已知参考，不是P5重新测量的Hub结果。R2须对实际发布AMD64不可变镜像核对最终大小。
- Actions verify已要求安装ffmpeg并运行真实媒体测试，以及`go test -race -count=3 ./internal/modules/mediaprobe/...`。必须查看原生AMD64结果，模拟AMD64的SIGSEGV不能替代验收；失败阻止发布。

## 无迁移缓存与运行限制

Schema61不变，无Redis持久化依赖新增。Redis浏览缓存TTL10秒、2MiB上限、按新权限/分级及数据库快照换键；Redis不可用退回SQL，不缓存授权/PlaybackInfo/临时115链接。保守栅栏会因其他DB写入多失效，性能报告须区分首次未命中和新进程命中。

socket明确501，DisplayPreferences只读默认值，Localization为合法空数组；不承诺完整Emby管理员API或WebSocket。启动/图片匿名请求有IP限速，登录独立失败限流，已知请求并发64、超载503；管理路由不在媒体端口。

## R2必须保留的验收项

1. 原生AMD64 CI全部通过、Hub不可变镜像已发布；不先部署再等CI。
2. 8092/反代公开origin一致；System/Info/Public与本人System/Info为真实设置，管理路由在媒体端口404。
3. 以临时媒体用户走登录→Views/Items→PlaybackInfo→GET/HEAD空body302→Playing/Progress/Stopped；确认UA与115取链一致，不跟随/导出真实临时链接到公共产物。
4. 验证其他用户/隐藏库不可见、全局key拒绝、单设备撤销即时生效、字幕仅Embed；未实现外挂字幕不冒充可用。
5. 确认stills权限/持久性、处理开关与容量/GC正常；不删除业务卷或其他服务，只做本项目路径/标签范围清理。
6. 补充Emby **4.8** HDR10/HLG/DOVI枚举与Infuse/SenPlayer真机时序证据。当前本地报告是4.10.1.0合成consumer摘要，不能替代；只保存脱敏枚举/状态，不导出真实标题、路径、token或完整响应。
7. 旧Emby/CD2、账号、主密钥、115授权、原STRM正文和媒体均不变，近期回退备份保留。未完成上述步骤时必须报告阻碍，不能声称已发布/部署/真机通过。
