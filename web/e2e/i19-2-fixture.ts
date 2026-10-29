import type {Page} from '@playwright/test';
import {u6OpsFixture,opsSnapshot} from './u6-fixture';
const at='2026-10-29T00:00:00Z';
export const runtimeSnapshot={postgres:{state:'ready'},redis:{state:'ready'},databaseBytes:536870912,
 workers:[{owner:'i192-worker-original-identity',queue:'default',online:true,lastSeenAt:at},{owner:'i192-offline-worker',queue:'transfer',online:false,lastSeenAt:at}],
 queues:[{name:'default',state:'pending',count:2,expiredLeases:0},{name:'transfer',state:'retry',count:1,expiredLeases:1}],
 schedules:[{id:'backup',name:'系统备份',cron:'0 2 * * *',enabled:false,nextRunAt:null,lastRunAt:at,link:'/ops?tab=maintenance'},{id:'subscription',name:'订阅调度',cron:'*/30 * * * *',enabled:true,nextRunAt:at,lastRunAt:at,link:'/settings?tab=automation'}]};
export async function attentionFixture(page:Page){
 await u6OpsFixture(page);
 await page.addInitScript(()=>localStorage.setItem('media-tool:setup-guide-hidden:v1','1'));
 const writes:{path:string;body:unknown}[]=[],reads:string[]=[];
 let healthFailed=false,inboxFailed=false,nonempty=false,noRuntime=false,hold=false,lost=false,jobState='running';
 const release:(()=>void)[]=[];
 await page.route('**/api/v1/system/operations',async r=>{
  reads.push('health');if(hold)await new Promise<void>(resolve=>release.push(resolve));
  if(healthFailed)return r.fulfill({status:503,json:{error:{code:'local_fixture_unavailable',message:'本地模拟：运行检查读取失败'}}});
  return r.fulfill({json:{data:noRuntime?{...runtimeSnapshot,workers:[],queues:[],schedules:[]}:runtimeSnapshot}});
 });
 await page.route('**/api/v1/jobs/browse?*',async r=>{
  if(hold)await new Promise<void>(resolve=>release.push(resolve));
  if(inboxFailed)return r.fulfill({status:503,json:{error:{code:'local_fixture_unavailable',message:'本地模拟：失败任务读取失败'}}});
  return r.fulfill({json:{data:{items:[],total:0}}});
 });
 await page.route('**/api/v1/ops',r=>r.fulfill({json:{data:{...opsSnapshot,alerts:nonempty?opsSnapshot.alerts:[]}}}));
 await page.route('**/api/v1/readiness',r=>r.fulfill({json:{data:{components:Object.fromEntries(['open115','clouddrive2','qbittorrent'].map(k=>[k,{state:'ready',lastCheckedAt:at,message:'连接成功'}]))}}}));
 await page.route('**/api/v1/jobs',r=>r.fulfill({status:202,json:{data:{id:'i192-check'}}}));
 await page.route('**/api/v1/jobs/i192-check',r=>{reads.push('job');return r.fulfill({json:{data:{id:'i192-check',state:jobState}}})});
 await page.route('**/api/v1/**',r=>{
  if(['GET','HEAD'].includes(r.request().method()))return r.fallback();
  const path=new URL(r.request().url()).pathname;writes.push({path,body:r.request().postDataJSON()});
  if(path==='/api/v1/jobs'){if(lost)return r.abort('failed');return r.fallback()}
  return r.fulfill({status:405,json:{error:{code:'fixture_write_blocked',message:'本地测试不允许此写操作'}}});
 });
 return {writes,reads,failHealth:()=>healthFailed=true,failInbox:()=>inboxFailed=true,nonempty:()=>nonempty=true,emptyRuntime:()=>noRuntime=true,
  hold:()=>hold=true,release:()=>{hold=false;release.splice(0).forEach(f=>f())},lose:()=>lost=true,finish:()=>jobState='succeeded'};
}
