import {test,expect} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {u7Fixture} from './u7-fixture';
import {mediaUser} from './u5-fixture';
const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i1/screens');mkdirSync(output,{recursive:true});
const before=process.env.I1_BEFORE==='1';
for(const width of [1440,390])for(const theme of ['light','dark'])for(const area of ['users','subscriptions','library'])test(`I1 ${area} ${width} ${theme}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
 const path=area==='users'?'/playback':`/${area}`;await u7Fixture(page,path);
 const requests:URL[]=[];
 const pattern=area==='users'?'**/api/v1/media-server/users{,?*}':area==='subscriptions'?'**/api/v1/subscriptions?*':'**/api/v1/media/library?*';
 await page.route(pattern,async r=>{
  const url=new URL(r.request().url());requests.push(url);const size=Number(url.searchParams.get('pageSize')||(area==='library'?24:25)),p=Number(url.searchParams.get('page')||1);
  const items=Array.from({length:Math.max(0,Math.min(size,77-(p-1)*size))},(_,i)=>{const n=(p-1)*size+i;return area==='users'?{...mediaUser,id:`user-${n}`,username:`user${n}`,displayName:`家庭成员 ${n+1}`}:area==='subscriptions'?{id:`sub-${n}`,title:`订阅 ${n+1}`,media:{id:n+1,mediaType:'movie',title:`订阅 ${n+1}`,year:'2026'},enabled:true,revision:1,rules:{accountIds:[],include:[],exclude:[]}}:{id:`movie_${n}`,title:`作品 ${n+1}`,type:'Movie',mediaType:'movie',year:'2026'};});
  await r.fulfill({json:{data:{items,total:77,page:p,pageSize:size,categories:[]}}});
 });
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(area==='users'?'/playback?tab=users&usersPage=2':area==='subscriptions'?'/subscriptions?subscriptionsPage=2':'/library?view=all&tab=browse&browsePage=2');
 const nav=page.locator('.mt-pagination').first();await expect(nav).toBeVisible();await expect(nav).toContainText('第 2 页');
 if(!before){
  await expect(nav).toContainText('共 77 条');await expect(nav).not.toContainText(/\d+\s*–\s*\d+\s*\//);
  await nav.locator('.el-select').click();await page.getByRole('option',{name:'10',exact:true}).click();
  await expect(nav).toContainText('第 1 页');await expect.poll(()=>requests.at(-1)?.searchParams.get('pageSize')).toBe('10');await expect.poll(()=>requests.at(-1)?.searchParams.get('page')).toBe('1');
  await expect.poll(()=>new URL(page.url()).searchParams.get(area==='users'?'usersPageSize':area==='subscriptions'?'subscriptionsPageSize':'browsePageSize')).toBe('10');
  await page.reload();await expect(nav).toContainText('第 1 页');await expect.poll(()=>requests.at(-1)?.searchParams.get('pageSize')).toBe('10');
 }
 await nav.scrollIntoViewIfNeeded();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);expect(errors).toEqual([]);
 await nav.screenshot({path:resolve(output,`${before?'before':'after'}-${area}-${width}-${theme}-pagination.png`),animations:'disabled'});
 await page.evaluate(()=>scrollTo(0,0));
 await page.screenshot({path:resolve(output,`${before?'before':'after'}-${area}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
});
