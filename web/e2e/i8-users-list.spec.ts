import {test,expect,type Page,type Locator} from '@playwright/test';
import {mkdirSync,readFileSync} from 'node:fs';
import {basename,extname,resolve} from 'node:path';
import {u5PlaybackFixture,mediaUser} from './u5-fixture';
const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i8/screens');
const capture=process.env.I8_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const before=process.env.I8_BEFORE==='1';
const longName='家庭媒体用户超长显示名称用于检查省略并保留完整标题';
const rows=Array.from({length:61},(_,n)=>({...mediaUser,id:`i8-user-${n}`,username:n===0?'family-member-with-long-username':`family-${n+1}`,displayName:n===0?longName:`家庭成员 ${n+1}`,enabled:n!==1,lockedUntil:n===2?'2099-01-01T00:00:00Z':null,lastPlayedAt:n===0?'2026-10-26T11:10:04Z':null,lastIp:n===0?'8.8.8.8':'',deviceCount:3}));
async function fixture(page:Page,count=61){
 await u5PlaybackFixture(page);const calls:URL[]=[],writes:string[]=[];let fail=false;
 page.on('request',r=>{if(r.url().includes('/api/')&&r.method()!=='GET')writes.push(r.url())});
 await page.route('**/api/v1/media-server/users?*',r=>{const url=new URL(r.request().url());calls.push(url);if(fail)return r.fulfill({status:503,json:{error:{message:'用户读取失败'}}});const p=Number(url.searchParams.get('page')),size=Number(url.searchParams.get('pageSize'));return r.fulfill({json:{data:{items:rows.slice(0,count).slice((p-1)*size,p*size),total:count,page:p,pageSize:size}}})});
 await page.route('**/api/v1/ip-geolocation?*',r=>r.fulfill({json:{data:[{ip:'8.8.8.8',label:'美国 加利福尼亚州 Google',location:'美国 加利福尼亚州',isp:'Google'}]}}));
 return {calls,writes,fail:()=>{fail=true}};
}
async function geometry(page:Page,width:number){
 const list=page.locator('.media-user-list'),row=list.locator('.media-user-row').first();await expect(row).toBeVisible();await expect(row.locator('.f7-ip')).toContainText('美国');
 const layout=await row.evaluate(el=>{const r=el.getBoundingClientRect(),list=el.parentElement!.getBoundingClientRect(),identity=el.querySelector('.media-user-identity')!.getBoundingClientRect(),status=el.querySelector('.media-user-state')!.getBoundingClientRect(),recent=el.querySelector('.media-user-recent>div')!.getBoundingClientRect(),actions=el.querySelector('.media-user-actions')!.getBoundingClientRect();return{width:r.width,listWidth:list.width,overflow:document.documentElement.scrollWidth>innerWidth,identity:{x:identity.x,y:identity.y,bottom:identity.bottom},status:{x:status.x,y:status.y,bottom:status.bottom},recent:{x:recent.x,y:recent.y},actions:{x:actions.x,y:actions.y,right:actions.right},right:r.right}});
 expect(layout.width).toBeCloseTo(layout.listWidth,0);expect(layout.overflow).toBe(false);expect(Math.abs(layout.actions.right-layout.right)).toBeLessThan(1);
 if(width===1440){await expect(page.locator('.media-user-columns')).toBeVisible();expect(layout.actions.x).toBeGreaterThan(layout.status.x);expect(Math.abs(layout.identity.y-layout.actions.y)).toBeLessThan(10);const alignment=await page.locator('.media-users-panel').evaluate(el=>{const header=[...el.querySelectorAll('.media-user-columns>span')].map(n=>n.getBoundingClientRect().x),row=el.querySelector('.media-user-row')!,cells=[row.querySelector('.media-user-identity'),row.querySelector('.media-user-state'),...row.querySelectorAll('.media-user-recent>div'),row.querySelector('.media-user-actions')];return cells.map((n,i)=>Math.abs(n!.getBoundingClientRect().x-header[i]!))});expect(alignment.every(n=>n<1)).toBe(true);}
 else{await expect(page.locator('.media-user-columns')).toBeHidden();expect(layout.recent.y).toBeGreaterThanOrEqual(layout.identity.bottom);expect(layout.actions.y).toBeGreaterThanOrEqual(layout.status.bottom);await expect(row.locator('.media-user-action-text').first()).toBeHidden();await expect(row.locator('.media-user-action-icon').first()).toBeVisible();await expect(row.locator('.media-user-action-icon svg')).toHaveCount(3);}
 for(const name of ['编辑用户','用户详情','更多']){const b=row.getByRole('button',{name,exact:true});await expect(b).toHaveAttribute('title',name);const bounds=await b.boundingBox();expect(bounds!.height).toBeGreaterThanOrEqual(44);expect(bounds!.width).toBeGreaterThanOrEqual(44);}
 await expect(row.locator('.media-user-name h3')).toHaveAttribute('title',longName);
 await expect(row.locator('.media-user-name p')).toHaveAttribute('title',rows[0]!.username);
 return row;
}
for(const width of [1440,390])for(const theme of ['light','dark'])test(`I8 list geometry and existing operations ${width} ${theme}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);const f=await fixture(page,3);await page.goto('/playback?tab=users');const row=await geometry(page,width);
 await expect(page.locator('.media-user-row')).toHaveCount(3);for(const state of ['启用','停用','锁定'])await expect(page.locator('.media-user-state').getByText(state,{exact:true})).toBeVisible();
 const bounds=await page.locator('.media-user-row').evaluateAll(els=>els.map(el=>{const r=el.getBoundingClientRect();return{y:r.y,bottom:r.bottom,width:r.width}}));expect(bounds[1]!.y).toBeGreaterThanOrEqual(bounds[0]!.bottom);expect(bounds[1]!.width).toBe(bounds[0]!.width);
 await row.getByRole('button',{name:'编辑用户',exact:true}).click();const editor=page.getByRole('dialog',{name:'编辑媒体用户',exact:true});await expect(editor).toBeVisible();await editor.getByRole('button',{name:'取消',exact:true}).click();await expect(editor).toBeHidden();
 await row.getByRole('button',{name:'用户详情',exact:true}).click();const detail=page.getByRole('dialog',{name:/用户详情/});await expect(detail).toContainText('3 台设备');await expect(detail.getByText('年龄分级上限',{exact:true})).toHaveCount(0);await detail.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(detail).toBeHidden();
 await row.getByRole('button',{name:'更多',exact:true}).click();await expect(page.getByRole('menuitem',{name:'重置密码',exact:true})).toBeVisible();await page.getByRole('menuitem',{name:'删除用户',exact:true}).click();const confirm=page.getByRole('dialog',{name:'删除媒体用户',exact:true});await expect(confirm).toBeVisible();await confirm.getByRole('button',{name:'取消',exact:true}).click();await expect(confirm).toBeHidden();expect(f.writes).toEqual([]);
});
test('I8 intermediate viewport uses compact rows without horizontal overflow',async({page})=>{await page.setViewportSize({width:1180,height:1000});await fixture(page,3);await page.goto('/playback?tab=users');await geometry(page,1180)});
test('I8 server pagination preserves 25 default and URL priority, resets size to page 1 and survives reload',async({page})=>{
 const f=await fixture(page);await page.goto('/playback?tab=users');await expect(page.locator('.media-user-row')).toHaveCount(25);expect(f.calls.at(-1)?.searchParams.get('pageSize')).toBe('25');
 const nav=page.getByRole('navigation',{name:'分页',exact:true});await nav.getByRole('button',{name:'下一页',exact:true}).click();await expect(nav).toContainText('第 2 页');await expect.poll(()=>f.calls.at(-1)?.searchParams.get('page')).toBe('2');
 await nav.locator('.el-select').click();await page.getByRole('option',{name:'10',exact:true}).click();await expect(page.locator('.media-user-row')).toHaveCount(10);await expect.poll(()=>f.calls.at(-1)?.searchParams.get('page')).toBe('1');await expect(page).toHaveURL(/usersPageSize=10/);await page.reload();await expect(page.locator('.media-user-row')).toHaveCount(10);
 await page.goto('/playback?tab=users&usersPage=2&usersPageSize=50');await expect(page.locator('.media-user-row')).toHaveCount(11);expect(f.calls.at(-1)?.searchParams.get('page')).toBe('2');expect(f.calls.at(-1)?.searchParams.get('pageSize')).toBe('50');expect(f.writes).toEqual([]);
});
test('I8 single user spans the whole list and read failure keeps rows but disables edit',async({page})=>{
 const f=await fixture(page,1);await page.goto('/playback?tab=users');const row=await geometry(page,1440);await expect(page.locator('.media-user-row')).toHaveCount(1);f.fail();await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(row.getByRole('button',{name:'编辑用户',exact:true})).toBeDisabled();await expect(row.getByRole('button',{name:'用户详情',exact:true})).toBeEnabled();expect(f.writes).toEqual([]);
});
test.describe('I8 visual evidence @screenshots',()=>{
 test.skip(!capture,'Local explicit opt-in only: I8_SCREENSHOTS=1');
 for(const count of [1,3])for(const width of [1440,390])for(const theme of ['light','dark'])test(`${count===1?'single':'multiple'} ${width} ${theme}`,async({page})=>{
  await fixture(page,count);await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  if(before){const root=resolve(output,'../before-assets');await page.route('**/playback?*',r=>r.fulfill({contentType:'text/html',body:readFileSync(resolve(root,'index.html'))}));await page.route('**/assets/*',r=>{const file=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(file)==='.js'?'application/javascript':'text/css',body:readFileSync(resolve(root,'assets',file))})});}
  await page.goto('/playback?tab=users');const row=page.locator(before?'.media-user-card':'.media-user-row').first();await expect(row).toBeVisible();await expect(row.locator('.f7-ip')).toContainText('美国');await expect(page.locator('html')).toHaveAttribute('data-theme',theme);if(!before)await geometry(page,width);mkdirSync(output,{recursive:true});await page.screenshot({path:resolve(output,`${before?'before':'after'}-${count===1?'single':'multiple'}-${width}-${theme}.png`),animations:'disabled'});
 });
});
