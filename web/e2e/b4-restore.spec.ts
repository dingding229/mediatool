import {test,expect} from '@playwright/test'
import {mkdirSync} from 'node:fs'
import {resolve} from 'node:path'
import {u6OpsFixture,opsSnapshot} from './u6-fixture'
const backup='11111111-0000-4000-8000-000000000075',id='22222222-0000-4000-8000-000000000075',snapshot='33333333-0000-4000-8000-000000000075',digest='a'.repeat(64)
const receipt={id,backupSchema:75,programSchema:75,backupId:backup,digest,state:'preview',createdAt:'2026-10-22T00:00:00Z',expiresAt:'2099-10-22T00:10:00Z',message:''}
const preview={restore:receipt,confirmText:'恢复 '+backup,includes:['数据库、登记的缺失播放文件'],excludes:['主密钥、115媒体、现存STRM、Emby和CloudDrive2数据'],message:'SHA256、密文与向上迁移校验通过。'}
test.beforeEach(async({page})=>{await u6OpsFixture(page);await page.route('**/api/v1/ops',r=>r.fulfill({json:{data:{...opsSnapshot,backups:[{...opsSnapshot.backups[0],id:backup,digest,manifest:{schema:75}}]}}}));await page.route('**/api/v1/ops/restores',r=>r.fulfill({json:{data:[]}}))})
async function open(page:any){await page.goto('/ops?tab=maintenance');await page.getByRole('button',{name:'恢复与回滚',exact:true}).click();await expect(page.getByRole('heading',{name:'网页备份恢复',exact:true})).toBeVisible()}
test('B4 preview, exact phrase, queue-only result, and rollback use the API',async({page})=>{
 const writes:any[]=[];let state:any[]=[];await page.route('**/api/v1/ops/restores',r=>r.fulfill({json:{data:state}}))
 await page.route('**/api/v1/ops/restores/preview',r=>{writes.push(r.request().postDataJSON());return r.fulfill({json:{data:preview}})})
 await page.route('**/api/v1/ops/restores/'+id+'/execute',r=>{writes.push(r.request().postDataJSON());state=[{...receipt,state:'pending',message:'已排队，尚未恢复'}];return r.fulfill({status:202,json:{data:state[0]}})})
 await open(page);await expect(page.getByText('暂无网页恢复记录',{exact:true})).toBeVisible();await page.getByRole('button',{name:'预览网页恢复',exact:true}).click()
 const drawer=page.getByRole('dialog',{name:'网页恢复预览',exact:true});await expect(drawer).toContainText('主密钥、115媒体');await drawer.getByRole('button',{name:'确认并排队恢复',exact:true}).click()
 const confirm=page.getByRole('dialog',{name:'确认网页恢复',exact:true});await expect(confirm.getByRole('button',{name:'排队恢复',exact:true})).toBeDisabled();await confirm.getByPlaceholder('请输入上方确认短语').fill('错误短语');await expect(confirm.getByRole('button',{name:'排队恢复',exact:true})).toBeDisabled();await confirm.getByPlaceholder('请输入上方确认短语').fill(preview.confirmText)
 await confirm.getByRole('button',{name:'排队恢复',exact:true}).click();await expect(page.getByRole('status')).toContainText('已排队，尚未恢复');await expect(page.getByRole('heading',{name:'恢复成功',exact:true})).toHaveCount(0)
 expect(writes).toEqual([{backupId:backup},{digest,text:preview.confirmText}]);state=[{...receipt,state:'completed',message:'恢复成功',snapshotId:snapshot}]
 await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.getByRole('heading',{name:'恢复成功',exact:true})).toBeVisible();await page.getByRole('button',{name:'预览回滚到恢复前',exact:true}).click();expect(writes.at(-1)).toEqual({backupId:snapshot})
})
test('B4 recent-login failure is not presented as a successful queue',async({page})=>{
 await page.route('**/api/v1/ops/restores/preview',r=>r.fulfill({json:{data:preview}}));await page.route('**/api/v1/ops/restores/'+id+'/execute',r=>r.fulfill({status:403,json:{error:{code:'recent_login_required',message:'恢复需要10分钟内的新近登录，请重新登录后重新预览并确认'}}}))
 await open(page);await page.getByRole('button',{name:'预览网页恢复',exact:true}).click();await page.getByRole('button',{name:'确认并排队恢复',exact:true}).click();const dialog=page.getByRole('dialog',{name:'确认网页恢复',exact:true});await dialog.getByPlaceholder('请输入上方确认短语').fill(preview.confirmText);await dialog.getByRole('button',{name:'排队恢复',exact:true}).click();await expect(page.getByRole('dialog',{name:'网页恢复预览',exact:true}).getByRole('alert')).toBeVisible();await expect(page.getByRole('dialog',{name:'网页恢复预览',exact:true}).getByRole('alert')).toContainText('请重新登录');await expect(page.getByRole('button',{name:'确认并排队恢复',exact:true})).toBeDisabled();await expect(page.getByRole('heading',{name:'已排队',exact:true})).toHaveCount(0)
})
test('B4 unknown is read back without replay; loading, failure and empty are distinct',async({page})=>{
 let checks=0;await page.route('**/api/v1/ops/restores',r=>r.fulfill({json:{data:[{...receipt,state:'unknown',message:'中断后需核对'}]}}));await page.route('**/api/v1/ops/restores/'+id+'/reconcile',r=>{checks++;expect(r.request().postDataJSON()).toEqual({confirm:true});return r.fulfill({json:{data:{...receipt,state:'failed'}}})})
 await open(page);await page.getByRole('button',{name:'核对中断结果',exact:true}).click();expect(checks).toBe(1);await expect(page.getByRole('status')).toContainText('没有再次执行恢复')
 await page.route('**/api/v1/ops/restores',r=>r.fulfill({status:503,json:{error:{message:'恢复记录读取失败'}}}));await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.getByText('恢复记录读取失败',{exact:true})).toBeVisible();await expect(page.getByText('暂无网页恢复记录',{exact:true})).toHaveCount(0)
})
test('B4 restore screenshots at desktop and narrow dark layout',async({page})=>{
 await page.route('**/api/v1/ops/restores/preview',r=>r.fulfill({json:{data:preview}}));const dir=resolve('test-results/b4');mkdirSync(dir,{recursive:true})
 for(const [width,theme] of [[1440,'light'],[375,'dark']] as const){await page.setViewportSize({width,height:950});await page.addInitScript(theme=>localStorage.setItem('media-tool:theme-mode:v1',theme),theme);await open(page);await page.evaluate(()=>window.scrollTo(0,0));await expect.poll(()=>page.evaluate(()=>window.scrollY)).toBe(0);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(dir,`restore-${width}-${theme}.png`),fullPage:true,animations:'disabled'});await page.getByRole('button',{name:'预览网页恢复',exact:true}).click();await expect(page.getByRole('dialog',{name:'网页恢复预览',exact:true})).toBeVisible();await page.getByRole('dialog',{name:'网页恢复预览',exact:true}).screenshot({path:resolve(dir,`preview-${width}-${theme}.png`),animations:'disabled'});await page.getByRole('dialog',{name:'网页恢复预览',exact:true}).getByRole('button',{name:'关闭',exact:true}).click()}
})
test('B4 restore list loading is not rendered as empty',async({page})=>{
 let release!:()=>void;const pending=new Promise<void>(resolve=>{release=resolve})
 await page.route('**/api/v1/ops/restores',async r=>{await pending;await r.fulfill({json:{data:[]}})})
 await open(page);const panel=page.getByRole('region',{name:'网页备份恢复',exact:true})
 await expect(panel.getByText('正在读取…',{exact:true})).toBeVisible();await expect(panel.getByText('暂无网页恢复记录',{exact:true})).toHaveCount(0)
 release();await expect(panel.getByText('暂无网页恢复记录',{exact:true})).toBeVisible()
})
