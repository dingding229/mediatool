import {test,expect,type Page} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {u4Fixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
const dir=resolve(process.cwd(),'../scratchpad/t2');
const categories=[{id:'r:movie:west',name:'电影 / 欧美电影',group:'电影'},{id:'r:movie:other',name:'电影 / 其他电影',group:'电影'}];
const media=[{id:'fixture-a',source:'managed',title:'一生所爱',year:'2026',mediaType:'movie',type:'Movie',rating:6.681,size:7960117968,resolutions:[{width:1920,height:872}],categoryId:categories[0]!.id,category:categories[0]!.name,overview:'字幕与播放链路保持独立。',assetCount:1},{id:'fixture-b',source:'managed',title:'大濛',year:'2025',mediaType:'movie',type:'Movie',rating:7.7,size:5483098130,resolutions:[{width:1920,height:1080}],categoryId:categories[1]!.id,category:categories[1]!.name,overview:'读取已持久化的媒体信息。',assetCount:1}];
async function fixture(page:Page){
 await u4Fixture(page);
 await page.route('**/api/v1/media/library?*',r=>{const q=new URL(r.request().url()).searchParams;const category=q.get('categoryId');const items=media.filter(m=>!category||m.categoryId===category);return fixtureFulfill(r,{json:{data:{items,total:items.length,categories}}})});
 await page.route('**/api/v1/media/library/managed/*',r=>fixtureFulfill(r,{json:{data:media.find(m=>r.request().url().endsWith(m.id))}}));
 await page.route('**/api/v1/media/assets/*/subtitles',r=>{const id=new URL(r.request().url()).pathname.split('/')[5];return fixtureFulfill(r,{json:{data:{assetId:id,versions:[{id,name:'当前物理版本'}],items:[0,1].map(i=>({id:`embedded-${i}`,assetId:id,source:'embedded',filename:'内嵌字幕',language:i?'eng':'chs',title:i?'英文':'简体中文',extension:'srt',revision:1}))}}})});
}
test('T2 real detail mapping, configured category filter and one embedded subtitle notice',async({page})=>{
 await fixture(page);const obsolete:string[]=[];page.on('request',r=>{if(/\/media\/assets\/[^/]+\/strm/.test(r.url()))obsolete.push(r.url())});
 await page.goto('/library');await expect(page.locator('.catalog-poster')).toHaveCount(2);
 for(const [title,rating,size,dimensions,category]of [['一生所爱','6.7','7.41 GB','1080p（1920×872）','电影 / 欧美电影'],['大濛','7.7','5.11 GB','1080p（1920×1080）','电影 / 其他电影']]){
  await page.getByRole('button',{name:title,exact:true}).click();const d=page.getByRole('dialog',{name:title,exact:true});
  for(const value of [rating,size,dimensions,category])await expect(d.locator('dd').filter({hasText:value!})).toHaveCount(1);
  await expect(d.getByText('内嵌字幕随视频播放，仅编辑显示信息，不提供提取或删除。',{exact:true})).toHaveCount(1);
  await expect(d.getByRole('button',{name:/复制播放链接|获取播放链接/})).toHaveCount(0);
  await expect(d.locator('video,audio,iframe')).toHaveCount(0);await d.getByRole('button',{name:'关闭此对话框',exact:true}).click();
 }
 await page.locator('.catalog-filters .el-select').click();await page.getByRole('option',{name:'电影 / 欧美电影',exact:true}).click();
 await expect(page.locator('.catalog-poster')).toHaveCount(1);await expect(page.getByRole('button',{name:'大濛',exact:true})).toHaveCount(0);
 expect(obsolete).toEqual([]);
});
test('T2 screenshot matrix at 1440 and 390 in both themes without horizontal overflow',async({page})=>{
 test.skip(process.env.T2_SCREENSHOTS!=='1'||!!process.env.CI||!!process.env.GITHUB_ACTIONS,'Local explicit screenshot opt-in only');
 await fixture(page);mkdirSync(dir,{recursive:true});
 for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1100});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  await page.goto('/library');await expect(page.locator('.catalog-poster')).toHaveCount(2);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await page.screenshot({path:`${dir}/after-local-library-${width}-${theme}.png`,animations:'disabled'});
  await page.getByRole('button',{name:'一生所爱',exact:true}).click();const d=page.getByRole('dialog',{name:'一生所爱',exact:true});
  await expect(d.getByText('1080p（1920×872）',{exact:true})).toBeVisible();await expect(d.locator('.subtitle-row')).toHaveCount(2);
  // Enter transitions move the drawer from offscreen; verify its settled bounds, not a single animation frame.
  await expect.poll(()=>d.evaluate(el=>el.getBoundingClientRect().left>=0&&el.getBoundingClientRect().right<=innerWidth)).toBe(true);
  // Missing posters must not leave the summary trapped in the 120px image column.
  expect(await d.locator('.catalog-summary dl').evaluate(el=>el.getBoundingClientRect().width)).toBeGreaterThan(250);
  expect(await d.locator('dt').evaluateAll(els=>els.every(el=>el.getBoundingClientRect().height<32))).toBe(true);
  const layout=await d.evaluate(el=>({left:el.getBoundingClientRect().left,right:el.getBoundingClientRect().right,width:innerWidth,overflow:el.scrollWidth>el.clientWidth}));
  expect(layout.left).toBeGreaterThanOrEqual(0);expect(layout.right).toBeLessThanOrEqual(layout.width);expect(layout.overflow).toBe(false);
  await page.screenshot({path:`${dir}/after-local-detail-${width}-${theme}.png`,animations:'disabled'});
 }
});
