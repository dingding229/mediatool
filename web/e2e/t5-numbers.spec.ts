import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {u6OpsFixture} from './u6-fixture';
const dir=resolve(process.cwd(),'../scratchpad/t5');
const app={revision:3,value:{movieTemplate:'{title} ({year})/{title} - {videoFormat}',seriesTemplate:'{title}/Season {season}/{title} - {season_episode}',trustedProxyCidrs:[],searchCacheSeconds:600,tmdbCacheSeconds:86400,subscriptionCron:'*/30 * * * *',cleanupAfterImport:false,scraping:{movie:{nfo:true,poster:true,backdrop:true,logo:true},series:{nfo:true,poster:true,backdrop:true,logo:true},season:{nfo:true,poster:true},episode:{nfo:true,thumb:true}},playbackRetentionDays:90,logging:{retentionDays:7,debugUntil:null}}};
async function fixture(page:Page){
 await u6OpsFixture(page);let record=structuredClone(app);const writes:any[]=[];
 await page.route('**/api/v1/settings/application',r=>{if(r.request().method()==='PUT'){const b=r.request().postDataJSON();writes.push(b);record={value:b.value,revision:record.revision+1};}return r.fulfill({json:{data:record}})});
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{id:'test-site',name:'测试站点',kind:'ptsite',enabled:true,settings:{rssState:'ready'}}]}}));
 await page.route('**/api/v1/subscriptions?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));
 await page.route('**/api/v1/metadata/details?*',r=>r.fulfill({json:{data:{item:{id:209867,mediaType:'series',title:'山间故事',year:'2026'}}}}));
 return {writes};
}
async function openRepresentative(page:Page,name:string){
 if(name==='settings-automation'){await page.goto('/settings?tab=automation');await expect(page.locator('#pt-cache-seconds')).toBeVisible()}
 if(name==='ops-maintenance'){await page.goto('/ops?tab=maintenance');await page.getByRole('button',{name:'维护策略',exact:true}).click();await expect(page.getByLabel('重复提醒冷却（分钟）',{exact:true})).toBeVisible()}
 if(name==='ops-logs'){await page.goto('/ops?tab=logs');await page.getByRole('button',{name:'日志设置',exact:true}).click();await expect(page.getByLabel('播放统计保留天数',{exact:true})).toBeVisible()}
 if(name==='subscriptions'){await page.goto('/subscriptions?create=1&mediaType=series&tmdbId=209867&tab=list');await expect(page.getByLabel('目标季',{exact:true})).toBeVisible()}
 const dialogs=page.getByRole('dialog');if(await dialogs.count()){await expect.poll(()=>dialogs.first().evaluate(el=>{const b=el.getBoundingClientRect();return b.left>=0&&b.right<=innerWidth&&b.width>0})).toBe(true)}
}
test('T5 direct seconds retain invalid input and cannot submit, valid boundaries use numeric JSON',async({page})=>{
 const f=await fixture(page);await openRepresentative(page,'settings-automation');const save=page.getByRole('button',{name:'保存搜索订阅设置',exact:true});
 for(const value of ['','-1','86401','abc','1.5']){await page.locator('#pt-cache-seconds').fill(value);await expect(page.getByRole('alert').filter({hasText:'站点搜索缓存有效期'})).toBeVisible();await expect(save).toBeDisabled();expect(f.writes).toHaveLength(0);await expect(page.locator('#pt-cache-seconds')).toHaveValue(value)}
 await page.locator('#pt-cache-seconds').fill('0');await page.locator('#tmdb-cache-seconds').fill('2592001');await expect(save).toBeDisabled();await page.locator('#tmdb-cache-seconds').fill('2592000');await page.getByLabel('订阅调度时间',{exact:true}).fill('61 * * * *');await expect(save).toBeDisabled();await expect(page.getByRole('alert')).toContainText('分钟');await page.getByLabel('订阅调度时间',{exact:true}).fill('*/1 * * * *');await expect(save).toBeDisabled();await page.getByLabel('订阅调度时间',{exact:true}).fill('*/5 * * * *');await save.click();await expect(page.getByRole('status').filter({hasText:'当前分区已保存'})).toBeVisible();expect(f.writes).toHaveLength(1);expect(f.writes[0].value).toMatchObject({searchCacheSeconds:0,tmdbCacheSeconds:2592000,subscriptionCron:'*/5 * * * *',movieTemplate:app.value.movieTemplate});
});
test('T5 numeric controls and screenshots 1440/390 in light/dark',async({page})=>{
 await fixture(page);mkdirSync(dir,{recursive:true});const before=process.env.T5_BEFORE==='1';
 if(before){const root=join(dir,'before-assets');for(const name of ['settings','ops','subscriptions'])await page.route(`**/${name}{,?*}`,r=>new URL(r.request().url()).pathname==='/'+name?r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}):r.fallback());await page.route('**/assets/*',r=>{const name=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(name)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',name))})})}
 for(const name of ['settings-automation','subscriptions','ops-logs','ops-maintenance'])for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await openRepresentative(page,name);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  if(!before){await expect(page.locator('.el-input-number__increase,.el-input-number__decrease')).toHaveCount(0);for(const input of await page.locator('input[type=number]').all()){expect(['none','textfield']).toContain(await input.evaluate(el=>getComputedStyle(el).getPropertyValue('appearance')))}}
  if(name==='subscriptions')await page.getByLabel('目标季',{exact:true}).scrollIntoViewIfNeeded();
  if(name==='ops-maintenance')await page.getByLabel('重复提醒冷却（分钟）',{exact:true}).scrollIntoViewIfNeeded();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:`${dir}/${before?'before':'after'}-${name}-${width}-${theme}.png`,animations:'disabled'});
 }
});
