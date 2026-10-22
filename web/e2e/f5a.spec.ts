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
