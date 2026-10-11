# 自有小文件的真实 CloudDrive2 上传验证

更新：2026-10-10。此验收验证实际CloudDrive2/115上传和字节一致性，不伪造PT搜索、下载/做种、电影身份、STRM或Emby入库记录。

## 已获得的真实证据

在用户授权的本项目目录和既有CD2连接上，使用本项目自有的`Owned-Test-5s.mp4`：

- 原始本地文件49,982字节，只复制到本轮私密诊断目录，不移动/改名原文件。
- 通过生产`clouddrive2.Adapter.UploadFile`启动真实远程上传，upload_id在继续协议之前持久化到0600日志。
- 云端目的位置限制为配置的允许根下`.mediatool-upload-check/<runUUID>/Owned-Test-5s.mp4`，不覆盖已有文件、不清理用户媒体。
- CloudDrive2报告完成后，重新Find验证大小49,982及SHA-1一致。
- 再通过生产播放引擎的URL/DNS/拨号/TLS检查、受控Range读取整个小文件，HTTP206、49,982字节、SHA-256与本地一致。
- 未创建PT/下载业务记录，未给测试视频套用TMDB电影身份，未改已有授权/账号/主密钥。

服务器诊断目录：`/root/docker/mediatool/private/owned-upload-check-20261010`。其中upload.json含会话ID/目的路径，保持私密，不导出到公开日志或聊天。公开脱敏报告本地为data/remote-deploy/owned-upload-{preflight,result}.json；不含凭据和临时直链。

**完成和全文件校验通过，不代表已经证明网络实际上传了49,982字节。** CD2/115可能秒传/去重；该轮未统计逐ReadData确认字节，不伪造正常上传吞吐证据。协议的数据请求/哈希/重复区间另有fixture测试；PT资源真实上传与做种保留仍未通过。

## 可重复的维护工具

`cmd/cloud-upload-smoke`是本地操作者维护入口，镜像包含`/app/cloud-upload-smoke`，不是对公网开放的上传API。

限制：

1. 必须显式`-write-confirmed`；只接受basename为Owned-Test-5s.mp4的普通文件，0–2MiB（不含0）；仅用于操作者自有测试视频。
2. 使用数据库已启用CD2配置，配置cloudRoot必须在STRM_ALLOWED_ROOTS内；目标由工具固定生成，不能任意指定或越界。
3. 先记录starting，再发外部启动；OnStart持久化会话，之后才能继续数据协议。
4. 已有journal绝不自动重建会话；仅`-resume-confirmed`且连接ID/revision、源大小/摘要、固定目的路径一致才可继续。无session ID的未知启动需人工核对。
5. 完成进度不算通过；必须云大小/可用SHA-1和受控全字节SHA-256校验，最后再次验证源内容未变。
6. 全部日志原子替换/fsync、0600；公开输出仅状态/字节/匹配布尔值，不含会话ID、云路径、Token或签名URL。默认5分钟总限时，失败保留journal，不盲重启、不自动删除云文件。

示例（首次独立诊断；要先准备自己拥有的合法文件）：

```bash
# 私密目录及其source子目录只对容器APP_UID开放；不要递归修改已有下载目录。
# /private/path/upload-check/source/Owned-Test-5s.mp4 是只读输入。
docker compose --env-file .env -f deploy/compose.yaml run --rm --no-deps -T \
  -v /private/path/upload-check:/diagnostic \
  --entrypoint /app/cloud-upload-smoke server \
  -source /diagnostic/source/Owned-Test-5s.mp4 \
  -report /diagnostic/upload.json -write-confirmed
```

目录必须持久化，不能只用临时容器/tmp。源和journal已存在时不能再次假装首次运行；确认需核对同一会话后在相同命令追加-resume-confirmed。已完成文件存在时只再次验证，不发新的StartRemoteUpload。不自动撤销/清理远端测试文件，会话/文件留给操作者核对。

## 尚未证明

PT代表站点账号搜索/种子获取、qBittorrent真实下载和做种保留、业务任务的上传对账恢复、影视身份确认/STRM/NFO/Emby/通知全链路仍待逐项验收。不要把此诊断文件硬套某电影TMDB ID来“凑通”入库链路。
