import {request,post,checkedPage,type ListPage} from './client'
export interface MediaGrant { libraryId: string; canPlay: boolean; canDownload: boolean }
export interface MediaUser { lastPlayedAt?: string | null; lastIp?: string; deviceCount?: number; lockedUntil?: string | null; maxStreams: number; maxParentalRating: number | null; expiresAt: string | null; id: string; username: string; displayName: string; enabled: boolean; mustChangePassword: boolean; revision: number; grants: MediaGrant[]; activeSessions: number; createdAt: string; updatedAt: string }
export interface MediaLibrary { id: string; name: string; mediaType: string; canPlay: boolean }
export interface MediaProvider { id: string; name: string; contract: string; runtime: string; stage: string; note: string; capabilities: Record<string, boolean> }
export interface MediaItem { id: string; title: string; year: string; mediaType: string; overview: string; poster: string; season?: number; episode?: number }
export const listMediaUsers = (page=1) => request<ListPage<MediaUser>>(`/media-server/users?page=${page}&pageSize=25`).then(checkedPage<MediaUser>)
export const listNativeLibraries = () => request<MediaLibrary[]>('/media-server/libraries')
export interface NativeTMDBTarget { type: 'movie' | 'series' | 'season'; tmdbId: number; season?: number }
export const refreshNativeTMDB = (target: NativeTMDBTarget) => post<{ detailId: string; queued: boolean }>('/media-server/native/tmdb/refresh', target)
export const retryNativeProbe = (assetId: string) => request<{ assetId: string; queued: boolean }>(`/media-server/native/probe/${encodeURIComponent(assetId)}/retry`, { method: 'POST' })

export type ProcessingKind = 'probe' | 'still'
export interface ProcessingSettings { enabled: boolean; probeEnabled: boolean; stillsEnabled: boolean; revision: number }
export interface ProcessingSummary {
 settings: ProcessingSettings
 probe: { state: string; count: number }[]
 still: { state: string; count: number }[]
 capacity: { usedBytes: number; reservedBytes: number; limitBytes: number; unknownSizes: number }
}
export interface ProcessingTask { id: string; name: string; state: string; errorReason: string; attempts: number; nextAttemptAt: string | null }
export interface ProcessingPage { items: ProcessingTask[]; total: number; page: number; pageSize: number }
export interface ProcessingRetry { kind: ProcessingKind; id: string; state: string; bulk: boolean; confirmed: boolean }
export interface ProcessingRetried { selected: number; queued: number; skipped: number; failed: number; limitReached: boolean }
export interface ProcessingGC { scanned: number; reclaimed: number; reclaimedBytes: number; done: boolean; partial: boolean }
const processing = '/media-server/native/processing'
export const getProcessing = () => request<ProcessingSummary>(processing)
export const saveProcessing = (settings: ProcessingSettings) => request<ProcessingSettings>(processing + '/settings', { method: 'PUT', body: JSON.stringify(settings) })
export const listProcessing = (kind: ProcessingKind, state: string, page: number, pageSize: number) => request<ProcessingPage>(processing + '/tasks?' + new URLSearchParams({ kind, state, page: String(page), pageSize: String(pageSize) }))
export const retryProcessing = (input: ProcessingRetry) => post<ProcessingRetried>(processing + '/retry', input)
export const collectProcessing = () => request<ProcessingGC>(processing + '/gc', { method: 'POST' })

export interface ManagementPage<T> { items: T[]; total: number; startIndex: number; limit: number }
export interface MediaDevice { lastIp?:string; id: string; name: string; client: string; appVersion: string; activeTokens: number; lastSeenAt: string; createdAt: string }
export interface MediaHistory {season?:number|null;episode?:number|null;episodeTitle?:string;durationSeconds?:number; itemId: string; name: string; type: string; positionTicks: number; playCount: number; played: boolean; lastPlayedAt: string }
export interface NativeSession {type?:string;season?:number|null;episode?:number|null;episodeTitle?:string; id: string; userId: string; userName: string; deviceId: string; deviceName: string; itemId: string; itemName: string; state: string; positionTicks: number; lastProgressAt: string }
const userPath = (id: string) => '/media-server/users/' + encodeURIComponent(id)
const pageQuery = (startIndex: number, limit: number) => '?' + new URLSearchParams({ startIndex: String(startIndex), limit: String(limit) })
export const listMediaDevices = (id: string, startIndex = 0, limit = 50) => request<ManagementPage<MediaDevice>>(userPath(id) + '/devices' + pageQuery(startIndex, limit))
export const revokeMediaDevice = (id: string, deviceId: string) => request<{ revoked: boolean }>(userPath(id) + '/devices/' + encodeURIComponent(deviceId) + '/revoke', { method: 'POST' })
export const listMediaHistory = (id: string, startIndex = 0, limit = 50) => request<ManagementPage<MediaHistory>>(userPath(id) + '/history' + pageQuery(startIndex, limit))
export const listNativeSessions = (startIndex = 0, limit = 50) => request<ManagementPage<NativeSession>>('/media-server/native/sessions' + pageQuery(startIndex, limit))
export interface MediaUserInput { username: string; displayName: string; enabled: boolean; mustChangePassword: boolean; revision: number; grants: MediaGrant[]; password?: string; maxStreams?: number; maxParentalRating?: number | null; expiresAt?: string | null }
export const saveMediaUser = (id: string, input: MediaUserInput) => request<MediaUser>(id ? userPath(id) : '/media-server/users', { method: id ? 'PUT' : 'POST', body: JSON.stringify(input) })
export interface NativeServerInput { serverName: string; enabled: boolean; revision: number }
export interface NativeServerSettings extends NativeServerInput { publicUrl: string; listenPort: string; addressSource: string }
export const getNativeServerSettings = () => request<NativeServerSettings>('/media-server/native/settings')
export const saveNativeServerSettings = (input: NativeServerInput) => request<NativeServerInput>('/media-server/native/settings', { method: 'PUT', body: JSON.stringify(input) })
export interface NativeLibrary { id: string; itemId: string; name: string; collectionType: string; hidden: boolean; sortOrder: number; groupId: string; ruleId: string; groupName: string; ruleName: string; assetCount: number }
export const listNativeLibraryMappings = (startIndex = 0, limit = 50) => request<ManagementPage<NativeLibrary>>('/media-server/native/library-mappings' + pageQuery(startIndex, limit))
