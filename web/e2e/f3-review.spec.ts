import {test,expect,type Page} from '@playwright/test'
import {mkdirSync} from 'node:fs'
import {resolve} from 'node:path'
import {u6OpsFixture,rawLog} from './u6-fixture'
import {u3Fixture,u3Run} from './u3-fixture'
import {u5SitesFixture,siteAccount} from './u5-fixture'
const dir=resolve(import.meta.dirname,'../../data/private/f4/f3-review/screenshots')
mkdirSync(dir,{recursive:true})
async function shots(page:Page,name:string){
 for(const [mode,width,height]of [['desktop',1440,1000],['narrow',390,844]]as const){
  await page.setViewportSize({width,height});await page.evaluate(()=>window.scrollTo(0,0));await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  await page.screenshot({path:resolve(dir,`${name}-${mode}.png`),fullPage:true,animations:'disabled'})
 }
}
test('F3 review queue has aligned cards, unambiguous empty filters and complete naturally paged rows',async({page})=>{
 await u6OpsFixture(page);let pageNumber='1',withError=false;const rows=Array.from({length:25},(_,i)=>({id:'job-review-'+i,kind:i%2?'system.check':'download.manual.import',queue:'default',state:'succeeded',attempts:1,maxAttempts:3,startedAt:'2026-10-22T00:00:00Z',updatedAt:'2026-10-22T00:00:03Z'}))
 await page.route('**/api/v1/jobs/browse?*',r=>{pageNumber=new URL(r.request().url()).searchParams.get('page')||'1';return r.fulfill({json:{data:{items:rows.map((j,i)=>({...j,lastError:withError&&i===0?'上一轮重试的原始错误':''})),total:50}}})})
 await page.goto('/ops?tab=jobs');await expect(page.locator('.f3-job-row')).toHaveCount(25)
 await expect(page.getByLabel('搜索任务',{exact:true})).toHaveValue('');await expect(page.getByLabel('搜索任务',{exact:true})).toHaveAttribute('placeholder','搜索任务或错误内容');await expect(page.getByLabel('任务类别',{exact:true})).toHaveAttribute('placeholder','按任务类别筛选')
 const cards=await page.locator('.f4-runtime-card').evaluateAll(es=>es.map(e=>{const r=e.getBoundingClientRect();return {y:r.y,height:r.height}}));expect(cards).toHaveLength(4);expect(new Set(cards.map(e=>e.y)).size).toBe(1);expect(new Set(cards.map(e=>e.height)).size).toBe(1)
 expect(await page.locator('.f3-health').evaluate(e=>getComputedStyle(e).overflowY)).toBe('visible');expect(await page.locator('.f4-queue-rows').evaluate(e=>getComputedStyle(e).overflowY)).toBe('visible')
 await page.locator('.f3-health summary').click();await expect(page.getByText(/等待中 · 2 项/)).toBeVisible();await expect(page.locator('.f3-job-row').last()).toContainText('耗时 3 秒')
 const first=await page.locator('.f3-job-row').first().boundingBox();expect(first!.height).toBeLessThan(100)
 withError=true;await page.getByRole('button',{name:'筛选任务',exact:true}).click();await expect(page.locator('.f3-job-row').first().locator('.status-badge')).toHaveAttribute('title','上一轮重试的原始错误');expect((await page.locator('.f3-job-row').first().boundingBox())!.height).toBe(first!.height)
 await page.getByRole('button',{name:'下一页',exact:true}).click();await expect.poll(()=>pageNumber).toBe('2');await page.getByRole('button',{name:'上一页',exact:true}).click();await expect.poll(()=>pageNumber).toBe('1')
 await shots(page,'queue')
})
test('F3 review logs use shared controls, tooltips, compact rows and raw URLs',async({page})=>{
 await u6OpsFixture(page);const queries:URLSearchParams[]=[]
 await page.route('**/api/v1/logs/query?*',r=>{queries.push(new URL(r.request().url()).searchParams);return r.fulfill({json:{data:{items:[{time:'2026-10-22T00:00:00Z',level:'info',module:'httpapi',msg:'后台任务完成',source:{file:'worker.go',line:42}},{...rawLog,traceId:'trace-owned',spanId:'span-owned'}],total:2,modules:['httpapi','tmdb'],before:'2026-10-22T01:00:00Z'}}})})
 await page.goto('/ops?tab=logs');await expect(page.locator('.f3-log-row.u6-log')).toHaveCount(2)
 await expect(page.locator('.f3-log-filters select,.f3-log-filters input[type=datetime-local],.f3-log-footer select')).toHaveCount(0)
 await expect(page.locator('.f3-log-filters .el-date-editor')).toHaveCount(1);await expect(page.getByRole('switch',{name:'清爽模式',exact:true})).not.toBeChecked()
 await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'筛选日志等级',exact:true})}).click();await page.getByRole('option',{name:'WARN',exact:true}).click();await expect.poll(()=>queries.at(-1)?.get('level')).toBe('warn')
 await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'筛选日志模块',exact:true})}).click();await page.getByRole('option',{name:'TMDB元数据',exact:true}).click();await expect.poll(()=>queries.at(-1)?.get('module')).toBe('tmdb')
 await expect(page.locator('.f3-log-message').last()).toContainText('HTTP request url=');await expect(page.locator('.f3-log-message pre').last()).toContainText(rawLog.url);await expect(page.locator('.f3-log-message small')).toContainText('trace-owned / span-owned')
 const single=await page.locator('.f3-log-row.u6-log').first().boundingBox();expect(single!.height).toBeLessThan(32)
 await page.getByRole('button',{name:'刷新日志',exact:true}).hover();await expect(page.getByRole('tooltip',{name:'刷新日志',exact:true})).toBeVisible();await page.getByRole('heading',{name:'程序日志',exact:true}).hover()
 await page.getByText('清爽模式',{exact:true}).click();await expect(page.getByRole('switch',{name:'清爽模式',exact:true})).toBeChecked();await expect(page.locator('.f3-log-message pre')).toHaveCount(0);await page.getByText('清爽模式',{exact:true}).click();await expect(page.getByRole('switch',{name:'清爽模式',exact:true})).not.toBeChecked()
 await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'筛选日志等级',exact:true})}).click();await page.getByRole('option',{name:'全部等级',exact:true}).click();await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'筛选日志模块',exact:true})}).click();await page.getByRole('option',{name:'全部模块',exact:true}).click();await page.getByRole('heading',{name:'程序日志',exact:true}).hover();await expect(page.getByRole('tooltip')).toHaveCount(0);await shots(page,'logs');await page.getByRole('group',{name:'日志时间范围',exact:true}).getByPlaceholder('开始时间',{exact:true}).click();await expect(page.locator('.f4-log-date-panel:visible')).toBeVisible();const popup=(await page.locator('.f4-log-date-panel:visible').boundingBox())!;expect(popup.width).toBeLessThanOrEqual(366);expect(popup.x+popup.width).toBeLessThanOrEqual(391);const panes=await page.locator('.f4-log-date-panel:visible .el-date-range-picker__content').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().toJSON()));expect(panes).toHaveLength(2);expect(panes[1].y).toBeGreaterThanOrEqual(panes[0].y+panes[0].height-1);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(dir,'logs-range-narrow.png'),fullPage:true,animations:'disabled'});await page.keyboard.press('Escape');await expect(page.locator('.f4-log-date-panel:visible')).toBeHidden();await page.setViewportSize({width:1440,height:1000});await page.evaluate(()=>window.scrollTo(0,0));await page.getByRole('group',{name:'日志时间范围',exact:true}).getByPlaceholder('开始时间',{exact:true}).click();await expect(page.locator('.f4-log-date-panel:visible')).toBeVisible();await page.screenshot({path:resolve(dir,'logs-range-desktop.png'),fullPage:true,animations:'disabled'});await page.keyboard.press('Escape')
})
test('F3 review forty sites stay compact with failures first and all sortable headers show icons',async({page})=>{
 await u3Fixture(page);const sites=Array.from({length:40},(_,i)=>({accountId:'site-'+i,name:'示例站点 '+String(i+1).padStart(2,'0'),state:[9,39].includes(i)?'failed':'succeeded',count:i+1,message:[9,39].includes(i)?'测试站点暂时不可用':''}))
 await page.route('**/api/v1/search/run-u3?*',r=>r.fulfill({json:{data:{...u3Run,sites}}}));await page.goto('/search/results/run-u3');await expect(page.locator('.resource-entry')).toHaveCount(1)
 const toggle=page.getByRole('button',{name:/站点查询状态 · 成功/});await expect(toggle).toHaveAttribute('aria-expanded','false');await expect(page.locator('.f4-site-grid')).toBeHidden();await shots(page,'search-results-collapsed');await page.setViewportSize({width:1440,height:1000})
 await expect(page.locator('[role=columnheader] .f4-sort-icon')).toHaveCount(3);await toggle.click();await expect(page.locator('.f4-site-grid article')).toHaveCount(40);await expect(page.locator('.f4-site-grid article').first()).toHaveAttribute('data-state','failed')
 expect((await page.locator('.f4-site-grid').boundingBox())!.height).toBeLessThanOrEqual(240);const sizes=await page.locator('.f4-site-grid article').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().height));expect(Math.max(...sizes)).toBeLessThanOrEqual(40)
 await page.locator('.f4-site-grid article').first().hover();await expect(page.getByRole('tooltip')).toContainText('测试站点暂时不可用');await page.getByRole('heading',{name:/择优选择资源/}).hover();await expect(page.getByRole('tooltip')).toBeHidden()
 const header=await page.getByRole('columnheader',{name:'按文件大小排序',exact:true}).boundingBox(),value=await page.locator('.resource-entry>span').first().boundingBox();expect(header!.x+header!.width).toBeCloseTo(value!.x+value!.width,0)
 await shots(page,'search-results');expect((await page.locator('.f4-site-grid').boundingBox())!.height).toBeLessThanOrEqual(240)
})
for(const count of [1,10])test(`F3 review ${count} site accounts keep density, volume labels and narrow alignment`,async({page})=>{
 await u5SitesFixture(page);const rows=Array.from({length:count},(_,i)=>({...siteAccount,id:'site-review-'+i,name:'家庭站点 '+String(i+1).padStart(2,'0')}))
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:rows}}));await page.route('**/api/v1/sites/metrics',r=>r.fulfill({json:{data:rows.map((a,i)=>({accountId:a.id,name:a.name,enabled:true,state:'succeeded',stale:false,collectedAt:'2026-10-22T00:00:00Z',metrics:{uploaded:(50+i)*1024**3,downloaded:10*1024**3,ratio:5,bonus:1200,seeding:25+i,seedingBytes:80*1024**3}}))}}))
 await page.goto('/sites');await expect(page.locator('.f3-site-card')).toHaveCount(count);await expect(page.locator('.f4-list-count')).toContainText(`共 ${count} 个站点`);await expect(page.locator('.f3-site-card').first()).toContainText('做种体积 80.00 GiB')
 expect(await page.locator('.f4-sites-list').evaluate(e=>getComputedStyle(e).overflowY)).toBe('visible');await shots(page,count===1?'sites':'sites-ten')
})

for(const [environment,label]of [['development','开发环境'],['production','生产环境'],[undefined,'环境未知']]as const)test(`F3 review environment badge reports ${label} without treating unknown as development`,async({page})=>{
 await u6OpsFixture(page);await page.route('**/api/v1/system/status',r=>r.fulfill({json:{data:{version:'UI mock',environment,dependencies:[]}}}));await page.goto('/ops?tab=jobs');await expect(page.locator('.workbench-environment')).toHaveText(label);await expect(page.locator('.workbench-version')).toContainText('UI mock');
})

for(const width of [1440,390])test(`F3 review delayed multi-worker health keeps task list stationary at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:1000});await u6OpsFixture(page)
 let release!:()=>void;const gate=new Promise<void>(r=>release=r)
 await page.route('**/api/v1/system/operations',async r=>{await gate;await r.fulfill({json:{data:{postgres:{state:'ready',latencyMs:2},redis:{state:'ready'},databaseBytes:8388608,workers:Array.from({length:11},(_,i)=>({owner:'owned-worker-'+i,queue:'default',online:i<8,lastSeenAt:'2026-10-22T00:00:00Z'})),queues:[],schedules:[]}}})})
 await page.goto('/ops?tab=jobs');await expect(page.locator('.f4-runtime-grid')).toBeVisible();await expect(page.locator('.f3-queue-list')).toBeVisible()
 const before=await page.locator('.f3-queue-list').boundingBox();release();await expect(page.locator('.f4-runtime-card').nth(2)).toContainText('8.00 MiB')
 const after=await page.locator('.f3-queue-list').boundingBox();expect(after!.y).toBeCloseTo(before!.y,1)
 await expect(page.locator('.f4-runtime-card')).toHaveCount(4)
})
