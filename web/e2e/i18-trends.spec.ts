import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {u6OpsFixture} from './u6-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i18/screens');
const capture=process.env.I18_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS,before=process.env.I18_BEFORE==='1';
async function fixture(page:Page){
 await u6OpsFixture(page);const requests:number[]=[];
 await page.route('**/api/v1/ops/trends?*',r=>{
  const hours=Number(new URL(r.request().url()).searchParams.get('hours'));requests.push(hours);
  return r.fulfill({json:{data:Array.from({length:30},(_,i)=>({at:new Date(Date.UTC(2026,9,29,0,i*2)).toISOString(),memoryPercent:40+i%8,diskFreeGB:180-i*.2,load1:.3+(i%6)*.1,loadPerCPU:.1+(i%4)*.04,jobSeconds:2+i%7,pendingJobs:i%5,processBytes:(64+i%8)*1048576,failedJobs10m:i%2,succeededJobs10m:20+i%5}))}})
 });return requests;
}
async function geometry(page:Page,width:number){
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
 const chart=page.locator('.u6-trend').first(),workspace=page.locator('.runtime-trends');
 const a=await chart.boundingBox(),b=await workspace.boundingBox();expect(a!.width).toBeCloseTo(b!.width,0);
 const svg=chart.locator('svg'),line=svg.locator('polyline').first();expect((await line.boundingBox())!.width/(await svg.boundingBox())!.width).toBeGreaterThan(.9);
 const heading=await page.locator('.page-heading').boundingBox(),range=await page.getByRole('combobox',{name:'趋势时间范围'}).boundingBox();
 expect(range!.y).toBeGreaterThanOrEqual(heading!.y);expect(range!.y+range!.height).toBeLessThanOrEqual(heading!.y+heading!.height+1);
 if(width===1440){const title=await page.getByRole('heading',{name:'运行趋势',exact:true}).boundingBox();expect(Math.abs(title!.y-range!.y)).toBeLessThan(16)}
}
test('I18 navigation places trends after home in desktop and mobile; quick search owns trend keywords',async({page})=>{
 await fixture(page);await page.goto('/trends');
 const nav=page.getByRole('navigation',{name:'功能导航'});expect((await nav.getByRole('link').allTextContents()).slice(0,3)).toEqual(['首页','运行趋势','搜索']);
 await page.setViewportSize({width:390,height:1000});await page.getByRole('button',{name:'打开导航',exact:true}).click();
 const drawer=page.getByRole('dialog',{name:'主导航'});await expect(drawer.getByRole('link',{name:'运行趋势',exact:true})).toBeVisible();expect((await drawer.getByRole('link').allTextContents()).slice(0,3)).toEqual(['首页','运行趋势','搜索']);
 await drawer.getByRole('link',{name:'运行趋势',exact:true}).click();await expect(drawer).toBeHidden();
 await page.getByRole('button',{name:'快速前往',exact:true}).click();await page.getByRole('combobox',{name:'搜索功能入口'}).fill('CPU');await page.getByRole('option',{name:/运行趋势/}).click();await expect(page).toHaveURL(/\/trends/);
});
for(const old of ['/ops?tab=trends','/ops?tab=maintenance&section=trends','/maintenance?section=trends','/ops?tab=maintenance#trends'])test(`I18 old link ${old} redirects without losing bookmark filters`,async({page})=>{
 await fixture(page);const u=new URL(old,'http://fixture');u.searchParams.set('hours','168');u.searchParams.set('keep','yes');if(!u.hash)u.hash='#sample';await page.goto(u.pathname+u.search+u.hash);
 await expect(page).toHaveURL(/\/trends\?hours=168&keep=yes#/);await expect(page.getByRole('heading',{name:'运行趋势',exact:true})).toBeVisible();
});
test('I18 time range loads existing API, charts fill the page and original samples remain readable',async({page})=>{
 const requests=await fixture(page);await page.goto('/trends?hours=24');await expect(page.locator('.u6-trend')).toHaveCount(9);await expect.poll(()=>requests).toEqual([24]);
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});await geometry(page,width)}
 await page.locator('.trend-range .el-select__wrapper').click();await page.getByRole('option',{name:'最近 1 小时',exact:true}).click();await expect.poll(()=>requests).toEqual([24,1]);await expect(page).toHaveURL(/hours=1/);
 await expect(page.getByRole('button',{name:'查看原始采样记录',exact:true})).toHaveCount(0);await expect(page.locator('.technical-section')).toHaveCount(0);
 await page.goto('/ops?tab=maintenance');await expect(page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'运行趋势',exact:true})).toHaveCount(0);
});
test.describe('I18 local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit I18_SCREENSHOTS=1 only; never CI');
 for(const width of [1440,390])for(const theme of ['light','dark'])for(const view of ['ops','trends'])test(`${view}-${width}-${theme}`,async({page})=>{
  mkdirSync(dir,{recursive:true});await fixture(page);
  if(before){const root=resolve(dir,'../before-assets');await page.route('**/assets/*',r=>r.fulfill({contentType:extname(new URL(r.request().url()).pathname)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',basename(new URL(r.request().url()).pathname)))}));await page.route('**/ops{,?*}',r=>r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}))}
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  await page.goto(!before&&view==='trends'?'/trends':'/ops?tab=maintenance');
  if(before&&view==='trends')await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'运行趋势',exact:true}).click();
  if(view==='trends'){await expect(page.locator('.u6-trend')).toHaveCount(9);await expect(page.locator('.u6-trend svg')).toHaveCount(9);if(!before)await geometry(page,width)}
  else await expect(page.getByRole('heading',{name:'系统备份',exact:true})).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-theme',theme);expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  await page.screenshot({path:join(dir,`${before?'before':'after'}-${view}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
 });
});
