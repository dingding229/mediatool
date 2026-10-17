import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ElMessageBox } from 'element-plus'
import MediaUserActivity from './MediaUserActivity.vue'
import { deferred, flush, setupComponent } from '../test/setup-component'
import c from '../../../internal/platform/httpapi/testdata/contracts/native-management.json'
vi.mock('element-plus', () => ({ ElTabs: {}, ElTabPane: {}, ElMessage: { success: vi.fn() }, ElMessageBox: { confirm: vi.fn(async () => true) } }))
let view: ReturnType<typeof setupComponent>, fetch: ReturnType<typeof vi.fn>
beforeEach(async () => {
 fetch = vi.fn(async (url: string) => ({ ok: true, status: 200, json: async () => url.includes('/history?') ? c.history.response : url.endsWith('/revoke') ? c.revoke.response : c.devices.response }))
 vi.stubGlobal('fetch', fetch); vi.mocked(ElMessageBox.confirm).mockResolvedValue(undefined as never)
 view = setupComponent(MediaUserActivity, { userId: c.devices.request.userId, userName: 'synthetic.member' }); await flush()
})
afterEach(() => { view.unmount(); vi.unstubAllGlobals(); vi.clearAllMocks() })
it('loads bounded device and history pages and requires confirmation before revoke', async () => {
 expect(view.state.devices.items).toEqual(c.devices.response.data.items)
 vi.mocked(ElMessageBox.confirm).mockRejectedValueOnce('cancel')
 await view.state.revoke(view.state.devices.items[0]); expect(fetch.mock.calls.some(([u]) => u.endsWith('/revoke'))).toBe(false)
 await view.state.revoke(view.state.devices.items[0]); expect(fetch.mock.calls.filter(([u]) => u.endsWith('/revoke'))).toHaveLength(1)
 view.state.tab = 'history'; view.state.changeTab(); await flush()
 expect(view.state.history.items).toEqual(c.history.response.data.items)
 view.state.page = 2; await view.state.load()
 expect(fetch.mock.calls.at(-1)![0]).toContain('startIndex=50&limit=50')
})
it('reports errors and prevents a stale device page from replacing a newer tab', async () => {
 const old = deferred<any>(); fetch.mockReturnValueOnce(old.promise)
 const oldLoad = view.state.load()
 view.state.tab = 'history'; view.state.changeTab(); await flush()
 old.resolve({ ok: true, status: 200, json: async () => c.devices.response }); await oldLoad
 expect(view.state.tab).toBe('history'); expect(view.state.devices.items).toEqual([])
 fetch.mockRejectedValueOnce(new Error('记录暂不可用')); await view.state.load()
 expect(view.state.error).toBe('记录暂不可用'); expect(view.state.busy).toBe(false)
})
