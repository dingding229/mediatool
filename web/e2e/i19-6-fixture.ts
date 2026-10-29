import type {Page} from '@playwright/test';
import {u6OpsFixture,opsSnapshot} from './u6-fixture';
export const ownedID=(n:number)=>'19600000-0000-4000-8000-'+String(n).padStart(12,'0');
export const rollbackRevision='b'.repeat(40),frozenDigest='d'.repeat(64);
const at='2026-10-29T00:00:00Z';
export async function maintenanceFixture(page:Page,large=false){
 await u6OpsFixture(page);
 const backups=Array.from({length:large?26:5},(_,i)=>({id:ownedID(i+1),state:i===3?'failed':i===4?'running':'ready',pinned:i===1,bytes:1048576,origin:'manual',createdAt:new Date(Date.parse(at)-i*3600000).toISOString(),message:'本地模拟：完整备份说明保留',digest:frozenDigest,manifest:{schema:93}}));
 const drills=Array.from({length:large?26:3},(_,i)=>({id:ownedID(100+i),state:i===0?'verified':i===1?'failed':'interrupted',createdAt:at,message:'本地模拟：演练只在隔离库执行'}));
 const requests=Array.from({length:large?26:4},(_,i)=>({id:ownedID(200+i),kind:i%2?'rollback':'restore',state:i===1?'completed':i===3?'preview':'approved',digest:frozenDigest,expiresAt:'2099-01-01T00:00:00Z',createdAt:at,message:'本地模拟：仍须服务器受控执行',flow:i===2?{reason:'approval_expired'}:undefined,verifiedAt:undefined as string|undefined}));
 const snapshot:any={...opsSnapshot,backups,drills,requests,alerts:[],schema:93,mode:{paused:false,revision:19,reason:''},releases:[{revision:rollbackRevision,verified:true,current:false,schema:93},{revision:'c'.repeat(40),verified:true,current:true,schema:93},{revision:'e'.repeat(40),verified:false,current:false,schema:93}]};
 const writes:{path:string;body:any}[]=[],reads:string[]=[],webRecords:any[]=[];let lost=false,fault='',failRead=false;let latestPreview:any;
 await page.route('**/api/v1/ops',r=>{reads.push('ops');return failRead?r.fulfill({status:503,json:{error:{code:'fixture_unavailable',message:'本地模拟：维护记录读取失败'}}}):r.fulfill({json:{data:snapshot}})});
 await page.route('**/api/v1/ops/**',r=>{
  const path=new URL(r.request().url()).pathname,parts=path.split('/'),id=parts[5],action=parts[6],body=r.request().postData()?r.request().postDataJSON():{};
  if(r.request().method()==='GET')return path==='/api/v1/ops/restores'?r.fulfill({json:{data:webRecords}}):r.fallback();
  if(lost)return r.abort('failed');
  if(fault==='permission')return r.fulfill({status:403,json:{error:{code:'forbidden',message:'本地模拟：无维护权限'}}});
  const rejected=(message:string)=>r.fulfill({status:400,json:{error:{code:'maintenance_rejected',message}}});
  if(path==='/api/v1/ops/backups'){const row={...backups[0],id:ownedID(900),state:'pending'};backups.push(row as any);return r.fulfill({status:202,json:{data:{id:row.id}}})}
  if(parts[4]==='backups'){
   const row=backups.find(b=>b.id===id)!;
   if(action==='delete'){if(id===ownedID(3))return rejected('固定、最近两份成功备份或被维护任务引用的备份不可删除');row.state='deleted';return r.fulfill({json:{data:{deleted:true}}})}
   if(action==='pin'){row.pinned=body.pinned;return r.fulfill({json:{data:{saved:true}}})}
   if(action==='drill'){const d={...drills[0],id:ownedID(901),state:'pending'};drills.push(d);return r.fulfill({status:202,json:{data:{id:d.id}}})}
  }
  if(parts[4]==='drills'&&action==='cleanup')return r.fulfill({json:{data:{cleaned:true}}});
  if(path==='/api/v1/ops/requests/preview'){
   latestPreview={id:ownedID(902),kind:body.kind,state:'preview',digest:frozenDigest,expiresAt:'2099-01-01T00:00:00Z',createdAt:at,message:'本地预览，尚未执行'};requests.push(latestPreview);
   return r.fulfill({json:{data:{request:latestPreview,confirmText:(body.kind==='restore'?'恢复备份 ':'回滚版本 ')+body.id,command:'media-toolctl --root /root/docker/mediatool execute '+latestPreview.id}}});
  }
  if(parts[4]==='requests'){
   const row=requests.find(r=>r.id===id)!;
   if(action==='approve'){if(fault==='expired')return rejected('审批已过期，请重新预览');row.state='approved';return r.fulfill({json:{data:{state:'approved'}}})}
   if(action==='cancel'){row.state='cancelled';return r.fulfill({json:{data:{cancelled:true}}})}
   if(action==='verify'){row.verifiedAt=at;return r.fulfill({json:{data:{verified:true}}})}
  }
  if(path==='/api/v1/ops/resume'){snapshot.mode={paused:false,revision:20,reason:''};return r.fulfill({json:{data:snapshot.mode}})}
  if(path==='/api/v1/ops/restores/preview'){
   const restore={id:ownedID(903),backupId:body.backupId,state:'preview',digest:frozenDigest,expiresAt:'2099-01-01T00:00:00Z',backupSchema:93,programSchema:93,tableChanges:[]};webRecords.push(restore);
   return r.fulfill({json:{data:{restore,confirmText:'恢复备份 '+body.backupId,warnings:['管理员账号与密码将回到备份时状态','115授权将回到备份时的令牌'],includes:['全部业务表'],excludes:['现存媒体和STRM'],message:'本地模拟预览，不执行真实恢复'}}});
  }
  if(parts[4]==='restores'&&action==='execute')return r.fulfill({status:403,json:{error:{code:'recent_login_required',message:'恢复需要10分钟内的新近登录，请重新登录后重新预览并确认'}}});
  return r.fulfill({status:405,json:{error:{code:'fixture_write_blocked',message:'本地测试禁止未声明的运维动作'}}});
 });
 await page.route('**/api/v1/**',r=>{
  if(['GET','HEAD'].includes(r.request().method()))return r.fallback();const path=new URL(r.request().url()).pathname;writes.push({path,body:r.request().postDataJSON()});
  if(path.startsWith('/api/v1/ops/'))return r.fallback();return r.fulfill({status:405,json:{error:{code:'fixture_write_blocked',message:'本地测试禁止其他写操作'}}});
 });
 return {snapshot,writes,reads,lose:()=>lost=true,deny:()=>fault='permission',expire:()=>fault='expired',noTools:()=>snapshot.toolsReady=false,
  failRead:()=>failRead=true,empty:()=>{snapshot.backups=[];snapshot.drills=[];snapshot.requests=[]},paused:()=>snapshot.mode={paused:true,revision:19,reason:'本地维护状态'}};
}
