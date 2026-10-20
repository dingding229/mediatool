import {expect,it} from 'vitest'
import {readFileSync,readdirSync} from 'node:fs'
import {resolve} from 'node:path'
import ts from 'typescript'
const root=resolve(__dirname,'..')
const calls=new Set<string>()
for(const name of readdirSync(resolve(root,'playback')).filter(f=>f.endsWith('.vue'))){const text=readFileSync(resolve(root,'playback',name),'utf8').match(/<script[^>]*>([\s\S]*?)<\/script>/)?.[1]||'',ast=ts.createSourceFile(name,text,ts.ScriptTarget.Latest,true);function visit(n:ts.Node){if(ts.isCallExpression(n)){calls.add(n.expression.getText(ast));if(n.expression.getText(ast)==='useResource'&&n.arguments[0]&&ts.isIdentifier(n.arguments[0]))calls.add(n.arguments[0].text);}ts.forEachChild(n,visit)}visit(ast)}
it('keeps every former native management adapter invocation in executable playback components',()=>{for(const name of ['getNativeServerSettings','saveNativeServerSettings','listNativeLibraryMappings','listMediaUsers','listNativeLibraries','saveMediaUser','listMediaDevices','listMediaHistory','revokeMediaDevice','listNativeSessions','getProcessing','saveProcessing','listProcessing','retryProcessing','collectProcessing'])expect(calls.has(name),name).toBe(true)})
it('keeps development terminology out of new playback templates and main descriptions',()=>{for(const name of readdirSync(resolve(root,'playback')).filter(f=>f.endsWith('.vue'))){const text=readFileSync(resolve(root,'playback',name),'utf8').split('<template>')[1]||'';expect(text.match(/\b(?:Go|TypeScript|Provider|SDK)\b/g),name).toBeNull()}})
