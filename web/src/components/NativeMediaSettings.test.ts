import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import NativeMediaSettings from './NativeMediaSettings.vue'
import { flush, setupComponent } from '../test/setup-component'
import c from '../../../internal/platform/httpapi/testdata/contracts/native-management.json'
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn() } }))
let view: ReturnType<typeof setupComponent>, fetch: ReturnType<typeof vi.fn>
beforeEach(async () => {
 fetch = vi.fn(async (url: string, options?: RequestInit) => ({ ok: true, status: 200, json: async () => url.includes('library-mappings?') ? c.mappings.response : options?.method === 'PUT' ? c.settings.saved : c.settings.response }))
 vi.stubGlobal('fetch', fetch); view = setupComponent(NativeMediaSettings); await flush()
})
afterEach(() => { view.unmount(); vi.unstubAllGlobals() })
it('loads mappings without a physical path and saves exactly the settings contract', async () => {
 expect(view.state.settings).toEqual(c.settings.response.data)
 expect(view.state.libraries.items).toEqual(c.mappings.response.data.items)
 Object.assign(view.state.form, c.settings.request); await view.state.save()
 const call = fetch.mock.calls.find(([, options]) => options?.method === 'PUT')!
 expect(JSON.parse(String(call[1]!.body))).toEqual(c.settings.request)
 expect(Object.keys(JSON.parse(String(call[1]!.body)))).toEqual(['serverName', 'enabled', 'revision'])
 view.state.page = 2; await view.state.changePage(); expect(fetch.mock.calls.at(-1)![0]).toContain('startIndex=50&limit=50')
})
it('validates name and shows failures without reporting a successful save', async () => {
 view.state.form.serverName = ' '; await view.state.save()
 expect(view.state.error).toContain('1–100'); expect(fetch.mock.calls.some(([, options]) => options?.method === 'PUT')).toBe(false)
 view.state.form.serverName = c.settings.request.serverName; fetch.mockRejectedValueOnce(new Error('设置已变化'))
 await view.state.save(); expect(view.state.error).toBe('设置已变化'); expect(view.state.saving).toBe(false)
})
