import {fixtureFulfill} from './collection-fixture'
import {test,expect,type Page} from '@playwright/test'
import {mkdirSync,writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {u6OpsFixture,u6PluginsFixture,rawLog} from './u6-fixture'
import {u5SitesFixture,u5PlaybackFixture} from './u5-fixture'
import {u3Fixture,u3Run} from './u3-fixture'
import {u4Fixture,organizeFixture} from './u4-fixture'

const out=process.env.F3_SCREENSHOT_DIR||resolve(import.meta.dirname,'../../data/private/f3/screenshots')
mkdirSync(out,{recursive:true})
async function metrics(page:Page){await page.addInitScript(()=>{(window as any).__f3CLS=0;new PerformanceObserver(list=>{for(const e of list.getEntries() as any)if(!e.hadRecentInput)(window as any).__f3CLS+=e.value}).observe({type:'layout-shift',buffered:true})})}
async function sse(page:Page){await page.addInitScript(()=>{(window as any).EventSource=class{onopen:any;onerror:any;listeners=new Map<string,()=>void>();timer:any;constructor(){setTimeout(()=>this.onopen?.({}),0);this.timer=setInterval(()=>this.listeners.get('tick')?.(),3000)}addEventListener(name:string,cb:()=>void){this.listeners.set(name,cb)}close(){clearInterval(this.timer)}}})}
async function common(page:Page){await page.route('**/api/v1/system/runtime',r=>fixtureFulfill(r,{json:{data:{daily:{sites:3,subscriptions:2,downloads:4,media:18,todaySearches:5},server:{memoryUsed:1000*1024**2,memoryTotal:8*1024**3,diskAvailable:1023*1024**3,diskTotal:2*1024**4,loadAverage:[.2,.3,.4],uptimeSeconds:864000,sampledAt:'2026-10-22T00:00:00Z'}}}}));await page.route('**/api/v1/library/activity?*',r=>fixtureFulfill(r,{json:{data:{items:[],total:0}}}));await page.route('**/api/v1/subscriptions?*',r=>fixtureFulfill(r,{json:{data:[{id:'f3-sub',title:'自有跟进影片',enabled:true,media:{id:42,title:'自有影片',year:'2026',mediaType:'movie'},source:'search',revision:1,rules:{accountIds:[],keyword:'自有影片',include:[],exclude:[],rejectHR:true},lastState:'succeeded',lastMessage:'等待下次跟进'}]}}))}
async function fixture(page:Page,kind:string){
 if(kind==='ops')await u6OpsFixture(page);else if(kind==='sites')await u5SitesFixture(page);else if(kind==='playback')await u5PlaybackFixture(page);else if(kind==='organize')await organizeFixture(page);else if(kind==='plugins')await u6PluginsFixture(page);else if(kind==='acquisition')await u3Fixture(page);else await u4Fixture(page)
 await common(page)
}
async function shot(page:Page,name:string){await page.waitForLoadState('networkidle');await page.screenshot({path:resolve(out,name+'.png'),fullPage:true,animations:'disabled'})}

test('F3 queue SSE has no stacked poll and preserves container, filters, details and scrolling',async({page})=>{
 await metrics(page);await sse(page);await page.clock.install();await fixture(page,'ops')
 let jobs=0,health=0
 await page.route('**/api/v1/jobs/browse?*',r=>{jobs++;return fixtureFulfill(r,{json:{data:{items:Array.from({length:25},(_,i)=>({id:`aaaaaaaa-0000-4000-8000-${String(i).padStart(12,'0')}`,kind:'system.check',queue:'default',state:'succeeded',attempts:1,maxAttempts:3,startedAt:'2026-10-22T00:00:00Z',updatedAt:'2026-10-22T00:00:03Z',createdAt:'2026-10-22T00:00:00Z'})),total:60}}})})
 page.on('request',r=>{if(r.url().endsWith('/system/operations'))health++})
 await page.goto('/ops?tab=jobs');await expect(page.locator('.f3-job-row')).toHaveCount(25)
 await page.getByLabel('搜索任务',{exact:true}).fill('系统检查')
 const area=page.locator('.f3-queue-list');await expect(area.locator('.f3-stable-scroll')).toHaveCount(0);await page.evaluate(()=>window.scrollTo(0,120))
 const before=await area.boundingBox(),scrollBefore=await page.evaluate(()=>window.scrollY)
 for(let i=0;i<8;i++){const n=jobs;await page.clock.runFor(15050);await expect.poll(()=>jobs).toBeGreaterThan(n);await expect(area.locator('.state-loading')).toHaveCount(0)}
 expect(await area.boundingBox()).toEqual(before);expect(await page.evaluate(()=>window.scrollY)).toBe(scrollBefore)
 await expect(page.getByLabel('搜索任务',{exact:true})).toHaveValue('系统检查');await expect(page.locator('.f3-health')).toHaveCount(0)
 expect(jobs).toBeLessThanOrEqual(10);expect(health).toBe(0)
 const cls=await page.evaluate(()=>(window as any).__f3CLS);expect(cls).toBeLessThan(.1)
 writeFileSync(resolve(out,'queue-stability.json'),JSON.stringify({virtualSeconds:120,jobs,health,CLS:cls,before,after:await area.boundingBox(),scrollBefore,scrollAfter:await page.evaluate(()=>window.scrollY)},null,2))
 await page.getByLabel('搜索任务',{exact:true}).fill('');await page.evaluate(()=>window.scrollTo(0,0));
 await shot(page,'queue-desktop');await page.setViewportSize({width:390,height:844});await shot(page,'queue-narrow');
 const n=jobs,h=health;await page.locator('.media-workspace>.page-tabs').getByRole('button',{name:'日志',exact:true}).click();await page.clock.runFor(45000);expect(jobs).toBe(n);expect(health).toBe(h)
})

test('F3 home is overview-only and log table retains original URLs during refresh',async({page})=>{
 await metrics(page);await fixture(page,'ops');const requests:string[]=[];page.on('request',r=>requests.push(new URL(r.url()).pathname))
 await page.goto('/dashboard');await expect(page.getByRole('heading',{name:'服务器状态',exact:true})).toBeVisible()
 await expect(page.getByLabel('待处理收件箱')).toHaveCount(0);await expect(page.getByLabel('首次使用引导')).toHaveCount(0);expect(requests).not.toContain('/api/v1/readiness')
 await shot(page,'dashboard-desktop');await page.setViewportSize({width:390,height:844});await shot(page,'dashboard-narrow')
 await page.setViewportSize({width:1440,height:1000});await page.goto('/ops?tab=logs');await expect(page.locator('.f3-log-row.u6-log')).toHaveCount(1)
 await expect(page.locator('.u6-log pre')).toContainText(rawLog.url);await expect(page.locator('.u6-log pre')).toContainText(rawLog.location)
 const before=await page.locator('.f3-log-table').boundingBox();await page.getByRole('button',{name:'刷新日志',exact:true}).click();await expect(page.locator('.u6-log pre')).toContainText(rawLog.url);expect(await page.locator('.f3-log-table').boundingBox()).toEqual(before)
 await shot(page,'logs-desktop');await page.setViewportSize({width:390,height:844});await shot(page,'logs-narrow')
 await page.getByRole('button',{name:'查看完整链接',exact:true}).click();await expect(page.getByLabel('完整请求URL',{exact:true})).toHaveValue(rawLog.url)
})

test('F3 plain sortable column headings synchronize both directions with the selector and compact site details',async({page})=>{
 await fixture(page,'acquisition');const run={...u3Run,sites:Array.from({length:40},(_,i)=>({accountId:'site-'+i,name:'测试站点 '+i,state:i===0?'failed':'succeeded',count:i===0?0:1,message:i===0?'测试不可用':''}))}
 await page.route('**/api/v1/search/run-u3?*',r=>fixtureFulfill(r,{json:{data:run}}));await page.goto('/search/results/run-u3')
 await expect(page.locator('.resource-entry')).toHaveCount(1);await expect(page.locator('.resource-columns button')).toHaveCount(0)
 const header=page.getByRole('columnheader',{name:'按文件大小排序'});await header.click();await expect(header).toHaveAttribute('aria-sort','descending');await expect(page.locator('.acquire-section-heading .el-select')).toContainText('体积从大到小')
 await page.locator('.acquire-section-heading .el-select__wrapper').click();await page.getByRole('option',{name:'促销优先',exact:true}).click();await expect(page.getByRole('columnheader',{name:/促销/})).toHaveAttribute('aria-sort','ascending')
 await expect(page.locator('.f3-site-details')).toBeHidden();await page.getByRole('button',{name:/站点查询状态 · 成功/}).click();expect((await page.locator('.f3-site-details').boundingBox())!.height).toBeLessThanOrEqual(286)
 await shot(page,'search-results-desktop');await page.setViewportSize({width:390,height:844});await shot(page,'search-results-narrow')
})

test('F3 old manual task requires selected identity and explicit approval before one import handoff',async({page})=>{
 await fixture(page,'acquisition');const id='aaaaaaaa-0000-4000-8000-000000000003';let writes=0
 let task:any={id,title:'海边的故事',state:'completed',processingState:'download_only',progress:1,message:'历史任务不回溯'}
 await page.route('**/api/v1/downloads/manual{,?*}',r=>fixtureFulfill(r,{json:{data:[task]}}))
 await page.route(`**/api/v1/downloads/manual/${id}/import`,r=>{writes++;expect(r.request().postDataJSON()).toEqual({confirm:true,mediaType:'movie',tmdbId:42});task={...task,processingState:'identifying'};return fixtureFulfill(r,{status:202,json:{data:{queued:true}}})})
 await page.goto('/downloads?tab=manual');await page.getByRole('button',{name:'选择身份后入库',exact:true}).click()
 const detail=page.getByRole('dialog',{name:'手动下载详情',exact:true});await expect(detail.getByRole('button',{name:'确认所选身份后入库',exact:true})).toBeEnabled();expect(writes).toBe(0)
 await detail.getByRole('button',{name:'确认所选身份后入库',exact:true}).click();const confirm=page.getByRole('dialog',{name:'原任务后处理',exact:true});await expect(confirm).toContainText('不重新添加下载');await expect(confirm).toContainText('上传校验及入库成功');expect(writes).toBe(0)
 await shot(page,'manual-import-confirm-desktop');await page.setViewportSize({width:390,height:844});await shot(page,'manual-import-confirm-narrow')
 await confirm.getByRole('button',{name:'确认身份并入库',exact:true}).click();expect(writes).toBe(1);await expect(detail.getByText('原任务后处理已排队或回执待核对，尚未证明入库完成。',{exact:true})).toBeVisible();await expect(detail.getByRole('button',{name:'确认所选身份后入库',exact:true})).toBeDisabled()
})

const pages=[
 ['ops','/ops?tab=attention','ops-attention'],['ops','/ops?tab=maintenance','ops-maintenance'],['ops','/ops?tab=cache','ops-cache'],
 ['sites','/sites','sites'],['sites','/sites?tab=templates','site-templates'],['acquisition','/downloads?tab=tasks','downloads'],['acquisition','/downloads?tab=uploads','uploads'],['acquisition','/downloads?tab=manual','manual-downloads'],['acquisition','/search','search'],
 ['base','/subscriptions','subscriptions'],['base','/subscriptions?tab=history&subscription=f3-sub','subscription-history'],['organize','/organize','organize'],['organize','/library','library'],['organize','/files','files'],
 ['playback','/playback?tab=processing','playback-processing'],['playback','/playback?tab=service','playback-service'],['playback','/playback?tab=users','media-users'],['base','/connections?tab=storage','connections'],['base','/connections?tab=notifications','notifications'],
 ['base','/settings?tab=security','account-settings'],['base','/settings?tab=naming','naming-settings'],['plugins','/plugins','plugins'],['plugins','/plugins/11111111-1111-4111-8111-111111111111','plugin-detail']
] as const
for(const[kind,path,name]of pages)test(`F3 desktop and narrow review: ${name}`,async({page})=>{
 await fixture(page,kind)
 await page.route('**/api/v1/downloads/manual{,?*}',r=>fixtureFulfill(r,{json:{data:[{id:'aaaaaaaa-0000-4000-8000-000000000003',title:'自有小影片.2026.mkv',state:'completed',processingState:'needs_confirmation',progress:1,savePath:'/downloads',message:'待人工处理，原文件保留：标题匹配不唯一',syncedAt:'2026-10-22T00:00:00Z'}]}}))
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await page.goto(path);await expect(page.locator('main h1')).toBeVisible();await shot(page,name+'-desktop');await page.setViewportSize({width:390,height:844});await shot(page,name+'-narrow')
 expect(errors).toEqual([]);expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)).toBe(false)
})

test('F3 newly visible libraries default to play without rewriting existing browse-only grants',async({page})=>{
 await u5PlaybackFixture(page);let writes=0;
 const original={id:'u5-user',username:'family',displayName:'家庭成员',enabled:true,mustChangePassword:false,revision:1,grants:[{libraryId:'f:series',canPlay:false,canDownload:false}],maxStreams:2,maxParentalRating:null,expiresAt:null,activeSessions:1};
 await page.route('**/api/v1/media-server/users{,?*}',r=>{if(r.request().method()!=='GET')writes++;return fixtureFulfill(r,{json:{data:[original]}})});
 await page.route('**/api/v1/media-server/libraries',r=>fixtureFulfill(r,{json:{data:[{id:'f:series',name:'已有剧集库',mediaType:'series'},{id:'g:movie',name:'新增电影库',mediaType:'movie'}]}}));
 await page.goto('/playback?tab=users');await page.getByRole('button',{name:'编辑用户',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'编辑媒体用户',exact:true});
 await expect(dialog.getByLabel('已有剧集库 可见',{exact:true})).toBeChecked();
 await expect(dialog.getByLabel('已有剧集库 可播放',{exact:true})).not.toBeChecked();
 await expect(dialog.getByText('当前仅可浏览；播放请求将被拒绝。开启“可播放”后才能观看。',{exact:true})).toBeVisible();
 await dialog.getByLabel('新增电影库 可见',{exact:true}).check();
 await expect(dialog.getByLabel('新增电影库 可播放',{exact:true})).toBeChecked();
 await expect(dialog.getByLabel('新增电影库 可下载',{exact:true})).not.toBeChecked();
 await expect(dialog.getByLabel('已有剧集库 可播放',{exact:true})).not.toBeChecked();
 await page.setViewportSize({width:1440,height:1000});await shot(page,'media-user-permissions-desktop');
 await page.setViewportSize({width:390,height:844});await dialog.getByLabel('新增电影库 可播放',{exact:true}).scrollIntoViewIfNeeded();await shot(page,'media-user-permissions-narrow');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)).toBe(false);expect(writes).toBe(0);
 await dialog.getByRole('button',{name:'取消',exact:true}).click();
 expect(original.grants).toEqual([{libraryId:'f:series',canPlay:false,canDownload:false}]);
})
