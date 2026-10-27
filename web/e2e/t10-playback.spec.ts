import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {u5PlaybackFixture,mediaUser} from './u5-fixture';
import {fixtureFulfill} from './collection-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/t10');
const users=[mediaUser,...[2,3,4].map(n=>({...mediaUser,id:'user-'+n,username:'family-'+n,displayName:n===2?'一个很长很长的媒体用户显示名称需要省略并且保留完整标题':'家庭成员 '+n}))];
async function fixture(page:Page){
 await u5PlaybackFixture(page);
 await page.route('**/api/v1/media-server/users?*',r=>fixtureFulfill(r,{json:{data:users}}));
 await page.route('**/api/v1/media-server/users/'+mediaUser.id,r=>fixtureFulfill(r,{json:{data:{...mediaUser,activeSessions:1}}}));
 await page.route('**/api/v1/media-server/native/sessions?*',r=>fixtureFulfill(r,{json:{data:{items:[{id:'session-1',userId:mediaUser.id,userName:mediaUser.username,itemName:'测试影片',deviceName:'测试设备',state:'playing',lastProgressAt:'2026-10-26T08:00:00Z'}],total:1}}}));
}
test('T10 screenshots and geometry',async({page})=>{
 test.setTimeout(120000);mkdirSync(dir,{recursive:true});await fixture(page);
 const before=process.env.T10_BEFORE==='1',errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 if(before){const root=join(dir,'before-assets');await page.route('**/playback?*',r=>r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}));await page.route('**/assets/*',r=>{const n=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(n)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',n))})})}
 for(const kind of ['users','players','sessions'])for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/playback?tab='+kind);await expect(page.getByRole('heading',{name:kind==='users'?'媒体用户':kind==='players'?'播放器准入':'在线会话',exact:true})).toBeVisible();await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  if(kind==='users')await expect(page.locator('.media-user-card')).toHaveCount(4);
  if(kind==='sessions')await expect(page.getByRole('button',{name:'会话详情',exact:true})).toBeVisible();
  if(kind==='players')await expect(page.getByLabel('UA 策略模式')).toBeEnabled();
  const geometry=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth,cards:[...document.querySelectorAll('.media-user-card')].map(e=>{const b=e.getBoundingClientRect();return {x:b.x,y:b.y,width:b.width,height:b.height}})}));
  if(!before){expect(geometry.scroll).toBeLessThanOrEqual(width);if(kind==='users'&&width===1440){expect(geometry.cards[0]!.y).toBe(geometry.cards[1]!.y);expect(geometry.cards[0]!.height).toBeLessThan(320)}
   if(kind==='players'){const boxes=await page.locator('.ua-policy-actions').evaluate(e=>[...e.querySelectorAll('select,button')].map(n=>{const r=n.getBoundingClientRect();return {bottom:r.bottom,height:r.height}}));expect(boxes).toHaveLength(2);expect(Math.abs(boxes[0]!.bottom-boxes[1]!.bottom)).toBeLessThanOrEqual(1);expect(boxes[0]!.height).toBe(boxes[1]!.height)}
  }
  await page.screenshot({path:join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});writeFileSync(join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}.json`),JSON.stringify(geometry));
 }
 expect(errors).toEqual([]);
});
test('T10 sessions only open on explicit record selection, including retained userId',async({page})=>{
 await fixture(page);await page.goto('/playback?tab=users');await page.locator('.media-user-card').first().getByRole('button',{name:'用户详情',exact:true}).click();const userDetail=page.getByRole('dialog',{name:/用户详情/});await expect(userDetail).toBeVisible();await expect(userDetail).toContainText('权限摘要');await expect(userDetail).toContainText('同时播放上限');await userDetail.getByRole('link',{name:'查看会话与撤销登录'}).click();await expect(page.getByRole('heading',{name:'在线会话',exact:true})).toBeVisible();await expect(page.getByRole('dialog')).toHaveCount(0);
 await page.getByRole('button',{name:'媒体用户',exact:true}).click();await page.getByRole('button',{name:'在线会话',exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(0);
 await page.getByRole('button',{name:'会话详情',exact:true}).click();const detail=page.getByRole('dialog',{name:'在线会话详情',exact:true});await expect(detail).toBeVisible();await expect(detail).toContainText('测试影片');await detail.getByRole('button',{name:'关闭此对话框',exact:true}).click();
 await page.getByRole('button',{name:'用户登录详情',exact:true}).click();await expect(detail).toContainText('有效登录 1');await page.keyboard.press('Escape');
 await page.reload();await expect(page.getByRole('button',{name:'用户登录详情',exact:true})).toBeVisible();await expect(page.getByRole('dialog')).toHaveCount(0);
});
