import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {u6OpsFixture,opsSnapshot} from './u6-fixture';
import {u3Task} from './u3-fixture';
import {fixtureFulfill} from './collection-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/t8');
const task={...u3Task,id:'task-u3',title:'自有测试影片.2026.1080p.WEB-DL.H264.mkv',state:'downloading',progress:.5,resourceSize:1073741824,siteName:'测试站点',downloadSpeed:1048576,media:{id:42,mediaType:'movie',title:'海边的故事',year:'2026'},trackerError:'测试Tracker错误只在详情展示',message:'测试下载说明只在详情展示'};
const jobRows=['native.tmdb.fetch','site.metrics.refresh','plugin.v2.workflow','future.custom.job'].map((kind,i)=>({id:'t8-job-'+i,kind,queue:'default',state:i===1?'failed':i===2?'running':'succeeded',attempts:1,maxAttempts:3,startedAt:'2026-10-26T00:00:00Z',updatedAt:'2026-10-26T00:00:03Z',lastError:i===1?'测试任务失败原因':''}));
async function fixture(page:Page){
 await u6OpsFixture(page);const writes:string[]=[];let delay:string|undefined,releases:(()=>void)[]=[];
 page.on('request',r=>{if(r.url().includes('/api/')&&r.method()!=='GET')writes.push(new URL(r.url()).pathname)});
 await page.route('**/api/v1/downloads{,?*}',r=>fixtureFulfill(r,{json:{data:[task,{...task,id:'task-two',title:'另一部已经完成的自有影片.mkv',state:'imported',progress:1,cleanupState:'disabled'},{...task,id:'task-three',title:'等待同步的资源.mkv',state:'added',progress:null,syncedAt:null,downloadSpeed:0,siteName:''}]}}));
 await page.route('**/api/v1/jobs/browse?*',async r=>{if(delay==='jobs')await new Promise<void>(resolve=>releases.push(resolve));return r.fulfill({json:{data:{items:jobRows,total:jobRows.length}}})});
 await page.route('**/api/v1/jobs/t8-job-*',r=>r.fulfill({json:{data:{...jobRows.find(j=>r.request().url().endsWith(j.id)),payload:{note:'原始任务证明保留'}}}}));
 await page.route('**/api/v1/tasks/jobs/*',r=>r.request().method()==='GET'?r.fulfill({json:{data:{state:'failed',revision:7,reason:'已核对任务状态'}}}):r.fulfill({json:{data:{state:'cancelled',revision:8,reason:'已受理取消'}}}));
 await page.route('**/api/v1/cache/tmdb?*',r=>r.fulfill({json:{data:{items:[{id:'cache-owned',query:'海边的故事',count:8,bytes:2048,createdAt:'2026-10-26T00:00:00Z',ttlSeconds:3600}],total:1}}}));
 await page.route('**/api/v1/cache/tmdb/cache-owned',r=>r.fulfill({json:{data:{id:'cache-owned',query:'海边的故事',data:{title:'海边的故事'}}}}));
 await page.route('**/api/v1/ops',r=>r.fulfill({json:{data:opsSnapshot}}));
 return {writes,delayJobs:()=>{delay='jobs'},releaseJobs:()=>{delay=undefined;releases.splice(0).forEach(done=>done())}};
}
async function open(page:Page,kind:string){
 if(kind.startsWith('download')){await page.goto('/downloads');await expect(page.locator('.acquire-task')).toHaveCount(3);if(kind==='download-detail'){if(await page.locator('.t8-download-title').count())await page.locator('.t8-download-title').first().click();else await page.getByRole('button',{name:'查看详情',exact:true}).first().click();await expect(page.getByRole('dialog',{name:'任务详情',exact:true})).toBeVisible()}}
 else{await page.goto('/ops?tab='+kind);await expect(page.locator('.u6-workspace h1')).toBeVisible();if(kind==='jobs')await expect(page.locator('.f3-job-row')).toHaveCount(4);if(kind==='maintenance')await expect(page.getByRole('heading',{name:'系统备份',exact:true})).toBeVisible();if(kind==='logs')await expect(page.locator('.f3-log-row.u6-log')).toHaveCount(1);if(kind==='cache')await expect(page.getByRole('heading',{name:'海边的故事',exact:true})).toBeVisible();}
}
test('T8 before/after 1440/390 light/dark',async({page})=>{
 mkdirSync(dir,{recursive:true});const pageErrors:string[]=[];page.on('pageerror',e=>pageErrors.push(e.message));await fixture(page);const before=process.env.T8_BEFORE==='1';
 if(before){const root=join(dir,'before-assets');for(const name of ['downloads','ops'])await page.route(`**/${name}{,?*}`,r=>new URL(r.request().url()).pathname==='/'+name?r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}):r.fallback());await page.route('**/assets/*',r=>{const n=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(n)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',n))})})}
 for(const kind of (process.env.T8_PAGE?[process.env.T8_PAGE]:['downloads','download-detail','jobs','attention','maintenance','logs','cache']))for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await open(page,kind);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);await page.evaluate(async()=>{await document.fonts.ready;await new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve())))});
  if(!before){expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);if(kind==='jobs'){const heights=await page.locator('.f3-job-row').evaluateAll(es=>es.map(el=>el.getBoundingClientRect().height));expect(Math.max(...heights)-Math.min(...heights)).toBeLessThanOrEqual(1);await expect(page.locator('.f4-job-queue')).toHaveCount(0);await expect(page.locator('.f3-job-row').first()).toContainText('TMDB详情');await expect(page.locator('.f3-job-row').last()).toContainText('future.custom.job')}}
  if(!before&&kind==='downloads'){for(const row of await page.locator('.acquire-task').all()){const centers=await row.locator('.download-progress > *').evaluateAll(es=>es.map(el=>{const b=el.getBoundingClientRect();return b.y+b.height/2}));expect(Math.abs(centers[0]-centers[1])).toBeLessThanOrEqual(1);if(width<700){await expect(row.locator('.download-metrics')).toBeHidden();await expect(row.locator('.download-row-detail')).toBeHidden()}else await expect(row.locator('.download-metrics')).toBeVisible()}}
  if(!before&&kind==='jobs'){const controls=await page.locator('.u5-filters>label>input,.u5-filters .el-select__wrapper,.u5-filters>.el-button').evaluateAll(es=>es.map(el=>el.getBoundingClientRect().height));expect(controls).toHaveLength(5);expect(Math.max(...controls)-Math.min(...controls)).toBeLessThanOrEqual(1);expect(Math.min(...controls)).toBeGreaterThanOrEqual(44);const actionWidths=await page.locator('.ops-job-actions').evaluateAll(es=>es.map(el=>el.getBoundingClientRect().width));expect(new Set(actionWidths).size).toBe(1)}
  if(!before&&kind==='logs'){const frames=await page.locator('.f3-log-filters>label>input,.f3-log-filters .el-select__wrapper,.f3-log-filters .el-date-editor').evaluateAll(es=>es.map(el=>{const r=el.getBoundingClientRect();return {height:r.height,bottom:r.bottom}}));expect(frames).toHaveLength(4);expect(frames.map(f=>f.height)).toEqual([44,44,44,44]);if(width>=700)expect(Math.max(...frames.map(f=>f.bottom))-Math.min(...frames.map(f=>f.bottom))).toBeLessThanOrEqual(1)}
  const dialogs=page.getByRole('dialog');if(await dialogs.count())await expect.poll(()=>dialogs.last().evaluate(el=>{const r=el.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth})).toBe(true);else await page.evaluate(()=>window.scrollTo(0,0));
  await (await dialogs.count()?dialogs.last():page).screenshot({path:join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
  if(!before)expect(pageErrors).toEqual([]);
  const after=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth}));writeFileSync(join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}.json`),JSON.stringify(after));if(!before)expect(after.scroll).toBeLessThanOrEqual(width);
 }
});
test('T8 download detail retains information and job menu retains confirmation',async({page})=>{
 const f=await fixture(page);await open(page,'downloads');const row=page.locator('.acquire-task').first();await expect(row).not.toContainText(task.trackerError);await expect(row).not.toContainText(task.message);await expect(row).toContainText('测试站点');await open(page,'download-detail');const drawer=page.getByRole('dialog',{name:'任务详情',exact:true});await expect(drawer).toContainText(task.trackerError);await expect(drawer).toContainText(task.message);await expect(drawer).toContainText('正在做种');
 await open(page,'jobs');const failed=page.locator('.f3-job-row').nth(1);await failed.getByRole('button',{name:'更多',exact:true}).click();await page.getByRole('menuitem',{name:'取消后台任务',exact:true}).click();const confirm=page.getByRole('dialog',{name:'取消后台任务',exact:true});await expect(confirm).toBeVisible();expect(f.writes).toHaveLength(0);await confirm.getByRole('button',{name:'确认处理当前任务',exact:true}).click();await expect.poll(()=>f.writes.filter(p=>p.endsWith('/cancel')).length).toBe(1);
});

test('T8 first-read placeholder and refresh retain rows without duplicate mutations',async({page})=>{
 const f=await fixture(page);f.delayJobs();try{await page.goto('/ops?tab=jobs');await expect(page.locator('.ops-content-stage .state-view.state-loading')).toBeVisible();await expect(page.locator('.ops-content-stage')).not.toContainText('暂无记录');}finally{f.releaseJobs()}
 await expect(page.locator('.f3-job-row')).toHaveCount(4);const first=(await page.locator('.f3-job-row').first().boundingBox())!;f.delayJobs();try{await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.locator('.f3-job-row')).toHaveCount(4);expect((await page.locator('.f3-job-row').first().boundingBox())!.y).toBe(first.y);await expect(page.locator('.ops-content-stage .state-view.state-loading')).toHaveCount(0)}finally{f.releaseJobs()}
 expect(f.writes).toEqual([]);
});

for(const kind of ['downloads','jobs','logs','cache'])test(`T8 ${kind} empty and failed reads use one explicit state`,async({page})=>{
 const f=await fixture(page);const path=kind==='downloads'?'/downloads':'/ops?tab='+kind;
 const pattern=kind==='downloads'?'**/api/v1/downloads{,?*}':kind==='jobs'?'**/api/v1/jobs/browse?*':kind==='logs'?'**/api/v1/logs/query?*':'**/api/v1/cache/tmdb?*';
 let fail=false;const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await page.route(pattern,r=>r.fulfill(fail?{status:503,json:{error:{code:'unavailable',message:'测试读取暂不可用'}}}:{json:{data:{items:[],total:0,modules:[]}}}));
 await page.goto(path);await expect(page.locator('.state-view.state-empty')).toHaveCount(1);fail=true;await page.reload();await expect(page.locator('.state-view.state-error')).toHaveCount(1);expect(errors).toEqual([]);expect(f.writes).toEqual([]);
});
