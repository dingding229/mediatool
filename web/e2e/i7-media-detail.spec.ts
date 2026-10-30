import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync} from 'node:fs';
import {basename,extname,resolve} from 'node:path';
import {u4Fixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i7/screens');
const poster='/api/v1/media/tmdb/image?path=/i7-poster.svg';
const movie={id:'i7-movie',source:'managed',title:'海边的故事',year:'2026',mediaType:'movie',type:'Movie',poster,rating:7.7,size:5483098130,resolutions:[{width:1920,height:1080}],category:'电影 / 华语电影',assetCount:1,overview:'一次与故乡的重逢，让平凡的生活有了新的方向。保留已有媒体资料、文件和字幕管理能力。'};
const series={...movie,id:'i7-series',title:'山海之间',mediaType:'series',type:'Series',aggregate:true,assetCount:12,category:'电视剧 / 国产剧',overview:'山海之间，一群年轻人在各自的旅程中相遇。按已入库的季与集展示，不代表整部剧集已收齐。'};
const season={...series,id:'i7-season',title:'第一季',type:'Season',season:1};
const episode={...series,id:'i7-episode',title:'重逢',type:'Episode',season:1,episode:1,assetCount:1};
async function fixture(page:Page){
 await u4Fixture(page);
 await page.route(/\/api\/v1\/media\/tmdb\/image/,r=>r.fulfill({contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="320" height="480"><rect width="320" height="480" fill="#284b63"/><path d="M0 300Q160 220 320 300V480H0Z" fill="#386f82"/><text x="160" y="180" text-anchor="middle" fill="white" font-size="34">海边的故事</text><text x="160" y="230" text-anchor="middle" fill="white" font-size="17">MEDIA DETAIL FIXTURE</text></svg>'}));
 await page.route('**/api/v1/media/library?*',r=>{const parent=new URL(r.request().url()).searchParams.get('parentId');const items=parent===series.id?[season]:parent===season.id?[episode]:[movie,series];return fixtureFulfill(r,{json:{data:{items,total:items.length,categories:[]}}})});
 await page.route('**/api/v1/media/library/managed/*',r=>fixtureFulfill(r,{json:{data:[movie,series,season,episode].find(i=>r.request().url().endsWith('/'+i.id))}}));
 await page.route('**/api/v1/media/library/managed/*/metadata{,?*}',r=>{const id=new URL(r.request().url()).pathname.split('/')[6],item=id===series.id?series:movie;return fixtureFulfill(r,{json:{data:{target:{mediaId:id,kind:item.mediaType,season:-1,episode:-1},targetKey:id,revision:1,base:{title:item.title,year:2026},effective:{title:item.title,year:2026},overrides:{},matched:true,unmatched:[],choices:[]}}})});
 await page.route('**/api/v1/media/library/managed/*/images',r=>fixtureFulfill(r,{json:{data:{target:{mediaId:'root',kind:'movie',season:-1,episode:-1},targetKey:'root',revision:1,base:{},effective:{},overrides:{},matched:true,unmatched:[],choices:[]}}}));
 await page.route('**/api/v1/media/assets/*/subtitles',r=>{const id=new URL(r.request().url()).pathname.split('/')[5];return fixtureFulfill(r,{json:{data:{assetId:id,versions:[{id,name:'1080p · 原始物理版本'}],items:[]}}})});
}
async function open(page:Page,title:string){await page.getByRole('button',{name:title,exact:true}).click();const d=page.getByRole('dialog',{name:'作品详情',exact:true});await expect(d).toBeVisible();await expect(d.getByRole('heading',{name:'作品简介',exact:true})).toBeVisible();return d;}
test('I7 movie and series detail: title actions, exclusive editors, retained draft and season navigation',async({page})=>{
 await fixture(page);const writes:string[]=[],errors:string[]=[];page.on('request',r=>{if(r.url().includes('/api/')&&r.method()!=='GET')writes.push(r.url())});page.on('pageerror',e=>errors.push(e.message));
 await page.goto('/library');
 for(const item of [movie,series]){
  const d=await open(page,item.title);await expect(d.locator('.el-dialog')).toHaveCSS('width','1000px');await expect(d.locator('.catalog-detail-actions button')).toHaveCount(2);
  const row=await d.locator('.catalog-title-row').evaluate(el=>{const title=el.querySelector('h2')!.getBoundingClientRect(),actions=el.querySelector('.catalog-detail-actions')!.getBoundingClientRect();return {topDelta:Math.abs(title.top-actions.top),separated:actions.left>title.right}});expect(row.topDelta).toBeLessThan(2);expect(row.separated).toBe(true);
  const sections=d.locator('.catalog-detail-section>h3,.subtitle-panel>h3');expect(await sections.allTextContents()).toEqual(item===movie?['作品简介','版本与文件','字幕']:['作品简介','版本与文件']);
  await expect(d.locator('.metadata-editor')).toBeHidden();await expect(d.locator('.image-editor')).toBeHidden();
  await d.getByRole('button',{name:'编辑信息',exact:true}).click();await expect(d.locator('#metadata-title')).toHaveValue(item.title);await d.locator('#metadata-title').fill('未保存草稿');
  await d.getByRole('button',{name:'编辑图片',exact:true}).click();await page.getByRole('dialog',{name:'保留未保存草稿',exact:true}).getByRole('button',{name:'保留草稿并继续',exact:true}).click();await expect(d.locator('.image-row')).toHaveCount(3);await expect(d.locator('.metadata-editor')).toBeHidden();
  await d.getByRole('button',{name:'编辑信息',exact:true}).click();await expect(d.locator('#metadata-title')).toHaveValue('未保存草稿');await expect(d.locator('.image-editor')).toBeHidden();
  await d.getByRole('button',{name:'完成编辑',exact:true}).click();await page.getByRole('dialog',{name:'保留未保存草稿',exact:true}).getByRole('button',{name:'保留草稿并继续',exact:true}).click();await expect(d.locator('.metadata-editor')).toBeHidden();
  await expect(d.getByRole('heading',{name:'版本与文件',exact:true})).toBeVisible();
  if(item===series){await d.getByRole('button',{name:'第 1 季 · 第一季',exact:true}).click();await expect(d.getByRole('button',{name:'第 1 季 · 第 1 集 · 重逢',exact:true})).toBeVisible();await d.getByRole('button',{name:'返回季列表',exact:true}).click();await expect(d.getByRole('button',{name:'第 1 季 · 第一季',exact:true})).toBeVisible();}
  await d.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(d).toBeHidden();
 }
 expect(writes).toEqual([]);expect(errors).toEqual([]);
});
test('I7 mobile detail stays fullscreen and editor body follows the poster information',async({page})=>{
 await page.setViewportSize({width:390,height:1000});await fixture(page);await page.goto('/library');const d=await open(page,movie.title);
 await expect.poll(()=>d.evaluate(el=>{const r=el.getBoundingClientRect();return Math.round(r.width)===390&&r.left>=0&&el.scrollWidth<=el.clientWidth})).toBe(true);
 await expect(d.locator('.catalog-detail-actions button').first()).toHaveCSS('min-height','44px');
 await d.getByRole('button',{name:'编辑信息',exact:true}).click();await expect(d.locator('#metadata-title')).toBeVisible();const before=await d.locator('.catalog-summary').boundingBox();const editor=await d.locator('.metadata-editor').boundingBox();expect(editor!.y).toBeGreaterThanOrEqual(before!.y+before!.height-1);
 await page.keyboard.press('Escape');await expect(d).toBeHidden();
});
const capture=process.env.I7_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const before=process.env.I7_BEFORE==='1';
test.describe('I7 local screenshot evidence @screenshots',()=>{
 test.skip(!capture,'Local explicit opt-in only: I7_SCREENSHOTS=1');
 for(const item of [movie,series])for(const width of [1440,390])for(const theme of ['light','dark'])test(`${item.mediaType} ${width} ${theme}`,async({page})=>{
  await fixture(page);await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  if(before){const root=resolve(output,'../before-assets');await page.route('**/library',r=>r.fulfill({contentType:'text/html',body:readFileSync(resolve(root,'index.html'))}));await page.route('**/assets/*',r=>{const name=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(name)==='.js'?'application/javascript':'text/css',body:readFileSync(resolve(root,'assets',name))})});}
  await page.goto('/library');const d=await open(page,item.title);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);await expect(d.locator('.catalog-summary>img')).toBeVisible();await expect.poll(()=>d.locator('.catalog-summary>img').evaluate((img:HTMLImageElement)=>img.naturalWidth)).toBeGreaterThan(0);
  await expect.poll(()=>d.evaluate(el=>{const r=el.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&el.scrollWidth<=el.clientWidth})).toBe(true);
  mkdirSync(output,{recursive:true});await page.screenshot({path:resolve(output,`${before?'before':'after'}-${item.mediaType}-${width}-${theme}.png`),animations:'disabled'});
 });
});
