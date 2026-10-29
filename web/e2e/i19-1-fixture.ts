import type {Page} from '@playwright/test';
import {u6OpsFixture} from './u6-fixture';
export const jobRows=['native.tmdb.fetch','site.metrics.refresh','library.scan','future.custom.job'].map((kind,i)=>({id:'i191-'+i,kind,queue:i===2?'library':'default',state:i===1?'failed':i===2?'running':'succeeded',attempts:1,maxAttempts:3,startedAt:new Date(Date.now()-30000).toISOString(),updatedAt:new Date().toISOString(),lastError:i===1?'本地模拟：先核对业务再重试':''}));
export async function jobsFixture(page:Page,large=false){
 await u6OpsFixture(page);
 const data=large?Array.from({length:26},(_,i)=>({...jobRows[i%4]!,id:'i191-'+i})):jobRows;
 const reads:URLSearchParams[]=[],writes:{path:string;body:unknown}[]=[],states=new Map<string,any>();
 let hold=false,failed=false,empty=false,lost=false;const releases:(()=>void)[]=[];
 await page.route('**/api/v1/jobs/browse?*',async r=>{
  const q=new URL(r.request().url()).searchParams;reads.push(q);
  if(hold)await new Promise<void>(resolve=>releases.push(resolve));
  if(failed)return r.fulfill({status:503,json:{error:{code:'local_fixture_unavailable',message:'本地模拟：任务暂时不可读取'}}});
  const number=Number(q.get('page')||1),size=Number(q.get('pageSize')||25),all=empty?[]:data;
  return r.fulfill({json:{data:{items:all.slice((number-1)*size,number*size),total:all.length}}});
 });
 await page.route('**/api/v1/jobs/i191-*',r=>r.fulfill({json:{data:{...data.find(j=>r.request().url().endsWith(j.id)),payload:{note:'仅供本地任务详情验证'}}}}));
 await page.route('**/api/v1/tasks/jobs/**',async r=>{
  const path=new URL(r.request().url()).pathname,parts=path.split('/'),id=parts[5]!;
  if(r.request().method()==='GET')return r.fulfill({json:{data:states.get(id)||{state:'failed',revision:7,reason:'已核对原任务'}}});
  writes.push({path,body:r.request().postDataJSON()});
  if(lost){states.set(id,{state:'unknown',revision:8,reason:'保留未知状态，需继续核对'});return r.abort('failed')}
  const result={state:'checking',revision:8,queued:true,reason:'请求已受理，尚未确认完成'};states.set(id,result);return r.fulfill({status:202,json:{data:result}});
 });
 return {reads,writes,hold:()=>hold=true,release:()=>{hold=false;releases.splice(0).forEach(f=>f())},fail:()=>failed=true,empty:()=>empty=true,lose:()=>lost=true};
}
