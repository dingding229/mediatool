export const stateLabel = (s?: string) => s ? ({ pending: '等待中', running: '执行中', retry: '等待重试', succeeded: '已完成', failed: '失败', paused: '已暂停', cancelled: '已取消', ready: '备份就绪', verified: '演练通过', interrupted: '已中断', deleted: '已删除', deleting: '删除中', preview: '待审批', approved: '待服务器执行', executing: '服务器执行中', completed: '执行完成（待核验）', expired: '已过期', active: '告警中', normal: '正常', checking: '核对中', unknown: '状态未知', disabled: '已关闭', partial: '部分完成' } as Record<string, string>)[s] || '状态待核对' : '—';
export const queueLabel = (s: string) => ({ default: '默认队列', library: '媒体入库', transfer: '文件传输', maintenance: '系统维护', probe:'媒体探测',still:'截图采集',notification:'通知投递',plugin:'插件任务',scheduler:'计划调度' } as Record<string, string>)[s] || (s?'其他任务':'—');
export const jobKindLabels:Record<string,string>={
 'system.check':'系统检查','search.run':'站点资源搜索','download.add':'下载提交','download.manual.add':'手动下载提交','download.manual.import':'手动下载识别入库','download.cleanup':'下载源清理','transfer.upload':'上传入库','transfer.import.resume':'恢复已有入库','library.scan':'云盘扫描','library.strm.repair':'STRM修复','files.batch':'文件整理批次','library.refresh':'媒体库刷新','library.emby.confirm':'媒体库可见性核对','emby.refresh':'Emby刷新','emby.confirm':'Emby可见性核对','native.probe':'媒体探测','native.still':'媒体截图','native.assign':'原生媒体归属','native.tmdb.fetch':'TMDB详情采集','ops.backup':'系统备份','ops.drill':'恢复演练','subscription.run':'订阅检查','subscription.evaluate':'订阅规则评估','site.metrics.refresh':'站点数据刷新','plugin.v2.workflow':'插件工作流','plugin.v2.service':'插件服务','plugin.service':'旧插件服务收尾','plugin.dispatch':'旧插件分发收尾'
};
export const kindLabel=(s:string)=>jobKindLabels[s]||s||'—';
export const canDeleteBackup = (state: string) => ['ready', 'failed', 'interrupted'].includes(state);
export const canPinBackup = (state: string) => state === 'ready';
export const ticketLabel = (ticket: any) => ticket.flow?.reason === 'approval_expired' ? '审批已过期，请重新预览' : ticket.verifiedAt ? '核验通过（仍须明确解除维护）' : stateLabel(ticket.state);
export const policyFields = [{ key: 'keepDays', label: '备份保留天数', min: 1, max: 365 }, { key: 'keepCount', label: '备份最大份数（至少两份）', min: 2, max: 100 }, { key: 'sampleDays', label: '采样保留天数', min: 1, max: 90 }, { key: 'memoryPercent', label: '内存使用率阈值 %', min: 1, max: 100 }, { key: 'diskFreeGB', label: '剩余空间阈值 GiB', min: 0, max: 100000 }, { key: 'loadPerCPU', label: '每CPU负载阈值', min: 0.1, max: 100, step: 0.1 }, { key: 'failedJobs', label: '十分钟失败任务阈值', min: 1, max: 100000 }, { key: 'consecutive', label: '连续命中次数', min: 1, max: 60 }, { key: 'cooldownMinutes', label: '重复提醒冷却（分钟）', min: 5, max: 10080 }];
export function trendSegments(rows: any[], metric: string, gapMs: number) {
  const values = rows.map((row, i) => ({ i, time: Date.parse(row.at), value: row[metric] })).filter(v => typeof v.value === 'number' && Number.isFinite(v.value) && Number.isFinite(v.time));
  if (!values.length)
    return [];
  const min = Math.min(0, ...values.map(v => v.value)), max = Math.max(1, ...values.map(v => v.value)), start = values[0]!.time, end = values.at(-1)!.time, groups: string[][] = [];
  let prior: any;
  for (const v of values) {
    if (!prior || v.i !== prior.i + 1 || v.time - prior.time > gapMs * 1.5)
      groups.push([]);
    groups.at(-1)!.push(`${8 + (end > start ? (v.time - start) / (end - start) : 0) * 304},${92 - (v.value - min) / (max - min) * 80}`);
    prior = v;
  }
  return groups.map(g => g.join(' '));
}
export function taskTarget(row: any) {
  return row.kind === 'native.probe' || row.kind === 'native.still' ? { path: '/playback', query: { tab: 'processing', kind: row.kind === 'native.probe' ? 'probe' : 'still', jobId: row.id } } : row.queue === 'transfer' ? { path: '/downloads', query: { tab: 'uploads' } } : row.queue === 'library' ? { path: '/organize', query: { tab: 'scan' } } : row.kind?.startsWith('subscription') ? { path: '/subscriptions', query: { tab: 'runs' } } : null;
}
export async function copyOriginal(text: string, fallback: (text: string) => void) {
  try {
    if (!navigator.clipboard?.writeText)
      throw Error('unavailable');
    await navigator.clipboard.writeText(text);
    return '已复制原始内容，请妥善保管。';
  }
  catch {
    fallback(text);
    return '复制失败，请在显示的字段内手动选择并复制。';
  }
}
export function pageResult(value: any) {
  if (!value || !Array.isArray(value.items))
    throw Error('列表回执不完整，请重新读取。');
  return value;
}
export { formatBytes } from '../utils/format-bytes';
export const requestStep = (r: any) => r?.flow?.reason === 'approval_expired' ? 2 : r?.verifiedAt || r?.state === 'completed' ? 4 : ['approved', 'executing'].includes(r?.state) ? 3 : 2;
