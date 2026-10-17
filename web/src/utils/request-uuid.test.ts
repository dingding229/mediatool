import { describe, expect, it } from 'vitest'
import { requestUUID } from './request-uuid'

describe('durable cleanup request UUID', () => {
  it('needs only getRandomValues, including insecure HTTP contexts', () => {
    const provider = { getRandomValues: globalThis.crypto.getRandomValues.bind(globalThis.crypto) }
    const ids = Array.from({ length: 100 }, () => requestUUID(provider))
    for (const id of ids) expect(id).toMatch(/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/)
    expect(new Set(ids).size).toBe(ids.length)
  })
  it('does not silently use a weak or missing random source', () => {
    expect(() => requestUUID({} as Crypto)).toThrow()
  })
})
