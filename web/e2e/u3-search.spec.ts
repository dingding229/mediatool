import {test,expect} from '@playwright/test'
import {u3Fixture,u3Media,u3Run} from './u3-fixture'
test.beforeEach(async({page})=>u3Fixture(page))
test('U3 media selection is explicit before site search, and acceptance is not a finished result',async({page})=>{
 let starts=0,body:any;await page.route('**/api/v1/search',r=>{if(r.request().method()==='POST'){starts++;body=r.request().postDataJSON();return r.fulfill({status:202,json:{data:{id:'run-u3'}}})}return r.fulfill({json:{data:[u3Run]}})})
 await page.route('**/api/v1/search/run-u3?*',r=>r.fulfill({json:{data:{...u3Run,state:'pending',resources:[],sites:[{...u3Run.sites[0],state:'pending'}],total:0}}}))
 await page.goto('/search');await page.getByLabel('TMDB媒体名称',{exact:true}).fill(u3Media.title);await page.getByRole('button',{name:'搜索媒体',exact:true}).click();expect(starts).toBe(0);await page.getByRole('button',{name:u3Media.title,exact:true}).click();expect(starts).toBe(0);await page.getByRole('button',{name:'搜索站点资源',exact:true}).click();await expect(page).toHaveURL(/\/search\/results\/run-u3/);expect(body.media).toEqual({id:42,mediaType:'movie'});await expect(page.getByText('站点正在查询…',{exact:true})).toBeVisible();expect(starts).toBe(1)
})
test('U3 history is read-only, query-synchronized, and links cache maintenance to ops',async({page})=>{
 let mutations=0;page.on('request',r=>{if(r.url().includes('/cache/')&&r.method()!=='GET')mutations++})
 await page.goto('/search?tab=history');await expect(page.getByRole('heading',{name:'继续上次的搜索',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'删除',exact:true})).toHaveCount(0);await expect(page.getByRole('link',{name:'前往缓存管理',exact:true})).toHaveAttribute('href','/ops?tab=cache&cacheKind=search');await page.getByRole('button',{name:'查看记录详情',exact:true}).click();await expect(page.getByRole('dialog',{name:'搜索记录详情'})).toBeVisible();await expect(page.locator('.technical-json')).toHaveCount(0);expect(mutations).toBe(0)
})
test('U3 result filters cover the full snapshot and ranking is sent to the server',async({page})=>{
 let sort='';await page.route('**/api/v1/search/run-u3?*',r=>{sort=new URL(r.request().url()).searchParams.get('sort')||'';return r.fulfill({json:{data:u3Run}})})
 await page.goto('/search/results/run-u3');await page.getByRole('button',{name:'促销',exact:true}).click();await expect.poll(()=>sort).toBe('promotion_desc');await page.getByRole('button',{name:'促销',exact:true}).click();await expect.poll(()=>sort).toBe('promotion_asc');await expect(page.locator('.promotion-badge')).toHaveText('免费')
})
test('U3 download 202 remains queued and a second click cannot submit the immutable snapshot again',async({page})=>{
 let posts=0;await page.route('**/api/v1/downloads',r=>{if(r.request().method()==='POST'){posts++;expect(r.request().postDataJSON()).toMatchObject({searchRunId:'run-u3',resourceId:'resource-u3',tmdbId:42,mediaType:'movie'});return r.fulfill({status:202,json:{data:{id:'task-u3'}}})}return r.fulfill({json:{data:[]}})})
 await page.goto('/search/results/run-u3');await page.getByRole('button',{name:'确认并下载',exact:true}).click();const dialog=page.getByRole('dialog',{name:'确认媒体身份与下载'});await dialog.getByRole('button',{name:'创建下载任务',exact:true}).click();await expect(dialog.locator('.async-result-banner')).toContainText('下载已排队，尚未确认下载器接收');await expect(dialog.getByRole('button',{name:'创建下载任务',exact:true})).toHaveCount(0);expect(posts).toBe(1)
})
test('U3 unknown download outcome persists through reload and only permits readback',async({page})=>{
 let posts=0,reads=0;await page.route('**/api/v1/downloads',r=>{if(r.request().method()==='POST'){posts++;return r.abort()}reads++;return r.fulfill({json:{data:[]}})})
 await page.goto('/search/results/run-u3');await page.getByRole('button',{name:'确认并下载',exact:true}).click();await page.getByRole('button',{name:'创建下载任务',exact:true}).click();await expect(page.locator('.async-result-banner')).toContainText('回执未知');await page.reload();await page.getByRole('button',{name:'确认并下载',exact:true}).click();await expect(page.locator('.async-result-banner')).toContainText('回执未知');await page.locator('.async-result-banner').getByRole('button',{name:'核对结果',exact:true}).click();expect(posts).toBe(1);expect(reads).toBe(1)
})

test('U3 search and results screenshots fit 375 and 1440 in both themes',async({page})=>{
 const {mkdirSync}=await import('node:fs');const dir='/tmp/media-tool-u3-screenshots';mkdirSync(dir,{recursive:true});const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='warning'&&m.text().includes('Failed to resolve component'))errors.push(m.text())})
 for(const width of [375,1440])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:width===375?950:1000});await page.addInitScript(value=>localStorage.setItem('media-tool:theme-mode:v1',value),theme)
  await page.goto('/search');await page.getByLabel('TMDB媒体名称',{exact:true}).fill(u3Media.title);await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await page.getByRole('button',{name:u3Media.title,exact:true}).click();await expect(page.locator('html')).toHaveAttribute('data-theme',theme);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:`${dir}/search-${width}-${theme}.png`,fullPage:true,animations:'disabled'})
  await page.goto('/search/results/run-u3');await expect(page.locator('.resource-entry')).toHaveCount(1);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);for(const button of await page.locator('.acquire-workspace button:visible').all())expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44);await page.screenshot({path:`${dir}/search-results-${width}-${theme}.png`,fullPage:true,animations:'disabled'})
 }
 expect(errors).toEqual([])
})
