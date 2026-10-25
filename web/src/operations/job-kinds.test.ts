import {expect,it} from 'vitest';
import {readdirSync,readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {jobKindLabels,kindLabel} from './model';
const root=resolve(import.meta.dirname,'../../..');
function goFiles(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?goFiles(resolve(dir,e.name)):e.name.endsWith('.go')&&!e.name.endsWith('_test.go')?[resolve(dir,e.name)]:[])}
// Read actual worker map registrations, including future multi-handler literals.
function mapKeys(code:string,start:number):string[]{
 let depth=1,i=start;const keys:string[]=[];
 while(i<code.length&&depth){
  const c=code[i]!;
  if(c==='/'&&code[i+1]==='/'){i=code.indexOf('\n',i);if(i<0)break;continue}
  if(c==='/'&&code[i+1]==='*'){const end=code.indexOf('*/',i+2);i=end<0?code.length:end+2;continue}
  if(c==='"'||c==='\''||c==='`'){const quote=c,from=i++;while(i<code.length){if(code[i]==='\\'&&quote!=='`'){i+=2;continue}if(code[i++]===quote)break}if(depth===1&&quote==='"'&&code.slice(i).trimStart().startsWith(':'))keys.push(JSON.parse(code.slice(from,i)));continue}
  if(c==='{')depth++;if(c==='}')depth--;i++;
 }
 return keys;
}
it('every registered production job kind has an explicit Chinese label',()=>{
 const kinds=new Set<string>();for(const file of [...goFiles(resolve(root,'cmd')),...goFiles(resolve(root,'internal'))]){
  const code=readFileSync(file,'utf8');for(const m of code.matchAll(/\bHandlers\[\s*"([^"]+)"\s*\]\s*=/g))kinds.add(m[1]!);
  for(const m of code.matchAll(/map\[string\](?:jobs\.)?Handler\s*\{/g))for(const key of mapKeys(code,m.index!+m[0].length))kinds.add(key);
 }
 expect(kinds.size).toBeGreaterThan(20);expect([...kinds].filter(k=>!jobKindLabels[k])).toEqual([]);for(const kind of kinds)expect(kindLabel(kind)).toMatch(/[\u3400-\u9fff]/);
});
it('unknown kinds retain their original value rather than a generic other label',()=>{expect(kindLabel('future.custom.job')).toBe('future.custom.job');expect(kindLabel('')).toBe('—')});
it('map traversal does not miss a registration after a function body',()=>{expect(mapKeys('"one.job":func(){if true{print("}")}},"two.job":next}',0)).toEqual(['one.job','two.job'])});
