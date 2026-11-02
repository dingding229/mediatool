import {expect,it} from 'vitest';import {readFileSync} from 'node:fs';import {createHash} from 'node:crypto';import ts from 'typescript';import baseline from './i42b-action-baseline.json';
for(const [file,functions]of Object.entries(baseline))it('I42b preserves reviewed validation, writes, confirmation and reconciliation: '+file,()=>{
 const source=readFileSync(new URL('../'+file,import.meta.url),'utf8').match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]!,ast=ts.createSourceFile(file+'.ts',source,ts.ScriptTarget.Latest,true);
 const current=new Map(ast.statements.filter(ts.isFunctionDeclaration).map(n=>[n.name!.text,createHash('sha256').update(n.getText(ast)).digest('hex')]));
 for(const [name,hash]of Object.entries(functions))expect(current.get(name),file+':'+name).toBe(hash);
});
