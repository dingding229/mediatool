import {test,expect,type Page} from '@playwright/test';
import {restoreID,restoreDigest,snapshotDigest,webRestoreFixture} from './i19-7-fixture';
async function open(page:Page){await page.goto('/ops?tab=maintenance');await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'恢复与回滚',exact:true}).click();return page.getByRole('region',{name:'网页备份恢复',exact:true})}
async function previewFirst(page:Page){const panel=page.getByRole('region',{name:'网页备份恢复',exact:true});await panel.getByRole('button',{name:'预览网页恢复',exact:true}).click();const preview=page.getByRole('dialog',{name:'网页恢复预览',exact:true});await expect(preview.getByRole('button',{name:'确认并排队恢复',exact:true})).toBeEnabled();return preview}
async function confirmRestore(page:Page,backup=restoreID(1)){
 await page.getByRole('dialog',{name:'网页恢复预览',exact:true}).getByRole('button',{name:'确认并排队恢复',exact:true}).click();const confirm=page.getByRole('dialog',{name:'确认网页恢复',exact:true});await confirm.getByPlaceholder('请输入上方确认短语').fill('恢复 '+backup);return confirm;
}

test('I19-7 original ready filter, backup/snapshot IDs, full digests and labels remain unchanged; initial reads never preview',async({page})=>{
 const f=await webRestoreFixture(page);const panel=await open(page);await expect(panel.locator('.ops-web-backup-row')).toHaveCount(2);await expect(panel.locator('.ops-web-record-row')).toHaveCount(4);expect(f.writes).toEqual([]);
 await expect(panel.locator('.ops-web-backup-row').nth(0)).toContainText(restoreID(1));await expect(panel.locator('.ops-web-backup-row').nth(1)).toContainText(restoreID(2));await expect(panel.locator('.ops-web-backup-row').filter({hasText:restoreID(3)})).toHaveCount(0);
 await expect(panel.locator('.ops-web-backup-row').nth(0)).toContainText(restoreDigest);await expect(panel.locator('.ops-web-backup-row').nth(1)).toContainText(snapshotDigest);await expect(panel.locator('.ops-web-backup-row').nth(1).getByRole('button',{name:'预览回滚到恢复前',exact:true})).toBeVisible();
 await expect(panel.getByRole('note')).toContainText('确认需10分钟内的新近登录；执行前自动保留恢复前快照。');await expect(panel.getByRole('note')).toContainText('账号、115授权、连接、设置、媒体记录、插件配置及KV全部随备份回退');await expect(panel.getByText('包含数据库、清单和登记的受管输出',{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'刷新',exact:true}).click();expect(f.writes).toEqual([]);
});

test('I19-7 desktop full-width rows and mobile 44px actions preserve complete identifiers without overflow',async({page})=>{
 const f=await webRestoreFixture(page,true);const panel=await open(page);await expect(panel.locator('.ops-web-backup-row')).toHaveCount(26);await expect(panel.locator('.ops-web-record-row')).toHaveCount(27);
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});await expect(panel.locator('.ops-web-backup-row button').first()).toHaveCSS('height',width===1440?'32px':'44px');expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  if(width===390)for(const b of await panel.locator('article button').all()){await expect(b).toHaveCSS('height','44px');const box=await b.boundingBox();expect(box!.width).toBeGreaterThanOrEqual(44)}
  for(const proof of await panel.locator('.ops-web-restore-digest').all())await expect(proof).toHaveCSS('text-overflow','clip');
 }
 expect(f.writes).toEqual([]);
});

for(const cancel of ['确认网页恢复','网页恢复预览'])test(`I19-7 closing ${cancel} performs no execute and keeps exact warning/phrase before cancellation`,async({page})=>{
 const f=await webRestoreFixture(page);await open(page);const p=await previewFirst(page);await expect(p).toContainText('管理员账号与密码将回到备份时状态');await expect(p).toContainText('115授权将回到备份时的令牌');
 if(cancel==='确认网页恢复'){const c=await confirmRestore(page);await expect(c).toContainText('全部业务数据回到备份状态；先创建快照，失败自动回滚');await expect(c).toContainText('当前会话失效后请使用备份账号重新登录查看结果');const b=c.getByRole('button',{name:'排队恢复',exact:true});await expect(b).toBeEnabled();await c.getByPlaceholder('请输入上方确认短语').fill('恢复 '+restoreID(1)+' ');await expect(b).toBeDisabled();await c.getByRole('button',{name:'取消',exact:true}).click();await expect(c).not.toBeVisible()}
 else{await p.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(p).not.toBeVisible()}
 expect(f.writes).toEqual([{path:'/api/v1/ops/restores/preview',body:{backupId:restoreID(1)}}]);
});

test('I19-7 queued response is not completion; original digest/exact phrase are sent only after explicit confirmation',async({page})=>{
 const f=await webRestoreFixture(page);const panel=await open(page);await previewFirst(page);const c=await confirmRestore(page);await c.getByPlaceholder('请输入上方确认短语').fill('错误短语');await expect(c.getByRole('button',{name:'排队恢复',exact:true})).toBeDisabled();expect(f.writes).toHaveLength(1);await c.getByPlaceholder('请输入上方确认短语').fill('恢复 '+restoreID(1));await c.getByRole('button',{name:'排队恢复',exact:true}).click();await expect(panel.getByRole('status')).toContainText('已排队，尚未恢复');expect(f.writes).toEqual([{path:'/api/v1/ops/restores/preview',body:{backupId:restoreID(1)}},{path:`/api/v1/ops/restores/${restoreID(900)}/execute`,body:{digest:restoreDigest,text:'恢复 '+restoreID(1)}}]);await expect(panel.locator('.ops-web-record-row').first()).toContainText('已排队');await expect(panel.locator('.ops-web-record-row').first()).not.toContainText('恢复成功');
});

for(const reason of ['expired','recent','permission'])test(`I19-7 ${reason} rejection stays explicit, disables repeat confirmation and never claims success`,async({page})=>{
 const f=await webRestoreFixture(page);f.reject(reason);await open(page);const p=await previewFirst(page),c=await confirmRestore(page);await c.getByRole('button',{name:'排队恢复',exact:true}).click();await expect(p.getByRole('alert')).toContainText(reason==='expired'?'恢复预览已过期':reason==='recent'?'10分钟内的新近登录':'没有恢复权限');await expect(p.getByRole('button',{name:'确认并排队恢复',exact:true})).toBeDisabled();expect(f.writes.filter(w=>w.path.endsWith('/execute'))).toHaveLength(1);await page.reload();expect(f.writes.filter(w=>w.path.endsWith('/execute'))).toHaveLength(1);
});

test('I19-7 unknown receipt persists across reload and explicit reconciliation never replays execute',async({page})=>{
 await page.clock.install();const f=await webRestoreFixture(page);f.lose();await open(page);await previewFirst(page);await (await confirmRestore(page)).getByRole('button',{name:'排队恢复',exact:true}).click();await expect(page.getByRole('dialog',{name:'网页恢复预览',exact:true}).getByRole('button',{name:'确认并排队恢复',exact:true})).toBeDisabled();await expect.poll(()=>f.writes.filter(w=>w.path.endsWith('/execute')).length).toBe(1);
 await page.reload();let panel=page.getByRole('region',{name:'网页备份恢复',exact:true});if(!await panel.isVisible())panel=await open(page);await expect(panel.locator('.ops-web-record-row').first()).toContainText('结果未知，需核对');expect(await page.evaluate(()=>sessionStorage.getItem('media-tool:pending-web-restore'))).toBe(restoreID(900));
 await page.clock.runFor(16000);expect(f.writes.filter(w=>w.path.endsWith('/execute'))).toHaveLength(1);await panel.locator('.ops-web-record-row').first().getByRole('button',{name:'核对中断结果',exact:true}).click();await expect(panel.getByRole('status')).toContainText('没有再次执行恢复');expect(f.writes.at(-1)).toEqual({path:`/api/v1/ops/restores/${restoreID(900)}/reconcile`,body:{confirm:true}});expect(f.writes.filter(w=>w.path.endsWith('/execute'))).toHaveLength(1);
});

for(const state of ['rolled_back','failed'])test(`I19-7 ${state} compensation/failure labels and snapshot preview never imply successful recovery`,async({page})=>{
 const f=await webRestoreFixture(page);const panel=await open(page);await previewFirst(page);await (await confirmRestore(page)).getByRole('button',{name:'排队恢复',exact:true}).click();await expect(panel.getByRole('status')).toContainText('尚未恢复');f.result(state);await page.getByRole('button',{name:'刷新',exact:true}).click();const row=panel.locator('.ops-web-record-row').first();await expect(row).toContainText(state==='rolled_back'?'失败后已自动回滚':'未执行恢复');await expect(row).not.toContainText('恢复成功');await expect(row).toContainText('恢复前快照 '+restoreID(2));
 if(state==='rolled_back'){await row.getByRole('button',{name:'预览回滚到恢复前',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(3);expect(f.writes.at(-1)).toEqual({path:'/api/v1/ops/restores/preview',body:{backupId:restoreID(2)}})}
 else await expect(row.getByRole('button',{name:'预览回滚到恢复前',exact:true})).toHaveCount(0);
 expect(f.writes.filter(w=>w.path.endsWith('/execute'))).toHaveLength(1);
});

test('I19-7 pre-restore backup choice uses the original ID and digest, not another snapshot or a default value',async({page})=>{
 const f=await webRestoreFixture(page);const panel=await open(page);await panel.locator('.ops-web-backup-row').nth(1).getByRole('button',{name:'预览回滚到恢复前',exact:true}).click();const c=await confirmRestore(page,restoreID(2));await c.getByRole('button',{name:'排队恢复',exact:true}).click();expect(f.writes).toEqual([{path:'/api/v1/ops/restores/preview',body:{backupId:restoreID(2)}},{path:`/api/v1/ops/restores/${restoreID(900)}/execute`,body:{digest:snapshotDigest,text:'恢复 '+restoreID(2)}}]);
});

for(const mode of ['empty','records-error','backups-error','preview-error'])test(`I19-7 ${mode} remains separate from loading/success, without accidental preview or execution`,async({page})=>{
 const f=await webRestoreFixture(page);if(mode==='empty')f.empty();if(mode==='records-error')f.failRecords();if(mode==='backups-error')f.failBackups();const panel=await open(page);
 if(mode==='empty'){await expect(panel.getByText('暂无可恢复备份',{exact:true})).toBeVisible();await expect(panel.getByText('暂无网页恢复记录',{exact:true})).toBeVisible();expect(f.writes).toEqual([])}
 if(mode==='records-error'){await expect(panel.getByText('本地模拟：恢复记录读取失败',{exact:true})).toBeVisible();await expect(panel.locator('.ops-web-record-row')).toHaveCount(0);await expect(panel.getByText('暂无网页恢复记录',{exact:true})).toHaveCount(0);expect(f.writes).toEqual([])}
 if(mode==='backups-error'){await expect(panel.locator('.ops-web-backup-row')).toHaveCount(0);await expect(panel.getByRole('button',{name:'预览网页恢复',exact:true})).toHaveCount(0);expect(f.writes).toEqual([])}
 if(mode==='preview-error'){f.failPreview();await panel.getByRole('button',{name:'预览网页恢复',exact:true}).click();const p=page.getByRole('dialog',{name:'网页恢复预览',exact:true});await expect(p).toContainText('备份校验失败，不允许预览');await expect(p.getByRole('button',{name:'确认并排队恢复',exact:true})).toHaveCount(0);expect(f.writes).toHaveLength(1)}
});

test('I19-7 first read is not empty and refresh preserves the existing full record set until readback',async({page})=>{
 const f=await webRestoreFixture(page);f.hold();let panel;try{panel=await open(page);await expect(panel.getByText('正在读取…',{exact:true})).toBeVisible();await expect(panel.getByText('暂无网页恢复记录',{exact:true})).toHaveCount(0)}finally{f.release()}
 await expect(panel!.locator('.ops-web-record-row')).toHaveCount(4);
 f.hold();try{await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(panel!.locator('.ops-web-record-row')).toHaveCount(4);await expect(panel!.getByText('暂无网页恢复记录',{exact:true})).toHaveCount(0)}finally{f.release()}
 expect(f.writes).toEqual([]);
});

test('I19-7 parent CLI empty option and ready-backup choices are unchanged; selecting/clearing does not preview',async({page})=>{
 const f=await webRestoreFixture(page);await open(page);const cli=page.getByRole('region',{name:'命令行维护与维护单',exact:true}),select=cli.locator('.ops-cli-filters .el-select__wrapper').first();
 await select.click();await expect(page.getByRole('option')).toHaveCount(3);await expect(page.getByRole('option').first()).toHaveText('请选择');await page.getByRole('option').nth(1).click();await expect(page.locator('.el-select__popper:visible')).toHaveCount(0);await expect(cli.getByRole('button',{name:'预览恢复',exact:true})).toBeEnabled();expect(f.writes).toEqual([]);
 await select.click();await page.getByRole('option').first().click();await expect(cli.getByRole('button',{name:'预览恢复',exact:true})).toBeDisabled();expect(f.writes).toEqual([]);
});
