import {expect,it} from 'vitest'
import {readFileSync,readdirSync} from 'node:fs'
import {resolve} from 'node:path'
import ts from 'typescript'
const root=resolve(__dirname,'..')
function sourceFiles(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?sourceFiles(resolve(dir,e.name)):/\.(vue|ts)$/.test(e.name)&&!e.name.includes('.test.')?[resolve(dir,e.name)]:[])}
const required=['POST /files/identify', 'GET /files/roots', 'GET /files', 'GET /files/operations', 'POST /files/preview', 'POST /files/operations/{id}/execute', 'POST /files/operations/{id}/reconcile', 'POST /media/recognize-name', 'GET /settings/application', 'POST /files/batches/preview', 'GET /files/batches/{id}', 'POST /files/batches/{id}/approve', 'POST /files/batches/{id}/reconcile', 'GET /library/outputs', 'GET /library/outputs/{id}/content', 'POST /strm/preview', 'POST /strm/batch-preview', 'POST /cloudStorage/diagnose', 'POST /strm/repairs/preview', 'GET /strm/repairs/{id}', 'POST /strm/repairs/{id}/approve', 'POST /strm/repairs/{id}/retry']
it('keeps every file and STRM API invocation in executable frontend source, independent of documentation categories',()=>{
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
