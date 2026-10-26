import {failureText} from '../presentation/failures';
import {failedJobs,uniqueItems} from './summaries';
import type { Readiness } from '../workflows/readiness';
export interface InboxItem {
  id: string;
  title: string;
  note?: string;
  path: string;
  query: Record<string, string>;
  alert?: string;
  count?:number;
  latestAt?:string;
}
export interface InboxGroup {
  id: string;
  title: string;
  items: InboxItem[];
  errors: string[];
}
export interface Slot<T = any> {
  data: T | null;
  loading: boolean;
  error: unknown;
}
export const list = (data: any): any[] => Array.isArray(data) ? data : Array.isArray(data?.items) ? data.items : [];
export {failedJobTarget} from './summaries';
export function inboxGroups(slots: Record<string, Slot>): InboxGroup[] {
  const groups: InboxGroup[] = [
    { id: 'jobs', title: '失败任务', items: failedJobs(slots), errors: [] },
    { id: 'candidates', title: '待确认识别', items: list(slots.candidates?.data).filter(r => ['pending', 'needs_confirmation', 'needs_output_reconciliation'].includes(r.state)).map(r => ({ id: r.id, title: r.name || r.path || '—', path: '/organize', query: { tab: 'candidates', candidateId: r.id } })), errors: [] },
    { id: 'uploads', title: '上传结果待核对', items: [...list(slots.transfers?.data).filter(r => r.state === 'uncertain' || ['reconcile_upload', 'preview_output'].includes(r.flow?.nextAction)).map(r => ({ id: r.id, title: r.destination || '上传任务', note: r.message, path: '/downloads', query: { tab: 'uploads', transferId: r.id } })), ...list(slots.manual?.data).filter(r => r.state === 'uncertain').map(r => ({ id: r.id, title: r.title || '手动下载', note: r.message, path: '/downloads', query: { tab: 'manual', taskId: r.id } }))], errors: [] },
    { id: 'batches', title: '批次结果待核对', items: list(slots.batches?.data).filter(r => r.state === 'needs_reconcile').map(r => ({ id: r.id, title: r.message?failureText(r.message):'文件批次待核对', path: '/organize', query: { tab: 'history', kind: r.kind || 'batch', batchId: r.batchId || r.id } })), errors: [] },
    { id: 'alerts', title: '未确认告警', items: (slots.alerts?.data?.alerts ?? []).filter((r: any) => r.state === 'active' && !r.acknowledged_at).map((r: any) => ({ id: r.code, title: r.message?failureText(r.message):'需要关注的系统告警', path: '/ops', query: { tab: 'maintenance' }, alert: r.code })), errors: [] },
    { id: 'plugins', title: '插件权限待审核', items: list(slots.plugins?.data).filter(r => r.reason === 'permission_review_required').map(r => ({ id: r.id, title: r.metadata?.name || '插件', note: '权限已变化，实例已停止或正在停止；需要重新确认', path: '/plugins', query: { instance: r.id } })), errors: [] },
  ];
  for (const group of groups) {
    group.items=uniqueItems(group.items);
    for (const source of group.id === 'uploads' ? ['transfers', 'manual'] : [group.id])
      if (slots[source]?.error)
        group.errors.push(source);
  }
  return groups;
}
export function guideSteps(readiness: Readiness | null, integrations: any, classification: any) {
  const connected=(key:string)=>['ready','check_required'].includes(readiness?.components?.[key]?.state||'');
  const needsCheck=(keys:string[])=>keys.some(k=>readiness?.components?.[k]?.state==='check_required');
  return [
    { label: '连接 115/CD2', done: connected('open115')&&connected('clouddrive2'), note:connected('open115')&&connected('clouddrive2')&&needsCheck(['open115','clouddrive2'])?'已连接，需要检测':'', path: '/connections', tab: 'storage' },
    { label: '添加站点', done: list(integrations).some(r => r.kind === 'ptsite' && r.enabled), path: '/sites', tab: 'accounts' },
    { label: '设置分类规则', done: !!classification?.value?.groups?.some((g: any) => g.enabled && g.directory), path: '/settings', tab: 'naming' },
  ];
}
