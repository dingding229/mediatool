import { describe, it, expect } from 'vitest'
import { readFileSync, existsSync } from 'node:fs'
import { resolve } from 'node:path'

const projectRoot = resolve(__dirname, '../../..')
const apiRoutesMdPath = resolve(projectRoot, 'docs/FRONTEND_API_ROUTES.md')

interface RouteItem {
  method: string
  path: string
  usage: string
  evidenceFiles: string[]
}

describe('API Coverage Guard (FRONTEND_API_ROUTES.md)', () => {
  const mdContent = readFileSync(apiRoutesMdPath, 'utf8')
  const inScopeRoutes: RouteItem[] = []

  for (const line of mdContent.split('\n')) {
    const parts = line.split('|').map(s => s.trim()).filter(Boolean)
    if (parts.length >= 5 && parts[2] === 'management') {
      const [method, path, , usage, evidence] = parts
      if (
        usage.includes('前端直接调用') ||
        usage.includes('经适配调用') ||
        usage.includes('仅适配层调用') ||
        usage.includes('资源/链接使用')
      ) {
        const matches = Array.from(evidence.matchAll(/`([^`:]+):(\d+)`/g)).map(m => m[1])
        inScopeRoutes.push({
          method,
          path,
          usage,
          evidenceFiles: matches
        })
      }
    }
  }

  it('verifies that each active management route in FRONTEND_API_ROUTES has existing code reference in web/src', () => {
    const missing: Array<{ method: string; path: string; files: string[] }> = []

    for (const route of inScopeRoutes) {
      if (!route.evidenceFiles.length) {
        missing.push({ method: route.method, path: route.path, files: ['(no evidence in table)'] })
        continue
      }

      const hasValidFile = route.evidenceFiles.some(fileRel => {
        const fullPath = resolve(projectRoot, fileRel)
        return existsSync(fullPath)
      })

      if (!hasValidFile) {
        missing.push({ method: route.method, path: route.path, files: route.evidenceFiles })
      }
    }

    if (missing.length > 0) {
      console.error('API routes whose evidence files were deleted or moved:', missing)
    }
    expect(missing).toEqual([])
    expect(inScopeRoutes.length).toBeGreaterThan(150)
  })
})
