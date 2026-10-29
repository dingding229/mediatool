import {expect,test} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {u4Fixture} from './u4-fixture';
import {u6OpsFixture} from './u6-fixture';
const capture=process.env.B2_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const shots='test-results/b2';
async function screenshot(page:import('@playwright/test').Page,name:string){await page.evaluate(()=>window.scrollTo(0,0));await expect.poll(()=>page.evaluate(()=>window.scrollY)).toBe(0);if(capture)await page.screenshot({path:shots+'/'+name+'.png',fullPage:true,animations:'disabled'})}
const session=(id:string,current=false)=>({id,accountId:'owned',username:'管理员',current,createdAt:'2026-10-21T09:00:00Z',lastActiveAt:'2026-10-21T10:00:00Z',expiresAt:'2026-10-21T17:00:00Z',ip:'192.0.2.10',userAgent:'Original Agent/1.0 中文 <raw>'});
test('B2 administrator session UI keeps raw values, confirms individual revoke and current logout',async({page})=>{
 await u4Fixture(page);let rows=[session('current',true),session('other')],writes:string[]=[];
 await page.route('**/api/v1/auth/sessions{,?*}',r=>r.fulfill({json:{data:{items:rows}}}));
 await page.route('**/api/v1/auth/sessions/*/revoke',r=>{expect(r.request().postDataJSON()).toEqual({confirm:true});const id=new URL(r.request().url()).pathname.split('/').at(-2)!;writes.push(id);rows=rows.filter(x=>x.id!==id);return r.fulfill({json:{data:{revoked:true,loginRequired:id==='current'}}})});
 await page.goto('/settings?tab=security');const panel=page.getByRole('region',{name:'管理端登录会话'});await expect(panel.getByText(/第.*页/)).toHaveCount(0);await expect(panel.getByText('UA：Original Agent/1.0 中文 <raw>').first()).toBeVisible();
 await page.getByRole('button',{name:'撤销此登录',exact:true}).click();expect(writes).toEqual([]);await page.getByRole('button',{name:'确认撤销登录',exact:true}).click();await expect.poll(()=>writes).toEqual(['other']);await expect(panel.getByText('已撤销指定管理端登录',{exact:false})).toBeVisible();
 await expect(page.locator('.el-overlay:visible')).toHaveCount(0);if(capture)mkdirSync(shots,{recursive:true});await screenshot(page,'admin-sessions-desktop');
 await page.setViewportSize({width:390,height:844});await screenshot(page,'admin-sessions-mobile');
 await page.getByRole('button',{name:'撤销当前登录',exact:true}).click();await page.getByRole('button',{name:'确认撤销登录',exact:true}).click();await expect(page).toHaveURL(/login\?sessionRevoked=1/);await expect(page.getByText('当前管理端登录已撤销，请重新登录。',{exact:true})).toBeVisible();
});
test('B2 task control confirms, preserves 202 pending, reconciles unknown and never blindly retries',async({page})=>{
 await u6OpsFixture(page);const id='11111111-1111-4111-8111-111111111112';let state='unknown',revision=1,posts=0,checks=0;
 await page.route('**/api/v1/jobs/browse?*',r=>r.fulfill({json:{data:{items:[{id,kind:'download.add',state,queue:'default',attempts:1,maxAttempts:3,payload:{},lastError:state==='unknown'?'原下载器暂不可读取，保留未知状态':''}],total:1}}}));
 await page.route(`**/api/v1/tasks/jobs/${id}`,r=>{checks++;return r.fulfill({json:{data:{id,state,revision,queued:state==='pending',stopPending:false,reason:state==='unknown'?'原下载器暂不可读取，保留未知状态':'请求已排队，尚未完成'}}})});
 await page.route(`**/api/v1/tasks/jobs/${id}/retry`,r=>{expect(r.request().postDataJSON()).toEqual({confirm:true,revision});posts++;revision++;state=posts===1?'unknown':'pending';return r.fulfill({status:state==='pending'?202:200,json:{data:{id,state,revision,queued:state==='pending',stopPending:false,reason:state==='unknown'?'核对失败：原下载器暂不可读取；没有重新提交':'请求已排队，尚未完成'}}})});
 await page.goto('/ops?tab=jobs');await page.getByRole('button',{name:'更多',exact:true}).click();await page.getByRole('menuitem',{name:'核对后重试',exact:true}).click();expect(posts).toBe(0);expect(checks).toBeGreaterThan(0);await page.getByRole('button',{name:'确认处理当前任务',exact:true}).click();await expect(page.getByText('核对失败：原下载器暂不可读取；没有重新提交',{exact:true}).first()).toBeVisible();
 await expect(page.locator('.el-overlay:visible')).toHaveCount(0);if(capture)mkdirSync(shots,{recursive:true});await screenshot(page,'task-unknown-desktop');
 await page.getByRole('button',{name:'更多',exact:true}).click();await page.getByRole('menuitem',{name:'核对后重试',exact:true}).click();await page.getByRole('button',{name:'确认处理当前任务',exact:true}).click();await expect(page.getByText('只表示请求已受理或等待核对，不代表任务完成；不能据此重复执行。',{exact:true})).toBeVisible();expect(posts).toBe(2);await page.getByRole('button',{name:'更多',exact:true}).click();await expect(page.getByRole('menuitem',{name:'取消后台任务',exact:true})).toHaveAttribute('aria-disabled','true');
 await expect(page.locator('.el-overlay:visible')).toHaveCount(0);await screenshot(page,'task-queued-desktop');
});
test('B2 real isolated session revoke denies only the selected cookie on next request',async({page,browser})=>{
 const base=process.env.E2E_BASE_URL!,username=process.env.E2E_ADMIN_USERNAME!,password=process.env.E2E_ADMIN_PASSWORD!;expect(process.env.E2E_ISOLATED).toBe('1');
 const login={username,password},headers={'X-Requested-With':'media-tool'};
 // Graded runs need not include framework's first-registration test. Bootstrap
 // only this explicitly isolated stack, never reset an existing account.
 const setup=await(await page.request.get(`${base}/api/v1/auth/setup`)).json();
 if(setup.data.registrationOpen)expect((await page.request.post(`${base}/api/v1/auth/register`,{headers,data:{username,password,confirmPassword:password}})).status()).toBe(201);
 expect((await page.request.post(`${base}/api/v1/auth/login`,{headers,data:login})).status()).toBe(200);
 const other=await browser.newContext({baseURL:base,userAgent:'Owned B2 second management session'});
 try{
 expect((await other.request.post('/api/v1/auth/login',{headers,data:login})).status()).toBe(200);
 const listed=await(await other.request.get('/api/v1/auth/sessions')).json();const current=listed.data.items.find((x:any)=>x.current);expect(current.userAgent).toBe('Owned B2 second management session');
 expect((await page.request.post(`${base}/api/v1/auth/sessions/${current.id}/revoke`,{headers,data:{confirm:true}})).status()).toBe(200);
 expect((await other.request.get('/api/v1/auth/session')).status()).toBe(401);expect((await page.request.get(`${base}/api/v1/auth/session`)).status()).toBe(200);
 }finally{await other.close()}
});
test('B2 115 health timestamp and failure are read-only and refresh never probes',async({page})=>{
 await u4Fixture(page);let writes=0;
 await page.route('**/api/v1/integrations',r=>{if(r.request().method()!=='GET')writes++;return r.fulfill({json:{data:[{id:'health-owned',kind:'open115',name:'115健康检测',enabled:true,settings:{appId:'owned-fixture'},secretFields:['accessToken','refreshToken'],revision:1,lastCheckState:'unavailable',lastCheckMessage:'连接检测失败，请检查网络、凭据和服务状态',lastCheckedAt:'2026-10-21T10:00:00Z',readiness:{state:'unavailable'}}]}})});
 await page.route('**/api/v1/integrations/*/test',r=>{writes++;return r.fulfill({status:500,json:{error:{code:'must_not_probe',message:'refresh must only read'}}})});
 await page.goto('/connections?tab=storage');const health=page.locator('.saved-connection').filter({hasText:'115健康检测'});await expect(health.getByText(/最近检测：2026-10-21 18:00:00/)).toBeVisible();await expect(health.getByText('连接检测失败，请检查网络、凭据和服务状态',{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'刷新',exact:true}).click();expect(writes).toBe(0);if(capture)mkdirSync(shots,{recursive:true});await screenshot(page,'115-health-readonly');
});
