import {test,expect} from '@playwright/test';
import {u4Fixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
const entries=[{id:'film-west',title:'美国电影',type:'Movie',mediaType:'movie',categoryId:'r:movie:movie-west',category:'电影 / 欧美电影'},{id:'film-tw',title:'台湾电影',type:'Movie',mediaType:'movie',categoryId:'r:movie:movie-chinese',category:'电影 / 华语电影'},{id:'series_hk',title:'香港剧',type:'Series',mediaType:'series',categoryId:'r:series:series-cn',category:'电视剧 / 国产剧'}].map(v=>({...v,source:'managed',year:'2026'}));
test('T2b list, category filter and detail use the same classification for all three identities',async({page})=>{
 await u4Fixture(page);
 await page.route('**/api/v1/media/library?*',r=>{const q=new URL(r.request().url()).searchParams,category=q.get('categoryId');const items=q.get('parentId')?[]:entries.filter(v=>!category||v.categoryId===category);return fixtureFulfill(r,{json:{data:{items,total:items.length,categories:entries.map(v=>({id:v.categoryId,name:v.category,group:v.mediaType==='movie'?'电影':'电视剧'}))}}})});
 await page.route('**/api/v1/media/library/managed/*',r=>fixtureFulfill(r,{json:{data:entries.find(v=>r.request().url().endsWith(v.id))}}));
 await page.goto('/library');await expect(page.locator('.catalog-poster')).toHaveCount(3);
 for(const entry of entries){
  await expect(page.getByRole('button',{name:entry.title,exact:true})).toContainText(entry.category);
  await page.getByRole('button',{name:entry.title,exact:true}).click();const d=page.getByRole('dialog',{name:entry.title,exact:true});await expect(d.locator('dd').filter({hasText:entry.category})).toHaveCount(1);await d.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(d).toBeHidden();
  await page.locator('.catalog-filters .el-select').click();await page.getByRole('option',{name:entry.category,exact:true}).click();await expect(page.locator('.catalog-poster')).toHaveCount(1);await expect(page.locator('.catalog-poster')).toContainText(entry.title);
  await page.locator('.catalog-filters .el-select').click();await page.getByRole('option',{name:'全部类型',exact:true}).click();await expect(page.locator('.catalog-poster')).toHaveCount(3);
 }
});
