import {beijingTime} from '../presentation/time';
import { APIError } from '../api/client'
export interface Media { id: number; mediaType: string; title: string; year?: string }
export interface Account { id: string; kind: string; name: string; enabled: boolean; secretFields?: string[] }
export interface Rules {
  accountIds: string[]; keyword: string; regexKeywords?: boolean; include: string[]; exclude: string[]
  resolution?: string; source?: string; codec?: string; minSize: number; maxSize: number; minSeeders: number
  freeOnly: boolean; rejectHR?: boolean; season: number; episodes: number[]; includeHistory: boolean
}
export interface Subscription { id: string; title: string; media: Media; enabled: boolean; source: string; rules: Rules; revision: number; lastState: string; lastMessage: string; nextRunAt: string|null }
export interface HistoryRun { id: string; state: string; preview: boolean; createdAt: string }
export interface Run { id: string; subscriptionId?: string; state: string; preview: boolean; decisions: Decision[] | null }
export interface Decision { id: string; title: string; action: string; reasons: string[]; episodes: number[]; checks?: { key: string; label: string; state: string; actual?: string; expected?: string; message: string }[] }
export interface PendingRun { subscriptionId: string; runId: string; preview: boolean; status: 'locked' | 'queued' | 'reconciling' }
export interface Draft {
  id: string; revision: number; media: Media | null; source: string; accountIds: string[]; keyword: string
  regexKeywords: boolean; includeText: string; excludeText: string; resolution: string; releaseSource: string; codec: string
  season: number; episodeText: string; freeOnly: boolean; rejectHR: boolean; includeHistory: boolean; enabled: boolean
  minSize: number; maxSize: number; minSeeders: number
}
export function draftFor(record: Subscription | null, accounts: Account[]): Draft {
  const r = record?.rules
  return { id: record?.id ?? '', revision: record?.revision ?? 0, media: record?.media ?? null, source: record?.source ?? 'search',
    accountIds: [...(r?.accountIds ?? accounts.map(a => a.id))], keyword: r?.keyword ?? '', regexKeywords: r?.regexKeywords ?? !record,
    includeText: r?.include?.join('\n') ?? '', excludeText: r?.exclude?.join('\n') ?? '', resolution: r?.resolution ?? '',
    releaseSource: r?.source ?? '', codec: r?.codec ?? '', season: r?.season ?? 1, episodeText: r?.episodes?.join(',') ?? '',
    freeOnly: r?.freeOnly ?? true, rejectHR: r?.rejectHR ?? true, includeHistory: r?.includeHistory ?? false, enabled: record?.enabled ?? false,
    minSize: r?.minSize ?? 0, maxSize: r?.maxSize ?? 0, minSeeders: r?.minSeeders ?? 0 }
}
export function payloadFor(d: Draft) {
  if (!d.media?.id || !d.accountIds.length) throw new Error('请先选择目标影视和至少一个 PT 账号')
  const episodes = d.episodeText.trim() ? d.episodeText.split(',').map(v => Number(v.trim())) : []
  if (episodes.some(n => !Number.isInteger(n) || n < 1)) throw new Error('集数请使用逗号分隔的正整数')
  const split = (text: string) => text.split(d.regexKeywords ? /\r?\n/ : /,|\r?\n/).map(v => v.trim()).filter(Boolean)
  return { mediaType: d.media.mediaType, tmdbId: d.media.id, source: d.source, revision: d.revision, enabled: d.enabled,
    rules: { accountIds: d.accountIds, keyword: d.keyword, regexKeywords: d.regexKeywords, include: split(d.includeText), exclude: split(d.excludeText),
      resolution: d.resolution, source: d.releaseSource, codec: d.codec, season: d.season, episodes, freeOnly: d.freeOnly, rejectHR: d.rejectHR,
      includeHistory: d.includeHistory, minSize: d.minSize, maxSize: d.maxSize, minSeeders: d.minSeeders } }
}
export const discovery: Record<string, string> = { search: '周期搜索', rss: 'RSS 新条目', both: '搜索 + RSS' }
export const outcomes: Record<string, string> = { skipped: '跳过', needs_confirmation: '需要人工确认', would_download: '预演通过 · 可下载', download_queued: '已加入下载队列', download_failed: '下载创建失败' }
export function quality(r: Rules | Draft) { return [r.resolution, ('releaseSource' in r ? r.releaseSource : r.source)?.toUpperCase(), r.codec?.toUpperCase()].filter(Boolean).join(' · ') || '不限画质' }
export const timestamp=beijingTime
export function asError(e: unknown) {
  if (e instanceof TypeError && /fetch|network|load/i.test(e.message)) return new Error('暂时无法连接服务器，请检查网络后重新读取。')
  return e instanceof Error ? e : new Error('请求未完成，请重新读取状态')
}
export function stateFor(e: Error) { return e instanceof APIError && [401, 403].includes(e.status) ? 'unauthorized' : e instanceof APIError && (e.status === 501 || /unsupported|not_supported/.test(e.code)) ? 'unsupported' : 'error' }
export function stateTitle(e: Error, subject: string) { return stateFor(e) === 'unauthorized' ? `没有访问${subject}的权限` : stateFor(e) === 'unsupported' ? `当前服务不支持${subject}` : `${subject}暂不可用` }
export function failure(e: unknown) {
  const err = asError(e)
  return /revision|subscription changed|修订|版本.*冲突|已.*修改/.test(err.message) || (err instanceof APIError && /revision|stale|version_conflict/.test(err.code))
    ? '订阅已被其他操作修改，请刷新列表后重新编辑。' : err.message
}
export const pendingStorageKey = 'media-tool:subscription-pending:v1'
export function restorePending(): PendingRun | null {
  try {
    const p = JSON.parse(sessionStorage.getItem(pendingStorageKey) ?? 'null')
    return p && typeof p.subscriptionId === 'string' && typeof p.runId === 'string' && typeof p.preview === 'boolean'
      ? { subscriptionId: p.subscriptionId, runId: p.runId, preview: p.preview, status: 'reconciling' } : null
  } catch { return null }
}
