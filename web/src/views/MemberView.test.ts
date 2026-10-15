import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import MemberView from './MemberView.vue'
import { APIError, request, post } from '../api/client'
import { deferred, flush, setupComponent } from '../test/setup-component'
vi.mock('../api/client', async (importOriginal) => ({ ...await importOriginal<typeof import('../api/client')>(), request: vi.fn(), post: vi.fn() }))
const member = (id: string) => ({ id, displayName: id, mustChangePassword: false })
let unmount: (() => void) | undefined
beforeEach(() => { vi.mocked(request).mockReset(); vi.mocked(post).mockReset() })
afterEach(() => { unmount?.(); unmount = undefined })
async function mountMember() {
  vi.mocked(request).mockImplementation(async (path) => path === '/member/auth/session' ? { user: member('A') } : path === '/member/libraries' ? [{ id: 'library-A' }] : { items: [{ id: 'initial' }], total: 1 })
  const view = setupComponent(MemberView)
  unmount = view.unmount
  await flush()
  return view.state
}
it('latest list request wins even when an earlier request finishes last', async () => {
  const state = await mountMember(), old = deferred(), current = deferred()
  vi.mocked(request).mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
  const first = state.loadItems(), second = state.loadItems()
  current.resolve({ items: [{ id: 'new' }], total: 1 }); await second
  old.resolve({ items: [{ id: 'old' }], total: 99 }); await first
  expect(state.items).toEqual([{ id: 'new' }]); expect(state.total).toBe(1)
})
it.each(['success', '401'])('ignores old account list %s after logout and login', async (outcome) => {
  const state = await mountMember(), old = deferred()
  vi.mocked(request).mockReturnValueOnce(old.promise)
  const pending = state.loadItems()
  vi.mocked(post).mockResolvedValueOnce({}).mockResolvedValueOnce({ user: member('B') })
  await state.logout()
  vi.mocked(request).mockImplementation(async (path) => path === '/member/libraries' ? [{ id: 'library-B' }] : { items: [{ id: 'private-B' }], total: 1 })
  await state.login()
  if (outcome === 'success') old.resolve({ items: [{ id: 'private-A' }], total: 90 })
  else old.reject(new APIError(401, 'expired', 'old session'))
  await pending
  expect(state.user.id).toBe('B'); expect(state.items).toEqual([{ id: 'private-B' }]); expect(state.error).toBe(''); expect(state.busy).toBe(false)
})
it('ignores a pending libraries response after a new login', async () => {
  const state = await mountMember(), old = deferred()
  vi.mocked(request).mockReturnValueOnce(old.promise)
  const pending = state.loadLibraries()
  vi.mocked(post).mockResolvedValueOnce({ user: member('B') })
  vi.mocked(request).mockImplementation(async (path) => path === '/member/libraries' ? [{ id: 'library-B' }] : { items: [{ id: 'private-B' }], total: 1 })
  await state.login()
  old.resolve([{ id: 'library-A' }]); await pending
  expect(state.selected).toBe('library-B'); expect(state.items).toEqual([{ id: 'private-B' }])
})
it('invalidates a pending initial session response on login', async () => {
  const initial = deferred()
  vi.mocked(request).mockReturnValueOnce(initial.promise)
  const view = setupComponent(MemberView); unmount = view.unmount
  vi.mocked(post).mockResolvedValue({ user: member('B') })
  vi.mocked(request).mockImplementation(async (path) => path === '/member/libraries' ? [{ id: 'library-B' }] : { items: [], total: 0 })
  await view.state.login()
  initial.resolve({ user: member('A') }); await flush()
  expect(view.state.user.id).toBe('B')
})
it('invalidates old media immediately when logout starts, not only after it completes', async () => {
  const state = await mountMember(), old = deferred(), logout = deferred()
  vi.mocked(request).mockReturnValueOnce(old.promise)
  const loading = state.loadItems()
  vi.mocked(post).mockReturnValueOnce(logout.promise)
  const leaving = state.logout()
  old.resolve({ items: [{ id: 'private-A' }], total: 1 }); await loading
  expect(state.items).toEqual([]); expect(state.total).toBe(0); expect(state.busy).toBe(true)
  logout.resolve({}); await leaving
  expect(state.user).toBeUndefined(); expect(state.busy).toBe(false)
})
