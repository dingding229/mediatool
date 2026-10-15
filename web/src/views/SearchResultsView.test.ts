import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import SearchResultsView from './SearchResultsView.vue'
import { request, post } from '../api/client'
import { deferred, flush, setupComponent } from '../test/setup-component'
vi.mock('vue-router', () => ({ useRoute: () => ({ params: { id: 'run-1' } }), useRouter: () => ({ push: vi.fn() }) }))
vi.mock('../api/client', () => ({ request: vi.fn(), post: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn() } }))
const oldMedia = { id: 1, mediaType: 'movie', title: 'Old', year: '2000' }
const newMedia = { id: 2, mediaType: 'movie', title: 'New', year: '2026' }
const resource = { id: 'resource-1' }
let view: ReturnType<typeof setupComponent>
beforeEach(async () => {
  vi.useFakeTimers(); vi.mocked(post).mockReset(); vi.mocked(request).mockReset()
  vi.mocked(request).mockImplementation(async (path) => path === '/integrations' ? [] : path === '/search' ? [] : { id: 'run-1', state: 'succeeded', media: oldMedia, resources: [resource], total: 1 })
  view = setupComponent(SearchResultsView); await flush()
})
afterEach(() => { view.unmount(); vi.useRealTimers() })
it('refreshes the current run and blocks stale downloads throughout identity save/reload', async () => {
  view.state.openDownload(resource)
  const saved = deferred(), reloaded = deferred()
  vi.mocked(post).mockReturnValueOnce(saved.promise)
  vi.mocked(request).mockImplementation((path) => path.startsWith('/search/run-1?') ? reloaded.promise : Promise.resolve([]))
  const saving = view.state.persistMedia(newMedia)
  expect(view.state.dialog).toBe(false); expect(view.state.chosen).toBeNull(); expect(view.state.downloadSearchIdentity).toBeNull()
  view.state.openDownload(resource); await view.state.download()
  expect(view.state.dialog).toBe(false); expect(post).toHaveBeenCalledTimes(1)
  saved.resolve({}); await flush()
  view.state.openDownload(resource); expect(view.state.dialog).toBe(false)
  reloaded.resolve({ id: 'run-1', state: 'succeeded', media: newMedia, resources: [resource], total: 1 }); await saving
  expect(view.state.run.media.id).toBe(2); expect(view.state.activeMedia.id).toBe(2)
  view.state.openDownload(resource)
  expect(view.state.downloadSearchIdentity.id).toBe(2); expect(view.state.chosen.id).toBe(2)
  await view.state.download()
  expect(post).toHaveBeenLastCalledWith('/downloads', expect.objectContaining({ tmdbId: 2, searchRunId: 'run-1' }))
})
it('keeps downloads disabled and shows the error if the run refresh fails', async () => {
  vi.mocked(post).mockResolvedValue({})
  vi.mocked(request).mockImplementation(async (path) => { if (path.startsWith('/search/run-1?')) throw new Error('reload failed'); return [] })
  await view.state.persistMedia(newMedia)
  view.state.openDownload(resource); await view.state.download()
  expect(view.state.dialog).toBe(false); expect(view.state.mediaReady).toBe(false)
  expect(view.state.error).toBe('reload failed'); expect(post).toHaveBeenCalledTimes(1)
})
it('does not allow an older in-flight run response to restore old media', async () => {
  const old = deferred()
  vi.mocked(request).mockReturnValueOnce(old.promise)
  const pending = view.state.refresh()
  vi.mocked(post).mockResolvedValue({})
  vi.mocked(request).mockImplementation(async (path) => path.startsWith('/search/run-1?') ? { id: 'run-1', media: newMedia, resources: [], total: 0 } : [])
  await view.state.persistMedia(newMedia)
  old.resolve({ id: 'run-1', media: oldMedia, resources: [resource], total: 1 }); await pending
  expect(view.state.run.media.id).toBe(2); expect(view.state.activeMedia.id).toBe(2)
})
it('can load another run while a previous run identity save is pending', async () => {
  const pending = deferred()
  vi.mocked(post).mockReturnValueOnce(pending.promise)
  const saving = view.state.persistMedia(newMedia)
  view.state.run = null
  vi.mocked(request).mockResolvedValue({ id: 'run-2', media: oldMedia, resources: [], total: 0 })
  await view.state.refresh('run-2')
  pending.resolve({}); await saving
  expect(view.state.run.id).toBe('run-2'); expect(view.state.mediaReady).toBe(true)
})
it('requires an explicit year-conflict acknowledgement and sends the real JSON field',async()=>{
 const conflicted={...resource,metadata:{yearWarning:'年份不一致，需人工确认'}}
 view.state.openDownload(conflicted)
 await view.state.download()
 expect(post).not.toHaveBeenCalled()
 await flush()
 view.state.yearMismatchConfirmed=true
 await view.state.download()
 expect(post).toHaveBeenLastCalledWith('/downloads',expect.objectContaining({confirmYearMismatch:true,tmdbId:1,resourceId:'resource-1'}))
})
it('clears year-conflict confirmation for each newly selected resource',async()=>{
 const conflicted={...resource,metadata:{yearWarning:'年份不一致，需人工确认'}}
 view.state.openDownload(conflicted);await flush();view.state.yearMismatchConfirmed=true
 view.state.openDownload({...conflicted,id:'resource-2'})
 expect(view.state.yearMismatchConfirmed).toBe(false)
})
