import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {u3Fixture,u3Run,u3Resource} from './u3-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/t6');
async function fixture(page:Page){
 await u3Fixture(page);let siteCount=6,state='partial';const queries:string[]=[];
 await page.route('**/api/v1/search/run-u3?*',r=>{queries.push(new URL(r.request().url()).search);return r.fulfill({json:{data:{...u3Run,state,sites:Array.from({length:siteCount},(_,i)=>({accountId:'site-'+i,name:'测试站点 '+(i+1),state:i===0?'failed':'succeeded',count:i===0?0:3,message:i===0?'站点暂时不可用，请稍后重试':''})),resources:[7,123456,null].map((n,i)=>({...u3Resource,id:'resource-'+i,title:i===1?'一条较长的资源标题 用于核对换行之后各列仍与表头对齐 2026.1080p.WEB-DL.H264':'自有测试影片 2026.1080p.WEB-DL.H264',seeders:n,size:6528350289+i*1024,downloadFactor:[1,0,.25][i]})),total:3}}})});
 return {queries,setSites:(n:number)=>siteCount=n,setState:(s:string)=>state=s};
}
async function metrics(page:Page){return page.evaluate(()=>{
 const box=(el:Element)=>{const b=el.getBoundingClientRect();return {x:b.x,y:b.y,right:b.right,bottom:b.bottom,width:b.width,height:b.height}};
 const card=document.querySelector('.f3-site-summary')!;const last=Array.from(card.children).filter(el=>getComputedStyle(el).display!=='none').at(-1)!;
 return {summary:box(card),contentBottom:box(last).bottom,columns:Array.from(document.querySelector('.resource-columns')!.children).map(box),rows:Array.from(document.querySelectorAll('.resource-entry')).map(el=>Array.from(el.children).map(box)),headerTracks:getComputedStyle(document.querySelector('.resource-columns')!).gridTemplateColumns,rowTracks:getComputedStyle(document.querySelector('.resource-entry')!).gridTemplateColumns,noHorizontalScroll:document.documentElement.scrollWidth<=innerWidth};
})}
test('T6 search results compact status and aligned columns 1440/390 light/dark',async({page})=>{
 mkdirSync(dir,{recursive:true});await fixture(page);const before=process.env.T6_BEFORE==='1';
 if(before){const root=join(dir,'before-assets');await page.route('**/search/results/run-u3',r=>r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}));await page.route('**/assets/*',r=>{const n=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(n)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',n))})})}
 for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/search/results/run-u3');await expect(page.locator('.resource-entry')).toHaveCount(3);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  const m=await metrics(page);writeFileSync(join(dir,`${before?'before':'after'}-metrics-${width}-${theme}.json`),JSON.stringify(m,null,2));expect(m.noHorizontalScroll).toBe(true);
  if(!before){expect(m.summary.bottom-m.contentBottom).toBeLessThan(24);expect(m.headerTracks).toBe(m.rowTracks);
   for(const [i,head]of (await page.locator('.resource-columns [role=columnheader]').all()).entries()){
    const label=(await head.locator('.resource-sort-label').boundingBox())!,icon=(await head.locator('.f4-sort-icon').boundingBox())!;
    expect(Math.abs(label.y+label.height/2-icon.y-icon.height/2)).toBeLessThanOrEqual(1);
    for(const row of await page.locator('.resource-entry').all()){const value=(await row.locator(':scope > *').nth(i+1).boundingBox())!;expect(Math.abs(label.x+label.width-value.x-value.width)).toBeLessThanOrEqual(1)}
   }
   expect(await page.locator('.resource-entry > :nth-child(3)').allTextContents()).toEqual(['7','123456','—']);
  }
  const prefix=join(dir,`${before?'before':'after'}-${width}-${theme}`);
  await page.locator('.f3-site-summary').screenshot({path:prefix+'-status-collapsed.png',animations:'disabled'});
  await page.locator('.f3-stable-scroll').screenshot({path:prefix+'-results.png',animations:'disabled'});
  await page.getByRole('button',{name:/站点查询状态/}).click();await expect(page.locator('.f4-site-grid article')).toHaveCount(6);
  await page.locator('.f3-site-summary').screenshot({path:prefix+'-status-expanded.png',animations:'disabled'});
  if(!before){const expanded=await metrics(page);expect(expanded.summary.bottom-expanded.contentBottom).toBeLessThan(24);expect((await page.locator('.f4-site-grid').boundingBox())!.height).toBeLessThanOrEqual(240)}
 }
});
test('T6 site status stays bounded; sorting and download confirmation retain behavior',async({page})=>{
 const f=await fixture(page);await page.goto('/search/results/run-u3');await expect(page.locator('.resource-entry')).toHaveCount(3);
 const header=page.getByRole('columnheader',{name:'按做种人数排序',exact:true});await header.press('Enter');await expect(header).toHaveAttribute('aria-sort','ascending');await header.press('Space');await expect(header).toHaveAttribute('aria-sort','descending');expect(f.queries.some(q=>q.includes('seeders_asc'))).toBe(true);
 const toggle=page.getByRole('button',{name:/站点查询状态/});await toggle.click();const small=(await page.locator('.f3-site-summary').boundingBox())!.height;
 f.setSites(40);await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.locator('.f4-site-grid article')).toHaveCount(40);const many=(await page.locator('.f3-site-summary').boundingBox())!.height;expect(many).toBeGreaterThan(small);expect((await page.locator('.f4-site-grid').boundingBox())!.height).toBeLessThanOrEqual(240);
 await toggle.click();expect((await page.locator('.f3-site-summary').boundingBox())!.height).toBeLessThan(140);f.setState('succeeded');await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.locator('.f3-site-summary > p')).toHaveCount(0);expect((await page.locator('.f3-site-summary').boundingBox())!.height).toBeLessThan(100);
 await page.getByRole('button',{name:'确认并下载',exact:true}).first().click();await expect(page.getByRole('dialog')).toBeVisible();
});
