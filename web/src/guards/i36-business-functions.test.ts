import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';import {createHash} from 'node:crypto';import ts from 'typescript';
import snapshot from './i36-business-functions.json';
// I36 removes display-only state/imports; all original action/validation/readback functions stay pinned.
for(const [file,functions]of Object.entries(snapshot))it('I36 retains existing business functions: '+file,()=>{
 const source=readFileSync(new URL('../'+file,import.meta.url),'utf8').match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!;
 const ast=ts.createSourceFile(file+'.ts',source,ts.ScriptTarget.Latest,true);
 const current=new Map(ast.statements.filter(ts.isFunctionDeclaration).map(n=>[n.name!.text,createHash('sha256').update(n.getText(ast)).digest('hex')]));
 for(const [name,hash]of Object.entries(functions))expect(current.get(name),file+': '+name).toBe(hash);
});
