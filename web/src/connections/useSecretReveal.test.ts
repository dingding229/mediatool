import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { post } from '../api/client'
import { setupComponent } from '../test/setup-component'
import { useSecretReveal } from './useSecretReveal'
vi.mock('../api/client', () => ({ post: vi.fn() }))
let view: ReturnType<typeof setupComponent>
beforeEach(() => { vi.mocked(post).mockReset(); view = setupComponent(defineComponent({ setup: () => useSecretReveal(() => ({ id: 'owned', revision: 4 })) })) })
afterEach(() => { view.unmount(); vi.unstubAllGlobals() })
it('reveals exactly the explicitly selected field, never on mount', async () => {
  expect(post).not.toHaveBeenCalled()
  vi.mocked(post).mockResolvedValue({ revision: 4, values: { token: 'private-fixture' } })
  await view.state.show('token')
  expect(post).toHaveBeenCalledWith('/integrations/owned/secrets/reveal', { revision: 4, fields: ['token'], confirm: true })
  expect(view.state.values.token).toBe('private-fixture'); view.state.hide('token'); expect(view.state.values).toEqual({})
})
it('discards a late secret response after editor close, save, or unmount', async () => {
  let done!: (value: unknown) => void
  vi.mocked(post).mockImplementation(() => new Promise(resolve => { done = resolve }))
  const pending = view.state.show('token'); view.state.hideAll(); done({ revision: 4, values: { token: 'too-late' } }); await pending
  expect(view.state.values).toEqual({})
})
it('never displays a different revision', async () => {
  vi.mocked(post).mockResolvedValue({ revision: 5, values: { token: 'wrong-revision' } }); await view.state.show('token')
  expect(view.state.values).toEqual({}); expect(view.state.message).toContain('重新读取')
})
it('copy failure keeps the selectable value and explains manual copying', async () => {
  vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(Error('denied')) } })
  view.state.values.token = 'owned-fixture'; await view.state.copy('token')
  expect(view.state.message).toContain('手动选择'); expect(view.state.values.token).toBe('owned-fixture')
  view.unmount(); expect(view.state.values).toEqual({})
})
