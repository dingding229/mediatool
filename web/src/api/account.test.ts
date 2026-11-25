import { afterEach, expect, it, vi } from 'vitest'
import { registerAdministrator } from './account'

afterEach(() => vi.unstubAllGlobals())

it('sends the initialization token only in the same-origin registration body', async () => {
  const fetch = vi.fn().mockResolvedValue({ ok: true, status: 201, json: async () => ({ data: {} }) })
  vi.stubGlobal('fetch', fetch)
  await registerAdministrator('test_admin', 'test-password', 'test-password', 'synthetic-initialization')
  await registerAdministrator('test_admin', 'test-password', 'test-password')
  const [path, options] = fetch.mock.calls[0]!
  expect(path).toBe('/api/v1/auth/register')
  expect(options.credentials).toBe('same-origin')
  expect(options.headers.get('X-Requested-With')).toBe('media-tool')
  expect(JSON.parse(options.body).initializationToken).toBe('synthetic-initialization')
  expect(JSON.parse(fetch.mock.calls[1]![1].body)).not.toHaveProperty('initializationToken')
})
