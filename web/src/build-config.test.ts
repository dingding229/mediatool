import { expect, it } from 'vitest'
import { fileURLToPath, URL } from 'node:url'
import config from '../vite.config'

it('preserves the tracked embed placeholder while building frontend assets', () => {
  expect(config.build?.emptyOutDir).toBe(false)
  expect(config.build?.outDir).toBe(fileURLToPath(new URL('../../internal/platform/webui/assets', import.meta.url)))
})
