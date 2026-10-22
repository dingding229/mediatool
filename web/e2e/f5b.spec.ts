import {test,expect,type Page} from '@playwright/test';
import {u6OpsFixture} from './u6-fixture';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
const dir=resolve(import.meta.dirname,'../../data/private/f5/f5b');mkdirSync(dir,{recursive:true});
export async function f5bShots(page:Page,name:string){for(const [mode,width,height] of [['desktop',1440,1000],['narrow',390,844]] as const){await page.setViewportSize({width,height});await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(dir,name+'-'+mode+'.png'),fullPage:true,animations:'disabled'})}}
test('F5b SQL-page client restores URL before loading and keeps filters through last/first/reload',async({page})=>{
 await u6OpsFixture(page);const reads:number[]=[];
 await page.route('**/api/v1/downloads?*',r=>{const n=Number(new URL(r.request().url()).searchParams.get('page'));reads.push(n);return r.fulfill({json:{data:{items:[{id:'download-'+n,title:'第 '+n+' 页任务',state:'imported',progress:1}],total:53,page:n,pageSize:25}}})});
 await page.route('**/api/v1/transfers?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));
 await page.goto('/downloads?tab=tasks&downloadsPage=2&keep=owned');await expect(page.getByRole('heading',{name:'第 2 页任务'})).toBeVisible();expect(reads[0]).toBe(2);
 const pager=page.getByRole('navigation',{name:'分页',exact:true});await expect(pager).toContainText('26 – 50 / 53');await pager.getByRole('button',{name:'末页',exact:true}).click();await expect(page.getByRole('heading',{name:'第 3 页任务'})).toBeVisible();await expect(page).toHaveURL(/downloadsPage=3/);await expect(page).toHaveURL(/keep=owned/);
 await page.reload();await expect(page.getByRole('heading',{name:'第 3 页任务'})).toBeVisible();await pager.getByRole('button',{name:'首页',exact:true}).click();await expect(page.getByRole('heading',{name:'第 1 页任务'})).toBeVisible();await f5bShots(page,'downloads');
});
