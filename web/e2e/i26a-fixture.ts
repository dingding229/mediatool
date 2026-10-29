import type {Page} from '@playwright/test';
import {attentionFixture} from './i19-2-fixture';
export const kinds=['open115','clouddrive2','qbittorrent'] as const;
export async function i26aFixture(page:Page){
 const original=await attentionFixture(page),at='2026-10-30T02:00:00Z';
 const names={open115:'115 官方授权',clouddrive2:'CloudDrive2',qbittorrent:'qBittorrent'};
 const rows=kinds.map(kind=>({id:`i26a-${kind}`,kind,name:names[kind],enabled:true,revision:1,settings:{},secretFields:[],lastCheckState:'ready',lastCheckedAt:at}));
 let unknown=false,hold=false,disabled=false;
 const writes:string[]=[],reads:string[]=[],release:(()=>void)[]=[];
 const states=Object.fromEntries(kinds.map(kind=>[kind,{state:'ready',lastCheckedAt:at,reason:'fresh_saved_probe',message:'连接成功'}]));
 await page.route('**/api/v1/integrations',r=>{reads.push('accounts');return r.fulfill({json:{data:rows.map(row=>({...row,enabled:!disabled}))}})});
 await page.route('**/api/v1/readiness',r=>{reads.push('readiness');return r.fulfill({json:{data:{components:{...states,strm:{state:'configured',reason:'stable_resolver_configured_not_player_verified',nextAction:'verify_player_302'}}}}})});
 await page.route('**/api/v1/integrations/*/test',async r=>{
  const kind=new URL(r.request().url()).pathname.split('/')[4]!.replace('i26a-','');writes.push(kind);
  if(hold)await new Promise<void>(resolve=>release.push(resolve));if(unknown)return r.abort('failed');
  const passed=kind!=='qbittorrent',checked='2026-10-30T02:01:00Z';
  Object.assign(states[kind]!,{state:passed?'ready':'check_failed',lastCheckedAt:checked,message:passed?'连接成功':'本地模拟：连接失败'});
  rows.find(row=>row.kind===kind)!.lastCheckedAt=checked;
  return r.fulfill({json:{data:{passed,state:passed?'ready':'check_failed'}}});
 });
 await page.route('**/api/v1/system/runtime',async r=>{
  reads.push('runtime');if(hold)await new Promise<void>(resolve=>release.push(resolve));
  return r.fulfill({json:{data:{server:{loadAverage:[.4,.5,.6],memoryUsed:2147483648,memoryTotal:8589934592,diskAvailable:107374182400,diskTotal:214748364800,uptimeSeconds:86400,sampledAt:at},daily:{sites:6,subscriptions:3,downloads:1,media:50,todaySearches:4}}}});
 });
 await page.route('**/api/v1/search',r=>r.fulfill({json:{data:[{id:'recent',keyword:'本地只读示例',total:24}]}}));
 await page.route('**/api/v1/ops/trends?*',r=>r.fulfill({json:{data:[0,1,2].map(i=>({at:new Date(Date.parse(at)+i*60000).toISOString(),memoryPercent:40+i,diskFreeGB:25,load1:.23,loadPerCPU:.05,pendingJobs:0,failedJobs10m:0,processBytes:67108864,jobSeconds:i===1?1:null,succeededJobs10m:i===1?10:null}))}}));
 return {writes,reads,original,lose:()=>unknown=true,
  failCloud:()=>Object.assign(states.clouddrive2!,{state:'check_failed',message:'本地模拟：连接检测超时，请检查网络后重试'}),
  disable:()=>{disabled=true;for(const state of Object.values(states))state.state='disabled'},hold:()=>hold=true,release:()=>{hold=false;release.splice(0).forEach(f=>f())}};
}
