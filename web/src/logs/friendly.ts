import { logLevel, type LogRecord } from "./format";
const modules: Record<string, string> = {
  app: "主程序",
  httpapi: "接口访问",
  playback: "播放服务",
  search: "资源搜索",
  subscriptions: "资源订阅",
  subscription: "资源订阅",
  library: "媒体入库",
  transfer: "文件传输",
  downloads: "下载管理",
  worker: "后台任务",
  scheduler: "定时任务",
  maintenance: "系统维护",
  tmdb: "TMDB元数据",
  notifications: "通知服务",
};
const stages: Record<string, string> = {
  resolve: "获取播放地址",
  cache_version: "核对播放缓存版本",
  cache_lookup: "查询播放缓存",
  cache_hit: "复用有效缓存",
  cache_miss: "缓存未命中",
  cache_joined: "合并相同取链请求",
  cache_wait: "等待取链结果",
  official_http_refresh: "通过HTTP刷新115授权",
  target_policy: "检查播放地址安全性",
  direct_conditions: "检查直连条件",
  direct_conditions_met: "满足直连条件",
  direct_validation_failed: "直连检查未通过",
  official_path: "定位115文件",
  official_path_lookup: "定位115文件",
  official_download: "获取115链接",
  official_http_path: "通过HTTP定位115文件",
  official_http_download: "通过HTTP获取115链接",
  official_token: "准备115授权",
  official_refresh: "刷新115授权",
  cloud_file_lookup: "查找云盘文件",
  cloud_profile_lookup: "读取云盘配置",
  source_queue: "等待上游解析",
  source_resolve: "解析存储来源",
  cdn_check: "检查CDN地址",
  file_identity_check: "核对文件身份",
  download_identity_check: "核对下载身份",
  official_link: "获取115临时链接",
  cd2_path: "定位云盘文件",
  cd2_link: "获取云盘链接",
  target_validation: "检查播放链接",
  direct_validate: "验证播放器直连",
  request_cancelled: "请求已取消",
};
const messages: Record<string, string> = {
  "HTTP stream aborted": "媒体连接已中止",
  "worker starting": "后台任务服务正在启动",
  "job claimed": "后台任务已开始处理",
  "job claim failed": "后台任务领取失败",
  "job heartbeat lost": "任务执行连接已中断",
  "job completion not committed": "任务结果未能保存，请先核对状态",
  "worker heartbeat unavailable": "后台任务心跳暂不可读取",
  "startup migrations complete": "启动检查与数据库准备完成",
  "HTTP server started": "管理服务已启动",
  "Media member listener started": "独立媒体入口已启动",
  "Emby gateway listener started": "Emby播放网关已启动",
  "runtime configuration validated": "启动配置检查通过",
  "scheduler reconciliation cycle": "正在检查到期任务",
  "application settings updated": "系统设置已保存",
  "public domain settings updated": "访问域名已保存",
  "application file logs cleared by administrator": "管理员已清理程序日志",
  "manual torrent queued": "手动下载已加入队列",
  "manual download sync failed": "手动下载进度同步失败",
  "download status observed": "已同步下载器进度",
  "download status synchronization failed": "下载器进度暂不可读取",
  "player UA denied": "播放器未通过UA白名单检查",
  "player UA admitted; media authorization still required":
    "播放器UA检查通过，仍需验证媒体访问权限",
  "player UA policy unavailable": "播放器访问规则暂不可读取",
  "HTTP panic recovered": "程序处理请求时发生内部错误",
  "download reconciliation unavailable": "下载任务核对暂不可用",
  "notification delivery unavailable": "通知发送暂不可用",
  "subscription schedule unavailable": "订阅调度暂不可用",
  "site metrics schedule unavailable": "站点数据刷新暂不可用",
  "library schedule unavailable": "媒体扫描调度暂不可用",
  "maintenance scheduling failed": "系统维护调度失败",
  "transfer preparation unavailable": "上传准备暂不可用",
  "artifact recovery unavailable": "托管输出恢复暂不可用",
};
export const moduleLabel = (value: unknown) =>
  modules[String(value ?? "app")] || String(value ?? "主程序");
export const levelLabel = (value: unknown) =>
  ({
    debug: "调试",
    info: "信息",
    warn: "警告",
    error: "错误",
    unknown: "未知",
  })[logLevel(value)];
export const stageLabel = (value: unknown) =>
  stages[String(value)] || String(value);
const outcomes: Record<string, string> = {
  ok: "完成",
  error: "失败，请查看详情",
  failed: "失败，请查看详情",
  cancelled: "请求已取消",
  timeout: "处理超时",
  authorization: "授权未通过",
  forbidden: "无访问权限",
  invalid_response: "服务返回了无法识别的数据",
  rate_limited: "请求频繁，稍后重试",
  provider_rejected: "上游服务拒绝请求",
  unavailable: "服务暂不可用",
};
export function friendlyMessage(row: LogRecord): string {
  const msg = String(row.msg ?? "未记录说明"),
    status = Number(row.status);
  if (msg === "job finished")
    return row.success === true
      ? "后台任务已完成"
      : "后台任务未完成，请查看详情";
  if (msg === "HTTP request") {
    if (!Number.isFinite(status) || status < 100)
      return "接口请求记录，结果尚未确认";
    if (status >= 300 && status < 400)
      return row.location
        ? "已返回跳转地址，等待客户端直连"
        : "请求已返回重定向响应";
    if (status === 401) return "请求未通过身份验证";
    if (status === 403) return "请求被访问规则拒绝";
    if (status >= 500) return "服务暂时无法完成请求";
    if (status >= 400) return "请求参数或资源状态需要核对";
    const path = String(row.path ?? row.url ?? "");
    if (/strm302|\/Videos\//i.test(path)) return "已处理播放请求";
    if (path.includes("/search")) return "已处理资源搜索请求";
    if (path.includes("/subscriptions")) return "已处理订阅请求";
    return "接口请求已处理";
  }
  if (msg === "Playback resolution stage")
    return `${stageLabel(row.stage)}：${outcomes[String(row.outcome)] || String(row.outcome ?? "未记录结果")}`;
  return messages[msg] || msg;
}
export function logAdvice(row: LogRecord): string {
  const status = Number(row.status),
    reason = String(row.reason ?? row["X-Media-Gateway-Reason"] ?? "");
  if (reason.includes("cancel") || row.msg === "HTTP stream aborted")
    return "客户端可能取消或切换了播放。若反复发生，请结合相同请求ID检查后续记录；中止不等于服务器返回502。";
  if (row.msg === "player UA denied" || reason.includes("ua_"))
    return "在播放器访问页面核对此UA，确认是自己的播放器后再加入白名单。";
  if (status === 401)
    return "检查账号会话或对应服务凭据是否有效；播放凭据与管理员密码不能混用。";
  if (status === 403) return "检查播放器白名单、媒体用户权限和反代来源配置。";
  if (status >= 500 || logLevel(row.level) === "error")
    return "先检查对应服务连接和网络是否正常，再用请求ID关联详细日志。不要重复执行结果未知的上传、下载或删除操作。";
  if (row.location)
    return "这里表示服务器已返回跳转地址，不代表客户端已成功播放；临时115链接可能过期。查看与复制不会访问该地址。";
  return "";
}
export function friendlyFields(row: LogRecord): string {
  const parts: string[] = [];
  if (row.status != null) parts.push(`HTTP ${row.status}`);
  if (row.durationMs != null) parts.push(`耗时 ${row.durationMs} ms`);
  if (row.responseBytes != null) parts.push(`响应 ${row.responseBytes} 字节`);
  if (row.stage != null) parts.push(stageLabel(row.stage));
  if (row.count != null) parts.push(`${row.count} 项`);
  if (row.clientIp) parts.push(`IP ${row.clientIp}`);
  return parts.join(" · ");
}
