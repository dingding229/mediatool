import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import PluginPageView from './PluginPageView.vue'
import { request, post } from '../api/client'
import { ElMessageBox } from 'element-plus'
import { flush, setupComponent } from '../test/setup-component'
const { confirm } = vi.hoisted(() => ({ confirm: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => ({ params: { id: 'owned', page: 'main' } }) }))
vi.mock('../api/client', () => ({ request: vi.fn(), post: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessageBox: { confirm } }))
let view: ReturnType<typeof setupComponent>
let postMessage: ReturnType<typeof vi.fn>
beforeEach(async () => {
  vi.mocked(request).mockReset(); vi.mocked(post).mockReset(); confirm.mockReset().mockResolvedValue('confirm')
  vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() })
  vi.mocked(request).mockImplementation(async (path) => path === '/plugins' ? [{ enabled: true, manifest: { id: 'owned', commands: [{ id: 'run' }] } }] : {})
  view = setupComponent(PluginPageView); await flush()
  postMessage = vi.fn(); view.state.frame = { contentWindow: { postMessage } }
})
afterEach(() => { view.unmount(); vi.unstubAllGlobals() })
async function command(id = 'run') {
  await view.state.bridge({ source: view.state.frame.contentWindow, data: { type: 'media-tool-plugin', requestId: 'request-1', action: 'command', id } })
  return postMessage.mock.calls[0]![0].data.executed
}
it.each(['cancel', 'close'])('reports executed:false on confirmation %s', async (reason) => {
  vi.mocked(ElMessageBox.confirm).mockRejectedValue(reason)
  expect(await command()).toBe(false); expect(post).not.toHaveBeenCalled()
})
it('reports executed:false for undeclared commands', async () => {
  expect(await command('unknown')).toBe(false); expect(post).not.toHaveBeenCalled()
})
it('reports executed:false for backend failure', async () => {
  vi.mocked(post).mockRejectedValue(new Error('denied'))
  expect(await command()).toBe(false); expect(view.state.error).toBe('denied')
})
it('reports executed:true only after backend success', async () => {
  vi.mocked(post).mockResolvedValue({})
  expect(await command()).toBe(true)
  expect(post).toHaveBeenCalledWith('/plugins/owned/view/command/run', { confirm: true, data: {} })
})
