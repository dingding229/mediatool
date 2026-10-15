import { afterEach, expect, it, vi } from 'vitest'
import { setupComponent, flush } from '../test/setup-component'
import STRMBatchTools from './STRMBatchTools.vue'

vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }), useRouter: () => ({ replace: vi.fn() }) }))
afterEach(() => vi.unstubAllGlobals())
it('posts the strict repair preview assetIds contract, not assets', async () => {
  const fetch = vi.fn(async (_url, options) => ({ ok: true, status: 200, json: async () => ({ data: options?.method === 'POST' ? { id: 'batch' } : { items: [] } }) }))
  vi.stubGlobal('fetch', fetch)
  const view = setupComponent(STRMBatchTools)
  await flush()
  view.state.selected = ['asset-1', 'asset-2']
  await view.state.check()
  const [, options] = fetch.mock.calls.find(([url]) => String(url).endsWith('/strm/repairs/preview'))!
  expect(JSON.parse(options.body)).toEqual({ assetIds: ['asset-1', 'asset-2'] })
  view.unmount()
})
