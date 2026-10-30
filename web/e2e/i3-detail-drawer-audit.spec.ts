import {test,expect,type Page,type Locator} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {i3Fixture} from './i3-fixture';

import {targets,navigate} from './i36-audit-targets';
const representative=new Set(['media','users','log','accounts','history','external']);
async function geometry(dialog:Locator,width:number,expected:number){
 const panel=dialog.locator('.el-dialog.detail-dialog');
 await expect.poll(()=>panel.evaluate((el,expected)=>{const r=el.getBoundingClientRect();return innerWidth<600?Math.abs(r.x)<1&&Math.abs(r.y)<1&&Math.abs(r.width-innerWidth)<1&&Math.abs(r.height-innerHeight)<1:r.width===expected&&r.height<=innerHeight*.85+1&&Math.abs(r.x-(innerWidth-r.width)/2)<2&&Math.abs(r.y-(innerHeight-r.height)/2)<2},expected)).toBe(true);
 await expect(panel.locator('.el-dialog__body')).toHaveCSS('overflow-y','auto');
 expect(await panel.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);
 if(width===390)expect(await panel.evaluate(el=>el.getBoundingClientRect().height)).toBe(1000);
}

async function checkFooterAlignment(dialog:Locator){
 const footerRow=dialog.locator('.el-dialog__footer');
 await expect.poll(()=>footerRow.evaluate(el=>{const style=getComputedStyle(el),row=el.getBoundingClientRect(),wrap=el.querySelector('.detail-footer')!.getBoundingClientRect(),button=el.querySelector('.detail-footer button:last-child')!.getBoundingClientRect(),left=row.left+el.clientLeft+parseFloat(style.paddingLeft),right=row.left+el.clientLeft+el.clientWidth-parseFloat(style.paddingRight);return Math.abs(wrap.left-left)<1&&Math.abs(wrap.right-right)<1&&Math.abs(button.right-right)<1})).toBe(true);
}

// One browser context/page and mocked authenticated session for the whole audit; no retries or timeout changes.
let shared:Page;
test.beforeAll(async({browser})=>{shared=await browser.newPage({viewport:{width:1440,height:1000}})});
test.afterAll(async()=>{await shared.close()});
for(const group of ['/search','/downloads','/files','/ops','/organize','/playback','/sites','/library','/plugins','/subscriptions','/settings'])test(`I3 behavior ${group}`,async()=>{
 for(const t of targets.filter(t=>t.path.split('?')[0]===group)){
  await test.step(t.id,async()=>{
   const dialog=await navigate(shared,t);
   if(representative.has(t.id)){
    await geometry(dialog,1440,t.size==='default'?800:1000);
    if(t.id==='external'){await checkFooterAlignment(dialog);await expect(dialog.locator('.el-dialog__footer')).toBeVisible();await expect(dialog.getByRole('button',{name:'刷新记录',exact:true})).toBeVisible();await shared.mouse.click(2,2);await expect(dialog).toBeVisible()}
    else await expect(dialog.locator('.el-dialog__footer')).toHaveCount(0);
   }
   await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(dialog).toBeHidden();
  });
 }
});

test('I3 behavior mobile representatives and footer',async()=>{
 await shared.setViewportSize({width:390,height:1000});
 for(const id of ['media','users','log','accounts','history','external']){
  const t=targets.find(t=>t.id===id)!;
  await test.step(id,async()=>{
   const dialog=await navigate(shared,t);await geometry(dialog,390,t.size==='default'?800:1000);
   if(id==='external'){
    const footer=dialog.locator('.detail-footer');await expect(footer).toHaveCSS('justify-content','flex-end');
    await checkFooterAlignment(dialog);
    const header=dialog.locator('.el-dialog__header'),body=dialog.locator('.el-dialog__body');
    const top=(await header.boundingBox())!.y,bottom=(await footer.boundingBox())!.y;
    expect(await body.evaluate(el=>el.scrollHeight>el.clientHeight)).toBe(true);
    await body.evaluate(el=>{el.scrollTop=el.scrollHeight});
    expect(Math.abs((await header.boundingBox())!.y-top)).toBeLessThan(1);
    expect(Math.abs((await footer.boundingBox())!.y-bottom)).toBeLessThan(1);
    await dialog.getByRole('button',{name:'关闭详情',exact:true}).click();
   }else await shared.keyboard.press('Escape');
   await expect(dialog).toBeHidden();
  });
 }
 await shared.setViewportSize({width:1440,height:1000});
});

const capture=process.env.I3_SCREENSHOTS==='1'&&process.env.CI!=='true'&&process.env.GITHUB_ACTIONS!=='true';
const before=process.env.I3_BEFORE==='1';
const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i3/screens');
const mobile=new Set(['media','users','log','accounts','history','external']);
test.describe('I3 visual review @screenshots',()=>{
 test.skip(!capture,'Local opt-in only: I3_SCREENSHOTS=1');
 for(const t of targets)for(const width of [1440,...(mobile.has(t.id)?[390]:[])])for(const theme of ['light','dark'])test(`${t.id} ${width} ${theme}`,async({page})=>{
  await page.setViewportSize({width,height:1000});await page.addInitScript(mode=>localStorage.setItem('media-tool:theme-mode:v1',mode),theme);
  await i3Fixture(page,t.path.split('?')[0]);const dialog=await navigate(page,t);
  if(!before)await geometry(dialog,width,t.size==='default'?800:1000);
  mkdirSync(output,{recursive:true});const path=resolve(output,`${before?'before':'after'}-${t.id}-${width}-${theme}.png`);
  await page.screenshot({path,animations:'disabled'});
  writeFileSync(path+'.json',JSON.stringify({file:t.file,title:t.title,size:t.size||'large',width,theme}));
 });
});
