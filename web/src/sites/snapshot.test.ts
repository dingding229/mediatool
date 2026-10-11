import { describe, expect, it } from 'vitest'
import { isSourceCommit, snapshotRequest } from './snapshot'

describe('immutable site snapshot requests', () => {
  it('keeps the default import and explicit recovery distinct', () => {
    expect(snapshotRequest(['owned'])).toEqual({ ids: ['owned'] })
    expect(snapshotRequest(['owned'], 'A'.repeat(40))).toEqual({ ids: ['owned'], sourceCommit: 'a'.repeat(40) })
  })
  it('rejects mutable refs, duplicate ids and traversal before networking', () => {
    expect(() => snapshotRequest(['owned'], 'main')).toThrow('40 位')
    expect(() => snapshotRequest(['owned', 'owned'])).toThrow('不可重复')
    expect(() => snapshotRequest(['../owned'])).toThrow('有效站点')
    expect(() => snapshotRequest([])).toThrow('1–30')
    expect(() => snapshotRequest(Array.from({ length: 31 }, (_, index) => `owned_${index}`))).toThrow('1–30')
  })
  it('does not treat old synthetic provenance as a restorable upstream commit', () => {
    expect(isSourceCommit('owned-fixture')).toBe(false)
    expect(isSourceCommit('a'.repeat(40))).toBe(true)
  })
})
