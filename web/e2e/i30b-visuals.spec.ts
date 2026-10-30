import {test,expect} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {u3Fixture,u3Media} from './u3-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i30b/screens');
const capture=process.env.I30B_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
for(const width of [1440,375])test(`I30b search helpers stay inside the form and share secondary typography ${width}`,async({page})=>{
 await u3Fixture(page);await page.setViewportSize({width,height:1000});await page.addInitScript(()=>localStorage.setItem('media-tool:theme-mode:v1','light'));
 await page.goto('/search');await page.getByLabel('TMDB媒体名称',{exact:true}).fill(u3Media.title);await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByRole('button',{name:`搜索《${u3Media.title}》(${u3Media.year}) 的站点资源`,exact:true})).toBeEnabled();
 const first=page.locator('.acquire-surface').first(),scope=first.locator('.search-site-scope'),note=first.locator('.search-helper-text');await expect(scope).toContainText('固定搜索全部 1 个已启用站点。');await expect(note).toHaveText('本次已查询TMDB');
 const typography=async()=>({note:await note.evaluate(e=>{const s=getComputedStyle(e);return {font:s.fontSize,color:s.color}}),scope:await scope.evaluate(e=>{const s=getComputedStyle(e);return {font:s.fontSize,color:s.color}})});
 const styles=await typography();expect(styles.note).toEqual(styles.scope);expect(styles.note.font).toBe('13px');
 const parent=(await first.boundingBox())!,box=(await scope.boundingBox())!,text=(await note.boundingBox())!,button=(await page.getByRole('button',{name:'搜索媒体',exact:true}).boundingBox())!;
 expect(box.x).toBeGreaterThanOrEqual(parent.x);expect(box.x+box.width).toBeLessThanOrEqual(parent.x+parent.width);expect(box.y+box.height).toBeLessThanOrEqual(parent.y+parent.height);
 if(width===1440)expect(Math.abs(text.y+text.height/2-button.y-button.height/2)).toBeLessThanOrEqual(1);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
 if(capture){mkdirSync(dir,{recursive:true});writeFileSync(resolve(dir,`search-helper-${width}.json`),JSON.stringify({styles,parent,box,text,button},null,2));await page.screenshot({path:resolve(dir,`after-media-${width}-light.png`),fullPage:true,animations:'disabled'})}
 await page.getByRole('button',{name:'站点直搜',exact:true}).click();await expect(scope).toContainText('固定搜索全部 1 个已启用站点。');expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
 if(capture)await page.screenshot({path:resolve(dir,`after-site-${width}-light.png`),fullPage:true,animations:'disabled'});
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[]}}));await page.reload();await expect(page.locator('.acquire-surface').first().getByRole('link',{name:'配置站点账号',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'开始搜索',exact:true})).toHaveCount(0);
});
