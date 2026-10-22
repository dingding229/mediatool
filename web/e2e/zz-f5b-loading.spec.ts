import {test,expect,type Page,type BrowserContext,type Request} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {primaryPages} from '../src/navigation/pages';
const dir=resolve(import.meta.dirname,'../../data/private/f5/f5b/loading');mkdirSync(dir,{recursive:true});
let cookies:Awaited<ReturnType<BrowserContext['cookies']>>;
async function authenticate(page:Page){if(cookies){await page.context().addCookies(cookies);return}await page.goto('/login');await page.getByLabel('管理员账号',{exact:true}).fill(process.env.E2E_ADMIN_USERNAME!);await page.getByLabel('密码',{exact:true}).fill(process.env.E2E_ADMIN_PASSWORD!);await page.getByRole('button',{name:'进入工作台',exact:true}).click();await expect(page.getByRole('heading',{name:'首页',exact:true})).toBeVisible();cookies=await page.context().cookies()}
for(const surface of primaryPages)for(const tab of surface.tabs.length?surface.tabs:[{id:'',label:''}])test(`F5b loading audit ${surface.id}/${tab.id}`,async({page})=>{
 test.setTimeout(90000);await authenticate(page);
 const active=new Set<Request>(),errors:string[]=[],network:any[]=[];page.on('request',r=>{if(r.url().includes('naming-preview'))network.push({at:Date.now(),stage:'start',url:r.url(),method:r.method()})});page.on('requestfinished',r=>{if(r.url().includes('naming-preview'))network.push({at:Date.now(),stage:'finished',url:r.url()})});page.on('response',r=>{if(r.url().includes('naming-preview'))network.push({at:Date.now(),stage:'response',status:r.status()})});page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>{if(r.url().includes('/api/v1/')&&!r.url().includes('/jobs/events'))active.add(r)});for(const event of ['requestfinished','requestfailed'] as const)page.on(event,r=>active.delete(r));
 page.on('framenavigated',frame=>{if(frame===page.mainFrame())active.clear()});
 await page.addInitScript(()=>{(window as any).__cls=0;(window as any).__shifts=[];new PerformanceObserver(list=>{for(const row of list.getEntries() as any[])if(!row.hadRecentInput){(window as any).__cls+=row.value;(window as any).__shifts.push({value:row.value,sources:row.sources?.map((s:any)=>({node:s.node?.outerHTML?.slice(0,150),before:s.previousRect,after:s.currentRect}))})}}).observe({type:'layout-shift',buffered:true})});
 await page.route('**/api/v1/**',async r=>{if(!r.request().url().includes('/jobs/events'))await new Promise(resolve=>setTimeout(resolve,180));await r.continue()});
 const result:any[]=[];
 for(const [mode,width,height] of [['desktop',1440,1000],['narrow',390,844]] as const){
  await page.setViewportSize({width,height});await page.goto('/'+surface.id+(tab.id?'?tab='+tab.id:''));await expect(page.locator('.page-heading').first()).toBeVisible();
  const heading=page.locator('.page-heading').first(),initial=await heading.boundingBox();await page.waitForTimeout(surface.id==='settings'&&tab.id==='naming'?850:0);
  try{await expect.poll(()=>[...active].map(r=>r.url()),{timeout:30000}).toEqual([])}catch(e){writeFileSync(resolve(dir,`pending-${surface.id}-${tab.id}.json`),JSON.stringify(network,null,2));throw e}await page.waitForTimeout(250);
  const settled=await heading.boundingBox();const selector='.media-surface:visible,.acquire-surface:visible,.settings-card:visible,.f3-log-table:visible';const area=page.locator(selector).first();const before=await area.count()?await area.boundingBox():await page.locator('#main-content').boundingBox();
  const refresh=page.locator('.mt-refresh').first();if(await refresh.count()){await refresh.click();await page.waitForTimeout(surface.id==='settings'&&tab.id==='naming'?850:250);await expect.poll(()=>[...active].map(r=>r.url()),{timeout:30000}).toEqual([]);await page.waitForTimeout(250)}
  const after=await area.count()?await area.boundingBox():await page.locator('#main-content').boundingBox();const cls=await page.evaluate(()=>(window as any).__cls||0);const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);
  const entry={page:surface.id,tab:tab.id,mode,cls,initialHeader:initial,settledHeader:settled,beforeRefresh:before,afterRefresh:after,selector:await area.count()?selector:'#main-content',overflow,errors:[...errors],shifts:await page.evaluate(()=>(window as any).__shifts)};result.push(entry);
  await page.screenshot({path:resolve(dir,`${surface.id}-${tab.id||'home'}-${mode}.png`),fullPage:true,animations:'disabled'});
 }
 writeFileSync(resolve(dir,`${surface.id}-${tab.id||'home'}.json`),JSON.stringify(result,null,2));
 for(const row of result){expect(row.errors,JSON.stringify(row)).toEqual([]);expect(row.overflow,JSON.stringify(row)).toBe(false);expect(row.cls,JSON.stringify(row)).toBeLessThan(.01);for(const key of ['x','y','width','height']){expect(Math.abs(row.initialHeader[key]-row.settledHeader[key]),JSON.stringify(row)).toBeLessThanOrEqual(1);expect(Math.abs(row.beforeRefresh[key]-row.afterRefresh[key]),JSON.stringify(row)).toBeLessThanOrEqual(1)}}
});
