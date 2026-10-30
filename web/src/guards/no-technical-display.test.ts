import {expect,it} from 'vitest';
import {readFileSync,readdirSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {join,relative} from 'node:path';
import {parse} from '@vue/compiler-sfc';
const root=fileURLToPath(new URL('..',import.meta.url));
function sources(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?sources(join(dir,e.name)):e.name.endsWith('.vue')&&!e.name.includes('.test.')?[join(dir,e.name)]:[])}
// Only this exact admin log screen may show original log JSON and full request/302 URLs.
const logs=new Set(['operations/LogsPanel.vue']);
it('never restores developer-only blocks, props, JSON copying or raw object pre elements',()=>{
 const violations:string[]=[];
 for(const file of sources(root)){
  const name=relative(root,file).replaceAll('\\','/'),source=readFileSync(file,'utf8'),template=parse(source).descriptor.template?.content||'';
  if(/technical-data|technicalData/.test(source))violations.push(name+': removed prop');
  if(!logs.has(name)&&/技术细节|原始记录|技术详情/.test(source))violations.push(name+': developer label');
  if(!logs.has(name)&&(/<pre\b/.test(template)||/\{\{\s*JSON\.stringify\(/.test(template)||/复制原始\s*JSON|查看原始数据/.test(template)))violations.push(name+': raw display');
 }
 expect(violations).toEqual([]);
});
it('admin logs retain full records and copying with the business-facing button label',()=>{
 const source=readFileSync(join(root,'operations/LogsPanel.vue'),'utf8');
 expect(source).toContain('copy(JSON.stringify(detail,null,2))">复制日志');
 expect(source).toContain('JSON.stringify(Object.fromEntries(Object.entries(detail)');
 expect(source).toContain('detail.location');expect(source).toContain('detail.url');
});
