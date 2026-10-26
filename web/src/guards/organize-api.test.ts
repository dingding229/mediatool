import {expect,it} from 'vitest'
import {readFileSync,readdirSync} from 'node:fs'
import {resolve} from 'node:path'
import ts from 'typescript'
const root=resolve(__dirname,'..')
function sourceFiles(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?sourceFiles(resolve(dir,e.name)):/\.(vue|ts)$/.test(e.name)&&!e.name.includes('.test.')?[resolve(dir,e.name)]:[])}
const required=['GET /library/candidates', 'GET /media/assets', 'GET /library/scans', 'GET /library/scan-schedules', 'GET /library/artifact-repairs', 'POST /library/candidates/{id}/recognize', 'GET /metadata/search', 'GET /metadata/episodes', 'POST /library/candidates/batch-confirm', 'POST /media/assets/{id}/identity-preview', 'POST /media/assets/{id}/identity', 'POST /media/assets/{id}/grant/rotate', 'POST /media/assets/{id}/grant/revoke', 'POST /library/organize-selection', 'GET /settings/application', 'POST /files/batches/preview', 'POST /files/batches/{id}/approve', 'GET /files/batches/{id}', 'POST /files/batches/{id}/reconcile', 'POST /files/operations/{id}/reconcile', 'POST /library/scans', 'GET /library/scans/{id}', 'POST /library/scans/{id}/{action}', 'POST /library/scan-schedules', 'GET /library/archive-policy', 'POST /library/archive-policy', 'GET /library/archives', 'POST /library/archives/cleanup-preview', 'POST /library/archives/cleanup-confirm', 'GET /library/archive-cleanups/{id}', 'POST /library/archive-cleanups/{id}/retry', 'POST /library/artifact-repairs/{id}/retry', 'GET /library/activity', 'POST /library/activity/remove', 'GET /transfers/{id}/output-preview', 'POST /transfers/{id}/resume-import', 'GET /media/library', 'GET /media/library/sources', 'GET /library/outputs']
it('keeps every library and organize API invocation in executable frontend source, independent of documentation categories',()=>{
 const observed:Array<{method:string;path:string}>=[]
 for(const file of sourceFiles(root)){
  let text=readFileSync(file,'utf8');if(file.endsWith('.vue'))text=text.match(/<script[^>]*>([\s\S]*?)<\/script>/)?.[1]||''
  const ast=ts.createSourceFile(file,text,ts.ScriptTarget.Latest,true)
  function visit(n:ts.Node){
   if(ts.isCallExpression(n)&&['request','post'].includes(n.expression.getText(ast))&&n.arguments.length){
    const value=n.arguments[0]!.getText(ast).replace(/\$\{suffix\}/g,'').replace(/\$\{[^}]+\}/g,'{}').split('?')[0].replace(/^["'`]|["'`]$/g,'')
    let methods=n.expression.getText(ast)==='post'?['POST']:['GET']
    const options=n.arguments[1];if(options&&ts.isObjectLiteralExpression(options)){const prop=options.properties.find(p=>p.name?.getText(ast)==='method');if(prop)methods=[...prop.getText(ast).matchAll(/['"](GET|POST|PUT|DELETE|PATCH)['"]/g)].map(m=>m[1]!)}
    for(const method of methods)observed.push({method,path:value})
   }
   ts.forEachChild(n,visit)
  }
  visit(ast)
 }
 const missing=required.filter(item=>{const [method,path]=item.split(' ');const pattern=path!.replace(/\{[^}]+\}/g,'{}');return !observed.some(call=>call.method===method&&(call.path===pattern||(pattern.startsWith('/cache/')&&call.path===pattern.replace('{}','search'))))})
 expect(missing).toEqual([])
})
