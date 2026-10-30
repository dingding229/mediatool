import {expect,it} from 'vitest';
import {readdirSync,readFileSync} from 'node:fs';
import {resolve} from 'node:path';
const root=resolve(import.meta.dirname,'..');
function files(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?files(resolve(dir,e.name)):[resolve(dir,e.name)])}
it('uses Element controls throughout Vue source (ElUpload owns its internal native picker)',()=>{
 for(const file of files(root).filter(p=>p.endsWith('.vue')&&!p.includes('.test.'))){const s=readFileSync(file,'utf8');expect(s,file).not.toMatch(/<select\b|<fieldset\b|type\s*=\s*["']file["']|<input\b[^>]*\btype\s*=\s*["']date["']/)}
});
it('capacity display never concatenates decimal GB/MB labels',()=>{
 for(const file of files(root).filter(p=>/\.(ts|vue)$/.test(p)&&!p.includes('.test.'))){expect(readFileSync(file,'utf8'),file).not.toMatch(/(?:\+\s*['"`]\s*(?:GB|MB)\b|['"`] (?:GB|MB)['"`]|\$\{[^}]+}\s*(?:GB|MB)\b)/)}
});
