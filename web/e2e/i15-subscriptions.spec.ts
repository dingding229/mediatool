import {test,expect,type Page} from '@playwright/test';
import {fixtureFulfill} from './collection-fixture';
import {u3Fixture} from './u3-fixture';
import {capture,screens,prepareScreen,shot} from './i15-fixture';

async function fixture(page:Page,empty=false){
 await u3Fixture(page);const reads:URL[]=[];
 const rows=Array.from({length:30},(_,i)=>({id:'i15-sub-'+i,title:'海边的故事 '+(i+1),media:{id:42,mediaType:'movie',year:'2026'},enabled:false,source:'search',revision:1,rules:{accountIds:[],include:[],exclude:[],schedule:'0 */6 * * *'},createdAt:'2026-10-29T00:00:00Z'}));
 await page.route('**/api/v1/subscriptions?*',r=>{reads.push(new URL(r.request().url()));return fixtureFulfill(r,{json:{data:empty?[]:rows}})});
 return reads;
}
async function geometry(page:Page,width:number){
 const card=page.locator('.subscription-workbench');
 const bounds=await card.boundingBox(),pager=await page.getByRole('navigation',{name:'订阅列表分页',exact:true}).boundingBox();
 expect(pager!.y+pager!.height).toBeLessThanOrEqual(bounds!.y+bounds!.height);
 const summary=await card.locator('.mt-page-summary').boundingBox();
 expect(Math.round(summary!.x-bounds!.x)).toBe(width===1440?25:17);
 expect(await card.evaluate(el=>el.scrollWidth<=el.clientWidth&&el.scrollHeight<=el.clientHeight)).toBe(true);
}
test('I15 follow-list pager remains inside a padded card for populated and empty results',async({page})=>{
 const reads=await fixture(page);await page.goto('/subscriptions');await expect(page.locator('.subscription-row')).toHaveCount(25);
 const pager=page.getByRole('navigation',{name:'订阅列表分页',exact:true});
 await pager.getByRole('button',{name:'下一页',exact:true}).click();await expect(page.locator('.subscription-row')).toHaveCount(5);
 expect(reads.at(-1)!.searchParams.get('page')).toBe('2');
 await page.locator('.subscription-title').first().click();await expect(page.getByRole('dialog',{name:'订阅详情',exact:true})).toBeVisible();
 await page.getByRole('dialog',{name:'订阅详情',exact:true}).getByRole('button',{name:'关闭此对话框',exact:true}).click();
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});await geometry(page,width)}
 await page.route('**/api/v1/subscriptions?*',r=>fixtureFulfill(r,{json:{data:[]}}));await page.reload();
 await expect(page.getByText('还没有订阅，从一部想看的影视开始',{exact:true})).toBeVisible();
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});await geometry(page,width);
  const empty=await page.locator('.state-view.state-empty').boundingBox(),footer=await page.locator('.subscription-list-footer').boundingBox();
  expect(empty!.height).toBeLessThanOrEqual(300);expect(footer!.y-(empty!.y+empty!.height)).toBeLessThanOrEqual(1);
 }
});
test('I15 follow-list initial empty read keeps pager stable',async({page})=>{
 await fixture(page,true);
 await page.route('**/api/v1/subscriptions?*',async r=>{await new Promise(resolve=>setTimeout(resolve,180));return fixtureFulfill(r,{json:{data:[]}})});
 await page.addInitScript(()=>{(window as any).__i15cls=0;new PerformanceObserver(list=>{for(const row of list.getEntries() as any[])if(!row.hadRecentInput)(window as any).__i15cls+=row.value}).observe({type:'layout-shift',buffered:true})});
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});await page.goto('/subscriptions');
  await expect(page.getByText('还没有订阅，从一部想看的影视开始',{exact:true})).toBeVisible();
  await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))));
  expect(await page.evaluate(()=>(window as any).__i15cls)).toBeLessThan(.01);
 }
});
test.describe('I15 subscriptions local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit local I15_SCREENSHOTS=1 only, never CI');
 for(const {width,theme} of screens)test(`subscriptions-${width}-${theme}`,async({page})=>{
  await fixture(page,true);await prepareScreen(page,width,theme,'/subscriptions');
  await expect(page.getByText('还没有订阅，从一部想看的影视开始',{exact:true})).toBeVisible();
  await shot(page,'subscriptions',width,theme);
 });
});
