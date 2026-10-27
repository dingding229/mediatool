import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync,writeFileSync,existsSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {u6OpsFixture,opsSnapshot,plugin} from './u6-fixture';
import {fixtureFulfill} from './collection-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/t9');
const capture=process.env.T9_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const app={revision:3,value:{movieTemplate:'{title} ({year})/{title} - {videoFormat}',seriesTemplate:'{title}/Season {season}/{title} - {season_episode}',trustedProxyCidrs:[],searchCacheSeconds:600,tmdbCacheSeconds:86400,subscriptionCron:'*/30 * * * *',logging:{retentionDays:7},scraping:{}}};
const schedule={cron:'0 */2 * * *',enabled:true,revision:1,timezone:'Asia/Shanghai',nextRunAt:'2026-10-27T09:00:00+08:00'};
async function fixture(page:Page,ended=false){
 await u6OpsFixture(page);const writes:string[]=[];
 page.on('request',r=>{if(r.url().includes('/api/')&&r.method()!=='GET'&&!r.url().endsWith('/scheduling/cron/preview'))writes.push(new URL(r.url()).pathname)});
 await page.route('**/api/v1/settings/application',r=>fixtureFulfill(r,{json:{data:app}}));
 await page.route('**/api/v1/sites/metrics/schedule',r=>fixtureFulfill(r,{json:{data:{...schedule,nextRunAt:ended?null:schedule.nextRunAt}}}));
 await page.route('**/api/v1/library/scan-schedules',r=>fixtureFulfill(r,{json:{data:[{...schedule,id:'scan-owned',root:'/115/自有测试目录',nextRunAt:ended?null:schedule.nextRunAt}]}}));
 await page.route('**/api/v1/subscriptions?*',r=>fixtureFulfill(r,{json:{data:[{id:'t9-sub',title:'测试订阅',media:{id:42,mediaType:'movie',title:'测试订阅',year:'2026'},rules:{accountIds:[],include:[],exclude:[],minSize:0,maxSize:0,minSeeders:0,season:0,episodes:[]},source:'search',enabled:true,revision:1,lastState:'idle',nextRunAt:ended?null:schedule.nextRunAt}]}}));
 await page.route('**/api/v1/library/scans?*',r=>fixtureFulfill(r,{json:{data:{items:[],total:0}}}));
 await page.route('**/api/v1/ops',r=>fixtureFulfill(r,{json:{data:{...opsSnapshot,policy:{...opsSnapshot.policy,nextBackupAt:ended?null:schedule.nextRunAt}}}}));
 await page.route('**/api/v1/jobs/browse?*',r=>fixtureFulfill(r,{json:{data:{items:[{id:'j1',kind:'site.metrics.refresh',state:'succeeded'},{id:'j2',kind:'native.tmdb.fetch',state:'failed'}],total:2}}}));
 await page.route('**/api/v1/plugins/'+plugin.id,r=>fixtureFulfill(r,{json:{data:{instance:{...plugin,pluginId:'site-checkin',metadata:{...plugin.metadata,id:'site-checkin',name:'站点签到'}},config:{value:{active:false,cron:'0 8 * * *'},revision:2},entries:[],fingerprint:'owned'}}}));
 return writes;
}
async function open(page:Page,kind:string){
 if(kind==='settings'){await page.goto('/settings?tab=automation');await expect(page.getByLabel('订阅调度时间',{exact:true})).toBeVisible()}
 if(kind==='metrics'){await page.goto('/sites');await page.getByRole('button',{name:'编辑采集计划',exact:true}).click();await expect(page.getByRole('dialog',{name:'指标采集计划',exact:true})).toBeVisible()}
 if(kind==='scan'){await page.goto('/organize?tab=scans');await page.getByRole('button',{name:'新增定时扫描',exact:true}).click();await expect(page.getByRole('dialog',{name:'定时扫描设置',exact:true})).toBeVisible()}
 if(kind==='backup'){await page.goto('/ops?tab=maintenance');await page.getByRole('button',{name:'维护策略',exact:true}).click();await expect(page.getByLabel('备份Cron',{exact:true})).toBeVisible()}
 if(kind==='jobs'){await page.goto('/ops?tab=jobs');await expect(page.locator('.f3-job-row')).toHaveCount(2)}
 if(kind==='plugin'){await page.goto('/plugins/'+plugin.id);await expect(page.getByRole('heading',{name:'插件配置',exact:true})).toBeVisible()}
}
test('T9 consumer geometry 1440/390 light/dark (screenshots local opt-in)' ,async({page})=>{
 mkdirSync(dir,{recursive:true});const before=process.env.T9_BEFORE==='1',writes=await fixture(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 if(before){const root=join(dir,'before-assets');for(const name of ['settings','sites','organize','ops','plugins'])await page.route(`**/${name}{,/**,?*}`,r=>new URL(r.request().url()).pathname.startsWith('/'+name)?r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}):r.fallback());await page.route('**/assets/*',r=>{const n=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(n)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',n))})})}
 for(const kind of process.env.T9_PAGE?[process.env.T9_PAGE]:['settings','metrics','scan','backup','jobs','plugin'])for(const width of [1440,390])for(const theme of ['light','dark']){
  if(before&&existsSync(join(dir,`before-${kind}-${width}-${theme}.png`)))continue;
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await open(page,kind);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  if(!before&&kind!=='jobs'){await expect(page.locator('.cron-input').first()).toBeVisible();await expect(page.locator('.cron-preview')).toHaveCount(0);await expect(page.locator('.cron-input').first().getByRole('button',{name:/生成$/})).toBeVisible()}
  if(!before&&kind==='jobs'){
   const boxes=await page.locator('.ops-job-actions').evaluateAll(es=>es.map(e=>{const r=e.getBoundingClientRect();const buttons=e.querySelectorAll('button');return {width:r.width,right:buttons[buttons.length-1]!.getBoundingClientRect().right}}));
   expect(boxes[0]!.width).toBe(boxes[1]!.width);expect(Math.abs(boxes[0]!.right-boxes[1]!.right)).toBeLessThanOrEqual(1);
  }
  const dialog=page.getByRole('dialog').last();const hasDialog=await dialog.count();if(hasDialog)await expect.poll(()=>dialog.evaluate(e=>e.getBoundingClientRect().left>=0&&e.getBoundingClientRect().right<=innerWidth)).toBe(true);
  const geometry=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth}));if(!before)expect(geometry.scroll).toBeLessThanOrEqual(width);
  if(capture){await (hasDialog?dialog:page).screenshot({path:join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});writeFileSync(join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}.json`),JSON.stringify(geometry))}
 }
 if(!before){expect(errors).toEqual([]);expect(writes).toEqual([])}
});
test('T9 generation, manual validation and stale preview cannot enable save',async({page})=>{
 const writes=await fixture(page);await open(page,'metrics');const save=page.getByRole('button',{name:'保存采集计划',exact:true}),input=page.getByLabel('站点指标 Cron',{exact:true});await expect(save).toBeEnabled();
 await page.getByRole('button',{name:'站点指标 Cron生成',exact:true}).click();const generator=page.getByRole('dialog',{name:'站点指标 Cron生成器',exact:true});await generator.locator('.cron-generator-fields .el-select__wrapper').first().click();await page.getByRole('option',{name:'每天',exact:true}).click();await generator.getByLabel('站点指标 Cron执行时刻',{exact:true}).fill('10:35');await expect(input).toHaveValue('0 */2 * * *');await expect(generator.getByRole('button',{name:'确定',exact:true})).toBeEnabled();await generator.getByRole('button',{name:'确定',exact:true}).click();await expect(input).toHaveValue('0 35 10 * * * *');await expect(save).toBeEnabled();
 await page.getByRole('dialog',{name:'指标采集计划',exact:true}).getByRole('button',{name:'取消',exact:true}).click();await page.getByRole('button',{name:'编辑采集计划',exact:true}).click();await expect(input).toHaveValue('0 */2 * * *');await page.getByRole('button',{name:'站点指标 Cron生成',exact:true}).click();await expect(generator.locator('.cron-generator-fields .el-select__wrapper').first()).toContainText('每 N 小时');await generator.getByRole('button',{name:'取消',exact:true}).click();await expect(input).toHaveValue('0 */2 * * *');
 await input.fill('61 * * * *');await expect(save).toBeDisabled();await expect(page.getByRole('alert').filter({hasText:'范围'})).toBeVisible();
 await input.fill('0 0 0 1 1 * 2199');await expect(save).toBeEnabled();
 let release!:()=>void,seen=false,settled=false;const gate=new Promise<void>(resolve=>release=resolve);
 await page.route('**/api/v1/scheduling/cron/preview',async r=>{seen=true;await gate;await fixtureFulfill(r,{json:{data:{}}});settled=true});
 await input.fill('0 0 0 1 1 * 2035');await expect.poll(()=>seen).toBe(true);
 await input.fill('* *');await expect(save).toBeDisabled();release();await expect.poll(()=>settled).toBe(true);await expect(save).toBeDisabled();await expect(input).toHaveValue('* *');expect(writes).toEqual([]);
});
test('T9 exhausted summaries remain explicit and NULL is not a date error',async({page})=>{
 await fixture(page,true);await page.goto('/subscriptions?tab=list');await expect(page.getByText('计划已结束',{exact:true})).toBeVisible();await page.goto('/sites');await expect(page.getByText(/下次 计划已结束/)).toBeVisible();await page.goto('/organize?tab=scans');await expect(page.getByText(/下次 计划已结束/)).toBeVisible();await page.goto('/ops?tab=maintenance');await expect(page.getByText(/计划已结束/).first()).toBeVisible();
});

test('T9 validation feedback is compact and remains authoritative without a future-times list',async({page})=>{
 await fixture(page);
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});let release!:()=>void,seen=false;const gate=new Promise<void>(r=>release=r);
  await page.route('**/api/v1/scheduling/cron/preview',async r=>{seen=true;await gate;await fixtureFulfill(r,{json:{data:{}}})});
  await open(page,'settings');await expect.poll(()=>seen).toBe(true);const feedback=page.locator('.cron-input > .cron-feedback');await expect(feedback).toContainText('正在校验');
  release();await expect(feedback).toContainText('校验通过');await expect(page.locator('.cron-preview')).toHaveCount(0);expect((await feedback.boundingBox())!.height).toBeLessThan(190);
  await page.getByLabel('订阅调度时间',{exact:true}).fill('* *');await expect(feedback.getByRole('alert')).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await page.unroute('**/api/v1/scheduling/cron/preview');
 }
});
