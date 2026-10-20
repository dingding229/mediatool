import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { request } from '../api/client'
import { setupComponent, deferred, flush } from '../test/setup-component'
import Composer from './SubscriptionComposer.vue'
import { draftFor } from './model'
vi.mock('../api/client', async original => ({ ...await original<typeof import('../api/client')>(), request: vi.fn() }))
const views: ReturnType<typeof setupComponent>[] = []
const media = { id: 42, mediaType: 'movie', title: '目标影视', year: '2026' }
function mount() { const v = setupComponent(Composer, { visible: true, draft: draftFor(null, [{ id: 'site', kind: 'ptsite', name: '站点', enabled: true }]), accounts: [], accountError: null, saving: false, error: '' }); views.push(v); return v.state }
beforeEach(() => { vi.mocked(request).mockReset() })
afterEach(() => { views.splice(0).forEach(v => v.unmount()) })
it('cannot move past identity selection without choosing a film', () => { const s = mount(); s.advance(); expect(s.step).toBe(0); expect(s.validation).toContain('目标影视') })
it('matching query is encoded and explicit selection advances into rules then review', async () => { vi.mocked(request).mockResolvedValue({ items: [media], cached: true, cacheSeconds: 60 }); const s = mount(); s.query = '影视 & 完整'; await s.findMedia(); expect(request).toHaveBeenCalledWith('/search/media?mediaType=movie&q=%E5%BD%B1%E8%A7%86%20%26%20%E5%AE%8C%E6%95%B4'); expect(s.cache).toEqual({ cached: true, seconds: 60 }); s.choose(media); s.advance(); expect(s.step).toBe(1); expect(s.draft.enabled).toBe(false); s.advance(); expect(s.step).toBe(2) })
it('search errors stay distinct from an empty match list', async () => { vi.mocked(request).mockRejectedValue(new Error('无法查询')); const s = mount(); s.query = '影视'; await s.findMedia(); expect(s.findError.message).toBe('无法查询'); expect(s.searched).toBe(false); expect(s.finding).toBe(false) })
it('changing media type invalidates a previously selected identity', async () => { const s = mount(); s.choose(media); s.mediaType = 'series'; await flush(); expect(s.draft.media).toBeNull(); expect(s.matches).toEqual([]) })
it('does not apply late media lookup after the composer unmounts', async () => { const pending = deferred(); vi.mocked(request).mockReturnValue(pending.promise); const s = mount(); s.query = '影视'; const find = s.findMedia(); views[0].unmount(); pending.resolve({ items: [media], cached: false, cacheSeconds: 60 }); await find; expect(s.matches).toEqual([]) })
