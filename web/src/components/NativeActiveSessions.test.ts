import { afterEach, expect, it, vi } from 'vitest'
import NativeActiveSessions from './NativeActiveSessions.vue'
import { flush, setupComponent } from '../test/setup-component'
import c from '../../../internal/platform/httpapi/testdata/contracts/native-sessions.json'
afterEach(() => vi.unstubAllGlobals())
it('uses the existing paged active session endpoint and displays failures', async () => {
 const fetch = vi.fn(async () => ({ ok: true, status: 200, json: async () => c.response }))
 vi.stubGlobal('fetch', fetch); const view = setupComponent(NativeActiveSessions); await flush()
 expect(view.state.data.items).toEqual(c.response.data.items)
 view.state.page = 2; await view.state.load(); expect((fetch.mock.calls.at(-1) as unknown as [string])[0]).toContain('startIndex=50&limit=50')
 fetch.mockRejectedValueOnce(new Error('会话暂不可用')); await view.state.load(); expect(view.state.error).toBe('会话暂不可用')
 view.unmount()
})
