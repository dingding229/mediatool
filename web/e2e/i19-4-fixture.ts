import type {Page} from '@playwright/test';
import {u6OpsFixture,opsSnapshot} from './u6-fixture';
export const maintenancePolicy={backupEnabled:false,backupCron:'0 2 * * *',keepDays:14,keepCount:7,sampleDays:14,alertsEnabled:true,memoryPercent:90,diskFreeGB:10,loadPerCPU:2,failedJobs:5,consecutive:3,cooldownMinutes:30};
export async function policyLogFixture(page:Page){
 await u6OpsFixture(page);
 let ops={...opsSnapshot,policy:{value:{...maintenancePolicy},revision:7,nextBackupAt:'2026-10-30T02:00:00+08:00'}},settings={revision:9,value:{playbackRetentionDays:30,logging:{retentionDays:7,debugUntil:'0001-01-01T00:00:00Z'},movieTemplate:'preserve-me',access:{private:'preserved'}}};
 const writes:{path:string;body:any}[]=[],previews:any[]=[],reads:string[]=[];
 let conflict=false,readError=false,lost=false,summaryError=false,empty=false;
 const files=[{name:'application.log',size:1024,modified:'2026-10-29T00:00:00Z'},{name:'application-with-a-long-owned-local-fixture-name.1.log',size:2048,modified:'2026-10-28T00:00:00Z'}];
 await page.route('**/api/v1/ops',r=>r.fulfill({json:{data:ops}}));
 await page.route('**/api/v1/ops/policy',r=>{
  const input=r.request().postDataJSON();if(lost)return r.abort('failed');
  if(conflict){ops={...ops,policy:{...ops.policy,revision:8,value:{...ops.policy.value,keepDays:21}}};return r.fulfill({status:409,json:{error:{code:'changed',message:'本地模拟：策略冲突'}}})}
  ops={...ops,policy:{...ops.policy,value:input.value,revision:input.revision+1}};return r.fulfill({json:{data:ops.policy}});
 });
 await page.route('**/api/v1/settings/application',r=>{
  reads.push('settings');if(readError)return r.fulfill({status:503,json:{error:{code:'fixture_unavailable',message:'本地模拟：日志设置读取失败'}}});
  if(r.request().method()==='GET')return r.fulfill({json:{data:settings}});
  if(lost)return r.abort('failed');if(conflict){settings={...settings,revision:10,value:{...settings.value,logging:{...settings.value.logging,retentionDays:14}}};return r.fulfill({status:409,json:{error:{code:'changed',message:'本地模拟：日志设置冲突'}}})}
  const input=r.request().postDataJSON();settings={value:input.value,revision:input.revision+1};return r.fulfill({json:{data:settings}});
 });
 await page.route('**/api/v1/logs/summary',r=>summaryError?r.fulfill({status:503,json:{error:{code:'fixture_unavailable',message:'本地模拟：日志文件读取失败'}}}):r.fulfill({json:{data:{level:'info',totalBytes:3072,limitBytes:268435456,debugActive:false,files:empty?[]:files}}}));
 await page.route('**/api/v1/scheduling/cron/preview',r=>{
  const input=r.request().postDataJSON();previews.push(input);if(input.expression==='* * * * *')return r.fulfill({status:400,json:{error:{code:'cron_too_frequent',message:'本地模拟：未满足备份最小间隔'}}});
  return r.fulfill({json:{data:{timezone:'Asia/Shanghai',fieldCount:input.expression.trim().split(/\s+/).length,serverNow:'2026-10-29T00:00:00Z',next:['2026-10-30T00:00:00Z'],exhausted:input.expression.includes('1970'),minimumIntervalSeconds:300}}});
 });
 await page.route('**/api/v1/**',r=>{
  if(['GET','HEAD'].includes(r.request().method()))return r.fallback();const path=new URL(r.request().url()).pathname;
  if(path==='/api/v1/scheduling/cron/preview')return r.fallback();writes.push({path,body:r.request().postDataJSON()});
  if(path==='/api/v1/ops/policy'||path==='/api/v1/settings/application')return r.fallback();
  return r.fulfill({status:405,json:{error:{code:'fixture_write_blocked',message:'本地测试禁止其他运维动作'}}});
 });
 return {writes,reads,previews,conflict:()=>conflict=true,failSettings:()=>readError=true,failSummary:()=>summaryError=true,emptyFiles:()=>empty=true,lose:()=>lost=true,
 activeDebug:(until:string)=>settings.value.logging.debugUntil=until};
}
