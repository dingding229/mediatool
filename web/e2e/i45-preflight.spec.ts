import {test,expect} from '@playwright/test';
import {resolve} from 'node:path';
import {mkdirSync} from 'node:fs';
import {i42cFixture} from './i42c-fixture';
import {waitForLayout} from './layout-stability';
for(const width of [1440,375])test(`I45 plugin heading keeps refresh alongside title ${width}`,async({page})=>{
 await page.setViewportSize({width,height:1000});const f=await i42cFixture(page);
 const torrent=f.instances.find(i=>i.pluginId==='torrent-assistant')!;
 await page.goto('/plugins/'+torrent.id);
 await expect(page.getByText('任务数为最近一次后台检查的任务数量，不含页面查看。',{exact:true})).toBeVisible();
 await expect(page.getByText('附加标签（保留原有归属标签）',{exact:true})).toBeVisible();
 await waitForLayout(page);
 const header=page.locator('.plugin-detail-heading'),title=(await header.locator('h1').boundingBox())!,refresh=(await header.getByRole('button',{name:'刷新',exact:true}).boundingBox())!;
 expect(title.height).toBeLessThanOrEqual(48);
 expect(title.width).toBeGreaterThanOrEqual(160);
 expect(refresh.y).toBeLessThan(title.y+title.height);
 expect(refresh.y+refresh.height).toBeGreaterThan(title.y);
 expect(refresh.x).toBeGreaterThanOrEqual(title.x+title.width);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
 if(process.env.I45_SCREENSHOTS==='1'){
  const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i45/screens');mkdirSync(dir,{recursive:true});
  await page.screenshot({path:resolve(dir,`plugin-detail-${width}-light.png`),fullPage:true,animations:'disabled'});
 }
});
