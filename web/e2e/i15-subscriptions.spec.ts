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
 const actions=await card.locator('.mt-page-actions').boundingBox(),gutter=width===1440?24:16;
 expect(summary!.x-bounds!.x).toBeGreaterThanOrEqual(gutter);expect(bounds!.x+bounds!.width-actions!.x-actions!.width).toBeGreaterThanOrEqual(gutter);
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

// I30: the history pager belongs to the timeline, not the two-column review.
async function historyFixture(page:Page,count:number){
 await fixture(page);const reads:URL[]=[];
 const rows=Array.from({length:count},(_,i)=>({id:'i30-run-'+i,state:'succeeded',preview:true,createdAt:`2026-10-29T${String(i%24).padStart(2,'0')}:00:00Z`}));
 await page.route('**/api/v1/subscriptions/*/history?*',r=>{const u=new URL(r.request().url());reads.push(u);const n=Number(u.searchParams.get('page')||1),size=Number(u.searchParams.get('pageSize')||25);return r.fulfill({json:{data:{items:rows.slice((n-1)*size,n*size),total:rows.length,page:n,pageSize:size}}})});
 await page.route('**/api/v1/subscription-runs/*',r=>r.fulfill({json:{data:{id:new URL(r.request().url()).pathname.split('/').at(-1),state:'succeeded',preview:true,decisions:[{id:'i30-decision',title:'本地规则预演样例',action:'would_download',reasons:['符合规则；仅用于本地布局测试'],checks:[]}]}}}));
 return reads;
}
const i30Capture=process.env.I30B_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
test.describe('I30b history local screenshots @screenshots',()=>{
 test.skip(!i30Capture,'Explicit local I30B_SCREENSHOTS=1 only, never CI');
 for(const width of [1440,375])for(const theme of ['light','dark'])test(`I30b screenshot ${width} ${theme}`,async({page})=>{
  await historyFixture(page,0);await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  await page.goto('/subscriptions?tab=history&subscription=i15-sub-0');await expect(page.getByText('暂无运行记录；预演或运行后会显示在这里。',{exact:true})).toBeVisible();
  await page.evaluate(async()=>{await document.fonts.ready;await new Promise<void>(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>r()))) });
  const {mkdirSync,writeFileSync}=await import('node:fs'),{resolve}=await import('node:path'),dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i30b/screens');mkdirSync(dir,{recursive:true});
  const name=`after-history-${width}-${theme}`;
  await page.screenshot({path:resolve(dir,name+'.png'),fullPage:true,animations:'disabled'});
  const facts=await page.evaluate(()=>{const pager=document.querySelector('[aria-label="订阅运行历史分页"]')!;const rect=(e:Element)=>{const {x,y,width,height}=e.getBoundingClientRect();return{x,y,width,height}};return {parent:pager.parentElement?.className,pager:rect(pager),summary:rect(pager.querySelector('.mt-page-summary')!),actions:rect(pager.querySelector('.mt-page-actions')!),card:rect(document.querySelector('.subscription-workbench')!),timeline:rect(document.querySelector('.run-timeline')!),review:rect(document.querySelector('.run-review')!),owner:rect(document.querySelector('.subscription-workbench')!),gutter:parseFloat(getComputedStyle(document.querySelector('.subscription-commandbar')!).paddingLeft),justifyContent:getComputedStyle(pager).justifyContent,overflow:document.documentElement.scrollWidth>innerWidth}});
  writeFileSync(resolve(dir,name+'.json'),JSON.stringify(facts,null,2));
 });
});

type Rect={x:number;y:number;width:number;height:number};
async function paginationInsets(page:Page,label:string){
 // One frame: tab changes can resize the card and scroll anchor together.
 return page.locator('.subscription-workbench').evaluate((card,label)=>{
  const pager=card.querySelector(`[aria-label="${label}"]`)!,bounds=card.getBoundingClientRect(),p=pager.getBoundingClientRect(),summary=pager.querySelector('.mt-page-summary')!.getBoundingClientRect(),actions=pager.querySelector('.mt-page-actions')!.getBoundingClientRect();
  return {left:summary.x-bounds.x,right:bounds.right-actions.right,bottom:bounds.bottom-actions.bottom,height:p.height,pagerLeft:p.x-bounds.x,pagerRight:bounds.right-p.right};
 },label);
}
async function historyGeometry(page:Page){
 await page.evaluate(async()=>{await document.fonts.ready;await new Promise<void>(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>r()))) });
 const pager=page.getByRole('navigation',{name:'订阅运行历史分页',exact:true}),card=page.locator('.subscription-workbench');
 const p=(await pager.boundingBox())!,bounds=(await card.boundingBox())!,workspace=(await page.locator('.subscription-history-workspace').boundingBox())!,review=(await page.locator('.subscription-history-workspace>.run-review').boundingBox())!;
 expect(Math.abs(p.width-await card.evaluate(e=>e.clientWidth))).toBeLessThanOrEqual(1);
 expect(p.x).toBeGreaterThanOrEqual(bounds.x);expect(p.x+p.width).toBeLessThanOrEqual(bounds.x+bounds.width);expect(p.y+p.height).toBeLessThanOrEqual(bounds.y+bounds.height);
 expect(p.y).toBeGreaterThanOrEqual(workspace.y+workspace.height);
 const separate=(a:Rect,b:Rect)=>expect(a.x+a.width<=b.x||b.x+b.width<=a.x||a.y+a.height<=b.y||b.y+b.height<=a.y).toBe(true);
 separate(p,workspace);separate(p,review);for(const record of await page.locator('.run-timeline>button').all())separate(p,(await record.boundingBox())!);
 const header=await page.locator('.subscription-commandbar').evaluate(e=>{const box=e.getBoundingClientRect(),style=getComputedStyle(e);return {left:box.x+parseFloat(style.paddingLeft),right:box.right-parseFloat(style.paddingRight)}}),summary=(await pager.locator('.mt-page-summary').boundingBox())!,actions=(await pager.locator('.mt-page-actions').boundingBox())!,subject=(await page.locator('.history-subject-picker>label').boundingBox())!;
 if((page.viewportSize()?.width||1440)>700)expect(Math.abs(summary.y-actions.y)).toBeLessThanOrEqual(1);
 expect(Math.abs(summary.x-header.left)).toBeLessThanOrEqual(1);expect(Math.abs(summary.x-subject.x)).toBeLessThanOrEqual(1);expect(Math.abs(actions.x+actions.width-header.right)).toBeLessThanOrEqual(1);
 expect(await pager.evaluate(e=>getComputedStyle(e).borderTopWidth)).toBe('1px');
 for(const name of ['首页','上一页','下一页','末页']){const button=pager.getByRole('button',{name,exact:true});await expect(button).toBeVisible();expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44)}
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(await page.evaluate(()=>innerWidth));
 const history=await paginationInsets(page,'订阅运行历史分页');
 await page.getByRole('button',{name:'订阅列表',exact:true}).click();await expect(page.getByRole('navigation',{name:'订阅列表分页',exact:true})).toBeVisible();const list=await paginationInsets(page,'订阅列表分页');
 for(const key of ['left','right','bottom','height','pagerLeft','pagerRight']as const)expect(Math.abs(history[key]-list[key]),key).toBeLessThanOrEqual(1);
 await page.getByRole('button',{name:'运行记录',exact:true}).click();await expect(page.getByRole('navigation',{name:'订阅运行历史分页',exact:true})).toBeVisible();const restored=await paginationInsets(page,'订阅运行历史分页');
 for(const key of ['left','right','bottom']as const)expect(Math.abs(restored[key]-history[key]),key).toBeLessThanOrEqual(1);
 if(i30Capture){const {mkdirSync,writeFileSync}=await import('node:fs'),{resolve}=await import('node:path'),dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i30b');mkdirSync(dir,{recursive:true});writeFileSync(resolve(dir,'geometry-'+test.info().title.replace(/[^a-zA-Z0-9-]/g,'-')+'.json'),JSON.stringify({history,list,restored},null,2))}
}
for(const width of [1440,375])for(const count of [0,3])test(`I30b full-width history pager sits below both columns ${width}px ${count} runs`,async({page})=>{
 await historyFixture(page,count);await page.setViewportSize({width,height:1000});await page.goto('/subscriptions?tab=history&subscription=i15-sub-0');
 await expect(page.locator('.run-timeline>button')).toHaveCount(count);await expect(page.getByRole('navigation',{name:'订阅运行历史分页',exact:true})).toBeVisible();await historyGeometry(page);
});
test('I30b full history pagination preserves page and page-size requests',async({page})=>{
 const reads=await historyFixture(page,30);await page.goto('/subscriptions?tab=history&subscription=i15-sub-0');await expect(page.locator('.run-timeline>button')).toHaveCount(25);
 const pager=page.getByRole('navigation',{name:'订阅运行历史分页',exact:true});await test.step('next history page',async()=>{await pager.getByRole('button',{name:'下一页',exact:true}).click()});await expect(page.locator('.run-timeline>button')).toHaveCount(5);expect(reads.at(-1)!.searchParams.get('page')).toBe('2');
 await test.step('previous history page',async()=>{await pager.getByRole('button',{name:'上一页',exact:true}).click()});await expect(page.locator('.run-timeline>button')).toHaveCount(25);
 await test.step('open history page size',async()=>{await pager.locator('.mt-page-summary .el-select__wrapper').click()});await page.getByRole('option',{name:'10',exact:true}).click();await expect(page.locator('.run-timeline>button')).toHaveCount(10);expect(reads.at(-1)!.searchParams.get('page')).toBe('1');expect(reads.at(-1)!.searchParams.get('pageSize')).toBe('10');
 await pager.getByRole('button',{name:'末页',exact:true}).click();await expect(pager).toContainText('第 3 页 / 共 3 页');await expect.poll(()=>reads.at(-1)?.searchParams.get('page')).toBe('3');
 await pager.getByRole('button',{name:'首页',exact:true}).click();await expect(pager).toContainText('第 1 页 / 共 3 页');expect(reads.at(-1)!.searchParams.get('page')).toBe('1');
 await historyGeometry(page);
});
test('I30 follow-list keeps its own padded pagination at 375px',async({page})=>{
 await fixture(page);await page.setViewportSize({width:375,height:1000});await page.goto('/subscriptions');await expect(page.locator('.subscription-row')).toHaveCount(25);await geometry(page,375);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
});
