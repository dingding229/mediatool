import {test,expect,type Page} from '@playwright/test';
import {maintenancePolicy,policyLogFixture} from './i19-4-fixture';
import {policyFields} from '../src/operations/model';
async function openPolicy(page:Page){await page.goto('/ops?tab=maintenance');await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'维护策略',exact:true}).click();await expect(page.getByRole('button',{name:'保存维护策略',exact:true})).toBeEnabled()}
async function openLogs(page:Page){await page.goto('/ops?tab=logs');await page.getByRole('button',{name:'日志设置',exact:true}).click();return page.getByRole('dialog',{name:'日志设置',exact:true})}

test('I19-4 policy switches/Cron span the row; numeric fields use two/one columns without double borders',async({page})=>{
 const f=await policyLogFixture(page);await openPolicy(page);
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});const grid=await page.locator('.ops-policy-grid').boundingBox();
  for(const e of await page.locator('.ops-policy-grid>.u5-check,.ops-policy-grid>.cron-input').all()){const b=await e.boundingBox();expect(Math.abs(b!.width-grid!.width)).toBeLessThanOrEqual(1)}
  const fields=await page.locator('.ops-policy-grid>label:not(.u5-check)').evaluateAll(es=>es.map(el=>{const b=el.getBoundingClientRect();return {x:b.x,y:b.y,width:b.width}}));
  expect(fields).toHaveLength(policyFields.length);if(width===1440){expect(fields[0]!.y).toBe(fields[1]!.y);expect(fields[0]!.x).toBeLessThan(fields[1]!.x)}else{expect(fields[0]!.x).toBe(fields[1]!.x);expect(fields[0]!.y).toBeLessThan(fields[1]!.y)}
  const input=page.getByRole('textbox',{name:'备份Cron',exact:true});await expect(input).toHaveCSS('border-top-width','0px');const wrapper=input.locator('..');await expect(wrapper).toHaveCSS('min-height','44px');expect(await wrapper.evaluate(e=>getComputedStyle(e).boxShadow)).not.toBe('none');expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
 }
 expect(f.writes).toEqual([]);
});

test('I19-4 every native policy constraint still blocks invalid/empty submit and discard restores the original draft',async({page})=>{
 const f=await policyLogFixture(page);await openPolicy(page);const save=page.getByRole('button',{name:'保存维护策略',exact:true});
 for(const field of policyFields){
  const input=page.getByRole('spinbutton',{name:field.label,exact:true});await expect(input).toHaveAttribute('min',String(field.min));await expect(input).toHaveAttribute('max',String(field.max));await expect(input).toHaveAttribute('step',String(field.step||1));await expect(input).toHaveAttribute('required','');
  const original=await input.inputValue();for(const bad of ['',String(field.min-1),String(field.max+1),String(field.min+(field.step||1)/2)]){await input.fill(bad);expect(await input.evaluate(el=>(el as HTMLInputElement).checkValidity())).toBe(false);await save.click();expect(f.writes).toEqual([])}await input.fill(original);
 }
 await page.getByRole('spinbutton',{name:'备份保留天数',exact:true}).fill('20');await page.getByRole('button',{name:'放弃草稿',exact:true}).click();await expect(page.getByRole('spinbutton',{name:'备份保留天数',exact:true})).toHaveValue('14');expect(f.writes).toEqual([]);
});

test('I19-4 policy saves original JSON/revision, and the shared Cron generator only fills back until save',async({page})=>{
 const f=await policyLogFixture(page);await openPolicy(page);await page.getByRole('button',{name:'备份Cron生成',exact:true}).click();const dialog=page.getByRole('dialog',{name:'备份Cron生成器',exact:true});await dialog.locator('.cron-generator-fields .el-select__wrapper').first().click();await page.getByRole('option',{name:'自定义',exact:true}).click();await dialog.getByRole('textbox',{name:'备份Cron自定义表达式',exact:true}).fill('0 4 * * *');await expect(dialog.getByRole('button',{name:'确定',exact:true})).toBeEnabled();await dialog.getByRole('button',{name:'确定',exact:true}).click();await expect(page.getByRole('textbox',{name:'备份Cron',exact:true})).toHaveValue('0 4 * * *');expect(f.writes).toEqual([]);
 await page.getByRole('button',{name:'保存维护策略',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes[0]).toEqual({path:'/api/v1/ops/policy',body:{revision:7,value:{...maintenancePolicy,backupCron:'0 4 * * *'}}});expect(f.previews.every(p=>p.purpose==='backup')).toBe(true);
});

for(const expression of ['* *','* * * * *','0 0 0 1 1 * 1970'])test(`I19-4 invalid/frequent/ended Cron ${expression} cannot enable policy save`,async({page})=>{
 const f=await policyLogFixture(page);await openPolicy(page);await page.getByRole('textbox',{name:'备份Cron',exact:true}).fill(expression);await expect(page.getByRole('button',{name:'保存维护策略',exact:true})).toBeDisabled();await expect(page.locator('.ops-policy-panel .cron-feedback')).toContainText(expression.includes('1970')?'计划已结束':expression==='* *'?'请输入':'未满足备份最小间隔');expect(f.writes).toEqual([]);
});

test('I19-4 policy conflict preserves draft/revision and disables saving until explicit reconciliation',async({page})=>{
 const f=await policyLogFixture(page);f.conflict();await openPolicy(page);await page.getByRole('spinbutton',{name:'备份保留天数',exact:true}).fill('20');await page.getByRole('button',{name:'保存维护策略',exact:true}).click();await expect(page.getByText('配置已被修改，已重新读取；你的输入已保留。',{exact:true})).toBeVisible();await expect(page.getByRole('spinbutton',{name:'备份保留天数',exact:true})).toHaveValue('20');await expect(page.getByRole('button',{name:'保存维护策略',exact:true})).toBeDisabled();expect(f.writes).toHaveLength(1);
});

test('I19-4 log settings are scoped, two/one-column, file records have three columns and no file actions',async({page})=>{
 const f=await policyLogFixture(page);const dialog=await openLogs(page);await expect(dialog.getByRole('button',{name:'保存日志设置',exact:true})).toBeEnabled();await expect(dialog.locator('.ops-log-file-row')).toHaveCount(2);
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});const fields=await dialog.locator('.ops-log-settings-grid>.u5-field').evaluateAll(es=>es.map(e=>{const b=e.getBoundingClientRect();return {x:b.x,y:b.y}}));
  if(width===1440){expect(fields[0]!.y).toBe(fields[1]!.y);expect(fields[0]!.x).toBeLessThan(fields[1]!.x)}else{expect(fields[0]!.x).toBe(fields[1]!.x);expect(fields[0]!.y).toBeLessThan(fields[1]!.y)}
  const cols=await dialog.locator('.ops-log-file-row').first().evaluate(e=>getComputedStyle(e).gridTemplateColumns.split(' ').length);expect(cols).toBe(width===1440?3:1);
  for(const input of await dialog.locator('.ops-log-settings-grid .el-input__inner').all())await expect(input).toHaveCSS('border-top-width','0px');
  for(const wrapper of await dialog.locator('.ops-log-settings-grid .el-input__wrapper').all())await expect(wrapper).toHaveCSS('min-height','44px');
  expect(await dialog.evaluate(e=>e.scrollWidth<=e.clientWidth)).toBe(true);expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
 }
 await expect(dialog.locator('.ops-log-file-row button,.ops-log-file-row a')).toHaveCount(0);await dialog.getByRole('button',{name:/技术细节与原始记录/}).click();await expect(dialog).toContainText('application-with-a-long-owned-local-fixture-name.1.log');await expect(dialog).toContainText('268435456');await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();expect(f.writes).toEqual([]);
});

for(const enabled of [true,false])test(`I19-4 Debug ${enabled?'enable':'disable'} preserves original bounds, payload, unrelated settings and auto-close rules`,async({page})=>{
 await page.clock.setFixedTime(new Date('2026-10-29T00:00:00Z'));const f=await policyLogFixture(page);if(!enabled)f.activeDebug('2026-10-29T12:00:00Z');const dialog=await openLogs(page);
 const playback=dialog.getByRole('spinbutton',{name:'播放统计保留天数',exact:true}),retention=dialog.getByRole('spinbutton',{name:'日志保留天数',exact:true});await expect(playback).toHaveAttribute('min','1');await expect(playback).toHaveAttribute('max','730');await expect(retention).toHaveAttribute('min','1');await expect(retention).toHaveAttribute('max','90');
 await playback.fill('60');await retention.fill('14');await dialog.getByRole('switch',{name:'Debug 日志',exact:true}).press('Space');
 if(enabled){await dialog.locator('.ops-log-settings-grid .el-select__wrapper').click();await page.getByRole('option',{name:'24 小时',exact:true}).click()}
 await dialog.getByRole('button',{name:'保存日志设置',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);
 expect(f.writes[0]).toEqual({path:'/api/v1/settings/application',body:{revision:9,value:{playbackRetentionDays:60,logging:{retentionDays:14,debugUntil:enabled?'2026-10-30T00:00:00.000Z':'0001-01-01T00:00:00Z'},movieTemplate:'preserve-me',access:{private:'preserved'}}}});
});

test('I19-4 editing retention does not extend an already-active Debug session and cancel never saves',async({page})=>{
 const f=await policyLogFixture(page),until='2099-10-29T12:00:00Z';f.activeDebug(until);let dialog=await openLogs(page);await dialog.getByRole('spinbutton',{name:'日志保留天数',exact:true}).fill('10');await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();expect(f.writes).toEqual([]);await page.reload();await page.getByRole('button',{name:'日志设置',exact:true}).click();dialog=page.getByRole('dialog',{name:'日志设置',exact:true});await dialog.getByRole('spinbutton',{name:'日志保留天数',exact:true}).fill('14');await dialog.getByRole('button',{name:'保存日志设置',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes[0]!.body.value.logging.debugUntil).toBe(until);
});

for(const mode of ['conflict','unknown','settings-error','summary-error','empty']as const)test(`I19-4 log settings ${mode} retains draft/readback/readonly file semantics`,async({page})=>{
 const f=await policyLogFixture(page);if(mode==='settings-error')f.failSettings();if(mode==='summary-error')f.failSummary();if(mode==='empty')f.emptyFiles();const dialog=await openLogs(page);
 if(mode==='settings-error'){await expect(dialog.getByText('本地模拟：日志设置读取失败',{exact:true})).toBeVisible();await expect(dialog.getByRole('button',{name:'保存日志设置',exact:true})).toHaveCount(0)}
 else if(mode==='summary-error'){await expect(dialog.getByText('本地模拟：日志文件读取失败',{exact:true})).toBeVisible();await expect(dialog.getByText('暂无日志文件记录',{exact:true})).toHaveCount(0)}
 else if(mode==='empty')await expect(dialog.getByText('暂无日志文件记录',{exact:true})).toBeVisible();
 else{
  if(mode==='conflict')f.conflict();else f.lose();await dialog.getByRole('spinbutton',{name:'日志保留天数',exact:true}).fill('10');await dialog.getByRole('button',{name:'保存日志设置',exact:true}).click();await expect(dialog.getByRole('button',{name:'保存日志设置',exact:true})).toBeDisabled();await expect(dialog.getByRole('spinbutton',{name:'日志保留天数',exact:true})).toHaveValue('10');await expect(dialog.getByText(/输入已保留|保存结果尚未确认/).first()).toBeVisible();expect(f.writes).toHaveLength(1);
 }
 if(!['conflict','unknown'].includes(mode))expect(f.writes).toEqual([]);
});

test('I19-4 unknown policy receipt survives readback/reload without replaying PUT',async({page})=>{
 const f=await policyLogFixture(page);f.lose();await openPolicy(page);await page.getByRole('spinbutton',{name:'备份保留天数',exact:true}).fill('20');await page.getByRole('button',{name:'保存维护策略',exact:true}).click();await expect(page.getByRole('button',{name:'保存维护策略',exact:true})).toBeDisabled();await page.getByRole('button',{name:'核对结果',exact:true}).click();await page.reload();await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'维护策略',exact:true}).click();await expect(page.getByRole('button',{name:'保存维护策略',exact:true})).toBeDisabled();expect(f.writes).toHaveLength(1);
});
