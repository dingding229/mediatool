import { describe, it, expect, vi } from 'vitest'
import { setupComponent } from '../test/setup-component'
import PageTabs from './PageTabs.vue'

const mockReplace = vi.fn()
vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: { tab: 'history' }
  }),
  useRouter: () => ({
    replace: mockReplace
  })
}))

describe('PageTabs logic', () => {
  it('computes currentTab from route query', () => {
    const tabs = [
      { id: 'query', label: '搜索' },
      { id: 'history', label: '历史' }
    ]
    const view = setupComponent(PageTabs, { tabs })
    expect(view.state.currentTab).toBe('history')
    view.unmount()
  })

  it('falls back to defaultTab or first tab if query missing', () => {
    const tabs = [
      { id: 'query', label: '搜索' },
      { id: 'history', label: '历史' }
    ]
    const view = setupComponent(PageTabs, { tabs, queryKey: 'otherTab', defaultTab: 'query' })
    expect(view.state.currentTab).toBe('query')
    view.unmount()
  })

  it('selectTab replaces route query', () => {
    const tabs = [
      { id: 'query', label: '搜索' },
      { id: 'history', label: '历史' }
    ]
    const view = setupComponent(PageTabs, { tabs })
    view.state.selectTab('query')
    expect(mockReplace).toHaveBeenCalledWith({ query: { tab: 'query' } })
    view.unmount()
  })
})
