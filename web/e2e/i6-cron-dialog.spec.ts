import {test,expect,type Page,type Locator} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {u7Fixture,settingsRecord} from './u7-fixture';
import {fixtureFulfill} from './collection-fixture';
const root=resolve(import.meta.dirname,'../../scratchpad/i-phase/i6');
async function fixture(page:Page,target:'settings'|'sites'){
 await u7Fixture(page,target==='settings'?'/settings':'/sites');
 await page.route('**/api/v1/scheduling/cron/preview',r=>{
  const {expression}=r.request().postDataJSON();
  if(expression==='0 */1 * * * * *')return r.fulfill({status:400,json:{error:{message:'Cron 执行间隔小于此业务允许的最小间隔'}}});
  if(expression==='0 0 0 1 1 * 2020')return r.fulfill({json:{data:{timezone:'Asia/Shanghai',fieldCount:7,serverNow:'2026-10-28T00:00:00+08:00',minimumIntervalSeconds:300,exhausted:true,next:[]}}});
  return fixtureFulfill(r,{json:{data:{}}});
 });
}
async function open(page:Page,target:'settings'|'sites'){
 await page.goto(target==='settings'?'/settings?tab=automation':'/sites');
 if(target==='sites')await page.getByRole('button',{name:'编辑采集计划',exact:true}).click();
 const label=target==='settings'?'订阅调度时间':'站点指标 Cron';await expect(page.getByLabel(label,{exact:true})).toBeVisible();return label;
}
async function generator(page:Page,label:string){await page.getByRole('button',{name:label+'生成',exact:true}).click();const d=page.getByRole('dialog',{name:label+'生成器',exact:true});await expect(d).toBeVisible();return d}
async function mode(page:Page,d:Locator,name:string){await d.locator('.cron-generator-fields .el-select__wrapper').first().click();await page.getByRole('option',{name,exact:true}).click()}
test('I6 behavior settings generate then save retains the existing settings DTO',async({page})=>{
 await fixture(page,'settings');let current=structuredClone(settingsRecord),body:any;
 await page.route('**/api/v1/settings/application',r=>{if(r.request().method()==='PUT'){body=r.request().postDataJSON();current={revision:current.revision+1,value:{...current.value,...body.value}}}return r.fulfill({json:{data:current}})});
 const label=await open(page,'settings'),input=page.getByLabel(label,{exact:true}),prior=await input.inputValue();const d=await generator(page,label);
 await mode(page,d,'每天');await d.getByLabel(label+'执行时刻',{exact:true}).fill('10:35');await expect(input).toHaveValue(prior);await expect(d.getByRole('button',{name:'确定',exact:true})).toBeEnabled();await d.getByRole('button',{name:'确定',exact:true}).click();await expect(input).toHaveValue('0 35 10 * * * *');
 const save=page.getByRole('button',{name:'保存搜索订阅设置',exact:true});await expect(save).toBeEnabled();await save.click();await expect.poll(()=>body).toBeTruthy();expect(body.revision).toBe(3);expect(body.value).toMatchObject({subscriptionCron:'0 35 10 * * * *',searchCacheSeconds:600,tmdbCacheSeconds:86400});await page.reload();await expect(page.getByLabel(label,{exact:true})).toHaveValue('0 35 10 * * * *');
});
test('I6 behavior nested site generator confirms and saves without changing enabled or revision fields',async({page})=>{
 await fixture(page,'sites');let current={enabled:false,cron:'0 */2 * * *',revision:1},body:any;
 await page.route('**/api/v1/sites/metrics/schedule',r=>{if(r.request().method()==='PUT'){body=r.request().postDataJSON();current={...body,revision:2}}return r.fulfill({json:{data:current}})});
 const label=await open(page,'sites'),d=await generator(page,label);await mode(page,d,'每小时第 M 分');await d.getByRole('spinbutton',{name:label+'分钟',exact:true}).fill('15');await expect(d.getByRole('button',{name:'确定',exact:true})).toBeEnabled();await d.getByRole('button',{name:'确定',exact:true}).click();
 await expect(page.getByLabel(label,{exact:true})).toHaveValue('0 15 * * * * *');await expect(page.getByRole('dialog',{name:'指标采集计划',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'保存采集计划',exact:true})).toBeEnabled();await page.getByRole('button',{name:'保存采集计划',exact:true}).click();await expect.poll(()=>body).toBeTruthy();expect(body).toEqual({enabled:false,cron:'0 15 * * * * *',revision:1});await expect(page.getByRole('dialog',{name:'指标采集计划',exact:true})).toBeHidden();
});
for(const width of [1440,390])test(`I6 behavior custom draft cancellation and authoritative confirmation gate ${width}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await fixture(page,'settings');const label=await open(page,'settings'),input=page.getByLabel(label,{exact:true}),prior=await input.inputValue();let d=await generator(page,label);await mode(page,d,'自定义');const custom=d.getByLabel(label+'自定义表达式',{exact:true});await custom.fill('0 */1 * * * * *');await expect(d.getByRole('alert')).toContainText('最小间隔');await expect(d.getByRole('button',{name:'确定',exact:true})).toBeDisabled();await expect(input).toHaveValue(prior);
 await custom.fill('0 0 0 1 1 * 2020');await expect(d.getByRole('status')).toContainText('计划已结束');await expect(d.getByRole('button',{name:'确定',exact:true})).toBeDisabled();await custom.fill('0 0 0 1 1 * 2199');await expect(d.getByRole('button',{name:'确定',exact:true})).toBeEnabled();await d.getByRole('button',{name:'取消',exact:true}).click();await expect(input).toHaveValue(prior);
 d=await generator(page,label);await expect(d.locator('.cron-generator-fields .el-select__wrapper').first()).toContainText('每 N 分钟');await page.keyboard.press('Escape');await expect(d).toBeHidden();await expect(input).toHaveValue(prior);await expect(page.locator('.cron-preview')).toHaveCount(0);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
});
const capture=process.env.I6_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS,before=process.env.I6_BEFORE==='1';
test.describe('I6 visual review @screenshots',()=>{
 test.skip(!capture,'Local explicit opt-in only: I6_SCREENSHOTS=1');
 for(const target of ['settings','sites'] as const)for(const width of [1440,390])for(const theme of ['light','dark'])test(`${target} ${width} ${theme}`,async({page})=>{
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await fixture(page,target);const label=await open(page,target);await expect(page.locator('.cron-input')).toBeVisible();const dir=resolve(root,'screens');mkdirSync(dir,{recursive:true});const prefix=`${before?'before':'after'}-${target}-${width}-${theme}`;
  if(before)await expect(page.locator('.cron-preview')).toContainText('接下来');else await expect(page.locator('.cron-input > .cron-feedback')).toContainText('校验通过');await page.screenshot({path:resolve(dir,prefix+'.png'),fullPage:true,animations:'disabled'});
  if(!before){const d=await generator(page,label);await expect(d.getByRole('button',{name:'确定',exact:true})).toBeEnabled();await page.screenshot({path:resolve(dir,prefix+'-dialog.png'),animations:'disabled'});const rect=await d.boundingBox();writeFileSync(resolve(dir,prefix+'.json'),JSON.stringify({width,theme,dialog:rect},null,2));}
 });
});
for(const width of [1440,390])test(`I6 behavior compact feedback does not resize the settings card ${width}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await fixture(page,'settings');let release!:()=>void;const gate=new Promise<void>(r=>release=r);
 await page.route('**/api/v1/scheduling/cron/preview',async r=>{await gate;return fixtureFulfill(r,{json:{data:{}}})});
 await open(page,'settings');const feedback=page.locator('.cron-input > .cron-feedback'),card=page.locator('.settings-card:visible').first();await expect(feedback).toContainText('正在校验');const before=(await card.boundingBox())!;
 release();await expect(feedback).toContainText('校验通过');const after=(await card.boundingBox())!;expect(Math.abs(before.height-after.height)).toBeLessThanOrEqual(1);expect((await feedback.boundingBox())!.height).toBeLessThan(100);
});
