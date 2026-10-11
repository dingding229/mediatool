# 工作台改版使用与部署

## 功能入口

- PT站点：定义导入与账号添加/编辑；保存时自动只读验证。资源搜索分TMDB媒体发现/站点直搜，两类缓存时间在系统设置配置。
- 媒体库：Emby首页真实分类、播放、继续/近期观看、最新入库；选定用户范围；Emby添加/编辑在本页。
- 云盘索引：CloudDrive2及115独立授权；无Token115草稿先保存后扫码，授权成功后自动检查，不借用其他应用AppID。
- 下载与传输：下载器；通知中心：通知渠道/记录；系统设置：TMDB、电影/剧集模板与缓存；服务连接页只有状态汇总，不添加配置。
- 文件管理：自动本地目录与CloudDrive2文件。0day解析提供标题/年份/类型/季集/编码线索；默认全局模板，自定义名称仍可选。实际修改仍需预览确认，历史在整理历史。
- 程序日志：级别筛选、大小、文件清单、确认清理及运行时级别；数据库浏览：PG/Redis分页与格式化只读内容，敏感数据隐藏。
- 插件扩展：内置识别/命名，JSON Manifest/HTTPS Webhook能力；新扩展默认停用，启用须明确授权字段与地址。

## 单Compose部署

```sh
python3 scripts/init-compose.py
# 直接编辑docker-compose.yml的环境字段、镜像、端口、挂载和外部服务地址。
# 初始化后的私密Compose不可提交，保持0600；保管原主密钥，不能替换已有密钥。
docker compose config --quiet
docker compose pull
docker compose up -d --wait --wait-timeout 180
```

不再引用外部.env部署参数。已有安装升级时先备份，将原Docker实际环境、数据库密码、原业务卷/挂载复制到服务器私密Compose，不能运行初始化脚本替换旧密钥/数据库密码。仍只有app、postgres、redis。

已有Emby/CD2外部依赖容器保留，不执行 `--remove-orphans` 删除它们。数据目录映射建议 `/root/docker/mediatool/data:/data`；下载源仍可追加只读挂载。仅容器可见目录可以自动显示，不挂载宿主机根目录。

更新通过Actions发布的AMD64镜像：

```sh
cd /root/docker/mediatool
bash scripts/update-server.sh dingding229/mediatool:sha-<完整40位commit>
```

脚本只修改主镜像、保留直接参数及数据，先备份后拉取/检查修订/更新。不会上传本地镜像、自动向下迁移或删除业务卷。

密码恢复详见 `WORKBENCH_SECURITY.md`。空 `password` 文件操作仅由部署所有者完成，后台恢复入口已取消。实际恢复不会作为生产自动测试；隔离数据库验证不修改真实账号。

## 验收边界

MoviePilot只作搜索/缓存/扩展设计参考，未复制GPLv3 Python实现，非其插件市场兼容产品。文件名猜测不等于正确TMDB身份；取链与1KiB CDN验证不等于所有播放器能解码，严格直链仍拒绝HLS/转码/服务器中转。
