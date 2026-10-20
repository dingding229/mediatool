import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import LegacyTabHost from './LegacyTabHost.vue'
import { setupComponent } from '../test/setup-component'
const mocks = vi.hoisted(() => ({ route: {} as any, router: { resolve: vi.fn(), push: vi.fn(), replace: vi.fn() } }))
vi.mock('vue-router', async original => ({ ...await original<typeof import('vue-router')>(), useRoute: () => mocks.route, useRouter: () => mocks.router }))
let view: ReturnType<typeof setupComponent>
beforeEach(() => { mocks.route = reactive({ path: '/settings', query: { tab: 'general', section: 'library', keep: 'yes' }, hash: '' }); mocks.router.resolve.mockImplementation(value => ({ path: value.path || '/settings', query: value.query ?? {}, hash: value.hash ?? '' })); view = setupComponent(LegacyTabHost, { settings: true }) })
afterEach(() => view.unmount())
it('projects the old tab without changing the real canonical route', () => { expect(view.state.projected.query.tab).toBe('library'); expect(mocks.route.query.tab).toBe('general'); mocks.route.query.section = 'automation'; expect(view.state.projected.query.tab).toBe('automation') })
it('rewrites inner settings navigation and preserves unrelated canonical query', () => { expect(view.state.target({ path: '/settings', query: { tab: 'proxy' } })).toEqual({ path: '/settings', query: { tab: 'general', section: 'proxy', keep: 'yes' }, hash: '' }); expect(view.state.target({ path: '/login' })).toEqual({ path: '/login' }) })
