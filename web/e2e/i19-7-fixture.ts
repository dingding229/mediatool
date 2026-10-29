import type {Page} from '@playwright/test';
import {u6OpsFixture,opsSnapshot} from './u6-fixture';
export const restoreID=(n:number)=>'19700000-0000-4000-8000-'+String(n).padStart(12,'0');
export const restoreDigest='a'.repeat(64),snapshotDigest='b'.repeat(64);
const at='2026-10-29T00:00:00Z';
export async function webRestoreFixture(page:Page,large=false){
 await u6OpsFixture(page);
 let backups=Array.from({length:large?27:3},(_,i)=>({id:restoreID(i+1),state:i===2?'failed':'ready',origin:i===1?'pre-restore':'manual',createdAt:at,bytes:1048576,digest:i===1?snapshotDigest:restoreDigest,manifest:{schema:93},pinned:false}));
 let records:any[]=Array.from({length:large?27:4},(_,i)=>({id:restoreID(100+i),backupId:restoreID(1),state:['unknown','completed','rolled_back','failed'][i%4],digest:restoreDigest,createdAt:at,snapshotId:i%4===1||i%4===2?restoreID(2):null,message:['本地模拟：请求中断，需核对原结果','本地模拟：恢复已完成，恢复前快照保留','本地模拟：恢复失败后已补偿回滚','本地模拟：校验失败，未执行恢复'][i%4]}));
 const writes:{path:string;body:any}[]=[],reads:string[]=[];let hold=false,readFailure=false,backupFailure=false,lost=false,executionFailure='',previewFailure=false,latest:any;const releases:(()=>void)[]=[];
 await page.route('**/api/v1/ops',r=>backupFailure?r.fulfill({status:503,json:{error:{code:'fixture_unavailable',message:'本地模拟：备份读取失败'}}}):r.fulfill({json:{data:{...opsSnapshot,backups,schema:93,requests:[],drills:[],alerts:[]}}}));
 await page.route('**/api/v1/ops/restores',async r=>{
  reads.push('restores');if(hold)await new Promise<void>(resolve=>releases.push(resolve));if(readFailure)return r.fulfill({status:503,json:{error:{code:'fixture_unavailable',message:'本地模拟：恢复记录读取失败'}}});
  return r.fulfill({json:{data:records}});
 });
 await page.route('**/api/v1/ops/restores/preview',r=>{
  if(previewFailure)return r.fulfill({status:400,json:{error:{code:'maintenance_rejected',message:'本地模拟：备份校验失败，不允许预览'}}});
  const backupId=r.request().postDataJSON().backupId,backup=backups.find(b=>b.id===backupId);
  latest={id:restoreID(900),backupId,state:'preview',digest:backup?.digest||restoreDigest,createdAt:at,expiresAt:'2099-01-01T00:00:00Z',backupSchema:93,programSchema:93,tableChanges:[{name:'media_users',currentRows:3,backupRows:2},{name:'open115_authorizations',currentRows:1,backupRows:1}]};
  records=[latest,...records.filter(r=>r.id!==latest.id)];return r.fulfill({json:{data:{restore:latest,confirmText:'恢复 '+backupId,includes:['全部public业务表随备份回退'],excludes:['主密钥、现存STRM正文、媒体及外部服务磁盘数据'],warnings:['管理员账号与密码将回到备份时状态','115授权将回到备份时的令牌，可能需要重新授权','当前登录会话将失效；请使用备份中的账号密码重新登录','外部API令牌、权限与插件启用/定时配置也会回退，请核对'],message:'本地模拟：已验证备份，确认后先创建恢复前快照。'}}});
 });
 await page.route('**/api/v1/ops/restores/*/execute',r=>{
  if(executionFailure){const status=executionFailure==='expired'?409:403,code=executionFailure==='recent'?'recent_login_required':executionFailure==='permission'?'forbidden':'maintenance_rejected',message=executionFailure==='recent'?'恢复需要10分钟内的新近登录，请重新登录后重新预览并确认':executionFailure==='permission'?'本地模拟：没有恢复权限':'本地模拟：恢复预览已过期，请重新预览';return r.fulfill({status,json:{error:{code,message}}})}
  latest.state=lost?'unknown':'pending';latest.message=lost?'本地模拟：回执丢失，需核对原结果':'本地模拟：已排队，尚未恢复';if(lost)return r.abort('failed');return r.fulfill({status:202,json:{data:latest}});
 });
 await page.route('**/api/v1/ops/restores/*/reconcile',r=>r.fulfill({json:{data:{reconciled:true}}}));
 await page.route('**/api/v1/**',r=>{
  if(['GET','HEAD'].includes(r.request().method()))return r.fallback();const path=new URL(r.request().url()).pathname;writes.push({path,body:r.request().postDataJSON()});
  if(path.startsWith('/api/v1/ops/restores/'))return r.fallback();return r.fulfill({status:405,json:{error:{code:'fixture_write_blocked',message:'本地测试禁止其他维护动作'}}});
 });
 return {writes,reads,hold:()=>hold=true,release:()=>{hold=false;releases.splice(0).forEach(f=>f())},failRecords:()=>readFailure=true,failBackups:()=>backupFailure=true,lose:()=>lost=true,reject:(kind:string)=>executionFailure=kind,failPreview:()=>previewFailure=true,empty:()=>{backups=[];records=[]},result:(state:string)=>{latest.state=state;latest.snapshotId=restoreID(2);latest.message=state==='rolled_back'?'本地模拟：失败后补偿回滚完成':'本地模拟：未执行恢复'}};
}
