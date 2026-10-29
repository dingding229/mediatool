import {test,expect,type Page} from '@playwright/test';
import {cacheAlertFixture} from './i19-3-fixture';
async function openAlerts(page:Page){await page.goto('/ops?tab=maintenance');await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'运维告警',exact:true}).click()}
for(const kind of ['tmdb','search'])test(`I19-3 ${kind} preserves default 30, paging, details, time semantics and all links`,async({page})=>{
 const f=await cacheAlertFixture(page,true);await page.goto('/ops?tab=cache&cacheKind='+kind);await expect(page.locator('.ops-cache-row')).toHaveCount(30);expect(f.cacheReads[0]!.get('pageSize')).toBe('30');
 const panel=page.locator('.ops-cache-panel');await expect(panel.getByRole('link',{name:'设置 · 搜索订阅',exact:true})).toHaveAttribute('href','/settings?tab=automation');
 const row=page.locator('.ops-cache-row').first();await expect(row).toContainText(kind==='tmdb'?'小时':'小时');await expect(page.locator('.ops-cache-row').nth(2)).toContainText('无自动到期');await expect(page.locator('.ops-cache-row').nth(3)).toContainText(kind==='tmdb'?'已到期':'历史结果');
 await expect(page.getByRole('link',{name:'查看搜索结果',exact:true})).toHaveCount(kind==='search'?30:0);
 if(kind==='search')await expect(row.getByRole('link',{name:'查看搜索结果',exact:true})).toHaveAttribute('href','/search/results/i193-search-0');
 await row.getByRole('button',{name:'缓存详情',exact:true}).click();const dialog=page.getByRole('dialog',{name:'缓存内容',exact:true});await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:/技术细节与原始记录/}).click();await expect(dialog).toContainText('原始缓存内容');await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(dialog).not.toBeVisible();
 await page.getByRole('button',{name:'下一页',exact:true}).click();await expect(page.locator('.ops-cache-row')).toHaveCount(1);expect(f.cacheReads.at(-1)!.get('page')).toBe('2');expect(f.cacheReads.at(-1)!.get('pageSize')).toBe('30');
 await page.getByRole('button',{name:kind==='tmdb'?'搜索缓存':'TMDB',exact:true}).click();await expect(page.locator('.ops-cache-row')).toHaveCount(30);expect(f.cacheReads.at(-1)!.get('page')).toBe('1');expect(f.writes).toEqual([]);
});
for(const kind of ['tmdb','search'])test(`I19-3 ${kind} compact rows and pagination/notes stay inside the card in both widths`,async({page})=>{
 await cacheAlertFixture(page);await page.goto('/ops?tab=cache&cacheKind='+kind);await expect(page.locator('.ops-cache-row')).toHaveCount(4);
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  const card=await page.locator('.ops-cache-panel').boundingBox();for(const e of await page.locator('.ops-cache-panel .mt-pagination,.ops-cache-ttl').all()){const b=await e.boundingBox();expect(b!.x-card!.x).toBeGreaterThanOrEqual(16);expect(card!.x+card!.width-b!.x-b!.width).toBeGreaterThanOrEqual(16);expect(card!.y+card!.height-b!.y-b!.height).toBeGreaterThanOrEqual(16)}
  const stage=await page.locator('.ops-cache-stage').evaluate(el=>{const b=el.getBoundingClientRect(),last=el.querySelector('.ops-cache-row:last-child')!.getBoundingClientRect();return b.bottom-last.bottom});expect(stage).toBeLessThanOrEqual(1);
  const row=await page.locator('.ops-cache-row').first().boundingBox();if(width===1440)expect(row!.height).toBeLessThanOrEqual(72);
  for(const el of await page.locator('.ops-cache-row .mt-list-actions>*').all()){
   if(width===390)await expect(el).toHaveCSS('height','44px');const b=await el.boundingBox();if(width===390)expect(b!.width).toBeGreaterThanOrEqual(44);expect(b!.x+b!.width).toBeLessThanOrEqual(card!.x+card!.width-16);
  }
 }
});
for(const kind of ['tmdb','search'])for(const whole of [false,true])test(`I19-3 ${kind} ${whole?'whole':'single'} clear keeps original confirmation and scoped request`,async({page})=>{
 const f=await cacheAlertFixture(page);await page.goto('/ops?tab=cache&cacheKind='+kind);await expect(page.locator('.ops-cache-row')).toHaveCount(4);
 await page.getByRole('button',{name:whole?'清理此类缓存':'删除缓存项',exact:true}).first().click();const dialog=page.getByRole('dialog',{name:whole?'清理此类缓存':'删除缓存项',exact:true});await expect(dialog).toContainText(kind==='search'?'保留底层结果、下载记录和媒体':'不删除媒体或其他业务数据');expect(f.writes).toEqual([]);
 const request=page.waitForRequest(r=>!['GET','HEAD'].includes(r.method())&&r.url().includes('/cache/'));await dialog.getByRole('button',{name:'确认清理',exact:true}).click();const sent=await request;await expect.poll(()=>f.writes.length).toBe(1);expect(sent.method()).toBe(whole?'POST':'DELETE');expect(new URL(sent.url()).pathname).toBe(`/api/v1/cache/${kind}/`+(whole?'clear':`i193-${kind}-0`));
 if(whole)expect(sent.postDataJSON()).toEqual({confirm:true});else expect(new URL(sent.url()).searchParams.get('confirm')).toBe('true');await expect(page.getByRole('button',{name:'清理此类缓存',exact:true})).toBeEnabled();expect(f.writes).toHaveLength(1);
});

for(const whole of [false,true])test(`I19-3 unknown ${whole?'whole':'single'} cache clear never replays across readback/reload`,async({page})=>{
 const f=await cacheAlertFixture(page);f.lose();await page.goto('/ops?tab=cache&cacheKind=search');await page.getByRole('button',{name:whole?'清理此类缓存':'删除缓存项',exact:true}).first().click();await page.getByRole('dialog',{name:whole?'清理此类缓存':'删除缓存项',exact:true}).getByRole('button',{name:'确认清理',exact:true}).click();
 await expect(page.getByRole('button',{name:'清理此类缓存',exact:true})).toBeDisabled();await page.getByRole('button',{name:'核对结果',exact:true}).click();await page.reload();await expect(page.getByRole('button',{name:'清理此类缓存',exact:true})).toBeDisabled();expect(f.writes).toHaveLength(1);
 if(!whole){expect(f.detailReads).toContain('i193-search-0');f.removeOriginal('search','i193-search-0');await page.getByRole('button',{name:'核对结果',exact:true}).click();await expect(page.getByRole('button',{name:'清理此类缓存',exact:true})).toBeEnabled();expect(f.writes).toHaveLength(1)}
});
for(const mode of ['empty','failed','rejected']as const)test(`I19-3 cache ${mode} stays explicit without lost content or automatic clearing`,async({page})=>{
 const f=await cacheAlertFixture(page);if(mode==='empty')f.emptyCache();if(mode==='failed')f.failCache();await page.goto('/ops?tab=cache');
 if(mode==='empty')await expect(page.locator('.ops-cache-stage').getByText('暂无记录',{exact:true})).toBeVisible();
 else if(mode==='failed'){await expect(page.getByText('暂时无法读取',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'清理此类缓存',exact:true})).toBeDisabled();await expect(page.getByText('暂无记录',{exact:true})).toHaveCount(0)}
 else{f.reject();await page.getByRole('button',{name:'删除缓存项',exact:true}).first().click();await page.getByRole('dialog',{name:'删除缓存项',exact:true}).getByRole('button',{name:'确认清理',exact:true}).click();await expect(page.getByText('本地模拟：拒绝清理',{exact:true})).toBeVisible();await expect(page.locator('.ops-cache-row')).toHaveCount(4)}
 expect(f.writes).toHaveLength(mode==='rejected'?1:0);await expect(page.locator('.ops-cache-panel .ops-cache-ttl')).toBeVisible();
});

test('I19-3 alerts retain independent active/acknowledged/normal states, row-end confirmation and local paging',async({page})=>{
 const f=await cacheAlertFixture(page,true);await openAlerts(page);await expect(page.locator('.ops-alert-row')).toHaveCount(25);
 const warning=page.getByRole('note').filter({hasText:'确认告警不等于问题已解决'});await expect(warning).toBeVisible();await expect(page.getByRole('link',{name:'查看通知投递记录',exact:true})).toHaveAttribute('href','/connections?tab=notifications');
 const row=page.locator('.ops-alert-row').first(),ack=page.locator('.ops-alert-row').nth(1),normal=page.locator('.ops-alert-row').nth(2);
 await expect(ack).toContainText('告警中');await expect(ack).toContainText('已确认');await expect(ack).not.toContainText('已恢复');await expect(normal).toContainText('正常');await expect(normal.getByRole('button',{name:'确认告警',exact:true})).toHaveCount(0);
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  const button=row.getByRole('button',{name:'确认告警',exact:true});await expect(button).toHaveCSS('height',width===390?'44px':'32px');const r=await row.boundingBox(),b=await button.boundingBox();expect(r!.x+r!.width-b!.x-b!.width).toBeLessThanOrEqual(1);if(width===390)expect(b!.width).toBeGreaterThanOrEqual(44);
 }
 await row.getByRole('button',{name:'确认告警',exact:true}).click();const dialog=page.getByRole('dialog',{name:'确认告警',exact:true});await expect(dialog).toContainText('确认告警不等于问题已解决');expect(f.writes).toEqual([]);await dialog.getByRole('button',{name:'确认继续',exact:true}).click();await expect(row).toContainText('已确认');await expect(row).toContainText('告警中');await expect(row).not.toContainText('已恢复');expect(f.writes).toEqual([{method:'POST',path:'/api/v1/ops/alerts/i193-alert-0/ack',body:{confirm:true}}]);
 const pagination=page.getByRole('navigation',{name:'告警记录分页',exact:true});await pagination.getByRole('button',{name:'下一页',exact:true}).click();await expect(page.locator('.ops-alert-row')).toHaveCount(1);await expect(warning).toBeVisible();await expect(page).toHaveURL(/alertsPage=2/);
});

for(const mode of ['empty','failed','unknown']as const)test(`I19-3 alert ${mode} retains the common warning and original pending protection`,async({page})=>{
 const f=await cacheAlertFixture(page);if(mode==='empty')f.emptyAlerts();if(mode==='failed')f.failOps();if(mode==='unknown')f.lose();await openAlerts(page);
 const warning=page.getByRole('note').filter({hasText:'确认告警不等于问题已解决'});await expect(warning).toBeVisible();
 if(mode==='empty'){await expect(page.getByText('暂无告警记录',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'确认告警',exact:true})).toHaveCount(0)}
 else if(mode==='failed'){await expect(page.getByText('暂时无法读取',{exact:true})).toBeVisible();await expect(page.getByText('暂无告警记录',{exact:true})).toHaveCount(0)}
 else{
  await page.getByRole('button',{name:'确认告警',exact:true}).first().click();await page.getByRole('dialog',{name:'确认告警',exact:true}).getByRole('button',{name:'确认继续',exact:true}).click();await expect(page.getByRole('button',{name:'确认告警',exact:true}).first()).toBeDisabled();await page.getByRole('button',{name:'核对结果',exact:true}).click();await openAlerts(page);await expect(page.getByRole('button',{name:'确认告警',exact:true}).first()).toBeDisabled();expect(f.writes).toHaveLength(1);
 }
 await expect(warning).toBeVisible();if(mode!=='unknown')expect(f.writes).toEqual([]);
});

test('I19-3 cache first load never invents empty data and refresh retains original rows',async({page})=>{
 const f=await cacheAlertFixture(page);f.hold();try{await page.goto('/ops?tab=cache');await expect(page.locator('.ops-cache-stage .state-view.state-loading')).toBeVisible();await expect(page.getByText('暂无记录',{exact:true})).toHaveCount(0)}finally{f.release()}
 await expect(page.locator('.ops-cache-row')).toHaveCount(4);f.lose();await page.getByRole('button',{name:'清理此类缓存',exact:true}).click();await page.getByRole('dialog',{name:'清理此类缓存',exact:true}).getByRole('button',{name:'确认清理',exact:true}).click();await expect(page.getByRole('button',{name:'核对结果',exact:true})).toBeVisible();
 f.hold();try{await page.getByRole('button',{name:'核对结果',exact:true}).click();await expect(page.locator('.ops-cache-row')).toHaveCount(4);await expect(page.locator('.ops-cache-stage .state-view.state-loading')).toHaveCount(0)}finally{f.release()}
 f.failCache();await page.getByRole('button',{name:'核对结果',exact:true}).click();await expect(page.getByText(/刷新失败，保留上次数据/)).toBeVisible();await expect(page.locator('.ops-cache-row')).toHaveCount(4);expect(f.writes).toHaveLength(1);
});
