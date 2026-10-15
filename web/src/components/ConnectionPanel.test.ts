import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ElMessageBox } from 'element-plus'
import ConnectionPanel from './ConnectionPanel.vue'
import { deferred, flush, setupComponent } from '../test/setup-component'
const { confirm } = vi.hoisted(() => ({ confirm: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }) }))
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn(), error: vi.fn() }, ElMessageBox: { confirm } }))
const record = { id: 'qb', kind: 'qbittorrent', name: 'QB', enabled: true, settings: { endpoint: 'http://old.example' }, secretFields: ['username', 'password'], revision: 3 }
let view: ReturnType<typeof setupComponent>
let fetch: ReturnType<typeof vi.fn<(url: string, options?: RequestInit) => Promise<any>>>
beforeEach(async () => {
  confirm.mockReset().mockResolvedValue('confirm')
  fetch = vi.fn(async (url: string, options?: RequestInit) => ({ ok: true, status: 200, json: async () => ({ data: url.endsWith('/secrets/reveal') ? { revision: 3, values: { username: 'owned-user', password: 'owned-password' } } : options?.method === 'PUT' ? record : [record] }) }))
  vi.stubGlobal('fetch', fetch)
  view = setupComponent(ConnectionPanel, { allowedKinds: ['qbittorrent'], title: '连接' })
  await flush(); view.state.edit(record); await flush()
})
afterEach(() => { view.unmount(); vi.unstubAllGlobals() })
function savedBody() {
  const [, options] = fetch.mock.calls.find(([, options]) => options?.method === 'PUT')!
  return JSON.parse(String(options!.body))
}
it('confirms reuse and explicitly serializes unchanged credentials for a new endpoint', async () => {
  view.state.form.settings.endpoint = 'http://new.example'
  await view.state.save()
  expect(ElMessageBox.confirm).toHaveBeenCalledOnce()
  expect(savedBody()).toMatchObject({ settings: { endpoint: 'http://new.example' }, secrets: { username: 'owned-user', password: 'owned-password' } })
})
it('keeps change-only secret payloads when the endpoint is unchanged', async () => {
  view.state.form.secrets.password = 'new-password'
  await view.state.save()
  expect(ElMessageBox.confirm).not.toHaveBeenCalled()
  expect(savedBody().secrets).toEqual({ password: 'new-password' })
})
it('does not save when credential reuse is cancelled', async () => {
  vi.mocked(ElMessageBox.confirm).mockRejectedValue('cancel')
  view.state.form.settings.endpoint = 'http://new.example'
  await view.state.save()
  expect(fetch.mock.calls.some(([, options]) => options?.method === 'PUT')).toBe(false)
  expect(view.state.error).toBe('')
})
it('does not save a new endpoint before original credentials have loaded', async () => {
  const pending = deferred()
  fetch.mockReturnValueOnce(pending.promise)
  view.state.edit(record)
  view.state.form.settings.endpoint = 'http://new.example'
  await view.state.save()
  expect(fetch.mock.calls.some(([, options]) => options?.method === 'PUT')).toBe(false)
  expect(view.state.error).toContain('完整读取')
  pending.resolve({ ok: true, status: 200, json: async () => ({ data: { values: { username: 'owned-user', password: 'owned-password' } } }) })
  await flush()
})
