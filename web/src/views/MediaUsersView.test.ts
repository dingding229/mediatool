import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import MediaUsersView from './MediaUsersView.vue'
import { flush, setupComponent } from '../test/setup-component'
import permissions from '../../../internal/platform/httpapi/testdata/contracts/media-user-permissions.json'
import policy from '../../../internal/platform/httpapi/testdata/contracts/media-user-password-policy.json'
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn() }, ElMessageBox: { confirm: vi.fn() } }))
let view: ReturnType<typeof setupComponent>
let fetch: ReturnType<typeof vi.fn<(url: string, options?: RequestInit) => Promise<any>>>
const user = { id: 'aaaaaaaa-0000-4000-8000-000000000041', username: 'synthetic.member', displayName: 'Synthetic Member', enabled: true, mustChangePassword: false, revision: 1, grants: [] }
beforeEach(async () => {
 fetch = vi.fn(async (url: string, options?: RequestInit) => ({ ok: true, status: options?.method ? 201 : 200, json: async () => ({ data: url.endsWith('/libraries') ? [] : options?.method ? user : [user] }) }))
 vi.stubGlobal('fetch', fetch)
 view = setupComponent(MediaUsersView)
 await flush()
})
afterEach(() => { view.unmount(); vi.unstubAllGlobals() })
it('creates a final password by default and sends the exact shared contract', async () => {
 view.state.edit()
 expect(view.state.form.mustChangePassword).toBe(false)
 Object.assign(view.state.form, policy.create, { confirm: policy.create.password })
 await view.state.save()
 const call = fetch.mock.calls.find(([url, options]) => url.endsWith('/users') && options?.method === 'POST')
 expect(call).toBeDefined()
 expect(JSON.parse(String(call![1]!.body))).toEqual(policy.create)
})
it('preserves an existing require-change setting and supports an explicit reset flag', async () => {
 view.state.edit({ ...user, mustChangePassword: true })
 expect(view.state.form.mustChangePassword).toBe(true)
 view.state.openReset(user)
 expect(view.state.resetMustChange).toBe(false)
 view.state.resetPassword = policy.reset.password
 view.state.resetConfirm = policy.reset.password
 view.state.resetMustChange = policy.reset.mustChangePassword
 await view.state.reset()
 const call = fetch.mock.calls.find(([url]) => url.endsWith('/password'))
 expect(call).toBeDefined()
 expect(JSON.parse(String(call![1]!.body))).toEqual(policy.reset)
})

it('sends the exact shared contract when permission limits are changed', async () => {
 const existing = { ...user, ...permissions.update, maxStreams: 2, maxParentalRating: null, expiresAt: null }
 view.state.edit(existing)
 Object.assign(view.state.form, permissions.update)
 await view.state.save()
 const call = fetch.mock.calls.find(([url, options]) => url.endsWith('/users/' + user.id) && options?.method === 'PUT')
 expect(call).toBeDefined()
 expect(JSON.parse(String(call![1]!.body))).toEqual(permissions.update)
})
it('never sends download permission without playback when playback is unchecked', async () => {
 view.state.edit({ ...user, ...permissions.update })
 view.state.form.play = []
 await view.state.save()
 const call = fetch.mock.calls.find(([, options]) => options?.method === 'PUT')
 expect(JSON.parse(String(call![1]!.body)).grants).toEqual([{ libraryId: 'g:synthetic', canPlay: false, canDownload: false }])
})

it('omits unchanged nullable limits and explicitly clears them with null', async () => {
 view.state.edit({ ...user, ...permissions.update })
 await view.state.save()
 let call = fetch.mock.calls.filter(([, options]) => options?.method === 'PUT').at(-1)!
 expect(JSON.parse(String(call[1]!.body))).toEqual(permissions.omittedUpdate)
 view.state.edit({ ...user, ...permissions.update })
 view.state.form.maxParentalRating = null
 view.state.form.expiresAt = null
 await view.state.save()
 call = fetch.mock.calls.filter(([, options]) => options?.method === 'PUT').at(-1)!
 expect(JSON.parse(String(call[1]!.body))).toEqual(permissions.clearUpdate)
})
it('validates the simultaneous stream limit and opens the existing user activity panel', async () => {
 view.state.edit(user)
 view.state.form.maxStreams = undefined
 await view.state.save()
 expect(view.state.formError).toContain('1–20')
 expect(fetch.mock.calls.some(([, options]) => options?.method === 'PUT')).toBe(false)
 view.state.activity(user)
 expect(view.state.activityDialog).toBe(true)
 expect(view.state.activityUser.id).toBe(user.id)
})
