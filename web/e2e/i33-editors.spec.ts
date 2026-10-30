import {test,expect,type Page} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {u4Fixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
const screens=resolve(import.meta.dirname,'../../scratchpad/i-phase/i33/screens');
async function fixture(page:Page,series=false,unmatchedTotal=65){
 await u4Fixture(page);
 const item={id:'i33-asset',title:'海边的故事',source:'managed',mediaType:series?'series':'movie',type:series?'Series':'Movie',year:'2026',overview:'只读作品简介，应在编辑时隐藏',poster:'/api/v1/media/tmdb/image?path=poster',path:'/115/电影/海边的故事.mkv',rating:7.2,size:1000000000};
 const target={mediaId:'i33-root',kind:series?'series':'movie',season:-1,episode:-1};let revision=1,overrides:Record<string,unknown>={title:item.title};
 const base={title:item.title,originalTitle:'Seaside',sortName:item.title,year:2026,premiereDate:'2026-04-15',overview:item.overview,tagline:'从重逢开始。',genres:['剧情'],tags:['温情'],communityRating:7.2,officialRating:'PG-13'};
 const unmatched=Array.from({length:Math.min(100,unmatchedTotal)},(_,i)=>({...target,kind:'episode',season:1,episode:i+1}));
 const record=()=>({target,targetKey:'i33-root',revision,base,effective:{...base,...overrides},overrides,matched:true,choices:[target],unmatched,unmatchedTotal});
 const counts={detail:0,metadata:0,images:0,subtitles:0,outputs:0,history:0,writes:0};const queries:string[]=[];
 await page.route('**/api/v1/media/library?*',r=>fixtureFulfill(r,{json:{data:{items:new URL(r.request().url()).searchParams.get('parentId')?[]:[{...item,...record().effective}],total:1,categories:[]}}}));
 await page.route('**/api/v1/media/library/managed/i33-asset',r=>{counts.detail++;return fixtureFulfill(r,{json:{data:{...item,...record().effective}}})});
 await page.route('**/api/v1/media/library/managed/*/metadata{,?*}',r=>{if(r.request().method()==='PATCH'){counts.writes++;const body=r.request().postDataJSON();expect(body.revision).toBe(revision);expect(body.targetKey).toBe('i33-root');for(const key of body.restore)delete overrides[key];overrides={...overrides,...body.set};revision++;}else{counts.metadata++;queries.push(new URL(r.request().url()).search)}return fixtureFulfill(r,{json:{data:record()}})});
 const img={url:'/api/v1/media/tmdb/image?path=poster'};
 await page.route('**/api/v1/media/library/managed/i33-asset/images',r=>{if(r.request().method()!=='GET')counts.writes++;else counts.images++;return fixtureFulfill(r,{json:{data:{target,targetKey:'i33-root',revision:1,base:{Primary:img,Backdrop:img,Logo:img},effective:{Primary:img,Backdrop:img,Logo:img},overrides:{Primary:'owned'},matched:true,choices:[target],unmatched:[]}}})});
 await page.route('**/api/v1/media/tmdb/image?*',r=>r.fulfill({contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="200" height="300"><rect width="200" height="300" fill="#52777a"/><circle cx="100" cy="100" r="60" fill="#e9dac2"/></svg>'}));
 await page.route('**/api/v1/media/assets/*/subtitles',r=>{counts.subtitles++;return fixtureFulfill(r,{json:{data:{assetId:item.id,versions:[{id:item.id,name:'原始版本'}],items:[]}}})});
 await page.route('**/api/v1/media/assets?*',r=>fixtureFulfill(r,{json:{data:{items:[item],total:1}}}));
 await page.route('**/api/v1/library/outputs?*',r=>{counts.outputs++;return fixtureFulfill(r,{json:{data:{items:[{...item,outputs:[{kind:'strm',path:'owned.strm'}]}],total:1}}})});
 await page.route('**/api/v1/library/activity?*',r=>{counts.history++;return fixtureFulfill(r,{json:{data:{items:[],total:0}}})});
 return {counts,queries};
}
async function openDetail(page:Page){await page.goto('/library');await page.getByRole('button',{name:'海边的故事',exact:true}).click();return page.getByRole('dialog',{name:'作品详情',exact:true})}
async function settled(page:Page){await page.evaluate(async()=>{await document.fonts.ready;await new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve())))});}
for(const width of [1440,375])test(`I33 aligned editor rows and hidden read-only sections at ${width}`,async({page})=>{
 const f=await fixture(page);await page.setViewportSize({width,height:1000});const dialog=await openDetail(page);await dialog.getByRole('button',{name:'编辑信息',exact:true}).click();await expect(dialog.locator('.metadata-row')).toHaveCount(11);await settled(page);
 await expect(dialog.getByRole('region',{name:'作品简介',exact:true})).toBeHidden();await expect(dialog.getByRole('region',{name:'版本与文件',exact:true})).toBeHidden();await expect(dialog.getByRole('region',{name:'字幕管理',exact:true})).toBeHidden();await expect(dialog.getByRole('navigation')).toHaveCount(0);
 const geometry=await dialog.locator('.metadata-row').evaluateAll(rows=>rows.map(row=>{const input=row.querySelector('input,textarea')!.getBoundingClientRect(),button=row.querySelector('.metadata-restore')!.getBoundingClientRect();return {left:input.x,center:Math.abs(input.y+input.height/2-button.y-button.height/2),top:Math.abs(input.y-button.y),overview:!!row.querySelector('textarea')}}));
 if(width===1440){for(const row of geometry){expect(row.overview?row.top:row.center).toBeLessThanOrEqual(2);expect(Math.abs(row.left-geometry[0]!.left)).toBeLessThanOrEqual(1)}}
 expect(f.queries).toEqual(['?unmatchedPage=1&unmatchedPageSize=100']);await dialog.locator('.metadata-unmatched summary').click();await expect(dialog.locator('.metadata-unmatched-items button')).toHaveCount(65);expect(await dialog.locator('.metadata-unmatched-items').evaluate(e=>e.scrollHeight>e.clientHeight)).toBe(true);
 await dialog.getByRole('button',{name:'编辑图片',exact:true}).click();await expect(dialog.locator('.image-row')).toHaveCount(3);await expect(dialog.locator('.metadata-editor')).toHaveCount(0);await settled(page);
 if(width===1440){const images=await dialog.locator('.image-row').evaluateAll(rows=>rows.map(row=>{const upload=row.querySelector('.image-upload')!.getBoundingClientRect(),restore=row.querySelector('.image-restore')!.getBoundingClientRect();return Math.abs(upload.y+upload.height/2-restore.y-restore.height/2)}));for(const diff of images)expect(diff).toBeLessThanOrEqual(2)}
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);expect(await dialog.evaluate(e=>e.scrollWidth<=e.clientWidth)).toBe(true);
 const reads={...f.counts};await dialog.getByRole('button',{name:'完成编辑',exact:true}).click();await expect(dialog.getByRole('region',{name:'作品简介',exact:true})).toBeVisible();await expect(dialog.getByRole('region',{name:'版本与文件',exact:true})).toBeVisible();await expect(dialog.getByRole('region',{name:'字幕管理',exact:true})).toBeVisible();expect(f.counts).toEqual(reads);
 await dialog.getByRole('button',{name:'编辑信息',exact:true}).click();expect(f.counts).toEqual(reads);
});
test('I33 dirty switch/collapse confirms and retains draft; successful save restores details',async({page})=>{
 const f=await fixture(page);const dialog=await openDetail(page);await dialog.getByRole('button',{name:'编辑信息',exact:true}).click();await dialog.locator('#metadata-title').fill('保留的人工草稿');await dialog.getByRole('button',{name:'编辑图片',exact:true}).click();
 const confirm=page.getByRole('dialog',{name:'保留未保存草稿',exact:true});await expect(confirm).toBeVisible();await confirm.getByRole('button',{name:'继续编辑',exact:true}).click();await expect(dialog.locator('#metadata-title')).toHaveValue('保留的人工草稿');await expect(dialog.locator('.image-row')).toHaveCount(0);
 await dialog.getByRole('button',{name:'编辑图片',exact:true}).click();await confirm.getByRole('button',{name:'保留草稿并继续',exact:true}).click();await expect(dialog.locator('.image-row')).toHaveCount(3);await expect(dialog.getByRole('region',{name:'版本与文件',exact:true})).toBeHidden();
 await dialog.getByRole('button',{name:'编辑信息',exact:true}).click();await expect(dialog.locator('#metadata-title')).toHaveValue('保留的人工草稿');expect(f.counts.metadata).toBe(1);expect(f.counts.writes).toBe(0);
 await dialog.getByRole('button',{name:'完成编辑',exact:true}).click();await confirm.getByRole('button',{name:'继续编辑',exact:true}).click();await expect(dialog.locator('#metadata-title')).toBeVisible();
 await dialog.getByRole('button',{name:'保存信息',exact:true}).click();await expect(page.locator('.catalog-title-row h2')).toHaveText('保留的人工草稿');await expect(page.locator('.catalog-readonly')).toBeVisible();await expect(page.locator('.metadata-form')).toHaveCount(0);expect(f.counts.writes).toBe(1);
});
test('I33 version pagination is only hidden and returns without new child reads',async({page})=>{
 await fixture(page,true);let children=0;page.on('request',r=>{if(new URL(r.url()).searchParams.get('parentId'))children++});const dialog=await openDetail(page);await expect(dialog.getByRole('navigation',{name:'分页',exact:true})).toBeVisible();const before=children;
 await dialog.getByRole('button',{name:'编辑信息',exact:true}).click();await expect(dialog.getByRole('navigation',{name:'分页',exact:true})).toBeHidden();await dialog.getByRole('button',{name:'完成编辑',exact:true}).click();await expect(dialog.getByRole('navigation',{name:'分页',exact:true})).toBeVisible();expect(children).toBe(before);
});
test('I33 organize image editor hides outputs/history/raw details without a reread',async({page})=>{
 const f=await fixture(page);await page.goto('/organize?tab=assets');await page.getByRole('button',{name:'查看资产详情',exact:true}).click();const dialog=page.getByRole('dialog',{name:'资产与输出详情',exact:true});await expect(dialog.getByRole('heading',{name:'当前输出与 STRM',exact:true})).toBeVisible();
 await dialog.getByRole('button',{name:'编辑图片',exact:true}).click();await expect(dialog.locator('.image-row')).toHaveCount(3);await expect(dialog.getByRole('region',{name:'资产输出与操作记录',exact:true})).toBeHidden();await expect(dialog.locator('.technical-section')).toHaveCount(0);const before={...f.counts};
 await dialog.getByRole('button',{name:'完成编辑',exact:true}).click();await expect(dialog.getByRole('region',{name:'资产输出与操作记录',exact:true})).toBeVisible();await expect(dialog.locator('.technical-section')).toBeVisible();expect(f.counts).toEqual(before);
 await dialog.getByRole('button',{name:'编辑图片',exact:true}).click();await dialog.getByRole('button',{name:'恢复为刮削值',exact:true}).first().click();await expect(dialog.getByRole('region',{name:'资产输出与操作记录',exact:true})).toBeVisible();expect(f.counts.writes).toBe(1);
});
test.describe('I33 local screenshots @screenshots',()=>{
 test.skip(process.env.I33_SCREENSHOTS!=='1'||!!process.env.CI||!!process.env.GITHUB_ACTIONS,'Local explicit opt-in only');
 for(const width of [1440,375])test(`I33 information and images ${width} light`,async({page})=>{
  await fixture(page);mkdirSync(screens,{recursive:true});await page.setViewportSize({width,height:1000});await page.addInitScript(()=>localStorage.setItem('media-tool:theme-mode:v1','light'));const dialog=await openDetail(page);
  for(const mode of ['metadata','images']){await dialog.getByRole('button',{name:mode==='metadata'?'编辑信息':'编辑图片',exact:true}).click();await expect(dialog.locator(mode==='metadata'?'.metadata-row':'.image-row')).toHaveCount(mode==='metadata'?11:3);await settled(page);await page.screenshot({path:resolve(screens,`after-${mode}-${width}-light.png`),animations:'disabled'})}
 });
});

test('I33 single-read maximum is explicit rather than hiding unmatched records behind pagination',async({page})=>{
 const f=await fixture(page,false,130);const dialog=await openDetail(page);await dialog.getByRole('button',{name:'编辑信息',exact:true}).click();await dialog.locator('.metadata-unmatched summary').click();await expect(dialog.getByText('已列出前 100 项，共 130 项；接口单次最多 100 项。',{exact:true})).toBeVisible();await expect(dialog.locator('.metadata-unmatched-items button')).toHaveCount(100);await expect(dialog.getByRole('navigation')).toHaveCount(0);expect(f.queries).toEqual(['?unmatchedPage=1&unmatchedPageSize=100']);
});
