import {test,expect} from '@playwright/test';
import {u5PlaybackFixture} from './u5-fixture';
for(const width of [1440,390])test(`I28 playback first-read overlay avoids stacked 300px jump and refresh keeps form ${width}`,async({page})=>{
 await u5PlaybackFixture(page);await page.setViewportSize({width,height:1000});
 await page.addInitScript(()=>{(window as any).__i28CLS=0;new PerformanceObserver(list=>{for(const row of list.getEntries() as any[])if(!row.hadRecentInput)(window as any).__i28CLS+=row.value}).observe({type:'layout-shift',buffered:true})});
 let hold=true,reads=0;const releases:(()=>void)[]=[];
 await page.route('**/api/v1/media-server/native/settings',async r=>{reads++;if(hold)await new Promise<void>(resolve=>releases.push(resolve));return r.fallback()});
 const release=()=>{hold=false;releases.splice(0).forEach(f=>f())};
 try{
  await page.goto('/playback?tab=settings');const card=page.locator('.media-surface').first();
  await expect(card.locator('.f5-first-read')).toBeVisible();await expect(page.getByRole('button',{name:'保存服务设置',exact:true})).toBeDisabled();
  await page.waitForTimeout(180);release();await expect(card.locator('.f5-first-read')).toHaveCount(0);await expect(page.getByRole('textbox',{name:'内置媒体库服务名称',exact:true})).toHaveValue('家庭媒体');
  await page.waitForTimeout(250);const cls=await page.evaluate(()=>(window as any).__i28CLS);expect(cls).toBeLessThan(.01);
  const before=await card.boundingBox(),n=reads;hold=true;await page.getByRole('button',{name:'刷新',exact:true}).click();await expect.poll(()=>reads).toBeGreaterThan(n);
  await expect(card.locator('.f5-first-read')).toHaveCount(0);expect(await card.boundingBox()).toEqual(before);release();await expect(page.getByRole('button',{name:'刷新',exact:true})).toBeEnabled();expect(await card.boundingBox()).toEqual(before);
  console.log(`I28 playback CLS ${width}: ${cls}`);
 }finally{release()}
});
