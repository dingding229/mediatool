import {test,expect,type Page,type Locator} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {u7Fixture} from './u7-fixture';
import {mediaUser} from './u5-fixture';
const policy={enabled:false,startTime:'02:00',endTime:'05:00',quietMinutes:60,revision:1,reason:''};
const root=resolve(import.meta.dirname,'../../scratchpad/i-phase/i5b');
async function fixture(page:Page,path:string){
 await u7Fixture(page,path);
 await page.route('**/api/v1/system/version',r=>r.fulfill({json:{data:{version:'UI fixture',commit:'a'.repeat(40),schema:90,minimumSchema:79}}}));
 await page.route('**/api/v1/system/updates',r=>r.fulfill({json:{data:{enabled:false,intervalHours:12,hasUpdate:false,result:{status:'no_release'}}}}));
 await page.route('**/api/v1/system/online-update',r=>r.fulfill({json:{data:{available:false,reason:'本地测试',recoveryRequired:[],job:null}}}));
 await page.route('**/api/v1/system/online-update/idle',r=>r.fulfill({json:{data:policy}}));
 await page.route('**/api/v1/media-server/users/'+mediaUser.id,r=>r.fulfill({json:{data:mediaUser}}));
}
const targets=[
 {id:'idle',path:'/settings',open:async(page:Page)=>{await page.goto('/settings?tab=about');return page.locator('section.idle-update')},labels:['开始时间','结束时间']},
 {id:'history',path:'/organize',open:async(page:Page)=>{await page.goto('/organize?tab=history');return page.locator('form.organize-history-filters')},labels:['历史开始时间','历史结束时间']},
 {id:'users',path:'/playback',open:async(page:Page)=>{await page.goto('/playback?tab=users');await page.getByRole('button',{name:'编辑用户',exact:true}).click();return page.getByRole('dialog',{name:'编辑媒体用户',exact:true})},labels:['媒体用户有效期']},
];
async function geometry(input:Locator){return input.evaluate(el=>{const r=el.getBoundingClientRect(),p=el.parentElement!.getBoundingClientRect(),s=getComputedStyle(el);return {x:r.x,y:r.y,width:r.width,height:r.height,parentWidth:p.width,padding:[s.paddingTop,s.paddingRight,s.paddingBottom,s.paddingLeft],border:[s.borderTopWidth,s.borderTopStyle,s.borderRadius],boxSizing:s.boxSizing,color:s.color,background:s.backgroundColor,type:el.getAttribute('type')}})}
for(const width of [1440,390])for(const theme of ['light','dark'])test(`I5 behavior consistent native time fields ${width} ${theme}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
 let standard:Awaited<ReturnType<typeof geometry>>|undefined;
 for(const target of targets){
  await fixture(page,target.path);const scope=await target.open(page);await expect(scope).toBeVisible();
  for(const label of target.labels){const input=scope.getByLabel(label,{exact:true});await expect(input).toHaveClass(/mt-native-time-input/);const g=await geometry(input);expect(g.height).toBe(44);expect(g.width).toBeGreaterThan(80);expect(g.width).toBeLessThanOrEqual(g.parentWidth+1);expect(g.x).toBeGreaterThanOrEqual(0);expect(g.x+g.width).toBeLessThanOrEqual(width+1);expect(g.boxSizing).toBe('border-box');expect(g.type).toBe(target.id==='idle'?'time':'datetime-local');if(standard){expect(g.padding).toEqual(standard.padding);expect(g.border).toEqual(standard.border)}else standard=g;}
  if(target.id==='idle'){const a=await geometry(scope.getByLabel('开始时间',{exact:true})),b=await geometry(scope.getByLabel('结束时间',{exact:true})),m=await geometry(scope.getByLabel('安静时长（分钟）',{exact:true})),reference=await geometry(page.getByLabel('检查间隔（小时）',{exact:true}));expect(a.width).toBe(b.width);for(const g of [a,b,m]){expect(g.height).toBe(reference.height);expect(g.border).toEqual(reference.border);expect(parseFloat(g.border[2]!)).toBeGreaterThan(0);expect(g.padding).toEqual(reference.padding)}if(width===1440)for(const g of [a,b,m])expect(g.width).toBe(reference.width);await expect(scope.getByLabel('开始时间',{exact:true})).toHaveValue('02:00');await expect(scope.getByLabel('结束时间',{exact:true})).toHaveValue('05:00');}
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
 }
});
test('I5 behavior idle HH:mm values and disabled-save validation retain the exact DTO',async({page})=>{
 await fixture(page,'/settings');let body:any;await page.route('**/api/v1/system/online-update/idle',r=>{if(r.request().method()==='PUT'){body=r.request().postDataJSON();return r.fulfill({json:{data:{...policy,...body,revision:2}}})}return r.fulfill({json:{data:policy}})});
 await page.goto('/settings?tab=about');const scope=page.locator('section.idle-update'),save=scope.getByRole('button',{name:'保存闲时设置',exact:true});
 await scope.getByLabel('开始时间',{exact:true}).fill('05:00');await expect(save).toBeDisabled();await scope.getByLabel('开始时间',{exact:true}).fill('');await expect(save).toBeDisabled();await scope.getByLabel('开始时间',{exact:true}).fill('23:30');await scope.getByLabel('结束时间',{exact:true}).fill('05:15');await expect(save).toBeEnabled();await save.click();await expect.poll(()=>body).toBeTruthy();expect(body).toEqual({enabled:false,startTime:'23:30',endTime:'05:15',quietMinutes:60,revision:1,password:'',confirm:false});
});
test('I5 behavior history datetime values keep Beijing query semantics',async({page})=>{
 await fixture(page,'/organize');let query:URLSearchParams|undefined;await page.route('**/api/v1/library/activity?*',r=>{query=new URL(r.request().url()).searchParams;return r.fulfill({json:{data:{items:[],total:0}}})});
 await page.goto('/organize?tab=history');await page.getByLabel('历史开始时间',{exact:true}).fill('2026-10-27T23:30');await page.getByLabel('历史结束时间',{exact:true}).fill('2026-10-28T05:15');await page.getByRole('button',{name:'筛选',exact:true}).click();await expect.poll(()=>query?.get('from')).toBe('2026-10-27T15:30:00.000Z');expect(query?.get('to')).toBe('2026-10-27T21:15:00.000Z');
});
const capture=process.env.I5_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const before=process.env.I5_BEFORE==='1';
test.describe('I5 visual comparison @screenshots',()=>{
 test.skip(!capture,'Local explicit opt-in only: I5_SCREENSHOTS=1');
 for(const target of targets)for(const width of [1440,390])for(const theme of ['light','dark'])test(`${target.id} ${width} ${theme}`,async({page})=>{
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await fixture(page,target.path);const scope=await target.open(page);await expect(scope).toBeVisible();await scope.getByLabel(target.labels[0]!,{exact:true}).scrollIntoViewIfNeeded();
  const dir=resolve(root,'screens');mkdirSync(dir,{recursive:true});const name=`${before?'before':'after'}-${target.id}-${width}-${theme}`;
  const measurements=[];for(const label of target.id==='idle'?[...target.labels,'安静时长（分钟）']:target.labels)measurements.push({label,...await geometry(scope.getByLabel(label,{exact:true}))});
  await page.screenshot({path:resolve(dir,name+'.png'),fullPage:true,animations:'disabled'});await scope.screenshot({path:resolve(dir,name+'-panel.png'),animations:'disabled'});writeFileSync(resolve(dir,name+'.json'),JSON.stringify(measurements,null,2));
 });
});
