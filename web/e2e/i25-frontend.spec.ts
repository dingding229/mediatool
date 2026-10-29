import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {maintenanceFixture} from './i19-6-fixture';
import {jobsFixture} from './i19-1-fixture';
import {u6PluginsFixture,plugin} from './u6-fixture';
const root=resolve(import.meta.dirname,'../../scratchpad/i-phase/i25');
const capture=process.env.I25_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
async function siteJobs(page:Page){
 const f=await jobsFixture(page);let accounts=0;
 await page.route('**/api/v1/integrations',r=>{accounts++;return r.fulfill({json:{data:[{id:'site-a',kind:'ptsite',name:'HDSKY',enabled:true}]}})});
 await page.route('**/api/v1/jobs/browse?*',r=>r.fulfill({json:{data:{items:[{id:'job-site',kind:'site.metrics.refresh',payload:{accountId:'site-a'},queue:'default',state:'succeeded',attempts:1,maxAttempts:1,updatedAt:'2026-10-29T00:00:00Z'},{id:'job-unknown',kind:'site.metrics.refresh',payload:{accountId:'missing-account-id'},queue:'default',state:'succeeded',attempts:1,maxAttempts:1,updatedAt:'2026-10-29T00:00:00Z'}],total:26}}}));
 return {f,accountReads:()=>accounts};
}
for(const width of [1440,390])test(`I25 site names use one account read and never IDs (${width})`,async({page})=>{
 const f=await siteJobs(page);await page.setViewportSize({width,height:1000});await page.goto('/ops?tab=jobs');
 await expect(page.locator('.ops-job-name').first()).toHaveText('站点数据刷新 · HDSKY');await expect(page.locator('.ops-job-name').nth(1)).toHaveText('站点数据刷新');await expect(page.locator('.ops-job-name')).not.toContainText(['missing-account-id']);
 await page.getByRole('button',{name:'下一页',exact:true}).click();await expect(page).toHaveURL(/jobsPage=2/);await page.getByRole('button',{name:'刷新',exact:true}).click();expect(f.accountReads()).toBe(1);expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);expect(f.f.writes).toEqual([]);
});
test('I25 recovery hierarchy, matching subsection styles and unchanged confirmation protection',async({page})=>{
 const f=await maintenanceFixture(page);await page.goto('/ops?tab=maintenance');await page.getByRole('button',{name:'恢复与回滚',exact:true}).click();
 await expect(page.getByRole('heading',{name:'受控恢复与回滚',level:2,exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'网页备份恢复',level:3,exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'命令行维护',level:3,exact:true})).toBeVisible();
 const styles=await page.locator('.ops-recovery-subtitle').evaluateAll(es=>es.map(e=>{const s=getComputedStyle(e);return {font:s.fontSize,weight:s.fontWeight,line:s.lineHeight,margin:s.marginBottom}}));expect(styles).toHaveLength(2);expect(styles[0]).toEqual(styles[1]);
 const a=await page.getByRole('heading',{name:'受控恢复与回滚',exact:true}).boundingBox(),b=await page.getByRole('heading',{name:'网页备份恢复',exact:true}).boundingBox();expect(b!.y-a!.y-a!.height).toBeGreaterThanOrEqual(16);
 await page.getByRole('button',{name:'预览网页恢复',exact:true}).first().click();const preview=page.getByRole('dialog',{name:'网页恢复预览',exact:true});await expect(preview).toContainText('115授权将回到备份时的令牌');await preview.getByRole('button',{name:'确认并排队恢复',exact:true}).click();const confirm=page.getByRole('dialog',{name:'确认网页恢复',exact:true});await expect(confirm.getByRole('button',{name:'排队恢复',exact:true})).toBeDisabled();await confirm.getByPlaceholder('请输入上方确认短语').fill('错误短语');await expect(confirm.getByRole('button',{name:'排队恢复',exact:true})).toBeDisabled();expect(f.writes.filter(w=>w.path.endsWith('/execute'))).toEqual([]);
});
test('I25 breadcrumb has two identical arrow separators and only its final level is bold/current',async({page})=>{
 await maintenanceFixture(page);await page.goto('/ops?tab=maintenance');const crumb=page.locator('.workbench-location');await expect(crumb.locator('.location-separator')).toHaveCount(2);await expect(crumb.locator('[aria-current="page"]')).toHaveText('备份与告警');await expect(crumb.locator('strong')).toHaveCount(1);await expect(crumb).not.toContainText('/');await expect(crumb.locator('.location-parent')).toHaveText('运维');
});
test('I25 stale route module reloads the complete target once, then shows existing error instead of looping',async({page})=>{
 await u6PluginsFixture(page);const target='/plugins/'+plugin.id;let documents=0,modules=0;
 page.on('request',r=>{if(r.isNavigationRequest()&&r.frame()===page.mainFrame()&&new URL(r.url()).pathname===target)documents++});
 await page.route(/\/(?:src\/views\/PluginDetailView\.vue|assets\/PluginDetailView-[^/]+\.js)(?:\?.*)?$/,r=>{modules++;return r.abort('failed')});
 await page.goto('/plugins?tab=installed');await page.getByRole('link',{name:'管理插件',exact:true}).click();
 await expect(page).toHaveURL(new RegExp('/plugins/'+plugin.id+'$'));await expect(page.getByText('页面资源加载失败，请刷新页面后重试。',{exact:true})).toBeVisible();expect(documents).toBe(1);expect(modules).toBeGreaterThanOrEqual(2);
 await page.goto('/plugins?tab=installed');await page.getByRole('link',{name:'管理插件',exact:true}).click();await expect(page.getByText('页面资源加载失败，请刷新页面后重试。',{exact:true})).toBeVisible();expect(documents).toBe(1);
});
test('I25 preloadError listener also recovers only once per full current URL',async({page})=>{
 await siteJobs(page);await page.goto('/dashboard?keep=yes#card');await expect(page.getByRole('heading',{name:'首页',exact:true})).toBeVisible();let documents=0;page.on('request',r=>{if(r.isNavigationRequest()&&r.frame()===page.mainFrame())documents++});
 const send=()=>page.evaluate(()=>window.dispatchEvent(Object.assign(new Event('vite:preloadError',{cancelable:true}),{payload:Error('Importing a module script failed')})));
 const reloaded=page.waitForNavigation({waitUntil:'domcontentloaded'});await send().catch(()=>{});await reloaded;await expect.poll(()=>documents).toBe(1);await expect(page.getByRole('heading',{name:'首页',exact:true})).toBeVisible();await send();await expect(page.getByText('页面资源加载失败，请刷新页面后重试。',{exact:true})).toBeVisible();expect(documents).toBe(1);await expect(page).toHaveURL(/keep=yes.*#card$/);
});
test('I25 local DOM has no clipped circle at the reference right-edge region',async({page})=>{
 await siteJobs(page);await page.setViewportSize({width:1440,height:1000});const evidence=[];
 for(const path of ['/dashboard','/ops?tab=jobs']){
  await page.goto(path);await expect(page.getByRole('heading',{name:path==='/dashboard'?'首页':'任务列表',exact:true})).toBeVisible();
  const data=await page.evaluate(()=>{
   const width=innerWidth,candidates=[...document.querySelectorAll('*')].filter(e=>{const b=e.getBoundingClientRect(),s=getComputedStyle(e);return b.width>0&&b.height>0&&b.left<width&&b.right>width&&b.top<600&&b.bottom>300&&['absolute','fixed'].includes(s.position)&&parseFloat(s.borderRadius)>0&&s.visibility!=='hidden'&&s.opacity!=='0'}).map(e=>({tag:e.tagName,class:e.className}));
   return{url:location.pathname+location.search,width,candidates,rightEdgeStacks:[400,450,460,480].map(y=>({y,tags:document.elementsFromPoint(width-1,y).map(e=>({tag:e.tagName,class:typeof e.className==='string'?e.className:''}))}))};
  });expect(data.candidates).toEqual([]);evidence.push(data);
 }mkdirSync(root,{recursive:true});writeFileSync(resolve(root,'edge-dom.json'),JSON.stringify(evidence,null,2));
});
test('I25 explicit local screenshots',async({page})=>{
 test.skip(process.env.I25_SCREENSHOTS!=='1'||!!process.env.CI||!!process.env.GITHUB_ACTIONS,'Local screenshot opt-in only');mkdirSync(resolve(root,'screens'),{recursive:true});await page.setViewportSize({width:1440,height:1000});await page.addInitScript(()=>localStorage.setItem('media-tool:theme-mode:v1','light'));
 await maintenanceFixture(page);await page.goto('/ops?tab=maintenance');await page.getByRole('button',{name:'恢复与回滚',exact:true}).click();await expect(page.getByRole('heading',{name:'网页备份恢复',level:3,exact:true})).toBeVisible();await page.screenshot({path:resolve(root,'screens/after-recovery-1440-light.png'),fullPage:true,animations:'disabled'});
 await page.screenshot({path:resolve(root,'screens/after-breadcrumb-1440-light.png'),clip:{x:0,y:0,width:1440,height:200},animations:'disabled'});
 await siteJobs(page);await page.goto('/ops?tab=jobs');await expect(page.locator('.ops-job-name').first()).toHaveText('站点数据刷新 · HDSKY');await page.screenshot({path:resolve(root,'screens/after-jobs-1440-light.png'),fullPage:true,animations:'disabled'});
 await page.setViewportSize({width:390,height:1000});await page.addInitScript(()=>localStorage.setItem('media-tool:theme-mode:v1','dark'));await page.reload();await expect(page.locator('html')).toHaveAttribute('data-theme','dark');await expect(page.locator('.ops-job-name').first()).toHaveText('站点数据刷新 · HDSKY');await page.screenshot({path:resolve(root,'screens/after-jobs-390-dark.png'),fullPage:true,animations:'disabled'});
});
