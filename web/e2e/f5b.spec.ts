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

test('F5b compact scan rows, expandable directory evidence and local history posters',async({page})=>{
 const {organizeFixture}=await import('./u4-fixture');await organizeFixture(page);
 await page.route('**/api/v1/library/scans?*',r=>r.fulfill({json:{data:{items:[{id:'scan-owned',root:'/115/媒体/电影',state:'running',files:24,directories:8,completedDirectories:3,updatedAt:'2026-10-23T00:00:00Z'}],total:1}}}));
 await page.route('**/api/v1/library/scans/scan-owned?*',r=>r.fulfill({json:{data:{scan:{id:'scan-owned',root:'/115/媒体/电影',state:'running',directories:1},directories:[{path:'/115/媒体/电影/一部作品',state:'completed',updatedAt:'2026-10-23T00:00:00Z',message:'目录已完整覆盖'}]}}}));
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[{id:'history-owned',kind:'file',state:'succeeded',media:{title:'一部作品',year:'2026'},poster:'/owned.jpg',source:'/原目录/一部作品.mkv',destination:'/电影/一部作品.mkv',category:'电影',updatedAt:'2026-10-23T00:00:00Z',details:{completedSteps:1}}],total:1}}}));
 await page.route('**/api/v1/metadata/image/w185/owned.jpg',r=>r.fulfill({contentType:'image/png',body:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a0YsAAAAASUVORK5CYII=','base64')}));
 await page.goto('/organize?tab=scans');await expect(page.locator('.f5-scan-row')).toHaveCount(1);expect((await page.locator('.f5-scan-row').boundingBox())!.height).toBeLessThan(150);await f5bShots(page,'scan-compact');
 await page.getByRole('button',{name:'查看目录记录',exact:true}).click();const dialog=page.getByRole('dialog',{name:'扫描目录覆盖'});await dialog.locator('.f5-directory-row summary').click();await expect(dialog.getByText('完整路径：/115/媒体/电影/一部作品',{exact:true})).toBeVisible();await f5bShots(page,'directory-expanded');
 await page.goto('/organize?tab=history');const cover=page.locator('.f5-history-cover');await expect(cover).toBeVisible();expect((await cover.boundingBox())!.width).toBe(44);await expect(cover.locator('img')).toHaveAttribute('loading','lazy');await expect(cover.locator('img')).toHaveAttribute('src',/\/api\/v1\/metadata\/image\/w185\/owned.jpg/);await expect(page.locator('.f5-history-row')).not.toContainText('原位置');await f5bShots(page,'history-compact');await page.getByRole('button',{name:'查看步骤详情'}).click();await expect(page.getByRole('dialog',{name:'整理步骤详情'})).toContainText('原位置：/原目录/一部作品.mkv');await f5bShots(page,'history-details');
});
