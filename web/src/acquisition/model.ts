import type { FlowStatus } from '../api/client';
export interface Media {
  id: number;
  mediaType: string;
  title: string;
  year?: string;
  originalTitle?: string;
  overview?: string;
  posterPath?: string;
  genres?: string[];
}
export interface Account {
  id: string;
  kind: string;
  name: string;
  enabled: boolean;
}
export interface Resource {
  id: string;
  title: string;
  siteName: string;
  size?: number;
  seeders?: number;
  leechers?: number;
  downloadFactor: number | null;
  metadata?: {
    subtitle?: string;
    yearWarning?: string;
    attributes?: {
      resolution?: string;
      source?: string;
      codec?: string;
    };
  };
}
export interface Run {
  id: string;
  keyword: string;
  state: string;
  total?: number;
  excluded?: number;
  media?: Media;
  createdAt?: string;
  sites: {
    accountId: string;
    name: string;
    state: string;
    message: string;
    count: number;
  }[];
  resources: Resource[];
}
export interface DownloadTask {
  resourceSize?: number;
  siteName?: string;
  id: string;
  resourceId: string;
  title: string;
  state: string;
  flow?: FlowStatus;
  media?: Media;
  progress: number | null;
  seedingState?: string;
  qbState?: string;
  downloadSpeed?: number;
  uploadSpeed?: number;
  seeds?: number;
  peers?: number;
  syncedAt?: string;
  syncError?: string;
  infoHash?: string;
  sourceRecoveryRequired?: boolean;
  sourceControlRevision?: number;
  cleanupState?: string;
  cleanupMessage?: string;
  message?: string;
  trackerError?: string;
  trackers?: {
    url: string;
    status: number;
    message: string;
  }[];
}
export const list = <T>(value: T[] | null | undefined): T[] => Array.isArray(value) ? value : [];
export { formatBytes as size } from '../utils/format-bytes';
export const progress = (n?: number | null) => n == null ? '—' : `${Math.max(0, Math.min(100, Math.round(n * 100)))}%`;
export const sameMedia = (a?: Media | null, b?: Media | null) => !!a && !!b && a.id === b.id && a.mediaType === b.mediaType;
export const stateLabels: Record<string, string> = { pending: '等待处理', running: '查询中', succeeded: '已完成', partial: '部分结果', failed: '失败', cancelled: '已取消', added: '已交给下载器', downloading: '下载中', local_complete: '本地完成，上传入库中', strm_ready: 'STRM 已生成，待核对入库证据', refresh_requested: '待核对入库证据', imported: '文件已入库', needs_output_reconciliation: '需核对输出布局', output_resume_approved: '已批准恢复入库', artifacts_pending: '输出待恢复', verified: '云端已校验', paused: '上传暂停', uploading: '上传中', uncertain: '结果待核对', starting: '正在建立上传', completed: '本地完成', submitting: '提交中', blocked: '提交受阻', absent_confirmed: '已核对未找到', not_found: '未找到', snapshot_unavailable: '记录暂不可用', invalid: '记录不可复用', cached: '可查看' };
export const label = (state?: string) => state ? stateLabels[state] || '状态待核对' : '—';
export const qbLabel = (state?: string) => ({ stalledDL: '下载停滞（等待有效连接）', downloading: '正在下载', metaDL: '获取元数据', checkingDL: '校验中', pausedDL: '下载已暂停', stoppedDL: '下载已停止', error: '下载器错误', missingFiles: '源文件缺失', queuedDL: '下载排队', allocating: '分配空间', uploading: '正在做种', stalledUP: '等待上传连接', pausedUP: '做种已暂停', stoppedUP: '做种已停止' } as Record<string, string>)[state || ''] || '状态未同步';
export { readIntent, writeIntent, type Intent } from '../workflows/intents'

export const seedingLabel = (state?:string) => ({pending:'待开始或未在做种',seeding:'持续做种中',stopped:'做种已停止',removed:'种子已移除'} as Record<string,string>)[state||'']||'—'
