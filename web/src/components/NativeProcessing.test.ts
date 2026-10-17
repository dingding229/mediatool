import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ElMessageBox } from 'element-plus'
import NativeProcessing from './NativeProcessing.vue'
import { deferred, flush, setupComponent } from '../test/setup-component'
import c from '../../../internal/platform/httpapi/testdata/contracts/native-processing.json'
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn() }, ElMessageBox: { confirm: vi.fn(async () => true) } }))
let view: ReturnType<typeof setupComponent>, fetch: ReturnType<typeof vi.fn>
let settings: typeof c.summary.response.data.settings
beforeEach(async () => {
 settings = { ...c.summary.response.data.settings }
 vi.mocked(ElMessageBox.confirm).mockResolvedValue(undefined as never)
 fetch = vi.fn(async (url: string, options?: RequestInit) => ({ ok: true, status: 200, json: async () => {
  if (url.endsWith('/settings')) { settings = { ...c.settings.response.data }; return c.settings.response }
  if (url.includes('/tasks?')) return c.tasks.response
  if (url.endsWith('/retry')) return JSON.parse(String(options?.body)).bulk ? c.batch.response : c.retry.response
  if (url.endsWith('/gc')) return c.gc.response
  return { data: { ...c.summary.response.data, settings } }
 } }))
 vi.stubGlobal('fetch', fetch)
 view = setupComponent(NativeProcessing); await flush()
})
afterEach(() => { view.unmount(); vi.unstubAllGlobals(); vi.clearAllMocks() })
it('loads real summary and paged tasks without changing subordinate preferences', async () => {
 expect(view.state.form).toEqual(c.summary.response.data.settings)
 expect(view.state.form.enabled).toBe(false)
 expect(view.state.form.probeEnabled).toBe(true)
 expect(view.state.tasks.items).toEqual(c.tasks.response.data.items)
 expect(view.state.reason('authorization')).toContain('授权失效')
 expect(view.state.reason('future_code')).toContain('future_code')
 view.state.form.enabled = true
 await view.state.save()
 const call = fetch.mock.calls.find(([, o]) => o?.method === 'PUT')!
 expect(JSON.parse(call[1].body)).toEqual(c.settings.request)
 expect(view.state.form.revision).toBe(2)
})
it('single retry and GC report their actual returned quantities', async () => {
 await view.state.retry(view.state.tasks.items[0])
 const call = fetch.mock.calls.find(([url]) => url.endsWith('/retry'))!
 expect(JSON.parse(call[1].body)).toEqual(c.retry.request)
 expect(view.state.notice).toContain('已排队 1 条')
 await view.state.gc()
 expect(view.state.notice).toContain('扫描 8 项，回收 2 个文件')
})
it('requires confirmation before bulk retry, captures the confirmed filter, and reports partial failures', async () => {
 vi.mocked(ElMessageBox.confirm).mockRejectedValueOnce('cancel')
 await view.state.retry()
 expect(fetch.mock.calls.some(([url]) => url.endsWith('/retry'))).toBe(false)
 vi.mocked(ElMessageBox.confirm).mockImplementationOnce(async () => { view.state.filter.state = 'pending'; return undefined as never })
 await view.state.retry()
 const call = fetch.mock.calls.find(([url]) => url.endsWith('/retry'))!
 expect(JSON.parse(call[1].body)).toEqual(c.batch.request)
 expect(view.state.notice).toContain('失败 1 条')
 expect(view.state.notice).toContain('500 条上限')
})
it('clears stale rows after a load error and never hides the error behind an empty success', async () => {
 fetch.mockRejectedValueOnce(new Error('暂不可用'))
 await view.state.load()
 expect(view.state.error).toBe('暂不可用')
 expect(view.state.tasks.items).toEqual([])
 expect(view.state.loading).toBe(false)
})
it('does not let an older filter response overwrite the latest page', async () => {
 const old = deferred<any>()
 const original = fetch.getMockImplementation() as (url: string, options?: RequestInit) => Promise<any>
 fetch.mockImplementation((url: string, options?: RequestInit) => url.includes('/tasks?') && url.includes('state=failed') ? old.promise : original(url, options))
 const first = view.state.load()
 view.state.filter.state = 'pending'
 await view.state.changeFilter()
 view.state.tasks.items = [{ ...c.tasks.response.data.items[0], name: '最新筛选' }]
 old.resolve({ ok: true, status: 200, json: async () => c.tasks.response })
 await first
 expect(view.state.tasks.items[0].name).toBe('最新筛选')
})
