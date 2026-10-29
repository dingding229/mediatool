import { failureText, jobNames } from '../presentation/failures';
import {jobObject} from '../presentation/job-object';
import type { InboxItem, Slot } from './model';
const list = (data: any): any[] => Array.isArray(data) ? data : Array.isArray(data?.items) ? data.items : [];
const time = (row: any) => Number.isFinite(Date.parse(row.updatedAt || row.createdAt)) ? Date.parse(row.updatedAt || row.createdAt) : 0;
export const hasResourceCount=(value:unknown)=>typeof value==='number'&&Number.isInteger(value)&&value>=0;
export function recentSearches(data: any) {
  const seen = new Set<string>();
  return [...list(data)].sort((a, b) => time(b) - time(a)).filter(r => {
    const key = String(r.keyword || '').trim();
    if (!key || seen.has(key))
      return false;
    seen.add(key);
    return true;
  }).slice(0, 5);
}
export function failedJobTarget(row: any):Pick<InboxItem,'path'|'query'> {
  const p = row.payload && typeof row.payload === 'object' ? row.payload : {};
  const id = p.accountId || p.integrationId;
  return row.kind?.startsWith('site.') && typeof id === 'string' && id ? { path: '/sites', query: { tab: 'accounts', accountId: id } } : { path: '/ops', query: { tab: 'jobs', q: row.id } };
}
export function failedJobs(slots: Record<string, Slot>): InboxItem[] {
  const groups = new Map<string, InboxItem>();
  for (const r of list(slots.jobs?.data)) {
    const {field,object,name:title}=jobObject(r,slots.accounts?.data,[...list(slots.transfers?.data),...list(slots.manual?.data)]);
    const key = JSON.stringify([r.kind, field || '', object]);
    const latestAt = r.updatedAt || r.createdAt;
    const old = groups.get(key), count = (old?.count || 0) + 1;
    if (old && time({ updatedAt: old.latestAt }) > time(r)) {
      old.count = count;
      continue;
    }
    groups.set(key, { id: r.id, title: (jobNames[r.kind] || r.kind || '后台任务') + '失败' + (title ? ' · ' + title : ''), note: failureText(r.lastError), count, latestAt,
      ...failedJobTarget(r) });
  }
  return [...groups.values()].sort((a, b) => time({ updatedAt: b.latestAt }) - time({ updatedAt: a.latestAt }));
}
// Only collapse identical records from the same source/object; never merge by title alone.
export function uniqueItems(items: InboxItem[]) {
  const seen = new Set<string>();
  return items.filter(i => {
    const key = JSON.stringify([i.path, i.query, i.id, i.note]);
    if (seen.has(key))
      return false;
    seen.add(key);
    return true;
  }).map(i => ({ ...i, note: i.note ? failureText(i.note) : undefined }));
}
