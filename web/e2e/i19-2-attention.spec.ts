import {test,expect,type Page} from '@playwright/test';
import {attentionFixture} from './i19-2-fixture';
const expand=async(page:Page)=>page.getByText('运行计划与执行器详情',{exact:true}).click();

test('I19-2 compact overview, full-width rows, trailing accessible actions and readonly queues',async({page})=>{
 const f=await attentionFixture(page);await page.goto('/ops?tab=attention');await expect(page.getByText('暂无待处理事项',{exact:true})).toBeVisible();
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});
  const summary=await page.locator('.f5-runtime-checks').boundingBox();expect(summary!.height).toBeLessThan(width===1440?230:320);
  const inbox=await page.locator('.dashboard-inbox').boundingBox();expect(inbox!.height).toBeLessThan(180);
  const service=await page.locator('.service-readiness').evaluate(e=>{const b=e.getBoundingClientRect(),cards=e.querySelector('.readiness-cards')!.getBoundingClientRect();return b.height-cards.height});expect(service).toBeLessThanOrEqual(1);
  await expand(page);await expect(page.locator('.ops-worker-row')).toHaveCount(2);await expect(page.locator('.ops-queue-row')).toHaveCount(2);await expect(page.locator('.ops-schedule-row')).toHaveCount(2);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  const row=await page.locator('.ops-worker-row').first().boundingBox();expect(row!.height).toBeLessThanOrEqual(72);
  const button=page.getByRole('button',{name:'执行器详情',exact:true}).first();await expect(button).toHaveCSS('height',width===390?'44px':'32px');
  const b=await button.boundingBox();expect(row!.x+row!.width-b!.x-b!.width).toBeLessThanOrEqual(1);if(width===390)expect(b!.width).toBeGreaterThanOrEqual(44);
  await expect(page.locator('.ops-queue-row button,.ops-queue-row a')).toHaveCount(0);
  await expect(page.getByRole('link',{name:'管理此计划',exact:true}).nth(0)).toHaveAttribute('href','/ops?tab=maintenance');
  await expect(page.getByRole('link',{name:'管理此计划',exact:true}).nth(1)).toHaveAttribute('href','/settings?tab=automation');
  if(width===390)for(const link of await page.getByRole('link',{name:'管理此计划',exact:true}).all()){const r=await link.boundingBox();expect(r!.width).toBeGreaterThanOrEqual(44);expect(r!.height).toBeGreaterThanOrEqual(44)}
  await expand(page);
 }
 await expand(page);await page.getByRole('button',{name:'执行器详情',exact:true}).first().click();
 const dialog=page.getByRole('dialog',{name:'运行检查详情',exact:true});await expect(dialog).toBeVisible();await dialog.getByText('技术细节与原始记录',{exact:true}).click();await expect(dialog).toContainText('i192-worker-original-identity');
 await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(dialog).not.toBeVisible();await page.getByRole('button',{name:'技术详情',exact:true}).click();await expect(dialog).toBeVisible();
 if(await dialog.locator('.technical-toggle').getAttribute('aria-expanded')==='false')await dialog.locator('.technical-toggle').click();await expect(dialog).toContainText('expiredLeases');expect(f.writes).toEqual([]);
});

test('I19-2 system check still requires original confirmation and pending readback never resubmits',async({page})=>{
 await page.clock.install();const f=await attentionFixture(page);await page.goto('/ops?tab=attention');await expect(page.getByText('暂无待处理事项',{exact:true})).toBeVisible();expect(f.writes).toEqual([]);
 await page.getByRole('button',{name:'运行系统检查',exact:true}).click();const dialog=page.getByRole('dialog',{name:'运行系统检查',exact:true});await expect(dialog).toContainText('只检查当前服务状态，不重放失败业务或修改媒体文件');expect(f.writes).toEqual([]);
 await dialog.getByRole('button',{name:'确认运行检查',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes[0]).toEqual({path:'/api/v1/jobs',body:{kind:'system.check'}});await expect(page.getByRole('button',{name:'运行系统检查',exact:true})).toBeDisabled();
 await expect(dialog).not.toBeVisible();await expect.poll(()=>f.reads.filter(s=>s==='job').length).toBe(1);
 await page.clock.runFor(31000);await expect.poll(()=>f.reads.filter(s=>s==='job').length).toBeGreaterThan(1);expect(f.writes).toHaveLength(1);
 f.finish();await page.clock.runFor(31000);await expect(page.getByRole('button',{name:'运行系统检查',exact:true})).toBeEnabled();expect(f.writes).toHaveLength(1);
});

test('I19-2 unknown check receipt stays locked across polling and reload without a second POST',async({page})=>{
 await page.clock.install();const f=await attentionFixture(page);f.lose();await page.goto('/ops?tab=attention');await page.getByRole('button',{name:'运行系统检查',exact:true}).click();await page.getByRole('dialog',{name:'运行系统检查',exact:true}).getByRole('button',{name:'确认运行检查',exact:true}).click();
 await expect(page.getByRole('button',{name:'运行系统检查',exact:true})).toBeDisabled();await page.clock.runFor(31000);await page.reload();await expect(page.getByRole('button',{name:'运行系统检查',exact:true})).toBeDisabled();await expect(page.getByText('检查请求尚未核验完成，先读取原任务，不重复提交。',{exact:true})).toBeVisible();expect(f.writes).toHaveLength(1);
});

for(const mode of ['empty','items','inbox-error','runtime-error','runtime-empty']as const)test(`I19-2 ${mode} is distinct and preserves existing handling links`,async({page})=>{
 const f=await attentionFixture(page);if(mode==='items')f.nonempty();if(mode==='inbox-error')f.failInbox();if(mode==='runtime-error')f.failHealth();if(mode==='runtime-empty')f.emptyRuntime();
 await page.goto('/ops?tab=attention');
 if(mode==='items'){
  const inbox=page.getByRole('region',{name:'待处理收件箱',exact:true});await expect(inbox).toContainText('内存使用率偏高');await expect(inbox.getByRole('link',{name:/去处理/})).toHaveAttribute('href','/ops?tab=maintenance');await expect(inbox.getByText('暂无待处理事项',{exact:true})).toHaveCount(0);
  const grid=await inbox.locator('.inbox-grid').boundingBox(),group=await inbox.locator('article').boundingBox();expect(Math.abs(grid!.width-group!.width)).toBeLessThanOrEqual(1);
 }
 else if(mode==='inbox-error'){await expect(page.getByText('这一组部分数据暂不可用，数量未知。',{exact:true})).toBeVisible();await expect(page.getByText('暂无待处理事项',{exact:true})).toHaveCount(0)}
 else await expect(page.getByText('暂无待处理事项',{exact:true})).toBeVisible();
 if(mode==='runtime-error')await expect(page.locator('.f5-runtime-checks').getByText('暂时无法读取',{exact:true})).toBeVisible();
 if(mode==='runtime-empty'){await expand(page);for(const title of ['暂无执行器记录','暂无队列记录','暂无运行计划'])await expect(page.getByText(title,{exact:true})).toBeVisible()}
 expect(f.writes).toEqual([]);
});

test('I19-2 initial loading, stale refresh and service failure stay distinguishable',async({page})=>{
 await page.clock.install();const f=await attentionFixture(page);f.hold();try{
  await page.goto('/ops?tab=attention');await expect(page.getByText('正在读取待处理事项…',{exact:true})).toBeVisible();await expect(page.locator('.f5-runtime-checks .f5-first-read')).toBeVisible();await expect(page.getByText('暂无待处理事项',{exact:true})).toHaveCount(0);
 }finally{f.release()}
 await expect(page.getByText('暂无待处理事项',{exact:true})).toBeVisible();await expand(page);await expect(page.locator('.ops-worker-row')).toHaveCount(2);
 f.failHealth();await page.clock.runFor(31000);await expect(page.locator('.f5-runtime-checks').getByText(/刷新失败，保留上次数据/)).toBeVisible();await expect(page.locator('.ops-worker-row')).toHaveCount(2);
 expect(f.writes).toEqual([]);
});
