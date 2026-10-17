import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { relative, join } from 'node:path'
import { expect, it } from 'vitest'

it('keeps secure-context-only UUID usage out of application source (including tests)', () => {
  const root = fileURLToPath(new URL('../', import.meta.url))
  const forbidden = ['random', 'UUID'].join('')
  const violations: string[] = []
  function scan(directory: string) {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const path = join(directory, entry.name)
      if (entry.isDirectory()) scan(path)
      else if (relative(root, path) !== 'utils/request-uuid.ts' && readFileSync(path, 'utf8').includes(forbidden)) {
        violations.push(relative(root, path))
      }
    }
  }
  scan(root)
  expect(violations).toEqual([])
})
