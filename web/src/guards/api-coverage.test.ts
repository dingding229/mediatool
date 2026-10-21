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

// Owner rule 2026-10-21: components/stores/composables do not own transports.
// Guard production UI sources; tests may use transport spies and source assertions.
import {readdirSync,statSync} from 'node:fs'
import ts from 'typescript'
const src=resolve(projectRoot,'web/src')
function files(dir:string):string[]{return readdirSync(dir).flatMap(name=>{const p=resolve(dir,name);return statSync(p).isDirectory()?files(p):/\.(ts|vue|js)$/.test(name)?[p]:[]})}
it('forbids backend transports outside the unified api layer',()=>{
 const violations:string[]=[]
 for(const p of files(src)){
  if(p.startsWith(resolve(src,'api')+'/')||/\.(test|spec)\./.test(p))continue
  const text=readFileSync(p,'utf8')
  if(/\bfetch\s*\(|new\s+(?:EventSource|WebSocket)\b|\bXMLHttpRequest\b/.test(text))violations.push(p.slice(src.length+1))
 }
 expect(violations).toEqual([])
})
it('registers UI request paths in both the route table and OpenAPI',()=>{
 const md=readFileSync(apiRoutesMdPath,'utf8'),spec=JSON.parse(readFileSync(resolve(projectRoot,'api/openapi.yaml'),'utf8'))
 const documented=new Set(Array.from(md.matchAll(/\|\s*(?:GET|POST|PUT|PATCH|DELETE|HEAD)\s*\|\s*([^|]+)\|/g),m=>m[1].trim().replace(/`/g,'')))
 const paths=Object.keys(spec.paths) // OpenAPI is JSON-compatible YAML, generated from Go routes.
 const missing:string[]=[]
 const normalize=(s:string)=>s.replace(/\{[^}]*\}/g,'{}')
 for(const p of files(src)){
  if(/\.(test|spec)\./.test(p))continue
  const raw=readFileSync(p,'utf8'),code=p.endsWith('.vue')?raw.match(/<script[^>]*>([\s\S]*?)<\/script>/)?.[1]||'':raw
  const ast=ts.createSourceFile(p,code,ts.ScriptTarget.Latest,true)
  function visit(n:ts.Node){
   if(ts.isCallExpression(n)&&['request','post','useRevision','fetch','EventSource'].includes(n.expression.getText(ast))&&n.arguments[0])check(n.arguments[0])
   if(ts.isNewExpression(n)&&n.expression.getText(ast)==='EventSource'&&n.arguments?.[0])check(n.arguments[0])
   ts.forEachChild(n,visit)
  }
  function check(arg:ts.Node){
   let path=arg.getText(ast)
   if(!/^[`'"]\//.test(path))return // Variable paths have their own typed adapter contract tests.
   path=path.replace(/\$\{[^}]+\}/g,'{}').replace(/^[`'"]|[`'"]$/g,'').split('?')[0]
   if(path.includes("' +")||path.includes("'+"))path=path.split("'")[0]+'{}'
   if(!path.startsWith('/api/'))path='/api/v1'+path
   const pattern=new RegExp('^'+normalize(path).split('{}').map(s=>s.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')).join('.*')+'$')
   const match=paths.find(candidate=>(pattern.test(normalize(candidate))||new RegExp('^'+normalize(candidate).split('{}').map(s=>s.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')).join('[^/]+')+'$').test(normalize(path))))
   if(!match||!Array.from(documented).some(d=>normalize(d)===normalize(match)))missing.push(p.slice(src.length+1)+': '+path)
  }
  visit(ast)
 }
 expect(missing).toEqual([])
})
