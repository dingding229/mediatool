import type { FileBatch, FileEntry } from './types'
export const roles: Record<string, string> = { workspace: '本地工作区', 'download-source': '下载 / 做种源', 'managed-output': '托管输出', 'cloud-target': '115 云盘', archive: '回收 / 归档区' }
export function canModify(file: FileEntry) { return !file.unsupported && file.capabilities?.write !== false && file.capabilities?.move !== false }
export function canApprove(batch: FileBatch | null) { return !!batch && batch.state === 'preview' && batch.items.length > 0 && batch.items.every(row => row.state === 'preview' && !!row.result) && new Date(batch.expiresAt).getTime() > Date.now() }
export function stepState(state: string, done: number, index: number) { return index < done ? 'succeeded' : index === done && ['running', 'needs_reconcile'].includes(state) ? 'needs_reconcile' : 'pending' }
export function scanPercent(done: number, known: number) { return known > 0 ? Math.min(100, Math.floor(done / known * 100)) : 0 }
export const kindLabels: Record<string, string> = {file:'文件整理',batch:'整理批次',scan:'云盘扫描',transfer:'上传 / 入库',output:'输出 / 归档',emby:'Emby 可见性'}
