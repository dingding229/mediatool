# 命令参考

[文档目录](INDEX.md)

文中的 `<...>` 为需要替换的参数，不是实际凭据或可直接运行的镜像引用。所有应用操作在 app 内执行；宿主 `media-toolctl` 只协调本项目 Docker 与私有文件，不提供远程命令服务。

## 宿主工具 media-toolctl

公共参数必须放在子命令前：

```text
media-toolctl --root DIR [--archive FILE] COMMAND [ARGUMENT]
```

| 命令 | 用法 |
| --- | --- |
| `init-compose` | 首次初始化根目录 Compose 占位值，设为私有权限；拒绝覆盖已初始化值 |
| `image-check IMAGE` | 检查正式 Hub 不可变 AMD64 镜像、数据结构与来源标签 |
| `update IMAGE` | 锁定部署目录、留存备份并受控更新 app |
| `register IMAGE` | 登记恢复/回滚可选镜像，不直接切换 |
| `execute REQUEST_ID` | 执行已批准且有效的维护单，重新核对保护条件 |
| `backup-verify` | 导出一致性快照，在专属临时库恢复核对；`--archive` 可选择私有归档 |
| `load-check` | 有界并发读取就绪状态，不提交媒体任务 |

```sh
./media-toolctl --root "$PWD" image-check 'dingding229/mediatool@sha256:<镜像摘要>'
./media-toolctl --root "$PWD" update 'dingding229/mediatool@sha256:<镜像摘要>'
./media-toolctl --root "$PWD" execute <维护单编号>
./media-toolctl --root "$PWD" --archive /私有备份目录/backup.dump backup-verify
```


```sh
./media-toolctl --root "$PWD" load-check --requests 500 --concurrency 16
```

`load-check` 支持 `--base-url`、`--requests`（1–10000）、`--concurrency`（1–64）、`--report`。报告包含私有运行信息，应保存在自己的受控目录。诊断成功不证明播放器解码、拖动和续播均成功；缺少配置或诊断失败返回非零状态。

## app 内 media-tool

```sh
docker compose exec app /app/media-tool <子命令> [参数]
```

| 子命令 | 用途 |
| --- | --- |
| `server`、`worker`、`healthcheck` | 服务、队列工作进程、健康检查；正常 Compose 启动不需要手动增加 worker 容器 |
| `strm` | 根据 `-cloud-path` 和 `-strm-relative` 处理指定 STRM |
| `account-info` | 查询已配置官方账号信息 |
| `migrate -action status` | 查看数据迁移状态；`up` 执行迁移，`down` 是有风险的逆向操作 |
| `admin reset` | 离线管理员重置；需要 `--username`、`--password-file`、`--offline-confirmed` |
| `secrets rekey` | 主密钥轮换检查；`--offline-confirmed` 确认全部应用/worker 停止，只有 `--apply` 才提交 |
| `sites sync` | 刷新已选模板；显式 `--ids` 才选择加载指定定义 |
| `backup verify` | 数据库快照/克隆内容核对，通常由宿主工具调用 |
| `ops` | 维护单、备份与执行后核验等操作，通常由网页或宿主工具协调 |
| `diagnose integrations` | 有界检查已配置的加密连接 |
| `diagnose playback` | 固定 STRM、授权和直接跳转诊断 |
| `diagnose cloud-events` | 有界读取文件变化通知能力 |
| `diagnose cloud-ua` | 查看云盘 UA；明确 `--apply` 才变更 |
| `diagnose cloud-upload` | 仅对自己的小文件做明确批准的上传诊断，保留私有记录，结果未知不重建会话 |
| `diagnose cloud-security` | 默认只检查；`--apply` 才调整允许范围内的云盘下载配置 |
| `diagnose emby-layout` | 检查布局；`--apply` 才执行受控调整 |
| `diagnose provider` | 提供方协议或回环诊断，不是媒体代理入口 |

离线命令不得在服务仍处理任务时运行。配置、密码文件及诊断输出需私有保存；不要在命令行参数中直接传递密码。具体危险操作先阅读[备份恢复](deploy/backup.md)和[账号安全](deploy/security.md)。

## 插件作者工具

源码工作区可使用：

```sh
go run ./cmd/media-tool-dev plugins check
go run ./cmd/media-tool-dev plugins assets --check
go run ./cmd/media-tool-dev plugins assets
go run ./cmd/media-tool-dev plugins matrix --check
```

这些命令读取锁定的插件依赖并检查或生成本地声明、资源和能力表，不是运行期安装接口。插件接口见[插件说明](PLUGINS.md)。

### 数据恢复（app内，与网页共用）

`media-tool ops restore-preview -id <备份UUID>` 返回逐表行数、固定风险与精确确认短语；`restore-execute -id <预览UUID> -digest <预览摘要> -confirm-text '恢复 <备份UUID>'`等待相同状态机执行；`restore-status -id <预览UUID>`查持久化结果；`restore-reconcile -id <预览UUID>`只核对不重放。正式部署请通过私有根Compose的 `docker compose exec -T app /app/media-tool ops ...` 执行。旧 `media-toolctl execute` 已批准的数据恢复也转交同一执行器，不停app或改名数据库。镜像回滚命令不变。

### 在线更新内部入口

- `media-tool updater <admitted-task-id>`：仅临时更新容器使用，读取持久化的已批准任务；不接受镜像/URL/命令/路径参数，不替代管理员密码确认。
- `media-tool update-policy-check`：operator 本地只读诊断；核验当前 app 的 Compose 身份，并用不可联网的 transport 证明危险 API 在到达 daemon 前被拒绝，不执行更新或 Docker 写入。
