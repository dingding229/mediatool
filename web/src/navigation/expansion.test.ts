import { describe, expect, it } from 'vitest'
import { toggleNavigationGroup } from './expansion'
describe('navigation disclosure state', () => {
  it('opens an initially collapsed group', () => {
    expect(toggleNavigationGroup([], 'browse')).toEqual(['browse'])
  })
  it('closes only the selected group', () => {
    expect(toggleNavigationGroup(['browse','operate'], 'browse')).toEqual(['operate'])
  })
  it('allows independent groups without mutating the shared input', () => {
    const current = Object.freeze(['browse'])
    expect(toggleNavigationGroup(current, 'operate')).toEqual(['browse','operate'])
    expect(current).toEqual(['browse'])
  })
})
