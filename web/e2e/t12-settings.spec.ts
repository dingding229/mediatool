import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {u5SitesFixture,siteAccount} from './u5-fixture';
import {u6OpsFixture} from './u6-fixture';
const dir=resolve('../scratchpad/t12'),before=process.env.T12_BEFORE==='1';
const cookie='synthetic-private-cookie',rss='https://hdsky.me/rss?synthetic',passkey='synthetic-passkey';
async function fixture(page:Page){
 await u6OpsFixture(page);await u5SitesFixture(page);
 await page.route('**/api/v1/settings/application',r=>r.fulfill({json:{data:{revision:1,value:{trustedProxyCidrs:[],searchCacheSeconds:600,tmdbCacheSeconds:86400}}}}));
 if(before)await page.route('**/api/v1/ip-geolocation/settings',r=>r.fulfill({json:{data:{source:'ip2region',version:'bundled',dataDate:'2026-10-20T00:00:00Z',ipv4:true,ipv6:true,memoryBytes:48400455}}}));
 await page.route('**/api/v1/auth/sessions{,?*}',r=>r.fulfill({json:{data:{items:[{id:'owned',username:'管理账号',current:true,ip:'203.0.113.9',userAgent:'验收浏览器',createdAt:'2026-10-25T00:00:00Z',lastActiveAt:'2026-10-26T00:00:00Z',expiresAt:'2026-11-02T00:00:00Z'}],hasNext:false}}}));
 await page.route('**/api/v1/ip-geolocation?*',r=>r.fulfill({json:{data:[{ip:'',label:'局域网/保留地址'}]}}));
}
async function open(page:Page,kind:string){
 await page.goto(kind==='guide'?'/ops?tab=attention':kind==='account'?'/sites?tab=accounts':'/settings?tab='+kind);
 if(kind==='account'){await page.getByRole('button',{name:'更多站点操作',exact:true}).first().click();await page.getByRole('menuitem',{name:'编辑账号',exact:true}).click();await expect(page.getByRole('dialog',{name:'编辑站点账号',exact:true})).toBeVisible();await expect(page.getByLabel('站点 User-Agent')).toHaveValue('Browser')}
 else if(kind==='guide')await expect(page.getByRole('heading',{name:'完成配置，开始自动跟进'})).toBeVisible();
 else if(kind==='security')await expect(page.getByRole('heading',{name:'管理端登录会话',exact:true})).toBeVisible();
 else await expect(page.locator('.settings-stack:visible .settings-card').first()).toBeVisible();
}
test('T12 responsive before/after settings, sessions, guide and account editor',async({page})=>{
 test.setTimeout(180000);mkdirSync(dir,{recursive:true});await fixture(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 for(const kind of ['general','security','guide','account'])for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await open(page,kind);
  const geometry=await page.evaluate(()=>{const guide=document.querySelector('[aria-label="首页配置引导"]')||document.querySelector('.setup-guide'),list=guide?.querySelector('.settings-actions')||guide?.querySelector('ol');return{width:innerWidth,scroll:document.documentElement.scrollWidth,guide:guide?.getBoundingClientRect().toJSON(),list:list?.getBoundingClientRect().toJSON(),parents:guide&&[guide.parentElement,guide.parentElement?.parentElement,guide.parentElement?.parentElement?.parentElement].map(e=>e&&({cls:e.className,rect:e.getBoundingClientRect().toJSON(),minHeight:getComputedStyle(e).minHeight,display:getComputedStyle(e).display})),guideMinHeight:guide&&getComputedStyle(guide).minHeight,guideParentMinHeight:guide?.parentElement&&getComputedStyle(guide.parentElement).minHeight,guideAlignContent:guide?.parentElement&&getComputedStyle(guide.parentElement).alignContent}});
  expect(geometry.scroll).toBeLessThanOrEqual(width);
  if(!before){
   if(kind==='general')await expect(page.getByRole('heading',{name:'IP 归属地',exact:true})).toHaveCount(0);
   if(kind==='security'){await expect(page.getByRole('region',{name:'管理端登录会话'}).getByText(/第.*页/)).toHaveCount(0);expect(geometry.guideMinHeight).toBe('0px');if(geometry.guide&&geometry.list)expect(geometry.guide.bottom-geometry.list.bottom).toBeLessThanOrEqual(30)}
   if(kind==='account'){const d=page.getByRole('dialog',{name:'编辑站点账号',exact:true});await expect(d.getByLabel('站点 Cookie')).toHaveValue(cookie);await expect(d.getByLabel('站点 RSS 地址')).toHaveValue(rss);await expect(d.getByLabel('站点订阅密钥')).toHaveValue(passkey);for(const label of ['站点 Cookie','站点 RSS 地址','站点订阅密钥'])await expect(d.getByLabel(label)).toHaveAttribute('type','text');await expect(d.getByRole('button',{name:/显示.*(Cookie|地址|密钥)/})).toHaveCount(0)}
  }
  const name=`${before?'before':'after'}-${kind}-${width}-${theme}`;await page.screenshot({path:resolve(dir,name+'.png'),fullPage:true,animations:'disabled'});writeFileSync(resolve(dir,name+'.json'),JSON.stringify(geometry));
 }
 expect(errors).toEqual([]);
});

test('T12 displays more than fifty active sessions without a pager or page query',async({page})=>{
 await fixture(page);const urls:string[]=[];await page.route('**/api/v1/auth/sessions{,?*}',r=>{urls.push(r.request().url());return r.fulfill({json:{data:{items:Array.from({length:75},(_,i)=>({id:'session-'+i,username:'管理账号 '+i,current:i===0,createdAt:'2026-10-25T00:00:00Z',lastActiveAt:'2026-10-26T00:00:00Z',expiresAt:'2026-11-02T00:00:00Z',ip:'',userAgent:'验收浏览器'}))}}})});
 await open(page,'security');const list=page.getByRole('region',{name:'管理端登录会话',exact:true});await expect(list.locator('article')).toHaveCount(75);await expect(list.getByText(/第.*页/)).toHaveCount(0);expect(urls).toHaveLength(1);expect(new URL(urls[0]).search).toBe('');
});
test('T12 saves all three plaintext edits through the existing validated account API',async({page})=>{
 await fixture(page);let payload:any;await page.route('**/api/v1/pt/accounts',r=>r.fulfill({json:{data:[{...siteAccount,definitionId:'hdsky',revision:payload?2:1,values:{cookie,rssUrl:rss,passkey}}]}}));await page.route('**/api/v1/pt/accounts/u5-site',r=>{payload=r.request().postDataJSON();return r.fulfill({json:{data:{rssState:'ready'}}})});await open(page,'account');const d=page.getByRole('dialog',{name:'编辑站点账号',exact:true});await d.getByLabel('站点 Cookie').fill('synthetic-edited-cookie');await d.getByLabel('站点 RSS 地址').fill('https://hdsky.me/rss?edited');await d.getByLabel('站点订阅密钥').fill('synthetic-edited-key');await d.getByRole('button',{name:'验证并保存',exact:true}).click();await expect(d).not.toBeVisible();expect(payload).toMatchObject({revision:1,values:{cookie:'synthetic-edited-cookie',userAgent:'Browser',passkey:'synthetic-edited-key'},rssUrl:'https://hdsky.me/rss?edited'});
});
