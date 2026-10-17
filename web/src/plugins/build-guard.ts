import type { Plugin } from 'vite'
import { resolve, dirname, sep } from 'node:path'
// Build-time only. Plugins may statically import their own pinned files and the
// shared facade. Reject remote/bare imports and runtime import expressions.
export function pluginUIBuildGuard():Plugin {
 return {name:'pinned-plugin-ui-only',enforce:'pre',resolveId(source,importer){
  if(!importer?.replaceAll('\\','/').includes('/plugin-ui/'))return
  if(source==='@mediatool/plugin-ui'||source.startsWith('\0vite/'))return
  const normalized=importer.replaceAll('\\','/'),root=normalized.split('/plugin-ui/')[0]+'/plugin-ui/'+normalized.split('/plugin-ui/')[1]!.split('/')[0]
  const facade=normalized.split('/plugin-ui/')[0]+'/src/plugins/ui-runtime.ts'
  if(source===facade)return
  const target=resolve(dirname(importer),source)
  if((!source.startsWith('.')&&!source.startsWith(root+'/'))||!(target===root||target.startsWith(root+sep)))throw new Error('Plugin UI import must stay in its pinned plugin-ID namespace: '+source)
 },transform(code,id){if(!id.replaceAll('\\','/').includes('/plugin-ui/'))return
  const ast=this.parse(code)
  const visit=(node:unknown):void=>{if(!node||typeof node!=='object')return;const v=node as Record<string,unknown>;if(v.type==='ImportExpression')throw new Error('Runtime plugin UI imports are prohibited');for(const x of Object.values(v))if(Array.isArray(x))x.forEach(visit);else if(x&&typeof x==='object')visit(x)};visit(ast)
 }}
}
