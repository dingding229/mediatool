import {test,expect,type Page} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {attentionFixture,runtimeSnapshot} from './i19-2-fixture';
import {opsSnapshot} from './u6-fixture';
import {fixtureFulfill} from './collection-fixture';

const next='2026-10-26T03:00:00+08:00';
async function fixture(page:Page,enabled=false,ended=false){
 const f=await attentionFixture(page);
 const nextRunAt=enabled&&!ended?next:null;
 await page.route('**/api/v1/system/operations',r=>r.fulfill({json:{data:{...runtimeSnapshot,schedules:[
  {id:'core',name:'核心协调',cron:'*/15 * * * * *',enabled:true,nextRunAt:'2026-10-29T22:00:15+08:00',link:'/logs'},
  {id:'ops-backup',name:'系统备份',cron:'0 3 * * *',enabled,nextRunAt,link:'/ops?tab=maintenance'},
 ]}}}));
 await page.route('**/api/v1/ops',r=>r.fulfill({json:{data:{...opsSnapshot,alerts:[],schema:93,policy:{...opsSnapshot.policy,value:{...opsSnapshot.policy.value,backupEnabled:enabled},nextBackupAt:nextRunAt}}}}));
 await page.route('**/api/v1/scheduling/cron/preview',r=>fixtureFulfill(r,{}));
 return f;
}
for(const enabled of [false,true])test(`I22 schedules and backup summaries enabled=${enabled}`,async({page})=>{
 const f=await fixture(page,enabled);
 await page.goto('/ops?tab=attention');
 await page.getByText('运行计划与执行器详情',{exact:true}).click();
 const core=page.locator('.ops-schedule-row').filter({has:page.getByRole('heading',{name:'核心协调',exact:true})});
 await expect(core).toContainText('每 15 秒');
 await expect(page.getByText('核心协调（秒 分 时 日 月 周）',{exact:true})).toHaveCount(0);
 const backup=page.locator('.ops-schedule-row').filter({has:page.getByRole('heading',{name:'系统备份',exact:true})});
 if(enabled)await expect(backup).toContainText('下次 2026-10-26');
 else{await expect(backup).toContainText('已停用');await expect(backup).not.toContainText('下次');await expect(backup).not.toContainText('计划已结束');await expect(backup).not.toContainText('2026-10-26')}
 await page.goto('/ops?tab=maintenance');
 const header=page.locator('.ops-panel-header');
 if(enabled)await expect(header).toContainText('下次 2026-10-26');
 else{await expect(header).toContainText('已停用');await expect(header).not.toContainText('下次');await expect(header).not.toContainText('计划已结束')}
 await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'维护策略',exact:true}).click();
 const policy=page.locator('.ops-policy-panel');
 if(enabled)await expect(policy).toContainText('下次备份 2026-10-26');
 else{await expect(policy).toContainText('已停用；关闭计划不删除已有备份');await expect(policy).not.toContainText('计划已结束')}
 expect(f.writes).toEqual([]);
});

test('I22 enabled exhausted backup remains ended, not disabled',async({page})=>{
 await fixture(page,true,true);await page.goto('/ops?tab=attention');await page.getByText('运行计划与执行器详情',{exact:true}).click();
 const row=page.locator('.ops-schedule-row').filter({has:page.getByRole('heading',{name:'系统备份',exact:true})});
 await expect(row).toContainText('计划已结束');await expect(row).not.toContainText('已停用');
 await page.goto('/ops?tab=maintenance');await expect(page.locator('.ops-panel-header')).toContainText('计划已结束');
});

test('I22 local-only 1440 light attention screenshot',async({page})=>{
 test.skip(process.env.I22_SCREENSHOTS!=='1'||!!process.env.CI||!!process.env.GITHUB_ACTIONS,'Explicit local screenshots only');
 await fixture(page);await page.setViewportSize({width:1440,height:1000});
 await page.addInitScript(()=>localStorage.setItem('media-tool:theme-mode:v1','light'));
 await page.goto('/ops?tab=attention');await expect(page.getByText('暂无待处理事项',{exact:true})).toBeVisible();
 await page.getByText('运行计划与执行器详情',{exact:true}).click();
 await expect(page.locator('.ops-schedule-row').filter({has:page.getByRole('heading',{name:'系统备份',exact:true})})).toContainText('已停用');
 await expect(page.locator('html')).toHaveAttribute('data-theme','light');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(1440);
 const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i22/screens');mkdirSync(dir,{recursive:true});
 await page.screenshot({path:resolve(dir,'after-attention-1440-light.png'),fullPage:true,animations:'disabled'});
});
