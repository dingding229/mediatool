import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {u6OpsFixture} from './u6-fixture';
const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i24');
const capture=process.env.I24_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
async function fixture(page:Page){
 await u6OpsFixture(page);const requests:number[]=[];
 await page.route('**/api/v1/ops/trends?*',r=>{
  requests.push(Number(new URL(r.request().url()).searchParams.get('hours')));
  return r.fulfill({json:{data:Array.from({length:12},(_,i)=>({
   at:new Date(Date.parse('2026-10-28T15:55:00Z')+(i+(i>=8?20:0))*60000).toISOString(),
   memoryPercent:i===5?null:40+i*1.2,diskFreeGB:25.38-i*.01,load1:.23+i*.01,loadPerCPU:.0575+i*.002,
   pendingJobs:0,processBytes:(64+i*.3)*1048576,failedJobs10m:i%3,
   jobSeconds:[1,6,10].includes(i)?1.234+i*.1:null,
   succeededJobs10m:[1,6,10].includes(i)?10+i:null,
  }))}});
 });return requests;
}
const chart=(page:Page,title:string)=>page.locator('.u6-trend').filter({has:page.getByRole('heading',{name:title,exact:true})});
const luminance=(c:string)=>c.match(/[\d.]+/g)!.slice(0,3).map(Number).map(n=>{const v=n/255;return v<=.04045?v/12.92:((v+.055)/1.055)**2.4}).reduce((s,v,i)=>s+v*[.2126,.7152,.0722][i]!,0);
const contrast=(a:string,b:string)=>{const x=[luminance(a),luminance(b)].sort((a,b)=>b-a);return(x[0]!+.05)/(x[1]!+.05)};
for(const width of [1440,390])for(const theme of ['light','dark'])test(`I24 readable axes, gaps and round isolated points ${width}/${theme}`,async({page})=>{
 const requests=await fixture(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
 await page.goto('/trends?hours=24');await expect(page.locator('.u6-trend')).toHaveCount(9);await expect(page.locator('.trend-y-axis span')).toHaveCount(27);await expect(page.locator('.trend-grid-line')).toHaveCount(27);await expect(page.locator('.trend-x-axis span')).toHaveCount(27);
 const load=chart(page,'系统负载（1 分钟平均）');await expect(load).toContainText('最近 0.34');await expect(load).not.toContainText('1min');await expect(load.getByRole('heading')).not.toContainText('—');await expect(chart(page,'每CPU负载').getByRole('heading')).toHaveText('每CPU负载');
 await expect(chart(page,'内存使用率 · %')).toContainText('最高 53.2%');await expect(chart(page,'剩余空间 · GiB')).toContainText('最近 25.27 GiB');await expect(chart(page,'应用堆内存 · MiB')).toContainText('最近 67 MiB');
 const zero=chart(page,'待处理任务 · 条');await expect(zero.locator('.trend-y-axis')).toContainText('0 条');await expect(zero.locator('polyline')).toHaveCount(2);await expect(zero).toContainText('最近 0 条');
 const ys=await zero.locator('polyline').first().getAttribute('points');expect(ys!.split(' ').map(p=>Number(p.split(',')[1])).every(y=>y>0&&y<105)).toBe(true);
 const sparse=chart(page,'任务执行耗时 · 秒');await expect(sparse.locator('.trend-dot')).toHaveCount(3);await expect(sparse.locator('polyline')).toHaveCount(0);await expect(sparse.locator('svg line:not(.trend-grid-line)')).toHaveCount(0);
 const rate=chart(page,'十分钟任务成功率 · %');await expect(rate.locator('.trend-dot')).toHaveCount(3);await expect(rate.locator('polyline')).toHaveCount(0);
 await expect(chart(page,'内存使用率 · %').locator('polyline')).toHaveCount(3);await expect(page.getByText(/缺失采样不补零/)).toBeVisible();
 for(const dot of await sparse.locator('.trend-dot').all()){
  const box=await dot.boundingBox();expect(box!.width).toBeCloseTo(6,1);expect(box!.height).toBeCloseTo(6,1);await expect(dot).toHaveCSS('border-radius','50%');
 }
 await expect(load.locator('.trend-x-axis')).toContainText('10-28');await expect(load.locator('.trend-x-axis')).toContainText('10-29');expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
 const styles=await load.evaluate(el=>{const tick=el.querySelector('.trend-y-axis')!,grid=el.querySelector('.trend-grid-line')!,line=el.querySelector('polyline')!;return{background:getComputedStyle(el).backgroundColor,text:getComputedStyle(tick).color,grid:getComputedStyle(grid).stroke,line:getComputedStyle(line).stroke}});
 expect(contrast(styles.background,styles.text)).toBeGreaterThanOrEqual(4.5);expect(contrast(styles.background,styles.grid)).toBeGreaterThanOrEqual(3);expect(contrast(styles.background,styles.line)).toBeGreaterThanOrEqual(3);
 if(capture){mkdirSync(resolve(output,'screens'),{recursive:true});await page.screenshot({path:resolve(output,'screens',`after-trends-${width}-${theme}.png`),fullPage:true,animations:'disabled'});writeFileSync(resolve(output,`geometry-${width}-${theme}.json`),JSON.stringify({width,theme,styles,contrastText:contrast(styles.background,styles.text),contrastGrid:contrast(styles.background,styles.grid)},null,2))}
 expect(requests).toEqual([24]);expect(errors).toEqual([]);
});
test('I24 empty/invalid data does not invent axes, values or points',async({page})=>{
 await fixture(page);await page.route('**/api/v1/ops/trends?*',r=>r.fulfill({json:{data:[{at:'invalid',memoryPercent:NaN},{at:'2026-10-29T00:00:00Z',memoryPercent:null}]}}));await page.goto('/trends');await expect(page.getByText('暂无有效采样',{exact:true})).toHaveCount(9);await expect(page.locator('.trend-frame,.trend-dot')).toHaveCount(0);
});
