import { describe, expect, it } from 'vitest'
import { APIError } from '../api/client'
import { draftFor, failure, payloadFor, quality, stateFor, timestamp } from './model'
import type { Subscription } from './model'
const record: Subscription = { id: 'one', title: '电影', media: { id: 42, title: '电影', mediaType: 'movie' }, enabled: false, source: 'both', revision: 8, lastState: '', lastMessage: '', nextRunAt: '', rules: { accountIds: ['site'], keyword: 'Film', include: ['WEB'], exclude: [], minSize: 2, maxSize: 20, minSeeders: 4, freeOnly: true, rejectHR: true, season: 1, episodes: [1, 2], includeHistory: false } }
describe('subscription data transformations (no UI or transport)', () => {
  it('new draft defaults to paused and safe downloads', () => { expect(draftFor(null, [{ id: 'site', kind: 'ptsite', enabled: true, name: '站点' }])).toMatchObject({ enabled: false, freeOnly: true, rejectHR: true, accountIds: ['site'], revision: 0 }) })
  it('edit retains identity, revision, hidden limits and all safety flags', () => { const payload = payloadFor(draftFor(record, [])); expect(payload).toMatchObject({ tmdbId: 42, revision: 8, source: 'both', rules: { minSize: 2, maxSize: 20, minSeeders: 4, episodes: [1, 2] } }); expect(record.rules.accountIds).toEqual(['site']) })
  it('plain text splits commas and lines, while regex preserves commas', () => { const draft = draftFor(record, []); draft.includeText = 'A,B\nC'; draft.regexKeywords = true; expect(payloadFor(draft).rules.include).toEqual(['A,B', 'C']); draft.regexKeywords = false; expect(payloadFor(draft).rules.include).toEqual(['A', 'B', 'C']) })
  it.each(['0', '-1', '1.5', '1,abc'])('rejects invalid episode input %s', value => { const d = draftFor(record, []); d.episodeText = value; expect(() => payloadFor(d)).toThrow('正整数') })
  it('requires a media identity and at least one site', () => { expect(() => payloadFor(draftFor(null, []))).toThrow('影视'); const d = draftFor(record, []); d.accountIds = []; expect(() => payloadFor(d)).toThrow('PT 账号') })
  it.each([[403, 'denied', 'unauthorized'], [501, 'not_supported', 'unsupported'], [503, 'unavailable', 'error']])('keeps status %s distinct from empty', (status, code, state) => { expect(stateFor(new APIError(status as number, String(code), '错误'))).toBe(state) })
  it('translates actual revision conflict but not a run mode conflict', () => { expect(failure(new APIError(400, 'invalid_subscription', 'subscription changed; reload before saving'))).toContain('刷新列表'); expect(failure(new APIError(409, 'subscription_run_mode_conflict', '请等待任务结束'))).toBe('请等待任务结束') })
  it('does not fabricate missing dates or quality', () => { expect(timestamp('bad date')).toBe('—'); expect(timestamp()).toBe('—'); expect(quality(record.rules)).toBe('不限画质') })
})
