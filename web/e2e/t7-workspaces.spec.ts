import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {organizeFixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/t7');
const longPath='/115/媒体资料/待整理目录/系列作品/'.repeat(4)+'海边的故事.2026.1080p.WEB-DL.H264.mkv';
const destination='/115/电影/华语电影/海边的故事 (2026)/海边的故事.2026.1080p.WEB-DL.H264.mkv';
const scan={id:'scan-owned',root:longPath,state:'running',files:24,directories:8,completedDirectories:3,currentDirectory:destination,updatedAt:'2026-10-25T00:00:00Z',message:'正在完整覆盖，未读目录不作为缺失证据'};
const entry={id:'00000000-0000-4000-8000-000000000098',originalPath:longPath+'.strm',archivePath:'.media-tool-retired/'+longPath+'.strm.retired',state:'retained',eligibleAfter:'2020-02-01T00:00:00Z',message:'过期托管归档'};
async function fixture(page:Page){
 await organizeFixture(page);let confirmations=0;const writes:string[]=[];
 page.on('request',r=>{if(r.url().includes('/api/v1/')&&r.method()!=='GET')writes.push(new URL(r.url()).pathname)});
 await page.route('**/api/v1/subscriptions?*',r=>fixtureFulfill(r,{json:{data:[{id:'t7-sub',title:'海边的故事',media:{id:42,mediaType:'movie',title:'海边的故事',year:'2026'},rules:{accountIds:[]},enabled:false,source:'search',revision:1}]}}));
 await page.route('**/api/v1/subscriptions/t7-sub/history?*',r=>fixtureFulfill(r,{json:{data:[{id:'t7-run',state:'succeeded',preview:true,createdAt:'2026-10-25T00:00:00Z'}]}}));
 await page.route('**/api/v1/subscription-runs/t7-run',r=>r.fulfill({json:{data:{id:'t7-run',subscriptionId:'t7-sub',state:'succeeded',preview:true,decisions:[{id:'d1',title:'自有测试资源',action:'would_download',reasons:['符合规则'],checks:[]}]}}}));
 await page.route('**/api/v1/library/scans?*',r=>r.fulfill({json:{data:{items:[scan],total:1}}}));
 await page.route('**/api/v1/library/scans/scan-owned?*',r=>r.fulfill({json:{data:{scan,directories:[{path:destination,state:'completed',updatedAt:scan.updatedAt,message:'目录已完整覆盖'},{path:longPath,state:'failed',updatedAt:scan.updatedAt,lastError:'读取暂时不可用，未确认覆盖'}]}}}));
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[{id:'h1',kind:'file',state:'failed',source:longPath,destination,media:{title:'海边的故事',year:'2026'},category:'华语电影',updatedAt:scan.updatedAt,message:'测试失败原因仅在详情展示',details:{completedSteps:1}},{id:'h2',kind:'batch',state:'succeeded',source:'/115/待整理/第二部电影.mkv',destination:'/115/电影/第二部电影.mkv',media:{title:'第二部电影'},updatedAt:scan.updatedAt,message:'已完成'}],total:2}}}));
 await page.route('**/api/v1/library/artifact-repairs',r=>r.fulfill({json:{data:[{id:'repair-owned',path:longPath,kind:'strm',operation:'write',state:'failed'}]}}));
 await page.route('**/api/v1/library/archives?*',r=>r.fulfill({json:{data:{items:[entry],total:1}}}));
 await page.route('**/api/v1/library/archives/cleanup-preview',r=>r.fulfill({json:{data:{planDigest:'b'.repeat(64),policy:{retentionDays:30,revision:1,automaticCleanup:false},items:[entry],sourceFilesUntouched:true,physicalDeletion:true}}}));
 await page.route('**/api/v1/library/archives/cleanup-confirm',r=>{confirmations++;return r.fulfill({json:{data:[{id:'cleaned',archiveId:entry.id,state:'removed',message:'已清理'}]}})});
 await page.route('**/api/v1/transfers/transfer-owned/output-preview',r=>r.fulfill({json:{data:{ready:true,state:'needs_output_reconciliation',controlRevision:1,planDigest:'a'.repeat(64),message:'已有文件已校验；请核对输出再恢复。',existingPaths:[longPath],expectedPaths:[destination]}}}));
 return {writes,confirmations:()=>confirmations};
}
async function open(page:Page,kind:string){
 if(kind==='subscriptions'){await page.goto('/subscriptions?tab=history&subscription=t7-sub&run=t7-run');await expect(page.getByRole('region',{name:'运行结果',exact:true})).toContainText('自有测试资源')}
 else{await page.goto('/organize?tab='+({scans:'scans',directories:'scans',history:'history',recovery:'recovery',output:'recovery'}[kind]));await expect(page.locator('.media-workspace h1')).toBeVisible();
  if(kind==='directories'){await page.getByRole('button',{name:'查看目录记录',exact:true}).click();await expect(page.getByRole('dialog',{name:'扫描目录覆盖'})).toContainText('目录')}
  if(kind==='history')await expect(page.locator('.f5-history-row')).toHaveCount(2);
  if(kind==='recovery'){await page.locator('.organize-record').filter({hasText:'过期托管归档'}).getByRole('checkbox').check();await page.getByRole('button',{name:'预览清理所选 1',exact:true}).click();await expect(page.getByRole('region',{name:'归档清理计划'})).toBeVisible()}
  if(kind==='output'){await page.getByLabel('待恢复上传任务ID',{exact:true}).fill('transfer-owned');await page.getByRole('button',{name:'核对入库恢复',exact:true}).click();await expect(page.getByRole('dialog',{name:'核对并恢复已有入库'}).getByRole('heading',{name:'已有输出',exact:true})).toBeVisible()}
 }
}
test('T7 before/after 1440/390 light/dark workspace screenshots',async({page})=>{
 mkdirSync(dir,{recursive:true});await fixture(page);const before=process.env.T7_BEFORE==='1';
 if(before){const root=join(dir,'before-assets');for(const name of ['organize','subscriptions'])await page.route(`**/${name}{,?*}`,r=>new URL(r.request().url()).pathname==='/'+name?r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}):r.fallback());await page.route('**/assets/*',r=>{const n=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(n)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',n))})})}
 for(const kind of (process.env.T7_PAGE?[process.env.T7_PAGE]:['subscriptions','scans','directories','history','recovery','output']))for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await open(page,kind);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  await page.evaluate(async()=>{await document.fonts.ready;await new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve())))});
  const metrics=await page.evaluate(()=>({viewport:innerWidth,scroll:document.documentElement.scrollWidth,pagers:Array.from(document.querySelectorAll('.mt-pagination')).filter(el=>el.getBoundingClientRect().width).map(el=>el.getAttribute('aria-label'))}));
  writeFileSync(join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}.json`),JSON.stringify(metrics,null,2));if(!before)expect(metrics.scroll).toBeLessThanOrEqual(width);
  if(!before&&kind==='subscriptions'){expect(metrics.pagers).toEqual(['订阅运行历史分页']);const section=(await page.locator('.subscription-workbench').boundingBox())!,pager=(await page.getByRole('navigation',{name:'订阅运行历史分页',exact:true}).boundingBox())!;expect(pager.y+pager.height).toBeLessThanOrEqual(section.y+section.height)}
  if(!before&&kind==='recovery'){const control=(await page.locator('.archive-policy .el-input__wrapper').boundingBox())!,save=(await page.locator('.archive-policy .mt-save-button').boundingBox())!;expect(Math.abs(control.y+control.height-save.y-save.height)).toBeLessThanOrEqual(1)}
  const dialogs=page.getByRole('dialog');if(await dialogs.count()){await expect.poll(()=>dialogs.last().evaluate(el=>{const r=el.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&r.width>200})).toBe(true)}else await page.evaluate(()=>window.scrollTo(0,0));
  const shot=await (await dialogs.count()?dialogs.last():page).screenshot({path:join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
  const afterShot=await page.evaluate(()=>({viewport:innerWidth,scroll:document.documentElement.scrollWidth,overflow:Array.from(document.querySelectorAll('.f5-history-row,.history-paths,.mt-path,.organize-history-host,.organize-history-panel,.organize-history-filters')).filter(el=>el.getBoundingClientRect().right>innerWidth+1).map(el=>({class:el.className,width:el.getBoundingClientRect().width,right:el.getBoundingClientRect().right}))}));writeFileSync(join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}-post.json`),JSON.stringify(afterShot,null,2));if(!before){expect(afterShot.scroll).toBeLessThanOrEqual(width);if(!await dialogs.count())expect(shot.readUInt32BE(16)).toBe(width)}
 }
});
test('T7 history hides failure reason in details and archive cleanup still requires typed confirmation',async({page})=>{
 const f=await fixture(page);await open(page,'history');await expect(page.locator('.f5-history-row').first()).not.toContainText('测试失败原因');await expect(page.locator('.f5-history-row').first()).toContainText('源文件');await page.getByRole('button',{name:'查看步骤详情',exact:true}).first().click();await expect(page.getByRole('dialog',{name:'整理步骤详情'})).toContainText('测试失败原因仅在详情展示');
 await open(page,'recovery');expect(f.confirmations()).toBe(0);const plan=page.getByRole('region',{name:'归档清理计划'});await plan.getByRole('button',{name:'确认物理清理',exact:true}).click();const dialog=page.getByRole('dialog',{name:'确认物理清理归档',exact:true});await expect(dialog.getByRole('button',{name:'确认物理清理',exact:true})).toBeDisabled();await dialog.getByRole('button',{name:'返回检查',exact:true}).click();expect(f.confirmations()).toBe(0);await plan.getByRole('button',{name:'确认物理清理',exact:true}).click();await dialog.getByPlaceholder('请输入上方确认短语',{exact:true}).fill('清理归档');await dialog.getByRole('button',{name:'确认物理清理',exact:true}).click();await expect.poll(()=>f.confirmations()).toBe(1);expect(f.writes.filter(p=>p.endsWith('cleanup-confirm'))).toHaveLength(1);
});

test('T7 pagination owner, shared progress card and safe recovery guidance',async({page})=>{
 await fixture(page);await open(page,'subscriptions');await expect(page.getByRole('navigation',{name:'订阅列表分页',exact:true})).toHaveCount(0);await page.locator('.mt-subscriptions .page-tabs').getByRole('button',{name:'订阅列表',exact:true}).click();await expect(page.getByRole('navigation',{name:'订阅列表分页',exact:true})).toHaveCount(1);await expect(page.getByRole('navigation',{name:'订阅运行历史分页',exact:true})).toHaveCount(0);
 await open(page,'scans');const progress=page.locator('.f5-scan-row .scan-record-progress');await expect(progress).toContainText('%');const parts=await progress.locator(':scope > *').evaluateAll(es=>es.map(el=>{const r=el.getBoundingClientRect();return r.y+r.height/2}));expect(Math.abs(parts[0]-parts[1])).toBeLessThan(1);await expect(page.locator('.f5-scan-row .mt-path')).toHaveAttribute('title',longPath);for(const path of await page.locator('.f5-scan-row .mt-path').all())expect((await path.boundingBox())!.height).toBeLessThan(30);
 await page.getByRole('button',{name:'查看目录记录',exact:true}).click();const drawer=page.getByRole('dialog',{name:'扫描目录覆盖'});await expect(drawer.locator('.scan-record')).toHaveCount(3);await drawer.locator('.f5-directory-row summary').first().click();await expect(drawer).toContainText('目录已完整覆盖');await expect(drawer.locator('pre')).toHaveCount(0);await drawer.getByRole('button',{name:/技术细节与原始记录/}).click();await expect(drawer.locator('.technical-json')).toContainText('读取暂时不可用');
 await page.setViewportSize({width:390,height:1000});await open(page,'recovery');expect(await page.getByRole('region',{name:'归档清理计划'}).getByRole('button').allTextContents()).toEqual(['技术详情','确认物理清理']);
 const guide='只恢复已校验文件的入库；不重新上传、不改已确认媒体身份，也不重写 STRM 固定链接。';await page.route('**/api/v1/transfers/transfer-owned/output-preview',r=>r.fulfill({json:{data:{ready:false,state:'needs_output_reconciliation',message:guide,existingPaths:[],expectedPaths:[]}}}));await open(page,'output');const output=page.getByRole('dialog',{name:'核对并恢复已有入库'});await expect(output.getByText(guide,{exact:true})).toHaveCount(1);await expect(output.getByRole('button',{name:'明确恢复入库（不重新上传）',exact:true})).toBeDisabled();
});
