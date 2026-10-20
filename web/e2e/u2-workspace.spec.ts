import {test,expect,type Page} from '@playwright/test'
import {mkdirSync} from 'node:fs'
const ready={components:{open115:{state:'ready',reason:'fresh_saved_probe',nextAction:'none'},clouddrive2:{state:'ready',reason:'fresh_saved_probe',nextAction:'none'},emby:{state:'ready',reason:'fresh_saved_probe',nextAction:'none'},strm:{state:'configured',reason:'stable_resolver_configured_not_player_verified',nextAction:'verify_player_302'}}}
const connection={id:'fixture-cd2',kind:'clouddrive2',name:'文件服务',enabled:true,revision:2,settings:{endpoint:'http://nas.invalid:19798',cloudRoot:'/115/Media'},secretFields:['token'],lastCheckState:'ready',lastCheckMessage:'',lastCheckedAt:'2026-10-21T00:00:00Z'}
async function fixture(page:Page){
 await page.route('**/api/v1/**',r=>r.fulfill({json:{data:[]}}))
 await page.route('**/api/v1/auth/session',r=>r.fulfill({json:{data:{authenticated:true,account:{id:'u2-admin',username:'fixture'}}}}))
 await page.route('**/api/v1/system/status',r=>r.fulfill({json:{data:{version:'UI mock',environment:'development',dependencies:[],emby302Enabled:true,embyDirectPlayOnly:true,forceProxy:false,proxyFallback:false,legacyPlaybackEnabled:false}}}))
 await page.route('**/api/v1/readiness',r=>r.fulfill({json:{data:ready}}))
 await page.route('**/api/v1/system/runtime',r=>r.fulfill({json:{data:{server:{loadAverage:[.4,.5,.6],memoryUsed:2147483648,memoryTotal:8589934592,diskAvailable:107374182400,diskTotal:214748364800,uptimeSeconds:86400,sampledAt:'2026-10-21T00:00:00Z'},daily:{sites:2,subscriptions:3,downloads:1,media:50,todaySearches:4}}}}))
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[connection,{id:'site',kind:'ptsite',name:'测试站点',enabled:true}, {...connection,id:'emby',kind:'emby',name:'媒体服务',secretFields:['apiKey']},{...connection,id:'tmdb',kind:'tmdb',name:'TMDB',secretFields:['token']}]}}))
 await page.route('**/api/v1/classification',r=>r.fulfill({json:{data:{value:{groups:[{id:'movies',name:'电影',directory:'Movies',mediaType:'movie',enabled:true,rules:[]}]},revision:1}}}))
 await page.route('**/api/v1/jobs/browse?*',r=>r.fulfill({json:{data:{items:[{id:'job',kind:'媒体识别',state:'failed',lastError:'请核对站点连接'}],total:1}}}))
 await page.route('**/api/v1/library/candidates?*',r=>r.fulfill({json:{data:{items:[{id:'candidate',state:'needs_confirmation',cloudState:'present',name:'待识别影片'}],total:1}}}))
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[],total:0}}}))
 await page.route('**/api/v1/ops',r=>r.fulfill({json:{data:{alerts:[]}}}))
 await page.route('**/api/v1/search',r=>r.fulfill({json:{data:[{id:'search',keyword:'近期影视搜索',total:12,state:'succeeded'}]}}))
 await page.route('**/api/v1/cloud/accounts/info',r=>r.fulfill({json:{data:{info:{userName:'测试账号',checkedAt:'2026-10-21T00:00:00Z',space:{total:214748364800,used:107374182400,free:107374182400},memberships:[]},note:'仅使用官方返回的信息'}}}))
 await page.route('**/api/v1/settings/network-proxy',r=>r.fulfill({json:{data:{url:'http://user:private@proxy.invalid:7890',tmdb:false,sites:false,revision:2}}}))
}
test.beforeEach(async({page})=>fixture(page))
test('dashboard keeps configured STRM pending player verification and isolates one inbox failure',async({page})=>{
 await page.route('**/api/v1/jobs/browse?*',r=>r.fulfill({status:503,json:{error:{code:'jobs_unavailable',message:'isolated fixture'}}}))
 await page.goto('/dashboard');await expect(page.getByText('已配置，待播放器验证',{exact:true})).toBeVisible()
 await expect(page.getByRole('article',{name:'失败任务'}).getByRole('alert')).toContainText('数量未知')
 await expect(page.getByRole('article',{name:'待确认识别'})).toContainText('待识别影片')
 const readiness=page.getByRole('region',{name:'服务就绪状态'});await readiness.getByRole('link',{name:/Emby/}).click();await expect(page).toHaveURL(/\/connections\?tab=emby/);await expect(page.getByText('未启用媒体中转',{exact:true})).toBeVisible();await expect(page.getByText('未启用代理回退',{exact:true})).toBeVisible()
})
test('completed setup can be hidden then reopened from settings with guarded local preference',async({page})=>{
 await page.goto('/dashboard');const guide=page.getByRole('region',{name:'首次使用引导'});await guide.getByRole('button',{name:'收起引导',exact:true}).click();await expect(guide).toHaveCount(0);await page.reload();await expect(guide).toHaveCount(0)
 await page.goto('/settings');await page.getByRole('button',{name:'重新打开配置引导',exact:true}).click();await expect(guide).toBeVisible()
})
test('saved connection keys are never revealed and conflict keeps typed input until explicit revision confirmation',async({page})=>{
 let reveals=0,puts=0;await page.route('**/secrets/reveal',r=>{reveals++;return r.abort()})
 await page.route('**/api/v1/integrations/fixture-cd2',r=>{puts++;return r.fulfill({status:409,json:{error:{code:'integration_changed',message:'变化'}}})})
 await page.goto('/connections?tab=storage');await page.locator('.saved-connection').filter({hasText:'文件服务'}).getByRole('button',{name:'编辑',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'编辑连接',exact:true});await expect(dialog.getByLabel('CloudDrive2 API Token',{exact:true})).toHaveValue('');expect(reveals).toBe(0)
 await dialog.getByLabel('连接名称',{exact:true}).fill('我的未提交名称');await dialog.getByLabel('CloudDrive2 API Token',{exact:true}).fill('new-private-input');await dialog.getByRole('button',{name:'加密保存',exact:true}).click()
 await expect(dialog).toContainText('配置已被修改，已重新读取');await expect(dialog.getByLabel('连接名称',{exact:true})).toHaveValue('我的未提交名称');await expect(dialog.getByLabel('CloudDrive2 API Token',{exact:true})).toHaveValue('new-private-input');await expect(dialog.getByRole('button',{name:'加密保存',exact:true})).toBeDisabled();expect(puts).toBe(1)
})
test('202 connection test is only queued and further POST stays locked pending readback',async({page})=>{
 let tests=0;await page.route('**/api/v1/integrations/fixture-cd2/test',r=>{tests++;return r.fulfill({status:202,json:{data:{id:'test-job'}}})})
 await page.goto('/connections?tab=storage');const row=page.locator('.saved-connection').filter({hasText:'文件服务'});await row.getByRole('button',{name:'测试连接',exact:true}).click();await expect(page.locator('.async-result-banner')).toContainText('已排队');await expect(row.getByRole('button',{name:'测试连接',exact:true})).toBeDisabled();expect(tests).toBe(1)
})
test('old Emby and settings auxiliary destinations point to the new connection tabs',async({page})=>{
 for(const [from,tab] of [['/emby','emby'],['/settings?section=tmdb','aux'],['/settings?tab=general&section=proxy','aux'],['/integrations/emby','emby'],['/integrations/tmdb','aux']]){await page.goto(from!);await expect.poll(()=>new URL(page.url()).pathname).toBe('/connections');expect(new URL(page.url()).searchParams.get('tab')).toBe(tab)}
 await expect(page.getByLabel('代理地址',{exact:true})).toHaveValue('');await expect(page.locator('body')).not.toContainText('user:private')
})
test('alert acknowledgement stays explicitly separate from resolving the underlying issue',async({page})=>{
 let ack=0;await page.route('**/api/v1/ops',r=>r.fulfill({json:{data:{alerts:[{code:'disk',state:'active',acknowledged_at:ack?'now':null,message:'空间不足'}]}}}));await page.route('**/api/v1/ops/alerts/disk/ack',r=>{ack++;expect(r.request().postDataJSON()).toEqual({confirm:true});return r.fulfill({json:{data:{acknowledged:true}}})})
 await page.goto('/dashboard');await expect(page.getByText('确认告警不等于问题已解决',{exact:true})).toBeVisible();await page.getByRole('button',{name:'确认告警',exact:true}).click();await expect(page.getByText('空间不足',{exact:true})).toHaveCount(0);expect(ack).toBe(1)
})
test('dashboard and connection screenshots in both themes and widths contain no horizontal overflow',async({page})=>{
 const dir='/tmp/media-tool-u2-screenshots';mkdirSync(dir,{recursive:true});const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 for(const width of [375,1440])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:width===375?900:1000});await page.addInitScript(value=>localStorage.setItem('media-tool:theme-mode:v1',value),theme)
  for(const [name,url] of [['dashboard','/dashboard'],['connections','/connections?tab=storage']]){
   await page.goto(url!);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);await expect(page.locator('main h1')).toBeVisible();if(name==='dashboard')await expect(page.getByText('已配置，待播放器验证',{exact:true})).toBeVisible();else await expect(page.getByRole('heading',{name:'空间与会员',exact:true})).toBeVisible()
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);for(const button of await page.locator(name==='dashboard'?'.dashboard-workspace .el-button':'.connections-workspace .el-button').all()){if(await button.isVisible())expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44)};await page.screenshot({path:`${dir}/${name}-${width}-${theme}.png`,fullPage:true,animations:'disabled'})
  }
 }
 expect(errors).toEqual([])
})
