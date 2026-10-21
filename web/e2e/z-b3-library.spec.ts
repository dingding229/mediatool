import {test,expect,type Page} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {u4Fixture} from './u4-fixture';
const dir='test-results/b3';
async function shot(page:Page,name:string){mkdirSync(dir,{recursive:true});await page.evaluate(()=>window.scrollTo(0,0));await expect.poll(()=>page.evaluate(()=>scrollY)).toBe(0);await page.screenshot({path:`${dir}/${name}.png`,fullPage:true,animations:'disabled'})}
test('B3 managed hierarchy reads complete paged seasons and episodes without writes',async({page})=>{
 await u4Fixture(page);const parents:string[]=[];let writes=0;
 await page.route('**/api/v1/media/library?*',r=>{if(r.request().method()!=='GET')writes++;const q=new URL(r.request().url()).searchParams;const parent=q.get('parentId')||'',p=Number(q.get('page')||1);parents.push(`${parent}:${p}`);
 const items=parent==='series_owned'?Array.from({length:Math.min(24,30-(p-1)*24)},(_,i)=>({id:`season_owned_${(p-1)*24+i+1}`,type:'Season',title:`第 ${(p-1)*24+i+1} 季`,season:(p-1)*24+i+1,source:'managed',mediaType:'series'})):parent==='season_owned_30'?Array.from({length:Math.min(24,100-(p-1)*24)},(_,i)=>({id:`episode${(p-1)*24+i+1}`,type:'Episode',title:`旅程 ${(p-1)*24+i+1}`,season:30,episode:(p-1)*24+i+1,source:'managed',mediaType:'series',state:'present'})):[{id:'series_owned',type:'Series',title:'星河纪行',year:'2026',source:'managed',mediaType:'series',overview:'完整季集层级，不依赖当前文件页的分组。'}];return r.fulfill({json:{data:{items,total:parent==='series_owned'?30:parent==='season_owned_30'?100:1}}})});
 await page.goto('/library');await page.getByRole('button',{name:'星河纪行',exact:true}).click();const section=page.getByRole('region',{name:'季与集'});await expect(section).toContainText('季列表 · 共 30 项');await expect(page.getByRole('button',{name:'复制播放链接',exact:true})).toHaveCount(0);await section.getByRole('button',{name:'下一页',exact:true}).click();await expect(section.getByRole('button',{name:/第 30 季/})).toBeVisible();await shot(page,'seasons-desktop');await section.getByRole('button',{name:/第 30 季/}).click();await expect(section).toContainText('第 30 季 · 集列表 · 共 100 项');
 for(let i=0;i<4;i++){await section.getByRole('button',{name:'下一页',exact:true}).click();await expect.poll(()=>parents.includes(`season_owned_30:${i+2}`)).toBe(true)}
 await expect(section.getByRole('button',{name:'第 30 季 · 第 97 集 · 旅程 97',exact:true})).toBeVisible();await shot(page,'episodes-desktop');await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await shot(page,'episodes-mobile');expect(writes).toBe(0);
});
test('B3 plugin permission review explains the read-only reason and next step',async({page})=>{
 await u4Fixture(page);
 await page.route('**/api/v1/plugins/ui-surfaces',r=>r.fulfill({json:{data:[{instanceId:'owned-review',pluginId:'hello-go',pages:['hello'],dashboards:[]}]}}));
 await page.route('**/api/v1/plugins/owned-review/ui/**',r=>r.fulfill({status:403,json:{error:{code:'plugin_permission_denied',message:'插件处于只读状态（权限待确认），请在插件页确认权限后重试'}}}));
 await page.goto('/plugins/owned-review/pages/hello');await expect(page.getByText('插件处于只读状态（权限待确认），请在插件页确认权限后重试',{exact:true})).toBeVisible();await shot(page,'plugin-readonly-desktop');
});
