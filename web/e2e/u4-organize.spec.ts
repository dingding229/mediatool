import {test,expect} from '@playwright/test'
import {mkdirSync} from 'node:fs'
import {organizeFixture} from './u4-fixture'
test.beforeEach(async({page})=>organizeFixture(page))
test('U4 organize tabs retain exact query values and isolate authorization errors',async({page})=>{
 await page.route('**/api/v1/library/candidates?*',r=>r.fulfill({status:403,json:{error:{code:'unauthorized',message:'需要重新授权'}}}))
 await page.goto('/organize?tab=candidates&keep=owned');await expect(page.getByText('需要重新授权',{exact:true})).toBeVisible()
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'扫描',exact:true}).click();await expect(page).toHaveURL(/tab=scans/);expect(new URL(page.url()).searchParams.get('keep')).toBe('owned');await expect(page.getByRole('heading',{name:'扫描目录',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'查看目录证据',exact:true})).toBeVisible();await page.getByRole('button',{name:'新增定时扫描',exact:true}).click();const switchArea=page.locator('.el-switch').filter({has:page.getByRole('switch',{name:'启用定时扫描',exact:true})});await expect(switchArea).toBeVisible();const target=await switchArea.boundingBox();expect(target!.height).toBeGreaterThanOrEqual(44);expect(target!.width).toBeGreaterThanOrEqual(44)
})
test('U4 scan approval returns 202 queued and cannot replay a pending request',async({page})=>{
 let writes=0;await page.route('**/api/v1/library/scans',r=>{if(r.request().method()==='POST'){writes++;return r.fulfill({status:202,json:{data:{id:'new-scan',state:'pending'}}})}return r.fulfill({json:{data:[]}})})
 await page.goto('/organize?tab=scans');await page.getByRole('button',{name:'预览扫描范围',exact:true}).click();expect(writes).toBe(0);await page.getByRole('button',{name:'确认范围并开始扫描',exact:true}).click();const d=page.getByRole('dialog',{name:'开始目录扫描',exact:true});await expect(d).toContainText('不移动媒体');await d.getByRole('button',{name:'确认开始扫描',exact:true}).click();await expect(page.getByText('扫描已排队，尚未确认完成。',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'预览扫描范围',exact:true})).toBeDisabled();await page.reload();await expect(page.getByRole('button',{name:'预览扫描范围',exact:true})).toBeDisabled();expect(writes).toBe(1)
})
test('U4 candidates and scan screenshots fit both themes at 375 and 1440',async({page})=>{
 const dir='/tmp/media-tool-u4-screenshots';mkdirSync(dir,{recursive:true});const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='warning'&&m.text().includes('Failed to resolve component'))errors.push(m.text())})
 for(const tab of ['candidates','scans'])for(const width of [375,1440])for(const theme of ['light','dark']){await page.setViewportSize({width,height:width===375?950:1000});await page.addInitScript(v=>localStorage.setItem('media-tool:theme-mode:v1',v),theme);await page.goto('/organize?tab='+tab);await expect(page.getByRole('heading',{name:tab==='candidates'?'待确认文件':'扫描目录',exact:true})).toBeVisible();await expect(page.locator('.organize-record:visible').first()).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:`${dir}/organize-${tab}-${width}-${theme}.png`,fullPage:true,animations:'disabled'})}expect(errors).toEqual([])
})
