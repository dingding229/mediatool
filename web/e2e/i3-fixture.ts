import type {Page} from '@playwright/test';
import {fixtureFulfill} from './collection-fixture';
import {u7Fixture} from './u7-fixture';
import {libraryItem} from './u4-fixture';
import {u3Media,u3Run} from './u3-fixture';
import {mediaUser} from './u5-fixture';
import {opsSnapshot} from './u6-fixture';

const at='2026-10-25T00:00:00Z';
const asset={...libraryItem,path:'/115/电影/海边的故事.mkv',source:'cloud',cloudState:'present',artifactsState:'ready',revision:1,outputs:[{kind:'strm',path:'海边的故事.strm'}]};
const history={id:'i3-history',kind:'file',state:'succeeded',source:asset.path,destination:'/媒体/电影/海边的故事.mkv',createdAt:at,updatedAt:at,media:{title:asset.title,year:'2026'},details:{plan:{steps:Array.from({length:30},(_,i)=>({action:'mkdir',target:`/电影/核对目录 ${i+1}`}))}}};
const job={id:'11111111-1111-4111-8111-111111111112',kind:'system.check',queue:'default',state:'succeeded',attempts:1,maxAttempts:3,createdAt:at,updatedAt:at,payload:{}};
const key={id:'11111111-2222-4333-8444-555555555555',name:'只读核验凭据',scopes:['status.read','library.read'],createdAt:at,revokedAt:null};
const subscription={id:'i3-sub',title:asset.title,media:u3Media,enabled:true,revision:1,source:'both',rules:{accountIds:[],include:[],exclude:[],freeOnly:true,rejectHR:true},lastState:'succeeded',lastMessage:'等待下次检查'};
const run={id:'i3-run',subscriptionId:subscription.id,state:'succeeded',preview:true,decisions:[],createdAt:at};
const pageOf=(items:unknown[])=>({items,total:items.length});

// Browser-only fixtures: API transport remains in the app API layer. No real mutation reaches a backend.
export async function i3Fixture(page:Page,base:string){
 await page.unrouteAll({behavior:'wait'});
 await u7Fixture(page,base);
 await page.route('**/api/v1/**',async r=>{
  const url=new URL(r.request().url()),path=url.pathname.slice('/api/v1'.length);
  const data:Record<string,unknown>={
   '/media/library/sources':{items:[{instanceId:'plugin-owned',tmdbId:42,mediaType:'movie',title:asset.title,year:'2026'}],hasMore:false,truncated:false},
   '/metadata/details':{item:u3Media},
   '/cache/search':pageOf([{id:'run-u3',query:asset.title,state:'succeeded',count:1,createdAt:at}]),
   '/cache/search/run-u3':{id:'run-u3',query:asset.title,state:'succeeded',data:u3Run},
   '/cache/tmdb':pageOf([{id:'tmdb-owned',query:asset.title,createdAt:at}]),
   '/cache/tmdb/tmdb-owned':{id:'tmdb-owned',query:asset.title,data:{title:asset.title}},
   '/files/operations':[{id:'file-op-owned',state:'succeeded',kind:'rename',source:asset.path,destination:'/115/电影/海边的故事 (2026).mkv',createdAt:at,updatedAt:at}],
   '/library/outputs':pageOf([asset]),
   '/library/outputs/owned-asset/content':{content:'http://resolver.example.test/stable-fixture',path:'海边的故事.strm'},
   '/library/activity':pageOf([history]),
   '/library/scans/scan-owned':{scan:{id:'scan-owned',root:'/115/媒体',state:'running',files:24,directories:1,completedDirectories:1,updatedAt:at},directories:[{path:'/115/媒体/电影',state:'completed',updatedAt:at}]},
   '/media/assets':pageOf([asset]),
   '/media/assets/owned-asset':asset,
   '/jobs/browse':pageOf([job]),
   ['/jobs/'+job.id]:job,
   '/strm/repairs/preview':{id:'i3-repair',state:'preview',digest:'fixture',expiresAt:'2099-01-01T00:00:00Z',items:[{ordinal:1,state:'missing',path:'海边的故事.strm',message:'缺失文件可核对'}]},
   '/media-server/native/sessions':pageOf([{id:'i3-session',userId:mediaUser.id,userName:'家庭成员',deviceName:'验收播放器',itemName:asset.title,state:'playing',positionTicks:0,lastProgressAt:at}]),
   ['/media-server/users/'+mediaUser.id]:{...mediaUser,deviceCount:3,devices:[],activeSessions:1},
   '/media-server/native/processing':{settings:{enabled:false,probeEnabled:true,stillsEnabled:true,revision:1},probe:{},still:{},capacity:{usedBytes:0,reservedBytes:0,limitBytes:1048576}},
   '/media-server/native/processing/tasks':pageOf([]),
   '/subscriptions':[subscription],
   ['/subscriptions/'+subscription.id+'/history']:[run],
   ['/subscription-runs/'+run.id]:run,
   '/external-api/keys':[key],
   '/external-api/calls':Array.from({length:30},(_,i)=>({keyId:key.id,method:'GET',path:`/api/external/v1/status?item=${i}`,status:200,createdAt:at,clientIP:'192.0.2.10',ipSource:'direct'})),
   '/ops/restores':[],
   '/ops/restores/preview':{message:'只读核验预览；不会执行恢复',confirmText:'RESTORE FIXTURE',included:['账号、设置与媒体记录'],excluded:['主密钥与磁盘媒体'],warnings:['仅测试预览'],tableCounts:Array.from({length:20},(_,i)=>({name:'fixture_table_'+i,currentRows:1,backupRows:1})),restore:{id:'i3-restore',backupId:'owned-backup',state:'preview',digest:'fixture',backupSchema:90,programSchema:90,expiresAt:'2099-01-01T00:00:00Z'}},
  };
  if(path==='/plugins/operations')return r.fulfill({status:400,json:{error:{code:'invalid_operation',message:'fixture operation rejected',details:{reason:'audit-width'}}}});
  if(base==='/ops'&&path==='/ops')return fixtureFulfill(r,{json:{data:opsSnapshot}});
  if(path in data)return fixtureFulfill(r,{json:{data:data[path]}});
  return r.fallback();
 });
}
