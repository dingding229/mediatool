import {expect,it} from 'vitest'
import {readFileSync,readdirSync} from 'node:fs'
import {resolve} from 'node:path'
import ts from 'typescript'
const root=resolve(__dirname,'..')
function sourceFiles(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?sourceFiles(resolve(dir,e.name)):/\.(vue|ts)$/.test(e.name)&&!e.name.includes('.test.')?[resolve(dir,e.name)]:[])}
const required=[
 'GET /integrations','GET /search','POST /search','GET /search/media','GET /search/{id}','POST /search/{id}/media','GET /metadata/search','GET /metadata/details','GET /cache/{kind}','GET /cache/{kind}/{id}','DELETE /cache/{kind}/{id}','POST /cache/{kind}/clear',
 'GET /downloads','POST /downloads','POST /downloads/sync','POST /downloads/{id}/cleanup','POST /downloads/{id}/cleanup/reconcile','GET /downloads/{id}/source-preview','POST /downloads/{id}/reconfirm-source',
 'GET /downloads/manual','POST /downloads/manual','POST /downloads/manual/{id}/inspect','POST /downloads/manual/{id}/retry','GET /transfers','POST /pipeline/{kind}/{id}/retry','POST /transfers/{id}/control','GET /transfers/{id}/sessions','GET /transfers/{id}/output-preview','POST /transfers/{id}/resume-import',
]
it('keeps every acquisition API invocation in executable frontend source, independent of documentation categories',()=>{
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
