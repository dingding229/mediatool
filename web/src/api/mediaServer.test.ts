import { afterEach, expect, it, vi } from 'vitest'
import probeContract from '../../../internal/platform/httpapi/testdata/contracts/native-probe-retry.json'
import { refreshNativeTMDB, retryNativeProbe } from './mediaServer'

afterEach(() => { vi.unstubAllGlobals() })
it('sends the exact native TMDB refresh contract including season zero', async () => {
 const request = { type: 'season' as const, tmdbId: 900001, season: 0 }
 const response = { detailId: 'aaaaaaaa-0000-4000-8000-000000000058', queued: true }
 const fetch = vi.fn(async () => ({ ok: true, status: 202, json: async () => ({ data: response }) }))
 vi.stubGlobal('fetch', fetch)
 const result = await refreshNativeTMDB(request)
 expect(fetch).toHaveBeenCalledOnce()
 const [url, options] = fetch.mock.calls[0] as unknown as [string, RequestInit]
 expect(url).toBe('/api/v1/media-server/native/tmdb/refresh')
 expect(options.method).toBe('POST')
 expect(JSON.parse(String(options.body))).toEqual(request)
 expect(result).toEqual(response)
})

it('sends the native probe retry path contract without any raw payload', async () => {
 const fetch = vi.fn(async () => ({ ok: true, status: 202, json: async () => probeContract.response }))
 vi.stubGlobal('fetch', fetch)
 expect(await retryNativeProbe(probeContract.request.assetId)).toEqual(probeContract.response.data)
 const [url, options] = fetch.mock.calls[0] as unknown as [string, RequestInit]
 expect(url).toBe(`/api/v1/media-server/native/probe/${probeContract.request.assetId}/retry`)
 expect(options.method).toBe('POST')
 expect(options.body).toBeUndefined()
 expect(new Headers(options.headers).get('X-Requested-With')).toBe('media-tool')
})

import processingContract from '../../../internal/platform/httpapi/testdata/contracts/native-processing.json'
import { getProcessing, saveProcessing, listProcessing, retryProcessing, collectProcessing, type ProcessingKind } from './mediaServer'
it('uses the shared preprocessing settings, pagination, retry, bulk and GC contracts', async () => {
 const c = processingContract
 const fetch = vi.fn(async (url: string, options?: RequestInit) => ({ ok: true, status: 200, json: async () => url.endsWith('/settings') ? c.settings.response : url.includes('/tasks?') ? c.tasks.response : url.endsWith('/retry') ? (JSON.parse(String(options?.body)).bulk ? c.batch.response : c.retry.response) : url.endsWith('/gc') ? c.gc.response : c.summary.response }))
 vi.stubGlobal('fetch', fetch)
 expect(await getProcessing()).toEqual(c.summary.response.data)
 expect(await saveProcessing(c.settings.request)).toEqual(c.settings.response.data)
 expect(await listProcessing(c.tasks.request.kind as ProcessingKind, c.tasks.request.state, c.tasks.request.page, c.tasks.request.pageSize)).toEqual(c.tasks.response.data)
 expect(await retryProcessing({ ...c.retry.request, kind: c.retry.request.kind as ProcessingKind })).toEqual(c.retry.response.data)
 expect(await retryProcessing({ ...c.batch.request, kind: c.batch.request.kind as ProcessingKind })).toEqual(c.batch.response.data)
 expect(await collectProcessing()).toEqual(c.gc.response.data)
 expect(fetch.mock.calls[2][0]).toBe('/api/v1/media-server/native/processing/tasks?kind=probe&state=failed&page=1&pageSize=50')
 for (const [index, body] of [[1, c.settings.request], [3, c.retry.request], [4, c.batch.request]] as const) {
  const options = fetch.mock.calls[index][1]!
  expect(JSON.parse(String(options.body))).toEqual(body)
  expect(new Headers(options.headers).get('X-Requested-With')).toBe('media-tool')
 }
 expect(fetch.mock.calls[1][1]?.method).toBe('PUT')
 expect(fetch.mock.calls[5][1]?.method).toBe('POST')
 expect(fetch.mock.calls[5][1]?.body).toBeUndefined()
})

import management from '../../../internal/platform/httpapi/testdata/contracts/native-management.json'
import permissionLimits from '../../../internal/platform/httpapi/testdata/contracts/media-user-permissions.json'
import { listMediaDevices, listMediaHistory, revokeMediaDevice, getNativeServerSettings, saveNativeServerSettings, listNativeLibraryMappings, listNativeSessions, saveMediaUser } from './mediaServer'
it('uses exact bounded device/history/settings/mapping contracts without tokens', async () => {
 const c = management, id = c.devices.request.userId
 const fetch = vi.fn(async (url: string, options?: RequestInit) => ({ ok: true, status: 200, json: async () => url.includes('/devices/') ? c.revoke.response : url.includes('/devices?') ? c.devices.response : url.includes('/history?') ? c.history.response : url.includes('/library-mappings?') ? c.mappings.response : url.includes('/sessions?') ? { data: { items: [], total: 0 } } : options?.method === 'PUT' ? c.settings.saved : c.settings.response }))
 vi.stubGlobal('fetch', fetch)
 expect(await listMediaDevices(id)).toEqual(c.devices.response.data)
 expect(await listMediaHistory(id)).toEqual(c.history.response.data)
 expect(await revokeMediaDevice(id, c.revoke.request.deviceId)).toEqual(c.revoke.response.data)
 expect(await getNativeServerSettings()).toEqual(c.settings.response.data)
 expect(await saveNativeServerSettings(c.settings.request)).toEqual(c.settings.saved.data)
 expect(await listNativeLibraryMappings()).toEqual(c.mappings.response.data)
 await listNativeSessions()
 const urls = fetch.mock.calls.map(([url]) => url)
 expect(urls).toContain(`/api/v1/media-server/users/${id}/devices?startIndex=0&limit=50`)
 expect(urls).toContain(`/api/v1/media-server/users/${id}/history?startIndex=0&limit=50`)
 expect(urls).toContain('/api/v1/media-server/native/library-mappings?startIndex=0&limit=50')
 expect(urls).toContain('/api/v1/media-server/native/sessions?startIndex=0&limit=50')
 for (const [, options] of fetch.mock.calls) if (options?.method === 'POST' || options?.method === 'PUT') expect(new Headers(options.headers).get('X-Requested-With')).toBe('media-tool')
 expect(JSON.parse(String(fetch.mock.calls.find(([, o]) => o?.method === 'PUT')![1]!.body))).toEqual(c.settings.request)
 expect(fetch.mock.calls.find(([url]) => url.includes('/devices/'))![1]!.body).toBeUndefined()
})
it('serializes PUT omissions and explicit nullable clears separately', async () => {
 const fetch = vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ data: permissionLimits.response }) }))
 vi.stubGlobal('fetch', fetch)
 for (const body of [permissionLimits.omittedUpdate, permissionLimits.clearUpdate]) {
  await saveMediaUser(permissionLimits.response.id, body)
  const options = fetch.mock.calls.at(-1) as unknown as [string, RequestInit]
  expect(JSON.parse(String(options[1].body))).toEqual(body)
 }
})
