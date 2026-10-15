import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import SubscriptionsView from './SubscriptionsView.vue'
import { request } from '../api/client'
import { deferred, flush, setupComponent } from '../test/setup-component'
const { route, replace } = vi.hoisted(() => ({ route: { query: {} as Record<string, string> }, replace: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ replace }) }))
vi.mock('../api/client', () => ({ request: vi.fn(), post: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessage: { error: vi.fn(), success: vi.fn() }, ElMessageBox: {} }))
beforeEach(() => { vi.useFakeTimers(); vi.mocked(request).mockReset(); replace.mockReset(); route.query = {} })
afterEach(() => vi.useRealTimers())
it('does not register a timer after unmount during initial refresh', async () => {
  const pending = deferred()
  vi.mocked(request).mockReturnValueOnce(pending.promise).mockResolvedValue([])
  const view = setupComponent(SubscriptionsView)
  view.unmount(); pending.resolve([]); await flush()
  expect(vi.getTimerCount()).toBe(0)
})
it('does not register a timer or navigate after unmount during media initialization', async () => {
  const pending = deferred()
  route.query = { create: '1', mediaType: 'movie', tmdbId: '1' }
  vi.mocked(request).mockImplementation((path) => path.startsWith('/metadata/details') ? pending.promise : Promise.resolve([]))
  const view = setupComponent(SubscriptionsView); await flush()
  view.unmount(); pending.resolve({ item: { id: 1, mediaType: 'movie', title: 'Owned' } }); await flush()
  expect(vi.getTimerCount()).toBe(0); expect(replace).not.toHaveBeenCalled()
})
it('clears the polling timer on normal unmount', async () => {
  vi.mocked(request).mockResolvedValue([])
  const view = setupComponent(SubscriptionsView); await flush()
  expect(vi.getTimerCount()).toBe(1)
  view.unmount(); expect(vi.getTimerCount()).toBe(0)
})
