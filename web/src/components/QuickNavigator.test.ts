import { beforeEach, expect, it, vi } from 'vitest'
import QuickNavigator from './QuickNavigator.vue'
import { setupComponent, flush } from '../test/setup-component'
const mocks = vi.hoisted(() => ({ push: vi.fn() }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: mocks.push }) }))
beforeEach(() => mocks.push.mockReset())
it('navigates to page and tab, not a made-up primary route named after the result', async () => { const view = setupComponent(QuickNavigator, { modelValue: true }); view.state.query = '日志'; await flush(); expect(view.state.results).toHaveLength(1); await view.state.go(view.state.results[0]); expect(mocks.push).toHaveBeenCalledWith({ path: '/ops', query: { tab: 'logs' } }); view.unmount() })
it('Enter with zero results never navigates', async () => { const view = setupComponent(QuickNavigator, { modelValue: true }); view.state.query = 'not-a-feature'; await flush(); view.state.keydown({ key: 'Enter', preventDefault: vi.fn() }); expect(mocks.push).not.toHaveBeenCalled(); view.unmount() })
