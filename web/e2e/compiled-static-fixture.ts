import type {Page} from '@playwright/test'
import {readdirSync,readFileSync} from 'node:fs'
import {extname,join,resolve} from 'node:path'

type Asset={body:Buffer,contentType:string}
const mime:Record<string,string>={'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.json':'application/json','.svg':'image/svg+xml','.png':'image/png','.jpg':'image/jpeg','.jpeg':'image/jpeg','.webp':'image/webp','.ico':'image/x-icon','.woff':'font/woff','.woff2':'font/woff2','.ttf':'font/ttf','.webmanifest':'application/manifest+json','.wasm':'application/wasm'}
let compiled:Map<string,Asset>|undefined
function assets(){
 if(compiled)return compiled
 const root=resolve(import.meta.dirname,'../../internal/platform/webui/assets'),out=new Map<string,Asset>()
 function walk(dir:string,prefix:string){for(const file of readdirSync(dir,{withFileTypes:true})){
  if(file.isDirectory())walk(join(dir,file.name),prefix+'/'+file.name)
  else if(file.isFile())out.set(prefix+'/'+file.name,{body:readFileSync(join(dir,file.name)),contentType:mime[extname(file.name)]??'application/octet-stream'})
 }}
 walk(root,'')
 if(!out.has('/index.html'))throw Error('same-build compiled web assets required')
 compiled=out;return out
}
// Serve the exact build already embedded in the isolated test app. Preload before
// routing so each request has one terminal fulfill, without an APIResponse/fetch
// that outlives cancelled lazy assets. The browser keeps its real insecure HTTP
// origin; no OS DNS/proxy settings, isSecureContext override, or sleeps are used.
// Call before registering API fixtures, whose later routes take precedence.
export async function compiledStaticFixture(page:Page,origin:string){
 const files=assets(),index=files.get('/index.html')!
 await page.route(origin+'/**',route=>{
  const path=decodeURIComponent(new URL(route.request().url()).pathname)
  const asset=files.get(path)??(route.request().isNavigationRequest()?index:undefined)
  return asset?route.fulfill(asset):route.fulfill({status:404,contentType:'text/plain',body:'unknown compiled fixture asset'})
 })
}
