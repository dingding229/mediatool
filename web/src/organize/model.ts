export const rows = (v: any): any[] => Array.isArray(v) ? v : [];
export const names: Record<string, string> = { needs_confirmation: '待确认', pending: '等待处理', running: '进行中', paused: '已暂停', failed: '失败', cancelled: '已取消', confirmed: '已确认', artifacts_pending: '输出待恢复', needs_output_reconciliation: '需核对输出', output_resume_approved: '已批准恢复入库', ready: '输出就绪', present: '文件在库', missing: '文件缺失', suspected_missing: '待完整对账', trashed: '已回收', succeeded: '已完成', partial: '部分完成', needs_reconcile: '结果待核对', conflict: '冲突', imported: '已入库', visible: 'Emby 已可见', applied: '输出已落盘', verified: '已校验', retained: '保留中', approved: '已批准，待继续', quarantined: '已隔离，待完成', removed: '已清理（审计保留）', completed: '已完成', expired: '已过期', connected: '已连接', connecting: '正在连接', waiting: '等待重连', unavailable: '暂不可用', unsupported: '当前不支持', disabled: '未启用' };
export const label = (v?: string) => v ? names[v] || '状态待核对' : '—';
export const selectable = (row: any) => row.cloudState === 'present' && !['confirmed', 'artifacts_pending', 'needs_output_reconciliation'].includes(row.state);
export function episodes(text: string, series: boolean) {
  const out = text.trim() ? text.split(',').map(v => Number(v.trim())) : [];
  if (out.some(v => !Number.isInteger(v) || v < 1 || v > 9999) || new Set(out).size !== out.length || series && !out.length)
    throw Error('请逐文件填写实际覆盖集：逗号分隔、不重复的正整数。');
  return out;
}
export const removable = (row: any) => ['file', 'batch', 'scan', 'transfer'].includes(row.kind) && ['succeeded', 'partial', 'failed', 'cancelled', 'expired', 'imported'].includes(row.state) && !(row.kind === 'transfer' && !['imported', 'cancelled'].includes(row.state));
export const eligible = (row: any) => row.state === 'retained' && !row.deletedAt && Number.isFinite(Date.parse(row.eligibleAfter)) && Date.parse(row.eligibleAfter) <= Date.now();
export function savePrivate(key: string, value: unknown) {
  try {
    if (value == null)
      sessionStorage.removeItem(key);
    else
      sessionStorage.setItem(key, JSON.stringify(value));
  }
  catch {
  }
}
export function readPrivate(key: string) {
  try {
    return JSON.parse(sessionStorage.getItem(key) || 'null');
  }
  catch {
    return null;
  }
}
