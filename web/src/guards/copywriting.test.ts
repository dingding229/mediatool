import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { resolve, relative } from 'node:path'

const root = resolve(__dirname, '..')
const allowlistPath = resolve(__dirname, 'copywriting-allowlist.json')
const allowlist: Record<string, string[]> = JSON.parse(readFileSync(allowlistPath, 'utf8'))

const FORBIDDEN_WORDS = [
  '工程框架',
  '实施计划',
  'docs/',
  'DEVELOPMENT_PLAN',
  'SDK 契约',
  '契约 V2',
  'operationKey',
  'fingerprint',
  'Provider v1',
  '推荐 Go',
  'Codex',
  'Claude',
  'Q5',
  'TODO'
]

function collectVueFiles(dir: string): string[] {
  const results: string[] = []
  for (const entry of readdirSync(dir)) {
    const full = resolve(dir, entry)
    const st = statSync(full)
    if (st.isDirectory()) {
      results.push(...collectVueFiles(full))
    } else if (entry.endsWith('.vue')) {
      results.push(full)
    }
  }
  return results
}

describe('Copywriting Guard (Forbidden Developer Jargon)', () => {
  const vueFiles = collectVueFiles(root)

  it('scans all .vue templates for forbidden words and respects allowlist', () => {
    const violations: Array<{ file: string; word: string; line: number; text: string }> = []

    for (const filePath of vueFiles) {
      const relPath = 'web/src/' + relative(root, filePath)
      const allowedWords = allowlist[relPath] || []
      const content = readFileSync(filePath, 'utf8')

      // Extract template part
      const templateMatch = content.match(/<template>([\s\S]*?)<\/template>/)
      if (!templateMatch) continue

      const templateContent = templateMatch[1]
      const lines = templateContent.split('\n')

      lines.forEach((lineText, idx) => {
        // Strip comments and technical details blocks if marked
        if (lineText.includes('class="technical-json"') || lineText.includes('class="maintenance-json"')) {
          return
        }

        for (const word of FORBIDDEN_WORDS) {
          if (lineText.includes(word)) {
            if (!allowedWords.includes(word)) {
              violations.push({
                file: relPath,
                word,
                line: idx + 1,
                text: lineText.trim()
              })
            }
          }
        }
      })
    }

    if (violations.length > 0) {
      console.error('Found forbidden copywriting violations:', violations)
    }
    expect(violations).toEqual([])
  })
})
