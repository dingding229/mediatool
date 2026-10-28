import {test,expect,type Page} from '@playwright/test';
import {readFileSync,mkdirSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {organizeFixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
const out=resolve(import.meta.dirname,'../../scratchpad/i-phase/i12/screens');
const screenshots=process.env.I12_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const pending='12121212-0000-4000-8000-000000000004',conflict='12121212-0000-4000-8000-000000000005',transfer='12121212-0000-4000-8000-000000000006';
const at='2026-10-28T10:00:00Z';
async function fixture(page:Page){
 await organizeFixture(page);
 const writes:Array<{path:string;body:any}>=[],reads:string[]=[],unexpected:string[]=[];
 let ready=true;
 const rows=[{id:pending,state:'pending'},{id:conflict,state:'conflict'}].map(r=>({...r,kind:'output',source:`Movies/I12 本地样片/${r.state}.strm`,updatedAt:at,createdAt:at,details:{assetId:'owned-asset',kind:'strm',operation:'write'}}));
 page.on('request',r=>{const u=new URL(r.url());if(u.pathname.startsWith('/api/v1/library/archiv')||u.pathname==='/api/v1/library/artifact-repairs')unexpected.push(u.pathname)});
 await page.route('**/api/v1/library/activity?*',r=>{const q=new URL(r.request().url()).searchParams;reads.push(q.toString());const items=rows.filter(x=>(!q.get('kind')||q.get('kind')===x.kind)&&(!q.get('state')||q.get('state')===x.state));return fixtureFulfill(r,{json:{data:{items,total:items.length}}})});
 await page.route('**/api/v1/library/artifact-repairs/*/retry',r=>{const path=new URL(r.request().url()).pathname;writes.push({path,body:r.request().postDataJSON()});rows.find(x=>path.includes(x.id))!.state='applied';return r.fulfill({json:{data:{repaired:true}}})});
 await page.route('**/api/v1/transfers{,?*}',r=>fixtureFulfill(r,{json:{data:[{id:transfer,destination:'/115/I12 本地样片.mkv',state:'needs_output_reconciliation',bytes:5,total:5,controlRevision:1}]}}));
 await page.route(`**/api/v1/transfers/${transfer}/output-preview`,r=>r.fulfill({json:{data:{ready,state:'needs_output_reconciliation',assetId:'owned-asset',controlRevision:1,planDigest:'a'.repeat(64),needsMigration:!ready,existingPaths:['Movies/I12 本地样片/Movie.strm'],expectedPaths:['Movies/I12 本地样片/Movie.strm']}}}));
 await page.route(`**/api/v1/transfers/${transfer}/resume-import`,r=>{writes.push({path:new URL(r.request().url()).pathname,body:r.request().postDataJSON()});return r.fulfill({json:{data:{approved:true,uploadRestarted:false}}})});
 return {writes,reads,unexpected,setReady:(v:boolean)=>ready=v,rows};
}
test('I12 independent tab removed; pending/conflict outputs remain discoverable and retry requires confirmation',async({page})=>{
 const f=await fixture(page);await page.goto('/organize?tab=history');
 await expect(page.locator('.media-workspace .page-tabs').getByRole('button',{name:'恢复与归档',exact:true})).toHaveCount(0);
 await page.getByRole('button',{name:'待恢复输出',exact:true}).click();await expect(page.locator('.f5-history-row')).toHaveCount(1);await expect.poll(()=>f.reads.some(q=>q.includes('kind=output&state=pending'))).toBe(true);
 await page.getByRole('button',{name:'查看步骤详情',exact:true}).click();const detail=page.getByRole('dialog',{name:'整理步骤详情',exact:true});await detail.getByRole('button',{name:'重试恢复',exact:true}).click();
 const confirmation=page.getByRole('dialog',{name:'重试输出恢复',exact:true});expect(f.writes).toHaveLength(0);await confirmation.getByRole('button',{name:'取消',exact:true}).click();expect(f.writes).toHaveLength(0);
 await detail.getByRole('button',{name:'重试恢复',exact:true}).click();await confirmation.getByRole('button',{name:'确认重试恢复',exact:true}).click();await expect(detail).toBeHidden();await expect(page.locator('.f5-history-row')).toHaveCount(0);
 expect(f.writes).toEqual([{path:`/api/v1/library/artifact-repairs/${pending}/retry`,body:{}}]);
 await page.getByRole('button',{name:'冲突输出',exact:true}).click();await expect(page.locator('.f5-history-row')).toHaveCount(1);await expect.poll(()=>f.reads.some(q=>q.includes('kind=output&state=conflict'))).toBe(true);await page.getByRole('button',{name:'查看步骤详情',exact:true}).click();await expect(detail.getByRole('button',{name:'重试恢复',exact:true})).toBeEnabled();
 expect(f.unexpected).toEqual([]);
});
test('I12 downloads open the existing import recovery dialog directly and retain preview/approval gates',async({page})=>{
 const f=await fixture(page);f.setReady(false);await page.goto('/downloads?tab=uploads');await page.getByRole('button',{name:'核对入库恢复',exact:true}).click();const dialog=page.getByRole('dialog',{name:'核对并恢复已有入库',exact:true});const submit=dialog.getByRole('button',{name:'明确恢复入库（不重新上传）',exact:true});await expect(submit).toBeDisabled();expect(f.writes).toHaveLength(0);expect(new URL(page.url()).pathname).toBe('/downloads');await dialog.getByRole('button',{name:'关闭',exact:true}).click();
 f.setReady(true);await page.getByRole('button',{name:'核对入库恢复',exact:true}).click();await expect(submit).toBeEnabled();await submit.click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes[0]).toEqual({path:`/api/v1/transfers/${transfer}/resume-import`,body:{controlRevision:1,planDigest:'a'.repeat(64)}});expect(f.unexpected).toEqual([]);
});
test.describe('I12 screenshots @local-screenshots',()=>{
 test.skip(!screenshots,'Explicit local-only I12_SCREENSHOTS=1');
 for(const kind of ['tabs','pending','conflict','download'])for(const width of [1440,390])for(const theme of ['light','dark'])test(`${kind}-${width}-${theme}`,async({page})=>{
  mkdirSync(out,{recursive:true});const f=await fixture(page),before=process.env.I12_BEFORE==='1';
  if(before){
   const root=resolve(out,'../before-assets');for(const name of ['organize','downloads'])await page.route(`**/${name}{,?*}`,r=>r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}));
   await page.route('**/assets/*',r=>{const file=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(file)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',file))})});
   await page.route('**/api/v1/library/artifact-repairs',r=>r.fulfill({json:{data:f.rows.map(row=>({...row,path:row.source,kind:'strm',operation:'write'}))}}));
  }
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  if(kind==='download'){
   await page.goto('/downloads?tab=uploads');if(before){await page.getByRole('link',{name:'核对入库恢复',exact:true}).click();await page.getByRole('button',{name:'核对入库恢复',exact:true}).click()}else await page.getByRole('button',{name:'核对入库恢复',exact:true}).click();
   await expect(page.getByRole('dialog',{name:'核对并恢复已有入库',exact:true}).getByRole('button',{name:'明确恢复入库（不重新上传）',exact:true})).toBeEnabled();
  }else if(before){await page.goto('/organize?tab=recovery');await expect(page.getByRole('heading',{name:'旧输出归档',exact:true})).toBeVisible()}
  else{
   await page.goto('/organize?tab=history');await expect(page.locator('.f5-history-row')).toHaveCount(2);
   if(kind!=='tabs'){await page.getByRole('button',{name:kind==='pending'?'待恢复输出':'冲突输出',exact:true}).click();await expect(page.locator('.f5-history-row')).toHaveCount(1);await page.screenshot({path:join(out,`after-${kind}-filter-${width}-${theme}.png`),fullPage:true,animations:'disabled'});await page.getByRole('button',{name:'查看步骤详情',exact:true}).click();await expect(page.getByRole('dialog',{name:'整理步骤详情',exact:true}).getByRole('button',{name:'重试恢复',exact:true})).toBeVisible()}
  }
  const dialog=page.getByRole('dialog').filter({visible:true});if(await dialog.count())await page.locator('.el-overlay:not([class*="-enter-"]):not([class*="-leave-"])').filter({has:dialog}).waitFor({state:'visible'});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);await page.screenshot({path:join(out,`${before?'before':'after'}-${kind}-${width}-${theme}.png`),fullPage:!(await dialog.count()),animations:'disabled'});
  if(!before)expect(f.unexpected).toEqual([]);expect(f.writes).toHaveLength(0);
 });
});
