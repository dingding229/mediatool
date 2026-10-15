import { afterEach, expect, it, vi } from 'vitest'
import FilesView from './FilesView.vue'
import { flush, setupComponent } from '../test/setup-component'

afterEach(() => vi.unstubAllGlobals())
it.each(['default', 'template', 'direct'])('mkdir sends form.name in %s naming mode', async (mode) => {
  const fetch = vi.fn(async (url: string, _options?: RequestInit) => ({ ok: true, status: 200, json: async () => ({ data: url.endsWith('/files/roots') || url.endsWith('/files/operations') ? [] : { items: [], steps: [] } }) }))
  vi.stubGlobal('fetch', fetch)
  const view = setupComponent(FilesView)
  await flush()
  view.state.connection = 'local-root'
  Object.assign(view.state.form, { operation: 'mkdir', name: '新建目录', nameMode: mode })
  await view.state.makePreview()
  const [, options] = fetch.mock.calls.find(([url]) => url.endsWith('/files/preview'))!
  expect(JSON.parse(String(options!.body))).toMatchObject({ operation: 'mkdir', name: '新建目录', connectionId: 'local-root' })
  view.unmount()
})
