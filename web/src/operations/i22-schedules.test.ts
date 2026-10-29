import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
import {compile,createSSRApp} from 'vue';
import {renderToString} from 'vue/server-renderer';
import {date} from '../workflows/readiness';

// Render the real changed template fragments, without mocking their expressions.
const fragment=(file:string,marker:string)=>{
 const source=readFileSync(new URL(file,import.meta.url),'utf8');
 const line=source.split('\n').find(line=>line.includes(marker))!;
 expect(line).toBeTruthy();
 return line.match(/<p(?:\s[^>]*)?>.*?<\/p>/)![0];
};
const cases=[
 {name:'运行计划',template:fragment('RuntimeChecks.vue',"s.enabled?'下次 '"),bindings:(enabled:boolean,next:string|null)=>({s:{enabled,nextRunAt:next},date})},
 {name:'备份页头',template:fragment('MaintenancePanel.vue','当前备份 {{'),bindings:(enabled:boolean,next:string|null)=>({data:{data:{totalBytes:1024,policy:{value:{backupEnabled:enabled},nextBackupAt:next}}},bytes:()=> '1 KiB',date})},
 {name:'备份策略',template:fragment('PolicyPanel.vue',"?'下次备份 '"),bindings:(enabled:boolean,next:string|null)=>({read:{data:{policy:{value:{backupEnabled:enabled},nextBackupAt:next}}},date})},
];
const next='2026-10-26T03:00:00+08:00';
for(const c of cases){
 const render=(enabled:boolean,value:string|null)=>renderToString(createSSRApp({setup:()=>c.bindings(enabled,value),render:compile(c.template)}));
 it(`${c.name} shows disabled rather than stale time or exhausted schedule`,async()=>{
  for(const value of [null,next]){
   const html=await render(false,value);
   expect(html).toContain('已停用');expect(html).not.toContain(date(next));expect(html).not.toContain('计划已结束');expect(html).not.toContain('下次');
  }
 });
 it(`${c.name} preserves enabled time and exhausted schedule`,async()=>{
  expect(await render(true,next)).toContain(date(next));
  expect(await render(true,null)).toContain('计划已结束');
  expect(await render(true,next)).not.toContain('已停用');
 });
}
