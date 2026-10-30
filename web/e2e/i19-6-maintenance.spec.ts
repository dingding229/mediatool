import {test,expect,type Page} from '@playwright/test';
import {maintenanceFixture,ownedID,rollbackRevision,frozenDigest} from './i19-6-fixture';
async function openRecovery(page:Page){await page.goto('/ops?tab=maintenance');await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'恢复与回滚',exact:true}).click();return page.getByRole('region',{name:'命令行维护与维护单',exact:true})}
async function choose(page:Page,index:number,valueIndex=1){await page.locator('.ops-cli-filters .el-select__wrapper').nth(index).click();await page.getByRole('option').nth(valueIndex).click();await expect(page.locator('.el-select__popper:visible')).toHaveCount(0)}
async function confirm(page:Page,title:string){const dialog=page.getByRole('dialog',{name:title,exact:true});await expect(dialog).toBeVisible();return dialog}

test('I19-6 backup/drill lists are compact, actions trailing and 44px on mobile; both pagers stay inside',async({page})=>{
 const f=await maintenanceFixture(page,true);await page.goto('/ops?tab=maintenance');await expect(page.locator('.ops-backup-row')).toHaveCount(25);await expect(page.locator('.ops-drill-row')).toHaveCount(25);
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});await expect(page.locator('.ops-backup-row .mt-list-actions button').first()).toHaveCSS('height',width===390?'44px':'32px');expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  const margins=await page.locator('.ops-backups-panel').evaluate(el=>{const c=el.getBoundingClientRect();return [...el.querySelectorAll('.mt-pagination')].map(e=>{const b=e.getBoundingClientRect();return [b.left-c.left,c.right-b.right,c.bottom-b.bottom]})});for(const values of margins)for(const margin of values)expect(margin).toBeGreaterThanOrEqual(16);
  const row=page.locator('.ops-backup-row').first(),box=await row.boundingBox(),actions=await row.locator('.mt-list-actions').boundingBox();expect(box!.x+box!.width-actions!.x-actions!.width).toBeLessThanOrEqual(1);if(width===1440)expect(box!.height).toBeLessThanOrEqual(72);
  if(width===390)for(const item of await row.locator('.mt-list-actions>*').all()){await expect(item).toHaveCSS('height','44px');const b=await item.boundingBox();expect(b!.width).toBeGreaterThanOrEqual(44);expect(b!.height).toBeGreaterThanOrEqual(44)}
 }
 const backups=page.getByRole('navigation',{name:'系统备份分页',exact:true}),drills=page.getByRole('navigation',{name:'恢复演练分页',exact:true});await backups.getByRole('button',{name:'下一页',exact:true}).click();await expect(page.locator('.ops-backup-row')).toHaveCount(1);await expect(page.locator('.ops-drill-row')).toHaveCount(25);await drills.getByRole('button',{name:'下一页',exact:true}).click();await expect(page.locator('.ops-drill-row')).toHaveCount(1);expect(f.writes).toEqual([]);
});

test('I19-6 fixed/invalid-state backups remain protected, download and business details protected, tools unavailable disable creation and drills',async({page})=>{
 const f=await maintenanceFixture(page);f.noTools();await page.goto('/ops?tab=maintenance');await expect(page.getByRole('button',{name:'创建系统备份',exact:true})).toBeDisabled();for(const b of await page.getByRole('button',{name:'恢复演练',exact:true}).all())await expect(b).toBeDisabled();
 const pinned=page.locator('.ops-backup-row').nth(1);await expect(pinned.getByRole('button',{name:'删除备份',exact:true})).toBeDisabled();await expect(page.locator('.ops-backup-row').nth(4).getByRole('button',{name:'删除备份',exact:true})).toHaveCount(0);await expect(page.locator('.ops-backup-row').first().getByRole('link',{name:'下载备份',exact:true})).toHaveAttribute('href',`/api/v1/ops/backups/${ownedID(1)}/download`);
 await page.getByRole('button',{name:'备份详情',exact:true}).first().click();const d=page.getByRole('dialog',{name:'维护详情',exact:true});await expect(d.getByRole('button',{name:/技术细节与原始记录/})).toHaveCount(0);await expect(d).toContainText(frozenDigest);expect(f.writes).toEqual([]);
});

for(const [kind,title,label,index,path]of [
 ['backup','创建系统备份','创建系统备份',0,'/ops/backups'],['drill','恢复演练','恢复演练',0,`/ops/backups/${ownedID(1)}/drill`],['pin','更改备份固定状态','固定备份',0,`/ops/backups/${ownedID(1)}/pin`],['delete','删除本程序备份','删除备份',0,`/ops/backups/${ownedID(1)}/delete`],['cleanup','清理演练临时库','清理临时库',0,`/ops/drills/${ownedID(101)}/cleanup`]
]as const)test(`I19-6 ${kind} retains original confirmation, warning and exact payload`,async({page})=>{
 const f=await maintenanceFixture(page);await page.goto('/ops?tab=maintenance');await page.getByRole('button',{name:label,exact:true}).nth(index).click();const d=await confirm(page,title);await expect(d.locator('.impact-text')).not.toBeEmpty();if(kind==='delete')await expect(d).toContainText('固定、最近两份成功备份及使用中的备份不能删除');if(kind==='drill')await expect(d).toContainText('不恢复到正式数据库');if(kind==='backup')await expect(d).toContainText('不含主密钥、115原媒体或外部服务的数据卷');expect(f.writes).toEqual([]);
 await d.getByRole('button',{name:'确认继续',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes[0]).toEqual({path:'/api/v1'+path,body:kind==='pin'?{pinned:true,confirm:true}:{confirm:true}});if(['backup','drill'].includes(kind))await expect(page.getByText('维护请求尚未核验完成；请先查看当前结果，不重复执行。',{exact:true})).toBeVisible();
});

for(const lost of [true,false])test(`I19-6 ${lost?'unknown backup receipt':'recent/protected backup rejection'} cannot bypass original safety`,async({page})=>{
 const f=await maintenanceFixture(page);await page.goto('/ops?tab=maintenance');if(lost)f.lose();
 await (lost?page.getByRole('button',{name:'创建系统备份',exact:true}):page.locator('.ops-backup-row').nth(2).getByRole('button',{name:'删除备份',exact:true})).click();await (await confirm(page,lost?'创建系统备份':'删除本程序备份')).getByRole('button',{name:'确认继续',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);
 if(lost){await expect(page.getByRole('button',{name:'创建系统备份',exact:true})).toBeDisabled();await page.getByRole('button',{name:'核对结果',exact:true}).click();await page.reload();await expect(page.getByRole('button',{name:'创建系统备份',exact:true})).toBeDisabled();expect(f.writes).toHaveLength(1)}
 else{await expect(page.getByText('固定、最近两份成功备份或被维护任务引用的备份不可删除',{exact:true})).toBeVisible();await expect(page.locator('.ops-backup-row').nth(2)).toContainText('备份就绪');expect(f.writes).toHaveLength(1)}
});

test('I19-6 CLI selectors retain empty/filtered choices and never request preview on selection; rows/pager stay in card',async({page})=>{
 const f=await maintenanceFixture(page,true);const cli=await openRecovery(page);await expect(cli.locator('.ops-request-row')).toHaveCount(25);await expect(page.getByRole('button',{name:'预览恢复',exact:true})).toBeDisabled();await expect(page.getByRole('button',{name:'预览回滚',exact:true})).toBeDisabled();
 await page.locator('.ops-cli-filters .el-select__wrapper').nth(1).click();await expect(page.getByRole('option')).toHaveCount(2);await page.getByRole('option').nth(1).click();expect(f.writes).toEqual([]);await choose(page,1,0);await expect(page.getByRole('button',{name:'预览回滚',exact:true})).toBeDisabled();
 await choose(page,0);await expect(page.getByRole('button',{name:'预览恢复',exact:true})).toBeEnabled();expect(f.writes).toEqual([]);
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);await expect(cli.locator('select')).toHaveCount(0);if(width===390)for(const button of await cli.locator('.ops-request-row .mt-list-actions button').all())await expect(button).toHaveCSS('height','44px')}
 await cli.getByRole('navigation',{name:'维护单分页',exact:true}).getByRole('button',{name:'下一页',exact:true}).click();await expect(cli.locator('.ops-request-row')).toHaveCount(1);expect(f.writes).toEqual([]);
});

for(const kind of ['restore','rollback']as const)test(`I19-6 ${kind} preview/approval retain frozen digest and exact typed phrase, never execute or unpause`,async({page})=>{
 const f=await maintenanceFixture(page);await openRecovery(page);await choose(page,kind==='restore'?0:1);expect(f.writes).toEqual([]);await page.getByRole('button',{name:kind==='restore'?'预览恢复':'预览回滚',exact:true}).click();const preview=page.getByRole('dialog',{name:'恢复与回滚预览',exact:true});await expect(preview).toContainText('服务器执行前会新建快照并校验备份密文');expect(f.writes).toEqual([{path:'/api/v1/ops/requests/preview',body:{kind,id:kind==='restore'?ownedID(1):rollbackRevision}}]);
 await preview.getByRole('button',{name:'查看预览与执行命令',exact:true}).click();const detail=page.getByRole('dialog',{name:'维护详情',exact:true});await expect(detail.getByRole('button',{name:/技术细节与原始记录/})).toHaveCount(0);await expect(detail).toContainText(frozenDigest);await detail.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(detail).not.toBeVisible();
 await preview.getByRole('button',{name:'审批维护单（不立即执行）',exact:true}).click();const approval=await confirm(page,'确认维护审批'),button=approval.getByRole('button',{name:'审批维护单（不立即执行）',exact:true}),phrase=(kind==='restore'?'恢复备份 ':'回滚版本 ')+(kind==='restore'?ownedID(1):rollbackRevision);
 await expect(button).toBeDisabled();await approval.getByPlaceholder('请输入上方确认短语').fill(phrase+' ');await expect(button).toBeDisabled();expect(f.writes).toHaveLength(1);await approval.getByPlaceholder('请输入上方确认短语').fill(phrase);await expect(button).toBeEnabled();await button.click();await expect.poll(()=>f.writes.length).toBe(2);expect(f.writes[1]).toEqual({path:`/api/v1/ops/requests/${ownedID(902)}/approve`,body:{confirm:true,digest:frozenDigest,text:phrase}});await expect(preview.getByRole('button',{name:'审批维护单（不立即执行）',exact:true})).toBeDisabled();expect(f.writes.some(w=>/execute|resume|verify/.test(w.path))).toBe(false);
});

for(const [index,label,title,suffix]of [[0,'取消维护单','取消维护单','cancel'],[1,'执行后核验','执行后核验','verify']]as const)test(`I19-6 maintenance ${suffix} still requires its own confirmation and exact endpoint`,async({page})=>{
 const f=await maintenanceFixture(page);const cli=await openRecovery(page);await cli.locator('.ops-request-row').nth(index).getByRole('button',{name:label,exact:true}).click();const d=await confirm(page,title);if(suffix==='verify')await expect(d).toContainText('不会再次恢复、不会解除维护');expect(f.writes).toEqual([]);await d.getByRole('button',{name:'确认继续',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes[0]).toEqual({path:`/api/v1/ops/requests/${ownedID(200+index)}/${suffix}`,body:{confirm:true}});expect(f.snapshot.mode.paused).toBe(false);
});

test('I19-6 approved commands stay readonly/copyable, expired approvals do not offer execute commands',async({page})=>{
 const f=await maintenanceFixture(page);await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{value:{writeText:async()=>{throw Error('local clipboard denied')}}}));const cli=await openRecovery(page);
 await cli.locator('.ops-request-row').nth(2).getByRole('button',{name:'维护详情与执行命令',exact:true}).click();let d=page.getByRole('dialog',{name:'维护详情',exact:true});await expect(d.getByRole('textbox',{name:'宿主受控执行命令',exact:true})).toHaveCount(0);await d.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(d).not.toBeVisible();
 await cli.locator('.ops-request-row').first().getByRole('button',{name:'维护详情与执行命令',exact:true}).click();d=page.getByRole('dialog',{name:'维护详情',exact:true});const command='media-toolctl --root /root/docker/mediatool execute '+ownedID(200);await expect(d.getByRole('textbox',{name:'宿主受控执行命令',exact:true})).toHaveValue(command);await expect(d.getByRole('textbox',{name:'宿主受控执行命令',exact:true})).toHaveAttribute('readonly','');await d.getByRole('button',{name:'复制执行命令',exact:true}).click();await expect(d.getByRole('textbox',{name:'待复制执行命令',exact:true})).toHaveValue(command);expect(f.writes).toEqual([]);
});

for(const mode of ['unknown','expired','permission']as const)test(`I19-6 CLI ${mode} response never triggers browser execution or automatic reapproval`,async({page})=>{
 const f=await maintenanceFixture(page);await openRecovery(page);await choose(page,0);await page.getByRole('button',{name:'预览恢复',exact:true}).click();const preview=page.getByRole('dialog',{name:'恢复与回滚预览',exact:true});await preview.getByRole('button',{name:'审批维护单（不立即执行）',exact:true}).click();const approval=await confirm(page,'确认维护审批');await approval.getByPlaceholder('请输入上方确认短语').fill('恢复备份 '+ownedID(1));if(mode==='unknown')f.lose();if(mode==='expired')f.expire();if(mode==='permission')f.deny();await approval.getByRole('button',{name:'审批维护单（不立即执行）',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(2);
 if(mode==='unknown'){await expect(preview.getByRole('button',{name:'审批维护单（不立即执行）',exact:true})).toBeDisabled();await preview.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(preview).not.toBeVisible();await page.getByRole('button',{name:'核对结果',exact:true}).click();await openRecovery(page);await expect(page.getByRole('button',{name:'预览恢复',exact:true})).toBeDisabled()}
 else{await preview.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(page.locator('.ops-panel-header').getByRole('alert')).toContainText(mode==='expired'?'审批已过期，请重新预览':'本地模拟：无维护权限')}
 expect(f.writes).toHaveLength(2);expect(f.writes.some(w=>/execute|resume|verify/.test(w.path))).toBe(false);
});

test('I19-6 unpause keeps its exact phrase and current revision, with no automatic invocation',async({page})=>{
 const f=await maintenanceFixture(page);f.paused();await page.goto('/ops?tab=maintenance');expect(f.writes).toEqual([]);await page.getByRole('button',{name:'核对后解除维护',exact:true}).click();const d=await confirm(page,'解除维护');await d.getByPlaceholder('请输入上方确认短语').fill('解除维护');await expect(d.getByRole('button',{name:'确认继续',exact:true})).toBeDisabled();expect(f.writes).toEqual([]);await d.getByPlaceholder('请输入上方确认短语').fill('解除维护并恢复业务');await d.getByRole('button',{name:'确认继续',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes[0]).toEqual({path:'/api/v1/ops/resume',body:{confirm:true,text:'解除维护并恢复业务',revision:19}});
});

test('I19-6 unchanged web recovery still requires typed phrase and honors the ten-minute recent-login rejection',async({page})=>{
 const f=await maintenanceFixture(page);await openRecovery(page);expect(f.writes).toEqual([]);await page.getByRole('button',{name:'预览网页恢复',exact:true}).first().click();const preview=page.getByRole('dialog',{name:'网页恢复预览',exact:true});await expect(preview).toContainText('管理员账号与密码将回到备份时状态');await preview.getByRole('button',{name:'确认并排队恢复',exact:true}).click();const d=await confirm(page,'确认网页恢复'),button=d.getByRole('button',{name:'排队恢复',exact:true});await expect(d).toContainText('当前会话失效后请使用备份账号重新登录查看结果');await d.getByPlaceholder('请输入上方确认短语').fill('恢复备份');await expect(button).toBeDisabled();expect(f.writes).toHaveLength(1);await d.getByPlaceholder('请输入上方确认短语').fill('恢复备份 '+ownedID(1));await button.click();await expect(preview.getByRole('alert')).toContainText('恢复需要10分钟内的新近登录');await expect(preview.getByRole('button',{name:'确认并排队恢复',exact:true})).toBeDisabled();expect(f.writes[1]).toEqual({path:`/api/v1/ops/restores/${ownedID(903)}/execute`,body:{digest:frozenDigest,text:'恢复备份 '+ownedID(1)}});await page.reload();expect(f.writes).toHaveLength(2);
});

for(const mode of ['empty','failed']as const)test(`I19-6 ${mode} maintenance data remains explicit, never fabricates success or performs actions`,async({page})=>{
 const f=await maintenanceFixture(page);if(mode==='empty')f.empty();else f.failRead();await page.goto('/ops?tab=maintenance');
 if(mode==='empty'){await expect(page.getByText('尚无系统备份',{exact:true})).toBeVisible();await expect(page.getByText('暂无恢复演练记录',{exact:true})).toBeVisible();await openRecovery(page);await expect(page.getByText('暂无维护单',{exact:true})).toBeVisible()}
 else{await expect(page.getByText('暂时无法读取',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'创建系统备份',exact:true})).toBeDisabled();await expect(page.getByText('尚无系统备份',{exact:true})).toHaveCount(0)}expect(f.writes).toEqual([]);
});
