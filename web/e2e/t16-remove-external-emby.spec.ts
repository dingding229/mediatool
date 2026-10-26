import {test,expect} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {u5PlaybackFixture} from './u5-fixture';
const dir=resolve('../scratchpad/t16'),before=process.env.T16_BEFORE==='1';
test('T16 built-in library and connection navigation remain usable without external Emby',async({page})=>{
 test.setTimeout(120000);mkdirSync(dir,{recursive:true});await u5PlaybackFixture(page);
 await page.route('**/api/v1/media/library?*',r=>r.fulfill({json:{data:{items:[],total:0,categories:[]}}}));
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 for(const width of[1440,390])for(const theme of['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  for(const [name,url]of[['connections','/connections?tab=storage'],['library','/library?tab=browse'],['domains','/settings?tab=config']]){
   await page.goto(url);await expect(page.locator('h1').first()).toBeVisible();if(name==='library')await expect(page.getByLabel('搜索媒体标题',{exact:true})).toBeVisible();await page.locator('.page-progress').waitFor({state:'hidden'}).catch(()=>{});
   if(!before){await expect(page.getByRole('button',{name:'Emby 首页',exact:true})).toHaveCount(0);await expect(page.getByText('Emby 网关域名',{exact:true})).toHaveCount(0);await expect(page.getByRole('tab',{name:'Emby',exact:true})).toHaveCount(0)}
   const metrics=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth,theme:document.documentElement.className}));expect(metrics.scroll).toBeLessThanOrEqual(width);
   const stem=`${before?'before':'after'}-${name}-${width}-${theme}`;await page.screenshot({path:resolve(dir,stem+'.png'),fullPage:true,animations:'disabled'});writeFileSync(resolve(dir,stem+'.json'),JSON.stringify(metrics));
  }
 }
 expect(errors).toEqual([]);
});

test('T16B session initial render, slow empty page and null response never open details',async({page})=>{
 test.setTimeout(60000);await u5PlaybackFixture(page);
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 let release:()=>void=()=>{};let served:()=>void=()=>{};
 const pending=new Promise<void>(r=>release=r),responded=new Promise<void>(r=>served=r);
 await page.route('**/api/v1/media-server/native/sessions?*',async r=>{await pending;await r.fulfill({json:{data:{items:[],total:0,startIndex:0,limit:50}}});served()});
 await page.goto('/playback?tab=sessions');await expect(page.getByRole('heading',{name:'在线会话',exact:true})).toBeVisible();
 await expect(page.getByText('第 1 页 / 共 1 页',{exact:true})).toBeVisible();await expect(page.getByRole('dialog')).toHaveCount(0);
 release();await responded;await expect(page.getByText('暂无记录',{exact:true})).toBeVisible();
 await page.route('**/api/v1/media-server/native/sessions?*',r=>r.fulfill({json:{data:null}}));
 for(const width of[1440,390])for(const theme of['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  await page.goto('/playback?tab=sessions');await expect(page.getByRole('heading',{name:'在线会话',exact:true})).toBeVisible();
  await expect(page.getByText('第 1 页 / 共 1 页',{exact:true})).toBeVisible();await expect(page.getByRole('dialog')).toHaveCount(0);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await page.screenshot({path:resolve(dir,`after-sessions-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
 }
 expect(errors).toEqual([]);
});
