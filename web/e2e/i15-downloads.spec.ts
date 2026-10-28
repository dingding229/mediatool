import {test,expect,type Page} from '@playwright/test';
import {u3DownloadsFixture,u3Task} from './u3-fixture';
import {fixtureFulfill} from './collection-fixture';
import {before,capture,screens,prepareScreen,shot} from './i15-fixture';

async function rowGeometry(page:Page){
 const stage=page.locator('.download-content-stage'),rows=stage.locator('.t8-download-row');
 await expect(stage).toHaveCSS('min-height','0px');
 const bounds=await stage.boundingBox(),first=await rows.first().boundingBox(),last=await rows.last().boundingBox();
 expect(Math.abs(bounds!.height-(last!.y+last!.height-first!.y))).toBeLessThan(1);
 const footer=await page.locator('.download-list-footer p').boundingBox();
 expect(footer!.y-(last!.y+last!.height)).toBeLessThanOrEqual(12);
 const info=rows.first().locator('.download-main'),track=info.locator('.el-progress'),label=info.locator('.download-progress>span:last-child');
 const [i,b,p]=await Promise.all([info.boundingBox(),track.boundingBox(),label.boundingBox()]);
 expect(Math.abs(b!.x-i!.x)).toBeLessThan(1);expect(Math.abs(p!.x+p!.width-i!.x-i!.width)).toBeLessThan(1);
 expect(Math.abs(b!.width-(i!.width-p!.width-12))).toBeLessThan(1);
 await expect(track).toHaveCSS('max-width','none');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width);
}

test.beforeEach(async({page})=>u3DownloadsFixture(page));
test('I15 downloader management has a single retained entry in connections',async({page})=>{
 await page.goto('/downloads');await expect(page.locator('.t8-download-row')).toHaveCount(1);
 await expect(page.getByRole('link',{name:'管理下载器',exact:true})).toHaveCount(0);
 await page.goto('/connections?tab=downloader');
 await expect(page.getByRole('heading',{name:'下载器配置',exact:true})).toBeVisible();
 await expect(page.getByRole('button',{name:'添加连接',exact:true})).toBeVisible();
});
test('I15 download rows size to content and progress fills the available information width',async({page})=>{
 let count=1;await page.route('**/api/v1/downloads{,?*}',r=>fixtureFulfill(r,{json:{data:Array.from({length:count},(_,i)=>({...u3Task,id:'i15-download-'+i}))}}));
 for(const width of [1440,390])for(count of [1,3]){
  await page.setViewportSize({width,height:1000});await page.goto('/downloads');
  await expect(page.locator('.t8-download-row')).toHaveCount(count);await rowGeometry(page);
 }
});
test('I15 first-read skeleton resolves without CLS or retained space; refresh preserves settled geometry',async({page})=>{
 await page.addInitScript(()=>{(window as any).__downloadCLS=0;new PerformanceObserver(list=>{for(const row of list.getEntries() as any[])if(!row.hadRecentInput)(window as any).__downloadCLS+=row.value}).observe({type:'layout-shift',buffered:true})});
 for(const width of [1440,390])for(const count of [0,1,3]){
  let release!:()=>void,releaseRefresh!:()=>void;
  const gate=new Promise<void>(resolve=>{release=resolve}),refreshGate=new Promise<void>(resolve=>{releaseRefresh=resolve});let reads=0;
  await page.route('**/api/v1/downloads{,?*}',async r=>{reads++;await(reads===1?gate:refreshGate);return fixtureFulfill(r,{json:{data:Array.from({length:count},(_,i)=>({...u3Task,id:'i15-delayed-'+i}))}})});
  await page.setViewportSize({width,height:1000});await page.goto('/downloads');
  await expect(page.locator('.download-content-stage .state-view.state-loading')).toBeVisible();
  await expect(page.locator('.download-list-footer')).toBeHidden();release();
  await expect(page.locator('.t8-download-row')).toHaveCount(count);await expect(page.locator('.download-list-footer')).toBeVisible();
  if(count)await rowGeometry(page);else await expect(page.getByText('尚无下载任务',{exact:true})).toBeVisible();
  await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))));
  const cls=await page.evaluate(()=>(window as any).__downloadCLS);console.log(`I15 download CLS ${width}/${count}: ${cls}`);expect(cls).toBeLessThan(.01);
  const settled=await page.locator('.download-list-footer').boundingBox(),response=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/downloads');
  await page.locator('.mt-refresh').click();
  await expect.poll(()=>reads).toBeGreaterThan(1);await expect(page.locator('.download-list-footer')).toBeVisible();
  expect(await page.locator('.download-list-footer').boundingBox()).toEqual(settled);releaseRefresh();await response;
  await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))));
  expect(await page.locator('.download-list-footer').boundingBox()).toEqual(settled);
 }
});
test.describe('I15 downloads local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit local I15_SCREENSHOTS=1 only, never CI');
 for(const {width,theme} of screens)test(`downloads-${width}-${theme}`,async({page})=>{
  await prepareScreen(page,width,theme,'/downloads');await expect(page.locator('.t8-download-row')).toHaveCount(1);
  if(!before)await rowGeometry(page);
  await shot(page,'downloads',width,theme);
 });
});
