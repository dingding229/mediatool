import {test,expect,type Page} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {attentionFixture} from './i19-2-fixture';
import {opsSnapshot} from './u6-fixture';
const at='2026-10-29T13:00:00Z';
const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i23/screens');
const capture=process.env.I23_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
async function fixture(page:Page){
 const base=await attentionFixture(page);
 const alert=(code:string,value:number,threshold:number,unit:string,comparison='>=',state='normal',message='指标已恢复或告警已关闭')=>({code,value,threshold,unit,comparison,state,message,hits:state==='active'?3:0,last_seen_at:at,acknowledged_at:null as string|null});
 const alerts=[alert('disk',25.382095336914062,5,'GiB','<='),alert('memory',95.234,90,'percent','>=','active','宿主内存使用率过高：本地模拟原始说明'),alert('failures',8,3,'jobs_per_10m','>=','active','十分钟内失败任务过多'),alert('load',0.0575,2,'load_per_cpu'),alert('plugin_journal',1488,80000,'entries'),alert('workers',0,1,'queues')];
 alerts[2]!.acknowledged_at=at;
 const jobs=[{id:'old-job',kind:'site.metrics.refresh',payload:{accountId:'hdsky'},lastError:'site collection failed: site request failed; check login, network and compatibility',updatedAt:'2026-10-28T03:00:00+08:00'},{id:'latest-job',kind:'site.metrics.refresh',payload:{accountId:'hdsky'},lastError:'context deadline exceeded',updatedAt:'2026-10-29T03:00:00+08:00'},{id:'middle-job',kind:'site.metrics.refresh',payload:{accountId:'hdsky'},lastError:'site authentication could not be verified',updatedAt:'2026-10-28T12:00:00+08:00'}];
 await page.route('**/api/v1/ops',r=>r.fulfill({json:{data:{...opsSnapshot,alerts,schema:93}}}));
 await page.route('**/api/v1/jobs/browse?*',r=>r.fulfill({json:{data:{items:jobs,total:jobs.length}}}));
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{id:'hdsky',kind:'ptsite',name:'HDSKY',enabled:true}]}}));
 const writes:any[]=[];let lost=false;
 await page.route('**/api/v1/ops/alerts/*/ack',r=>{
  const path=new URL(r.request().url()).pathname;writes.push({method:r.request().method(),path,body:r.request().postDataJSON()});
  if(lost)return r.abort('failed');const row=alerts.find(a=>path.endsWith('/'+a.code+'/ack'));if(row)row.acknowledged_at=at;
  return r.fulfill({json:{data:{acknowledged:true}}});
 });
 return {alerts,jobs,writes,base,lose:()=>lost=true};
}
async function openAlerts(page:Page,count=6){await page.goto('/ops?tab=maintenance');await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'运维告警',exact:true}).click();await expect(page.locator('.ops-alert-row')).toHaveCount(count)}
const row=(page:Page,name:string)=>page.locator('.ops-alert-row').filter({has:page.getByRole('heading',{name,exact:true})});
for(const width of [1440,390])test(`I23 alert metadata and existing badge tones fit ${width}`,async({page})=>{
 const f=await fixture(page);await page.setViewportSize({width,height:1000});await openAlerts(page);
 const disk=row(page,'剩余存储空间');await expect(disk.locator('.ops-alert-value p')).toHaveText(/25.38 GiB/);await expect(disk.locator('.ops-alert-threshold')).toHaveText('阈值 ≤ 5.00 GiB');await expect(disk.locator('.status-badge.good')).toHaveText('正常');await expect(disk).not.toContainText('指标已恢复或告警已关闭');
 const active=row(page,'内存使用率');await expect(active).toContainText('95.2%');await expect(active).toContainText('阈值 ≥ 90.0%');await expect(active.locator('.status-badge.bad')).toHaveText('告警中');await expect(active).toContainText('宿主内存使用率过高：本地模拟原始说明');
 await expect(row(page,'十分钟失败任务').locator('.status-badge.caution')).toHaveText('已确认');await expect(row(page,'十分钟失败任务').getByRole('button',{name:'确认告警',exact:true})).toHaveCount(0);
 await expect(row(page,'插件日志占用')).toContainText('1488 条');await expect(row(page,'业务队列离线')).toContainText('0 个队列');await expect(row(page,'每 CPU 负载')).toContainText('0.06 /CPU');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);expect(f.writes).toEqual([]);
});
test('I23 confirmation retains original warning, exact POST, and active acknowledged badge',async({page})=>{
 const f=await fixture(page);await openAlerts(page);const active=row(page,'内存使用率');await active.getByRole('button',{name:'确认告警',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'确认告警',exact:true});await expect(dialog).toContainText('确认告警不等于问题已解决');expect(f.writes).toEqual([]);await dialog.getByRole('button',{name:'确认继续',exact:true}).click();
 await expect(active.locator('.status-badge.caution')).toHaveText('已确认');await expect(active).toContainText('宿主内存使用率过高：本地模拟原始说明');expect(f.writes).toEqual([{method:'POST',path:'/api/v1/ops/alerts/memory/ack',body:{confirm:true}}]);
});
test('I23 unknown acknowledgement is not replayed',async({page})=>{
 const f=await fixture(page);f.lose();await openAlerts(page);await row(page,'内存使用率').getByRole('button',{name:'确认告警',exact:true}).click();await page.getByRole('dialog',{name:'确认告警',exact:true}).getByRole('button',{name:'确认继续',exact:true}).click();
 await expect(row(page,'内存使用率').getByRole('button',{name:'确认告警',exact:true})).toBeDisabled();await page.reload();await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'运维告警',exact:true}).click();await expect(row(page,'内存使用率').getByRole('button',{name:'确认告警',exact:true})).toBeDisabled();expect(f.writes).toHaveLength(1);
});
test('I23 latest error and total count appear in the existing inbox, with latest source target',async({page})=>{
 await fixture(page);await page.goto('/ops?tab=attention');const group=page.getByRole('article',{name:'失败任务',exact:true});await expect(group.locator('.inbox-item')).toHaveCount(1);await expect(group).toContainText('站点数据更新失败 · HDSKY');await expect(group).toContainText('近期记录 3 次');await expect(group).toContainText('请求超时，请检查网络和服务状态，再核对任务结果。');await expect(group.getByRole('link',{name:/去处理/})).toHaveAttribute('href','/sites?tab=accounts&accountId=hdsky');
});
test('I23 unknown codes and missing metadata preserve raw values without threshold',async({page})=>{
 const f=await fixture(page);f.alerts.push({...f.alerts[0]!,code:'future-rule',value:2.345});f.alerts[3]!.unit='';await openAlerts(page,7);
 const unknown=row(page,'future-rule');await expect(unknown.locator('.ops-alert-value p')).toContainText('2.345');await expect(unknown.locator('.ops-alert-threshold')).toHaveCount(0);
 const missing=row(page,'每 CPU 负载');await expect(missing.locator('.ops-alert-value p')).toContainText('0.0575');await expect(missing.locator('.ops-alert-threshold')).toHaveCount(0);
});
test('I23 local-only requested screenshots',async({page})=>{
 test.skip(!capture,'Explicit local screenshots only');await fixture(page);mkdirSync(dir,{recursive:true});
 for(const [width,theme]of [[1440,'light'],[390,'dark']]as const){await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await openAlerts(page);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);await page.screenshot({path:resolve(dir,`after-alerts-${width}-${theme}.png`),fullPage:true,animations:'disabled'})}
 await page.setViewportSize({width:1440,height:1000});await page.addInitScript(()=>localStorage.setItem('media-tool:theme-mode:v1','light'));await page.goto('/ops?tab=attention');await expect(page.getByRole('article',{name:'失败任务'}).locator('.inbox-item')).toHaveCount(1);await page.screenshot({path:resolve(dir,'after-inbox-1440-light.png'),fullPage:true,animations:'disabled'});
});
