// Short explanations for inboxes only; original errors remain in task/log details.
export const failureReasons = [
  { match: /site request failed|site collection failed/i, text: '站点请求失败，请检查登录信息、网络或站点兼容性。' },
  { match: /unauthorized|authentication failed|invalid credentials|login (?:failed|required)|cookie.*expired/i, text: '登录信息不可用，请检查账号凭据并重新登录或授权。' },
  { match: /context deadline exceeded|request timeout|timed out|i\/o timeout/i, text: '请求超时，请检查网络和服务状态，再核对任务结果。' },
  { match: /connection refused|no such host|network is unreachable|dial tcp/i, text: '无法连接服务，请检查服务地址、网络和服务是否启动。' },
  { match: /permission denied|access denied|forbidden/i, text: '没有访问权限，请检查账号权限和允许访问的目录。' },
  { match: /no space left|disk.*full|insufficient.*space/i, text: '可用空间不足，请检查存储空间后再处理。' },
  { match: /file.*not found|no such file|source.*missing/i, text: '没有找到文件，请核对文件是否仍在原目录。' },
  { match: /revision.*(?:changed|conflict)|configuration.*changed/i, text: '配置已发生变化，请读取最新配置后再处理。' },
  { match: /rate limit|too many requests/i, text: '请求过于频繁，请稍后核对服务状态。' },
];
export function failureText(raw?: string) {
  return raw ? failureReasons.find(r => r.match.test(raw))?.text || raw : '—';
}
export const jobNames: Record<string, string> = { 'site.metrics.refresh': '站点数据更新', 'system.check': '系统检查', 'native.probe': '媒体信息读取', 'native.still': '媒体截图', 'search.run': '站点搜索', 'subscription.evaluate': '订阅运行', 'library.scan': '目录扫描', 'files.batch': '批量文件整理', 'library.strm.repair': 'STRM 修复', 'library.refresh': '媒体库更新', 'download.add': '添加下载', 'download.manual.add': '添加手动下载', 'download.cleanup': '下载清理', 'transfer.upload': '云盘上传',     'native.tmdb.fetch': '媒体资料更新', 'plugin.v2.service': '插件服务', 'plugin.v2.workflow': '插件工作流', 'ops.backup': '系统备份', 'ops.drill': '备份演练', 'transfer.import.resume': '入库恢复' };
