import type {Page} from '@playwright/test';
import {u6OpsFixture} from './u6-fixture';
const at='2026-10-29T00:00:00Z';
export const rawRequestURL='https://media.example.test/resolve?credential=administrator-original-value&path='+encodeURIComponent('/自有测试目录/完整文件名.mkv')+'&long='+('original-value-'.repeat(20))+'END-REQUEST';
export const rawRedirectURL='https://cdn.example.test/media.mkv?signature=complete-original-signature&expires=9999999999&long='+('cdn-value-'.repeat(20))+'END-302';
export const logRows=[
 {time:at,level:'info',module:'httpapi',msg:'HTTP request',source:{file:'internal/platform/httpapi/resolver.go',line:42},url:rawRequestURL,location:rawRedirectURL,status:302,traceId:'trace-original',spanId:'span-original',extra:{owned:true}},
 {time:at,level:'warn',module:'integrations',msg:'连接检测失败，请检查网络、凭据和服务状态',source:{file:'internal/modules/integrations/health.go',line:226}},
 {time:at,level:'error',module:'library',msg:'本地模拟整理失败；保留完整错误原文和来源',source:{file:'internal/modules/library/engine.go',line:100}},
 {time:at,level:'debug',module:'library',msg:'本地模拟 Debug 原始记录',source:'original-file.go:88'}
];
export async function logConsoleFixture(page:Page,large=false){
 await u6OpsFixture(page);
 let rows=large?Array.from({length:201},(_,i)=>({...logRows[i%4],msg:i===0?logRows[0]!.msg:'本地原始记录 '+i})):logRows;
 let failed=false,hold=false,lost=false;const releases:(()=>void)[]=[];
 const reads:URLSearchParams[]=[],writes:{path:string;body:unknown}[]=[],transports:string[]=[];
 page.on('request',r=>{if(r.url().includes('/api/'))transports.push(r.method()+' '+new URL(r.url()).pathname)});
 await page.route('**/api/v1/logs/query?*',async r=>{
  const q=new URL(r.request().url()).searchParams;reads.push(q);if(hold)await new Promise<void>(resolve=>releases.push(resolve));
  if(failed)return r.fulfill({status:503,json:{error:{code:'fixture_unavailable',message:'本地模拟：日志查询失败'}}});
  const size=Number(q.get('pageSize')||200),number=Number(q.get('page')||1);
  return r.fulfill({json:{data:{items:rows.slice((number-1)*size,number*size),total:rows.length,before:at,modules:['httpapi','library','integrations'],truncated:false}}});
 });
 await page.route('**/api/v1/logs/clear',r=>{if(lost)return r.abort('failed');rows=[];return r.fulfill({json:{data:{deleted:true}}})});
 await page.route('**/api/v1/**',r=>{
  if(['GET','HEAD'].includes(r.request().method()))return r.fallback();const path=new URL(r.request().url()).pathname;writes.push({path,body:r.request().postDataJSON()});
  if(path==='/api/v1/logs/clear')return r.fallback();return r.fulfill({status:405,json:{error:{code:'fixture_write_blocked',message:'本地测试禁止其他运维动作'}}});
 });
 return {reads,writes,transports,hold:()=>hold=true,release:()=>{hold=false;releases.splice(0).forEach(f=>f())},fail:()=>failed=true,empty:()=>rows=[],lose:()=>lost=true};
}
