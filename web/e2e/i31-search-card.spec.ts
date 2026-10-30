import {test,expect,type Page} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {u3Fixture,u3Media} from './u3-fixture';
const items=[u3Media,{...u3Media,id:43,title:'另一部作品',originalTitle:'Another Work'}];
const name=`搜索《${u3Media.title}》(${u3Media.year}) 的站点资源`;
async function fixture(page:Page){await u3Fixture(page);await page.route('**/api/v1/search/media?*',r=>r.fulfill({json:{data:{items,cached:false}}}));}
async function lookup(page:Page){await page.goto('/search');await page.getByLabel('TMDB媒体名称',{exact:true}).fill(u3Media.title);await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.locator('.media-search-card')).toHaveCount(2);await expect(page.getByRole('combobox',{name:'搜索站点',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'搜索站点资源',exact:true})).toHaveCount(0);}
for(const key of ['Enter','Space'])test(`I31 ${key} searches once and locks every card until accepted`,async({page})=>{
 await fixture(page);let starts=0,body:any,accept!:()=>void;const pending=new Promise<void>(r=>accept=r);
 await page.route('**/api/v1/search',async r=>{if(r.request().method()!=='POST')return r.fulfill({json:{data:[]}});starts++;body=r.request().postDataJSON();await pending;return r.fulfill({status:202,json:{data:{id:'run-u3'}}})});
 await lookup(page);const first=page.getByRole('button',{name,exact:true});await first.focus();await first.press(key==='Space'?' ':key);
 try{
  await expect(first).toHaveAttribute('aria-busy','true');await expect(first.getByRole('status')).toHaveText('正在搜索全部站点…');
  for(const card of await page.locator('.media-search-card').all())await expect(card).toBeDisabled();
  await expect(page.locator('.media-search-card').last()).toHaveAttribute('aria-busy','false');await page.keyboard.press('Enter');expect(starts).toBe(1);expect(body).toEqual({keyword:u3Media.title,refresh:false,accountIds:[],media:{id:42,mediaType:'movie'}});
 }finally{accept()}
 await expect(page).toHaveURL(/\/search\/results\/run-u3$/);expect(starts).toBe(1);
});
test('I31 unloaded/empty sites disable the cards and retain the configuration hint',async({page})=>{
 await fixture(page);let release!:()=>void,starts=0;const pending=new Promise<void>(r=>release=r);
 await page.route('**/api/v1/integrations',async r=>{await pending;return r.fulfill({json:{data:[]}})});
 page.on('request',r=>{if(r.url().endsWith('/api/v1/search')&&r.method()==='POST')starts++});
 try{await lookup(page);for(const card of await page.locator('.media-search-card').all())await expect(card).toBeDisabled();await expect(page.getByText('正在读取可用站点…',{exact:true})).toBeVisible()}finally{release()}
 await expect(page.getByText('还没有可用站点。',{exact:false})).toBeVisible();await expect(page.getByRole('link',{name:'配置站点账号',exact:true})).toHaveAttribute('href','/sites?tab=accounts');for(const card of await page.locator('.media-search-card').all())await expect(card).toBeDisabled();expect(starts).toBe(0);
});
test('I31 lost receipt remains blocked and reload reconciles without replay',async({page})=>{
 await fixture(page);let starts=0;await page.route('**/api/v1/search',r=>{if(r.request().method()==='POST'){starts++;return r.abort('failed')}return r.fulfill({json:{data:[]}})});
 await lookup(page);await page.getByRole('button',{name,exact:true}).click();await expect(page.locator('.async-result-banner')).toContainText('搜索回执未知');for(const card of await page.locator('.media-search-card').all())await expect(card).toBeDisabled();expect(starts).toBe(1);
 await lookup(page);await page.getByRole('button',{name,exact:true}).click();await expect(page.locator('.async-result-banner')).toContainText('搜索回执未知');expect(starts).toBe(1);
});
test('I31 direct site search has no selector and retains explicit cache bypass',async({page})=>{
 await fixture(page);let body:any,starts=0;await page.route('**/api/v1/search',r=>{if(r.request().method()==='POST'){starts++;body=r.request().postDataJSON();return r.fulfill({status:202,json:{data:{id:'run-u3'}}})}return r.fulfill({json:{data:[]}})});
 await page.goto('/search');await page.getByRole('button',{name:'站点直搜',exact:true}).click();await expect(page.getByRole('combobox',{name:'搜索站点',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'开始搜索',exact:true})).toBeDisabled();await page.getByLabel('资源关键词',{exact:true}).fill('自有资源关键词');await page.getByRole('button',{name:'忽略站点缓存重新搜索',exact:true}).click();await expect(page).toHaveURL(/\/search\/results\/run-u3$/);expect(starts).toBe(1);expect(body).toEqual({keyword:'自有资源关键词',refresh:true,accountIds:[]});
});
const capture=process.env.I31_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
test.describe('I31 local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit local I31_SCREENSHOTS=1 only, never CI');
 for(const width of [1440,375])for(const theme of ['light','dark'])test(`I31 search ${width} ${theme}`,async({page})=>{
  await fixture(page);await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await lookup(page);await expect(page.getByRole('button',{name,exact:true})).toBeEnabled();
  const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i31/screens');mkdirSync(dir,{recursive:true});
  for(const mode of ['media','site']){
   if(mode==='site')await page.getByRole('button',{name:'站点直搜',exact:true}).click();await page.evaluate(async()=>{await document.fonts.ready;await new Promise<void>(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>r()))) });
   expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);await page.screenshot({path:resolve(dir,`after-${mode}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
  }
 });
});
for(const width of [1440,375])test(`I31 initial site readiness does not shift the page at ${width}px`,async({page})=>{
 await fixture(page);await page.route('**/api/v1/integrations',async r=>{await new Promise(resolve=>setTimeout(resolve,180));return r.fulfill({json:{data:[{id:'site',name:'站点',kind:'ptsite',enabled:true}]}})});
 await page.setViewportSize({width,height:1000});await page.addInitScript(()=>{(window as any).__i31cls=0;(window as any).__i31shifts=[];new PerformanceObserver(list=>{for(const row of list.getEntries() as any[])if(!row.hadRecentInput){(window as any).__i31cls+=row.value;(window as any).__i31shifts.push({value:row.value,sources:row.sources?.map((s:any)=>({node:s.node?.outerHTML?.slice(0,160),before:s.previousRect,after:s.currentRect}))})}}).observe({type:'layout-shift',buffered:true})});
 const response=page.waitForResponse(r=>r.url().endsWith('/api/v1/integrations'));await page.goto('/search');await response;await expect(page.getByText('正在读取可用站点…',{exact:true})).toHaveCount(0);await page.evaluate(async()=>{await document.fonts.ready;await new Promise<void>(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>r()))) });await page.waitForTimeout(250);
 const cls=await page.evaluate(()=>(window as any).__i31cls),shifts=await page.evaluate(()=>(window as any).__i31shifts);const {writeFileSync,mkdirSync}=await import('node:fs'),dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i31');mkdirSync(dir,{recursive:true});writeFileSync(resolve(dir,`cls-${width}.json`),JSON.stringify({cls,shifts},null,2));expect(cls).toBeLessThan(.01);
});
