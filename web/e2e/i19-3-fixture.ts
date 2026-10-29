import type {Page} from '@playwright/test';
import {u6OpsFixture,opsSnapshot} from './u6-fixture';
const at='2026-10-29T00:00:00Z';
export async function cacheAlertFixture(page:Page,large=false){
 await u6OpsFixture(page);
 const caches=Object.fromEntries(['tmdb','search'].map(kind=>[kind,Array.from({length:large?31:4},(_,i)=>({id:`i193-${kind}-${i}`,query:i===1?'长名称缓存查询条目：手机换行时操作按钮不应被挤出卡片':'自有缓存样例 '+i,count:i+1,bytes:2048,createdAt:at,ttlSeconds:i===2?-1:i===3?0:3600,state:'succeeded',reusable:i!==3}))]));
 const alerts=Array.from({length:large?26:3},(_,i)=>({code:'i193-alert-'+i,message:i===0?'内存使用率偏高：本地只读样例告警':'本地告警 '+i,state:i===2?'normal':'active',value:95,hits:3,last_seen_at:at,acknowledged_at:i===1?at:null}));
 const writes:{method:string;path:string;body:unknown}[]=[],cacheReads:URLSearchParams[]=[],detailReads:string[]=[];
 let cacheFailed=false,opsFailed=false,cacheEmpty=false,alertEmpty=false,hold=false,lost=false,rejected=false;
 const releases:(()=>void)[]=[];
 await page.route('**/api/v1/cache/**',async r=>{
  const u=new URL(r.request().url()),parts=u.pathname.split('/'),kind=parts[4]!,id=decodeURIComponent(parts[5]||''),rows=caches[kind]||[];
  if(r.request().method()==='GET'){
   if(id){detailReads.push(id);const row=rows.find(row=>row.id===id);return row?r.fulfill({json:{data:{...row,data:{title:row.query,proof:'原始缓存内容'}}}}):r.fulfill({status:404,json:{error:{code:'not_found',message:'缓存项不存在'}}})}
   cacheReads.push(u.searchParams);if(hold)await new Promise<void>(resolve=>releases.push(resolve));
   if(cacheFailed)return r.fulfill({status:503,json:{error:{code:'fixture_unavailable',message:'本地模拟：缓存读取失败'}}});
   const size=Number(u.searchParams.get('pageSize')||30),number=Number(u.searchParams.get('page')||1),all=cacheEmpty?[]:rows;
   return r.fulfill({json:{data:{items:all.slice((number-1)*size,number*size),total:all.length,truncated:false}}});
  }
  if(lost)return r.abort('failed');if(rejected)return r.fulfill({status:400,json:{error:{code:'fixture_rejected',message:'本地模拟：拒绝清理'}}});
  if(id==='clear'){const deleted=rows.length;caches[kind]=[];return r.fulfill({json:{data:{deleted,remainingMayExist:false}}})}
  caches[kind]=rows.filter(row=>row.id!==id);return r.fulfill({json:{data:{deleted:true}}});
 });
 await page.route('**/api/v1/ops',r=>opsFailed?r.fulfill({status:503,json:{error:{code:'fixture_unavailable',message:'本地模拟：告警读取失败'}}}):r.fulfill({json:{data:{...opsSnapshot,alerts:alertEmpty?[]:alerts}}}));
 await page.route('**/api/v1/ops/alerts/*/ack',r=>{
  if(lost)return r.abort('failed');const code=new URL(r.request().url()).pathname.split('/')[5];const alert=alerts.find(a=>a.code===code);if(alert)alert.acknowledged_at=at;
  return r.fulfill({json:{data:{acknowledged:true}}});
 });
 await page.route('**/api/v1/**',r=>{
  if(['GET','HEAD'].includes(r.request().method()))return r.fallback();
  const path=new URL(r.request().url()).pathname;writes.push({method:r.request().method(),path,body:r.request().postData()?r.request().postDataJSON():null});
  if(path.startsWith('/api/v1/cache/')||/^\/api\/v1\/ops\/alerts\/[^/]+\/ack$/.test(path))return r.fallback();
  return r.fulfill({status:405,json:{error:{code:'fixture_write_blocked',message:'本地测试禁止其他运维写操作'}}});
 });
 return {writes,cacheReads,detailReads,failCache:()=>cacheFailed=true,failOps:()=>opsFailed=true,emptyCache:()=>cacheEmpty=true,emptyAlerts:()=>alertEmpty=true,
 hold:()=>hold=true,release:()=>{hold=false;releases.splice(0).forEach(f=>f())},lose:()=>lost=true,reject:()=>rejected=true,
 removeOriginal:(kind:string,id:string)=>caches[kind]=(caches[kind]||[]).filter(row=>row.id!==id)};
}
