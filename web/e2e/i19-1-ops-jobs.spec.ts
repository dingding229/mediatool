import {test,expect} from '@playwright/test';
import {jobsFixture} from './i19-1-fixture';

test('I19-1 compact columns, mobile two-layer rows, accessible icons and no page overflow',async({page})=>{
 await jobsFixture(page);await page.goto('/ops?tab=jobs');await expect(page.locator('.f3-job-row')).toHaveCount(4);
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  const rows=await page.locator('.f3-job-row').first().boundingBox();expect(rows!.height).toBeLessThanOrEqual(width===1440?72:112);
  const meta=await page.locator('.ops-job-meta .f3-job-meta').first().evaluate(el=>({overflow:getComputedStyle(el).overflow,whiteSpace:getComputedStyle(el).whiteSpace}));expect(meta).toEqual({overflow:'visible',whiteSpace:'normal'});
  const list=await page.locator('.ops-job-list').evaluate(el=>{const b=el.getBoundingClientRect(),last=el.querySelector('article:last-child')!.getBoundingClientRect();return b.bottom-last.bottom});expect(list).toBeLessThanOrEqual(1);
  if(width===390){
   const header=await page.getByRole('heading',{name:'运维',exact:true}).boundingBox(),refresh=await page.getByRole('button',{name:'刷新',exact:true}).boundingBox();expect(Math.abs(header!.y-refresh!.y)).toBeLessThan(16);
   const inputs=await page.locator('.ops-job-filters>label').evaluateAll(es=>es.map(el=>el.getBoundingClientRect().y));expect(inputs[0]).toBe(inputs[1]);expect(inputs[2]).toBe(inputs[3]);
   for(const button of await page.locator('.ops-job-actions button').all()){await expect(button).toHaveCSS('height','44px');const b=await button.boundingBox();expect(b!.width).toBeGreaterThanOrEqual(44);expect(b!.height).toBeGreaterThanOrEqual(44)}
   await expect(page.locator('.ops-job-name').first()).toBeVisible();await expect(page.locator('.ops-job-meta-queue').first()).toBeVisible();
  }
 }
 await page.getByRole('button',{name:'任务详情',exact:true}).first().click();await expect(page.getByRole('dialog',{name:'任务与运行技术详情',exact:true})).toBeVisible();
});
test('I19-1 filtering and pagination retain existing query fields and default 25',async({page})=>{
 const f=await jobsFixture(page,true);await page.goto('/ops?tab=jobs');await expect(page.locator('.f3-job-row')).toHaveCount(25);expect(f.reads[0]!.get('pageSize')).toBe('25');
 await page.getByRole('button',{name:'下一页',exact:true}).click();await expect.poll(()=>f.reads.some(q=>q.get('page')==='2')).toBe(true);await expect(page.locator('.f3-job-row')).toHaveCount(1);
 await page.locator('.ops-job-filters .el-select').nth(0).click();await page.getByRole('option',{name:'失败',exact:true}).click();await page.locator('.ops-job-filters .el-select').nth(1).click();await page.getByRole('option',{name:'媒体入库',exact:true}).click();
 await page.getByLabel('任务类别',{exact:true}).fill('site.metrics.refresh');await page.getByLabel('搜索任务',{exact:true}).fill('timeout');await page.getByRole('button',{name:'筛选任务',exact:true}).click();
 await expect.poll(()=>f.reads.at(-1)!.get('page')).toBe('1');expect(f.reads.at(-1)!.get('state')).toBe('failed');expect(f.reads.at(-1)!.get('queue')).toBe('library');expect(f.reads.at(-1)!.get('kind')).toBe('site.metrics.refresh');expect(f.reads.at(-1)!.get('q')).toBe('timeout');expect(f.writes).toEqual([]);
});
test('I19-1 initial skeleton is replaced by rows and refresh keeps the same visible data',async({page})=>{
 await page.addInitScript(()=>{(window as any).__i191CLS=0;new PerformanceObserver(list=>{for(const entry of list.getEntries() as any)if(!entry.hadRecentInput)(window as any).__i191CLS+=entry.value}).observe({type:'layout-shift',buffered:true})});
 const f=await jobsFixture(page);f.hold();try{await page.goto('/ops?tab=jobs');await expect(page.getByRole('status',{name:'正在读取任务',exact:true})).toBeVisible();await expect(page.locator('.ops-job-list')).not.toContainText('暂无记录')}finally{f.release()}
 await expect(page.locator('.f3-job-row')).toHaveCount(4);const top=(await page.locator('.f3-job-row').first().boundingBox())!.y;
 f.hold();try{await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.locator('.f3-job-row')).toHaveCount(4);expect((await page.locator('.f3-job-row').first().boundingBox())!.y).toBe(top);await expect(page.locator('.ops-job-loading')).toHaveCount(0)}finally{f.release()}
 expect(f.writes).toEqual([]);expect(await page.evaluate(()=>(window as any).__i191CLS)).toBeLessThan(.1);
});
for(const [verb,label]of [['cancel','取消后台任务'],['retry','核对后重试']]as const)test(`I19-1 ${verb} keeps original read/confirm/revision and pending lock`,async({page})=>{
 const f=await jobsFixture(page);await page.goto('/ops?tab=jobs');const row=page.locator('.f3-job-row').nth(1);
 await row.getByRole('button',{name:'更多',exact:true}).click();await page.getByRole('menuitem',{name:label,exact:true}).click();
 const confirm=page.getByRole('dialog',{name:verb==='cancel'?'取消后台任务':'核对外部状态后重试',exact:true});await expect(confirm).toBeVisible();await expect(confirm).toContainText('不保证撤销已发生的外部操作');expect(f.writes).toEqual([]);
 await confirm.getByRole('button',{name:'确认处理当前任务',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes[0]).toEqual({path:`/api/v1/tasks/jobs/i191-1/${verb}`,body:{confirm:true,revision:7}});
 await expect(row.getByText('请求已受理，尚未确认完成',{exact:true})).toBeVisible();await row.getByRole('button',{name:'更多',exact:true}).click();await expect(page.getByRole('menuitem',{name:label,exact:true})).toBeDisabled();expect(f.writes).toHaveLength(1);
});
test('I19-1 a lost receipt remains unknown and readback never repeats a mutation',async({page})=>{
 const f=await jobsFixture(page);f.lose();await page.setViewportSize({width:390,height:1000});await page.goto('/ops?tab=jobs');const row=page.locator('.f3-job-row').nth(1);
 await row.getByRole('button',{name:'更多',exact:true}).click();await page.getByRole('menuitem',{name:'取消后台任务',exact:true}).click();await page.getByRole('dialog',{name:'取消后台任务',exact:true}).getByRole('button',{name:'确认处理当前任务',exact:true}).click();
 await expect(row.getByText('任务状态或操作未确认',{exact:true})).toBeVisible();await expect(row.getByText(/只表示请求已受理或等待核对/)).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
 await row.getByRole('button',{name:'核对任务状态',exact:true}).click();await expect(row.getByText('保留未知状态，需继续核对',{exact:true})).toBeVisible();expect(f.writes).toHaveLength(1);
});
for(const mode of ['empty','failed']as const)test(`I19-1 ${mode} reads are not confused with loading or a successful empty page`,async({page})=>{
 const f=await jobsFixture(page);if(mode==='empty')f.empty();else f.fail();await page.goto('/ops?tab=jobs');
 await expect(page.locator('.ops-job-loading')).toHaveCount(0);
 if(mode==='empty'){await expect(page.locator('.ops-job-list .state-view.state-empty')).toBeVisible();const b=await page.locator('.ops-job-list').boundingBox();expect(b!.height).toBeLessThan(220)}
 else{await expect(page.getByText('暂时无法读取',{exact:true})).toBeVisible();await expect(page.getByText('暂无记录',{exact:true})).toHaveCount(0)}
 expect(f.writes).toEqual([]);
});
test('I19-1 a failed refresh keeps the original rows instead of inventing an empty result',async({page})=>{
 const f=await jobsFixture(page);await page.goto('/ops?tab=jobs');await expect(page.locator('.f3-job-row')).toHaveCount(4);f.fail();await page.getByRole('button',{name:'刷新',exact:true}).click();
 await expect(page.getByText(/刷新失败，保留上次数据/)).toBeVisible();await expect(page.locator('.f3-job-row')).toHaveCount(4);await expect(page.getByText('暂无记录',{exact:true})).toHaveCount(0);expect(f.writes).toEqual([]);
});
