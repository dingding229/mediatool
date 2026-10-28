import {test,expect,type Page,type Locator} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {u7Fixture} from './u7-fixture';

type Target={name:string;path:string;trigger:(page:Page)=>Locator;setup?:(page:Page)=>Promise<void>};
const targets:Target[]=[
 {name:'media-library',path:'/library',trigger:page=>page.locator('.catalog-poster').first()},
 {name:'user-detail',path:'/playback?tab=users',trigger:page=>page.getByRole('button',{name:'用户详情',exact:true}).first()},
 {name:'log-detail',path:'/ops?tab=logs',trigger:page=>page.getByRole('button',{name:'查看完整链接',exact:true})},
 {name:'site-account',path:'/sites',trigger:page=>page.locator('.f5-site-name').first()},
 {name:'organize-history',path:'/organize?tab=history',trigger:page=>page.getByRole('button',{name:'查看步骤详情',exact:true}).first(),setup:async page=>{
  await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[{id:'history-owned',kind:'file',state:'succeeded',source:'/115/电影/海边的故事.mkv',destination:'/媒体/电影/海边的故事.mkv',updatedAt:'2026-10-25T00:00:00Z',createdAt:'2026-10-25T00:00:00Z',media:{title:'海边的故事',year:'2026'},details:{plan:{steps:Array.from({length:30},(_,i)=>({action:'mkdir',target:`/电影/核对目录 ${i+1}`}))}}}],total:1}}}));
 }},
];

async function open(page:Page,target:Target,width:number,theme='light'){
 await page.setViewportSize({width,height:1000});
 await page.addInitScript(mode=>localStorage.setItem('media-tool:theme-mode:v1',mode),theme);
 await u7Fixture(page,target.path.split('?')[0]);await target.setup?.(page);
 await page.goto(target.path);const trigger=target.trigger(page);await trigger.focus();await trigger.click();
 const dialog=page.getByRole('dialog').last();await expect(dialog).toBeVisible();return {dialog,trigger};
}
async function checkGeometry(dialog:Locator,width:number,expectedWidth:number){
 const panel=dialog.locator('.el-dialog.detail-dialog');await expect(panel).toBeVisible();
 await expect.poll(()=>panel.evaluate((el,expected)=>{const r=el.getBoundingClientRect(),mobile=innerWidth<600;return mobile?Math.abs(r.x)<1&&Math.abs(r.y)<1&&Math.abs(r.width-innerWidth)<1&&Math.abs(r.height-innerHeight)<1:Math.abs(r.width-expected)<1&&r.height<=innerHeight*.85+1&&Math.abs(r.x-(innerWidth-r.width)/2)<2&&Math.abs(r.y-(innerHeight-r.height)/2)<2},expectedWidth)).toBe(true);
 await expect(panel.locator('.el-dialog__footer')).toHaveCount(0);
 await expect(dialog.getByRole('button',{name:'关闭',exact:true})).toHaveCount(0);
 await expect(panel.locator('.el-dialog__headerbtn')).toHaveCount(1);
 await expect(panel.locator('.el-dialog__body')).toHaveCSS('overflow-y','auto');
 if(width===390)expect(await panel.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);
 return panel;
}

// Fast behavior gate, with no screenshot calls, artificial sleeps, retries or forced clicks.
for(const width of [1440,390])for(const target of targets)test(`I2 behavior ${target.name} ${width}`,async({page})=>{
 const {dialog,trigger}=await open(page,target,width);const panel=await checkGeometry(dialog,width,1000);
 const close=panel.locator('.el-dialog__headerbtn');await close.focus();
 await page.keyboard.press('Shift+Tab');await expect.poll(()=>panel.evaluate(el=>el.contains(document.activeElement))).toBe(true);
 if(width===1440){await page.mouse.click(2,2);await expect(dialog).toBeVisible();}
 const body=panel.locator('.el-dialog__body'),header=panel.locator('.el-dialog__header');const original=(await header.boundingBox())!;
 if(target.name==='organize-history'){
  expect(await body.evaluate(el=>el.scrollHeight>el.clientHeight)).toBe(true);
  await body.evaluate(el=>{el.scrollTop=el.scrollHeight});
  expect(Math.abs((await header.boundingBox())!.y-original.y)).toBeLessThan(1);
 }
 await close.focus();await page.keyboard.press('Escape');await expect(dialog).toBeHidden();await expect(trigger).toBeFocused();
 await trigger.click();await expect(dialog).toBeVisible();await dialog.locator('.el-dialog__headerbtn').click();await expect(dialog).toBeHidden();
});

// Explicit local-only capture; never writes screenshot artifacts during CI.
const localScreenshots=process.env.I2_SCREENSHOTS==='1'&&process.env.CI!=='true'&&process.env.GITHUB_ACTIONS!=='true';
const before=process.env.I2_BEFORE==='1';
test.describe('I2 visual review @screenshots',()=>{
 test.skip(!localScreenshots,'Opt in locally with I2_SCREENSHOTS=1; screenshots are excluded from CI.');
 for(const width of [1440,390])for(const theme of ['light','dark'])for(const target of targets)test(`${target.name} ${width} ${theme}`,async({page})=>{
  const {dialog}=await open(page,target,width,theme);
  if(!before)await checkGeometry(dialog,width,1000);
  const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i2/screens');mkdirSync(output,{recursive:true});
  await page.screenshot({path:resolve(output,`${before?'before':'after'}-${target.name}-${width}-${theme}.png`),animations:'disabled'});
 });
});
