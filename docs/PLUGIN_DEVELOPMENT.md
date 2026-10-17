# Go 内置插件开发（V2）

## 仓库与构建

SDK、Host DTO、事件枚举、transport、mock host和golden工具均在独立Go module
`github.com/dingding229/mediatool-plugin/sdk`，插件在 `plugins/<id>/`，registry显式登记
元数据与工厂。不导入主程序internal，避免循环依赖。主程序已删除plugin-sdk/go和
examples/plugins。hello-go是F1真实worker示例，不是WASM或脚本插件。

1. 在插件仓库修改manifest和Go接口实现；依赖编译期锁定（可vendor），禁止包初始化I/O、
   init函数及非纯顶级求值，不能在import时创建实例。
2. 使用SDK测试工具跑单元、mock Host、真实Go二进制stdio golden；插件仓库CI vet/test/race。
3. 推送 `dingding229/mediatool-plugin`；主程序精确bump伪版本（不使用replace作为发布证据）。
4. `go run ./cmd/plugin-check` 审计registry全部非标准库传递依赖的初始化。
   `go run ./cmd/plugin-assets` 从固定pin导出本地只读资源清单；`--check`用于CI。
5. 主程序按标准Actions→Hub不可变镜像流程发布。运行时无编译器、包下载器、外部市场。

所有Go构建使用CGO_ENABLED=0的linux/amd64；同一app以plugin-worker子命令执行。
主程序和前端消费同一个 `library.json`，插件ESM位于其仓库`plugins/<id>/ui/dist/`，
构建期复制到`/plugins/<id>/ui/dist/`，不从网络加载。F1只提供资源清单，执行/共享Vue
加载器是F2范围；没有相关插件时资源数组为空。

## 接口与能力

必选 `InitPlugin/GetState/StopService` 与可选接口表达
GetForm/GetPage/GetAPI/GetService/GetCommand/GetDashboard(+Meta)/GetModule/GetActions。
辅助客户端提供Config、SaveData/GetData/DelData、DataPath、PostMessage等。
每次Host请求携带本实例身份/generation，Host强制声明scope与最新授权；不暴露SQL、
数据库连接、Redis连接或115令牌。配置/KV返回revision，写操作传expectedRevision。

F1 hello-go只使用当前已实现的config.read、data.read/write、state.read、ui.json。
业务Host覆盖在F3；F2-A将API/Service/Command/Action的Go函数直接绑定为稳定handler id，
并补充线上descriptor元数据。不要把G1现有字符串descriptor当作最终的作者绑定API。

数据在PG加密，AAD绑定实例/行；私有文件目录默认`/data/plugins/<UUID>/{data,tmp}`。
开发/隔离测试可用`PLUGIN_DATA_ROOT`设置干净绝对plugins目录，不影响生产默认。
子进程env不包含主程序秘密，不能通过环境获取数据库/主密钥/115授权。
调用超时不代表副作用未发生；保留operationKey并查询/重试同一操作，不能创建新幂等键
盲目重放。写入有CAS/fence；撤权/升级后旧generation拒绝。

## 资源与调试

同UID65532，rlimit和整树RSS/CPU看门狗，崩溃退避重启。进程组停止、Pdeathsig及subreaper
处理孤儿。Landlock只做best effort；同UID可读共享文件的残余风险由受信代码审核兜底。
不要在stdout输出普通日志（stdout专供RPC）；stderr有截断/限速，宿主module=plugins、
instance字段用于过滤。不得把私密日志、配置或令牌提交到公共仓库或CI工件。

本机测试用media_tool_test/Redis DB15且串行。真实隔离不能以macOS或CGO/race降级测试
冒充；主CI `scripts/test-plugin-kernel.sh`在原生AMD64验证静态worker、父proc拒读、
实例目录、Landlock warning、FD/rlimit、崩溃收敛与double-fork收割。
