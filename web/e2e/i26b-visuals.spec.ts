import {test,expect,type Page} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {i26aFixture} from './i26a-fixture';
import {policyLogFixture} from './i19-4-fixture';
import {u7Fixture} from './u7-fixture';
import {u5PlaybackFixture} from './u5-fixture';
import {i26bPlaybackFixture,stableLayout} from './i26b-fixture';
const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i26b');
const capture=process.env.I26B_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
async function init(page:Page,width:number,theme='light'){
 await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
}
async function screenshot(page:Page,name:string,fullPage=true){if(capture){mkdirSync(output,{recursive:true});await page.screenshot({path:resolve(output,name+'.png'),fullPage,animations:'disabled'})}}
async function noOverflow(page:Page){expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(page.viewportSize()!.width)}
test('I26b homepage server is 2x2 on mobile, populated search card shrinks and empty state stays reserved',async({page})=>{
 await i26aFixture(page);await init(page,390);await page.goto('/dashboard');await expect(page.locator('.dashboard-searches>a')).toHaveCount(1);await stableLayout(page);
 const cells=await page.locator('.dashboard-server dl>div').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().toJSON()));
 expect(cells[0]!.y).toBe(cells[1]!.y);expect(cells[2]!.y).toBe(cells[3]!.y);expect(cells[2]!.y).toBeGreaterThan(cells[0]!.y);expect(cells[0]!.x).toBe(cells[2]!.x);
 for(const field of await page.locator('.dashboard-server dd').all())expect(await field.evaluate(e=>e.scrollWidth<=e.clientWidth)).toBe(true);
 const box=await page.locator('.dashboard-searches').boundingBox();expect(box!.height).toBeLessThan(200);await noOverflow(page);await screenshot(page,'after-dashboard-390-light');
 await page.setViewportSize({width:1440,height:1000});await stableLayout(page);expect(new Set(await page.locator('.dashboard-server dl>div').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().y))).size).toBe(1);
 await page.route('**/api/v1/search',r=>r.fulfill({json:{data:[]}}));await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.getByText('暂无搜索记录，前往搜索发现作品。',{exact:true})).toBeVisible();await expect(page.locator('.dashboard-searches')).not.toHaveClass(/has-searches/);expect((await page.locator('.dashboard-searches').boundingBox())!.height).toBeGreaterThanOrEqual(260);
});
test('I26b log preference matches Debug switch without changing application settings payload; viewport is fully covered',async({page})=>{
 const f=await policyLogFixture(page);await init(page,390);await page.goto('/ops?tab=logs');await page.getByRole('button',{name:'日志设置',exact:true}).click();const dialog=page.getByRole('dialog',{name:'日志设置',exact:true});await expect(dialog).toBeVisible();await stableLayout(page);
 const friendly=dialog.getByRole('switch',{name:'友好说明',exact:true});await expect(friendly).toBeChecked();await dialog.locator('.ops-log-friendly .el-switch').click();await expect(friendly).not.toBeChecked();await expect(dialog.getByRole('switch',{name:'Debug 日志',exact:true})).not.toBeChecked();
 const preference=await dialog.locator('.ops-log-friendly').boundingBox(),other=await dialog.locator('.ops-log-settings-grid').boundingBox();expect(preference!.x).toBe(other!.x);
 const rect=await dialog.boundingBox();expect(rect!.y).toBe(0);expect(rect!.height).toBe(1000);await noOverflow(page);await screenshot(page,'after-log-settings-390-light',false);
 await dialog.getByRole('spinbutton',{name:'日志保留天数',exact:true}).fill('8');
 const response=page.waitForResponse(r=>r.url().endsWith('/settings/application')&&r.request().method()==='PUT');await dialog.getByRole('button',{name:'保存日志设置',exact:true}).click();const sent=(await response).request().postDataJSON();expect(sent).toEqual({revision:9,value:{playbackRetentionDays:30,logging:{retentionDays:8,debugUntil:'0001-01-01T00:00:00Z'},movieTemplate:'preserve-me',access:{private:'preserved'}}});expect(f.writes).toHaveLength(1);
});
test('I26b proxy checks center their input and text; no settings are saved',async({page})=>{
 await u7Fixture(page,'/connections');await page.route('**/api/v1/settings/network-proxy',r=>r.fulfill({json:{data:{revision:1,protocol:'http',host:'proxy.invalid',port:8080,username:'local',configured:true,hasPassword:true,needsInput:false,tmdb:false,sites:false}}}));await init(page,390);await page.goto('/connections?tab=aux');await expect(page.locator('.proxy-options .el-checkbox')).toHaveCount(4);await stableLayout(page);
 const rows=await page.locator('.proxy-options .el-checkbox').evaluateAll(es=>es.map(e=>{const a=e.querySelector('.el-checkbox__input')!.getBoundingClientRect(),b=e.querySelector('.el-checkbox__label')!.getBoundingClientRect();return Math.abs((a.top+a.bottom)/2-(b.top+b.bottom)/2)}));for(const delta of rows)expect(delta).toBeLessThanOrEqual(1);await noOverflow(page);
});
test('I26b enabled user field is centered and keeps standard form spacing and original payload',async({page})=>{
 await i26bPlaybackFixture(page);await init(page,1440);await page.goto('/playback?tab=users');await page.getByRole('button',{name:'编辑用户',exact:true}).click();const dialog=page.getByRole('dialog',{name:'编辑媒体用户',exact:true});await expect(dialog).toBeVisible();await stableLayout(page);
 const enabled=dialog.locator('.user-enabled'),r=await enabled.boundingBox(),input=await enabled.locator('input').boundingBox();expect(Math.abs(r!.y+r!.height/2-input!.y-input!.height/2)).toBeLessThanOrEqual(1);
 const gaps=await dialog.locator('.u5-form>label').evaluateAll(es=>es.slice(0,5).map((e,i,a)=>i?e.getBoundingClientRect().top-a[i-1]!.getBoundingClientRect().bottom:0).slice(1));expect(gaps).toEqual([16,16,4,16]);
 const following=await dialog.getByRole('spinbutton',{name:'同时播放上限',exact:true}).locator('..').boundingBox();expect(following!.y-input!.y-input!.height).toBe(16);expect(r!.height).toBeGreaterThanOrEqual(44);
 await expect(enabled.locator('input')).toBeChecked();await enabled.locator('input').uncheck();await expect(enabled.locator('input')).not.toBeChecked();await enabled.locator('input').check();await screenshot(page,'after-user-editor-1440-light',false);
 await expect(dialog.getByRole('spinbutton',{name:'同时播放上限',exact:true})).toHaveAttribute('min','1');await expect(dialog.getByRole('spinbutton',{name:'同时播放上限',exact:true})).toHaveAttribute('max','20');
});
test('I26b IP scope confirmation shrinks without changing self-block acknowledgement or save guards',async({page})=>{
 const f=await i26bPlaybackFixture(page);await init(page,1440,'dark');await page.goto('/playback?tab=ip-blocklist');await page.getByRole('button',{name:'添加 IP 规则',exact:true}).click();const editor=page.getByRole('dialog',{name:'添加 IP 黑名单规则',exact:true});await editor.getByRole('textbox',{name:'黑名单 IP 或 CIDR',exact:true}).fill('8.8.8.8');await editor.getByRole('button',{name:'核对并继续',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'确认 IP 黑名单范围',exact:true});await expect(dialog).toBeVisible();await stableLayout(page);
 await expect(dialog).toContainText('全部媒体用户从此 IP 或网段访问媒体均返回 403；管理后台仍可删除规则。此操作不改账号、网盘文件或 STRM。');await expect(dialog).toContainText('8.8.8.0/24');
 const save=dialog.getByRole('button',{name:'确认保存黑名单规则',exact:true});await expect(save).toBeDisabled();const ack=dialog.getByRole('checkbox');await dialog.locator('.el-checkbox').click();await expect(ack).toBeChecked();await expect(save).toBeEnabled();await dialog.locator('.el-checkbox').click();await expect(ack).not.toBeChecked();await expect(save).toBeDisabled();
 const whitespace=await dialog.evaluate(e=>{const b=e.querySelector('.el-dialog__body')!,last=b.lastElementChild!;return b.getBoundingClientRect().bottom-last.getBoundingClientRect().bottom});expect(whitespace).toBeLessThanOrEqual(24);await screenshot(page,'after-ip-confirm-1440-dark',false);
 await dialog.getByRole('button',{name:'返回修改',exact:true}).click();await expect(editor).toBeVisible();expect(f.writes).toEqual([]);
});
test('I26b history desktop actions already centered; mobile padding tightens and buttons stay 44px',async({page})=>{
 await i26bPlaybackFixture(page);await init(page,1440,'dark');await page.goto('/playback?tab=players');await expect(page.locator('.ua-observation-row')).toHaveCount(1);await stableLayout(page);
 const row=page.locator('.ua-observation-row'),box=await row.boundingBox(),actions=await row.locator('.mt-list-actions').boundingBox();expect(Math.abs(box!.y+box!.height/2-actions!.y-actions!.height/2)).toBeLessThanOrEqual(1);
 await page.setViewportSize({width:390,height:1000});await expect(page.locator('html')).toHaveClass(/dark/);await expect(page.locator('.ua-visit-row')).toHaveCount(1);await stableLayout(page);
 for(const row of await page.locator('.ua-observation-row,.ua-visit-row').all()){
  await expect(row).toHaveCSS('padding-top','8px');await expect(row).toHaveCSS('padding-bottom','8px');
  for(const button of await row.getByRole('button').all()){const b=await button.boundingBox();expect(b!.height).toBeGreaterThanOrEqual(44);expect(b!.width).toBeGreaterThanOrEqual(44)}
 }await noOverflow(page);await screenshot(page,'after-history-390-dark');
});
for(const value of [null,{items:[],total:0},[]])test(`I26b playback service empty ${JSON.stringify(value)} shows empty state without console errors`,async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='error')errors.push(m.text())});await u5PlaybackFixture(page);
 await page.route('**/api/v1/media-server/native/library-mappings?*',r=>r.fulfill({json:{data:value}}));await page.goto('/playback?tab=settings');await expect(page.getByText('暂无媒体库目录',{exact:true})).toBeVisible();await stableLayout(page);expect(errors).toEqual([]);
});
test('I26b fields inherit Element frames and native token spacing rather than nested input padding',async({page})=>{
 await policyLogFixture(page);await page.goto('/ops?tab=maintenance');await page.getByRole('navigation',{name:'备份与告警功能'}).getByRole('button',{name:'维护策略',exact:true}).click();await expect(page.getByRole('textbox',{name:'备份Cron',exact:true})).toBeVisible();await stableLayout(page);
 for(const input of await page.locator('.ops-policy-grid .el-input__inner').all())await expect(input).toHaveCSS('padding-left','0px');
 for(const wrapper of await page.locator('.ops-policy-grid .el-input__wrapper').all())await expect(wrapper).toHaveCSS('border-top-left-radius','8px');
});
test('I26b search history already centers desktop actions and uses compact mobile rows',async({page})=>{
 await u7Fixture(page,'/search');await page.route('**/api/v1/search',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/cache/search?*',r=>r.fulfill({json:{data:{items:[{id:'history',query:'本地历史样例',createdAt:'2026-10-30T02:00:00Z',total:3,state:'succeeded'}],total:1}}}));
 await page.goto('/search?tab=history');const row=page.locator('.acquire-history article');await expect(row).toHaveCount(1);await stableLayout(page);
 const box=await row.boundingBox(),actions=await row.locator('.acquire-actions').boundingBox();expect(Math.abs(box!.y+box!.height/2-actions!.y-actions!.height/2)).toBeLessThanOrEqual(1);
 await page.setViewportSize({width:390,height:1000});await stableLayout(page);await expect(row).toHaveCSS('padding-top','8px');await expect(row).toHaveCSS('padding-bottom','8px');
 for(const el of await row.locator('button,a').all())expect((await el.boundingBox())!.height).toBeGreaterThanOrEqual(44);await noOverflow(page);
});
test('I26b user activity empty mock does not read length from missing items',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='error')errors.push(m.text())});await i26bPlaybackFixture(page);
 await page.route('**/api/v1/media-server/users/*/devices?*',r=>r.fulfill({json:{data:[]}}));await page.goto('/playback?tab=users');await page.getByRole('button',{name:'用户详情',exact:true}).click();const dialog=page.getByRole('dialog',{name:'family · 用户详情',exact:true});await expect(dialog).toBeVisible();await expect(dialog.getByText('暂无记录',{exact:true})).toBeVisible();await stableLayout(page);expect(errors).toEqual([]);
});
