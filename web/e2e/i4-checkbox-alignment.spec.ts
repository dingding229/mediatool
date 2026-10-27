import {test,expect,type Page,type Locator} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {u7Fixture} from './u7-fixture';
import {mediaUser,siteAccount} from './u5-fixture';
import {plugin,definition} from './u6-fixture';

const connections=[
 {id:'i4-115',kind:'open115',name:'115 核验连接',secretFields:['accessToken','refreshToken'],settings:{appId:'fixture-app',cloudMountPrefix:'/115'}},
 {id:'i4-cd2',kind:'clouddrive2',name:'CD2 核验连接',secretFields:['token','username','password'],settings:{endpoint:'http://cd2.fixture',cloudRoot:'/115/Media'}},
 {id:'i4-qb',kind:'qbittorrent',name:'qB 核验连接',secretFields:['username','password'],settings:{endpoint:'http://qb.fixture',downloadPath:'/downloads',localPath:'/downloads'}},
].map(r=>({...r,enabled:true,revision:1,lastCheckState:'ready'}));
async function fixture(page:Page,path:string){
 await u7Fixture(page,path.startsWith('/plugins/')?'/plugins':path.split('?')[0]);
 await page.route('**/api/v1/settings/network-proxy',r=>r.fulfill({json:{data:{url:'http://proxy.fixture:7890',tmdb:false,sites:false,revision:1}}}));
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[...connections,siteAccount]}}));
 await page.route('**/api/v1/cloud/accounts/info',r=>r.fulfill({json:{data:{info:null}}}));
 await page.route('**/api/v1/media-server/users/'+mediaUser.id,r=>r.fulfill({json:{data:mediaUser}}));
 await page.route('**/api/v1/plugins/'+plugin.id,r=>r.fulfill({json:{data:{instance:{...plugin,reason:'permission_review_required'},config:{value:{},revision:2},fingerprint:definition.fingerprint}}}));
 await page.route('**/api/v1/plugins/'+plugin.id+'/review',r=>r.fulfill({json:{data:{instance:plugin,current:plugin.metadata,availability:'available',fingerprint:definition.fingerprint,baselineKnown:true,added:['events.listen'],removed:[],changed:[],needsReview:true}}}));
}
async function openConnection(page:Page,kind:string){
 const path=kind==='qbittorrent'?'/connections?tab=downloader':'/connections?tab=storage';
 await fixture(page,path);await page.goto(path);
 const row=page.locator('.saved-connection').filter({hasText:connections.find(c=>c.kind===kind)!.name});
 await row.getByRole('button',{name:'编辑',exact:true}).click();
 return page.getByRole('dialog',{name:'编辑连接',exact:true});
}
async function openProxy(page:Page){await fixture(page,'/connections?tab=aux');await page.goto('/connections?tab=aux');return page.getByRole('region',{name:'网络代理',exact:true});}
async function measure(input:Locator){
 return input.evaluate(el=>{
  const label=el.closest('label')!,glyph=label.querySelector('.el-checkbox__inner')||el;
  const span=label.querySelector('.el-checkbox__label')||label;
  const walker=document.createTreeWalker(span,NodeFilter.SHOW_TEXT);
  let node:Node|null=null;while((node=walker.nextNode())&&!node.textContent?.trim()){}
  if(!node)throw Error('Checkbox label text missing');
  const start=node.textContent!.search(/\S/),range=document.createRange();range.setStart(node,start);range.setEnd(node,node.textContent!.trimEnd().length);
  const text=[...range.getClientRects()].find(r=>r.width>0&&r.height>0)!,g=glyph.getBoundingClientRect(),l=label.getBoundingClientRect();
  return {glyph:{x:g.x,y:g.y,width:g.width,height:g.height},text:{x:text.x,y:text.y,width:text.width,height:text.height},label:{x:l.x,y:l.y,width:l.width,height:l.height},sameLine:g.bottom>text.top&&g.top<text.bottom&&g.right<=text.left};
 });
}
async function check(input:Locator){
 await expect(input).toHaveCount(1);
 await expect.poll(()=>input.evaluate(el=>{const panel=el.closest('.el-dialog');return ![panel,panel?.parentElement].some(node=>node?.getAnimations().some(a=>a.playState==='running'))})).toBe(true);
 const m=await measure(input);
 expect(m.sameLine).toBe(true);expect(m.glyph.width).toBeGreaterThan(0);expect(Math.round(m.glyph.width*100)/100).toBeLessThanOrEqual(20);
 expect(Math.round(m.glyph.height*100)/100).toBeLessThanOrEqual(20);expect(Math.round(m.label.height*100)/100).toBeGreaterThanOrEqual(44);
 return m;
}
async function checkHelp(input:Locator,help:Locator){
 const m=await check(input),r=await help.boundingBox();expect(r).toBeTruthy();expect(Math.abs(r!.x-m.text.x)).toBeLessThanOrEqual(1);expect(r!.y).toBeGreaterThan(m.text.y);
}
const targets=[
 {id:'115',open:(p:Page)=>openConnection(p,'open115')},
 {id:'cd2',open:(p:Page)=>openConnection(p,'clouddrive2')},
 {id:'qb',open:(p:Page)=>openConnection(p,'qbittorrent')},
 {id:'proxy',open:openProxy},
 {id:'settings',open:async(p:Page)=>{await fixture(p,'/settings?tab=naming');await p.goto('/settings?tab=naming');return p.locator('.settings-stack:visible').first()}},
 {id:'users',open:async(p:Page)=>{await fixture(p,'/playback?tab=users');await p.goto('/playback?tab=users');await p.getByRole('button',{name:'编辑用户',exact:true}).click();return p.getByRole('dialog',{name:'编辑媒体用户',exact:true})}},
 {id:'site',open:async(p:Page)=>{await fixture(p,'/sites');await p.goto('/sites');await p.getByRole('button',{name:'更多站点操作',exact:true}).click();await p.getByRole('menuitem',{name:'编辑账号',exact:true}).click();return p.getByRole('dialog',{name:'编辑站点账号',exact:true})}},
 {id:'plugin',open:async(p:Page)=>{const path='/plugins/'+plugin.id;await fixture(p,path);await p.goto(path);await p.getByRole('button',{name:'确认访问范围变化',exact:true}).click();return p.getByRole('dialog',{name:'插件权限确认',exact:true})}},
 {id:'subscription',open:async(p:Page)=>{await fixture(p,'/subscriptions');await p.goto('/subscriptions');await p.getByRole('button',{name:'编辑',exact:true}).first().click();return p.getByRole('dialog',{name:'编辑订阅',exact:true})}},
];
for(const width of [1440,390])for(const theme of ['light','dark'])test(`I4 behavior forms ${width} ${theme}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
 for(const target of targets)await test.step(target.id,async()=>{
  const scope=await target.open(page);await expect(scope).toBeVisible();const boxes=scope.getByRole('checkbox');
  expect(await boxes.count()).toBeGreaterThan(0);
  for(const input of await boxes.all())if(await input.locator('xpath=ancestor::label[1]').isVisible())await check(input);
  const helper=scope.locator('.mt-form-checkbox-help').first();
  if(target.id==='115')await checkHelp(scope.getByRole('checkbox',{name:'启用连接',exact:true}),helper);
  if(target.id==='proxy')await checkHelp(scope.getByRole('checkbox',{name:'PT站点搜索、RSS与站点请求使用代理',exact:true}),helper);
  for(const help of await scope.locator('.mt-form-checkbox-help').all()){
   const previous=help.locator('xpath=preceding-sibling::*[1]');
   if(await previous.evaluate(el=>el.tagName==='LABEL'))await checkHelp(previous.getByRole('checkbox'),help);
  }
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 });
});
test('I4 behavior label clicks and clear/save payload remain unchanged',async({page})=>{
 const proxy=await openProxy(page),box=proxy.getByRole('checkbox',{name:'明确清除已保存的代理地址',exact:true}),label=box.locator('xpath=ancestor::label[1]');
 await expect(box).not.toBeChecked();await label.click();await expect(box).toBeChecked();await label.click();await expect(box).not.toBeChecked();
 const dialog=await openConnection(page,'open115'),clear=dialog.getByRole('checkbox',{name:'明确清除Access Token（可选手动录入）',exact:true});
 await clear.locator('xpath=ancestor::label[1]').click();await expect(clear).toBeChecked();
 let body:any;await page.route('**/api/v1/integrations/i4-115',r=>{body=r.request().postDataJSON();return r.fulfill({json:{data:connections[0]}})});
 await dialog.getByRole('button',{name:'加密保存',exact:true}).click();await expect.poll(()=>body).toBeTruthy();expect(body.secrets).toEqual({accessToken:''});expect(body.enabled).toBe(true);
});
const capture=process.env.I4_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const before=process.env.I4_BEFORE==='1',dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i4/screens');
test.describe('I4 visual review @screenshots',()=>{
 test.skip(!capture,'Local opt-in only: I4_SCREENSHOTS=1');
 for(const target of targets)for(const width of [1440,390])for(const theme of ['light','dark'])test(`${target.id} ${width} ${theme}`,async({page})=>{
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  const scope=await target.open(page);await expect(scope).toBeVisible();const first=scope.getByRole('checkbox').first();await expect.poll(()=>first.evaluate(el=>{const panel=el.closest('.el-dialog');return ![panel,panel?.parentElement].some(node=>node?.getAnimations().some(a=>a.playState==='running'))})).toBe(true);if(['115','cd2','qb'].includes(target.id))await scope.getByRole('checkbox',{name:'启用连接',exact:true}).locator('xpath=ancestor::label[1]').scrollIntoViewIfNeeded();else await scope.scrollIntoViewIfNeeded();mkdirSync(dir,{recursive:true});const rows=[];
  for(const input of await scope.getByRole('checkbox').all())if(await input.locator('xpath=ancestor::label[1]').isVisible())rows.push(await measure(input));
  const path=resolve(dir,`${before?'before':'after'}-${target.id}-${width}-${theme}.png`);
  await page.screenshot({path,animations:'disabled'});writeFileSync(path+'.json',JSON.stringify({target:target.id,width,theme,before,checkboxes:rows},null,2));
 });
});
