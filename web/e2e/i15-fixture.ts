import {expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';

export const screenDir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i15/screens');
export const capture=process.env.I15_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
export const before=process.env.I15_BEFORE==='1';
export const screens=[1440,390].flatMap(width=>['light','dark'].map(theme=>({width,theme})));
export async function prepareScreen(page:Page,width:number,theme:string,path:string){
 if(before){
  const root=resolve(screenDir,'../before-assets');
  await page.route('**'+path+'{,?*}',r=>r.request().resourceType()==='document'
   ?r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}):r.fallback());
  await page.route('**/assets/*',r=>{const file=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(file)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',file))})});
 }
 await page.setViewportSize({width,height:1000});
 await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
 await page.goto(path);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
}
export async function shot(page:Page,name:string,width:number,theme:string){
 mkdirSync(screenDir,{recursive:true});
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
 await page.screenshot({path:join(screenDir,`${before?'before':'after'}-${name}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
}
