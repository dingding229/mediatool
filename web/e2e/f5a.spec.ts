import {test,expect,type Page} from '@playwright/test'
import {mkdirSync} from 'node:fs'
import {resolve} from 'node:path'
import {u6OpsFixture} from './u6-fixture'
const dir=resolve(import.meta.dirname,'../../data/private/f5/screenshots');mkdirSync(dir,{recursive:true})
async function shots(page:Page,name:string){for(const [mode,width,height]of [['desktop',1440,1000],['narrow',390,844]]as const){await page.setViewportSize({width,height});await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(dir,`${name}-${mode}.png`),fullPage:true,animations:'disabled'})}}
test('F5a operations starts with tasks; runtime controls belong to attention only',async({page})=>{
 await u6OpsFixture(page);const reads:string[]=[];page.on('request',r=>reads.push(new URL(r.url()).pathname));await page.goto('/ops');await expect(page.getByRole('heading',{name:'任务列表',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'运行系统检查',exact:true})).toHaveCount(0);expect(reads.filter(p=>p.endsWith('/system/operations'))).toHaveLength(0);await shots(page,'tasks')
 await page.goto('/ops?tab=attention');await expect(page.locator('.f5-runtime-summary')).toContainText('执行器在线 1 / 1');await expect(page.getByRole('button',{name:'运行系统检查',exact:true})).toBeVisible();await shots(page,'attention')
})
test('F5a removes data navigation and safely redirects old bookmarks without reading data',async({page})=>{
 await u6OpsFixture(page);const reads:string[]=[];page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/v1/data/'))reads.push(r.url())});await page.goto('/ops?tab=logs');await expect(page.locator('.page-tabs').getByText('数据浏览',{exact:true})).toHaveCount(0);await shots(page,'logs')
 await page.goto('/data');await expect(page).toHaveURL(/tab=jobs/);await expect(page.getByRole('heading',{name:'任务列表',exact:true})).toBeVisible();expect(reads).toEqual([])
})
test('F5a stopped plugin edits 61 seconds without worker calls or JSON and saves all retained values',async({page})=>{
 await u6OpsFixture(page);const id='11111111-1111-4111-8111-111111111111';let saved:any,revision=2;const calls:string[]=[]
 await page.route('**/api/v1/plugins/'+id,r=>r.fulfill({json:{data:{instance:{id,pluginId:'library-refresh-notify',metadata:{name:'入库刷新通知'},desiredState:'disabled',state:'stopped',revision},config:{value:saved||{intervalSeconds:60,provider:'builtin',active:false,retainedKey:'kept'},revision},fingerprint:'f5a-form-fixture'}}}))
 await page.route('**/api/v1/plugins/'+id+'/extensions',r=>r.fulfill({json:{data:{metadata:{descriptors:[]}}}}));await page.route('**/api/v1/plugins/'+id+'/runs?*',r=>r.fulfill({json:{data:[]}}))
 page.on('request',r=>{if(/\/plugins\/.*\/ui\/|\/state-snapshots|\/tools|\/auth\/status/.test(r.url()))calls.push(r.url())})
 await page.route('**/api/v1/plugins/operations',r=>{const input=r.request().postDataJSON();expect(input.kind).toBe('configure');saved=input.config;revision++;return r.fulfill({json:{data:{id,pluginId:'library-refresh-notify',state:'stopped',desiredState:'disabled',revision}}})})
 await page.goto('/plugins/'+id);const interval=page.getByRole('spinbutton',{name:'刷新间隔（秒）',exact:true});await expect(interval).toHaveValue('60');await interval.fill('61');await interval.blur();await page.getByRole('button',{name:'保存插件配置',exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click();await expect.poll(()=>saved?.intervalSeconds).toBe(61);expect(saved.retainedKey).toBe('kept');expect(saved.active).toBe(false);await expect(page.getByRole('dialog',{name:'确认插件操作',exact:true})).toBeHidden();await expect(page.getByText(/配置已在其他页面修改/)).toHaveCount(0);await expect(page.getByRole('switch',{name:'启用插件',exact:true})).not.toBeChecked()
 for(const label of ['插件配置 JSON','实例数据','查看 KV','技术详情'])await expect(page.getByText(label,{exact:true})).toHaveCount(0);expect(calls).toEqual([]);await shots(page,'plugin-detail-stopped')
})
test('F5a plugin switch waits for confirmed backend state and repeated cancel never submits',async({page})=>{
 await u6OpsFixture(page);const id='11111111-1111-4111-8111-111111111111';let writes=0;await page.route('**/api/v1/plugins/'+id,r=>r.fulfill({json:{data:{instance:{id,pluginId:'hello-go',state:'stopped',desiredState:'disabled',revision:2},config:{value:{},revision:2},fingerprint:'fixture'}}}));await page.route('**/api/v1/plugins/operations',r=>{writes++;return r.abort()});await page.goto('/plugins/'+id);const toggle=page.getByRole('switch',{name:'启用插件',exact:true});for(const width of [1440,390,1440]){await page.setViewportSize({width,height:950});await toggle.locator('..').click();const dialog=page.getByRole('dialog',{name:'确认插件操作',exact:true});await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:'取消',exact:true}).click();await expect(dialog).toBeHidden();await expect(toggle).not.toBeChecked()}expect(writes).toBe(0)
})
