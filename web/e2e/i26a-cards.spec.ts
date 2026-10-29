import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {i26aFixture,kinds} from './i26a-fixture';
const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i26a');
const capture=process.env.I26A_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
async function shot(page:Page,name:string){
 if(!capture)return;mkdirSync(output,{recursive:true});
 await page.screenshot({path:resolve(output,`${name}.png`),fullPage:true,animations:'disabled'});
}
async function overflow(page:Page,width:number){expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)}
for(const [width,theme] of [[1440,'light'],[390,'dark'],[768,'light']] as const)test(`I26a homepage compact cards and refresh ${width}/${theme}`,async({page})=>{
 const f=await i26aFixture(page);await page.setViewportSize({width,height:1000});
 await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/dashboard');
 await expect(page.locator('.dashboard-counters a')).toHaveCount(5);await expect(page.getByText('正在读取工作台…',{exact:true})).toHaveCount(0);
 const boxes=await page.locator('.dashboard-counters a').evaluateAll(es=>es.map(e=>{const b=e.getBoundingClientRect();return {x:b.x,y:b.y,width:b.width,height:b.height}}));
 if(width<=768){
  expect(boxes[0]!.y).toBe(boxes[1]!.y);expect(boxes[2]!.y).toBe(boxes[3]!.y);expect(boxes[2]!.y).toBeGreaterThan(boxes[0]!.y);
  expect(boxes[4]!.width).toBeCloseTo(boxes[0]!.width*2+12,0);expect(boxes[0]!.height).toBeLessThanOrEqual(56);
 }else{expect(new Set(boxes.map(b=>b.y)).size).toBe(1)}
 const header=page.locator('.dashboard-home-heading'),title=await header.locator('h1').boundingBox(),button=await header.getByRole('button',{name:'刷新',exact:true}).boundingBox();
 expect(button!.y).toBeLessThan(title!.y+title!.height);expect(button!.x).toBeGreaterThan(title!.x+title!.width);expect(button!.width).toBeGreaterThanOrEqual(44);expect(button!.height).toBeGreaterThanOrEqual(44);
 const cards=await page.locator('.is-overview .readiness-cards article').evaluateAll(es=>es.map(e=>{
  const b=e.getBoundingClientRect(),a=e.querySelector('a')!.getBoundingClientRect(),s=getComputedStyle(e);
  return {height:b.height,paddingTop:s.paddingTop,paddingBottom:s.paddingBottom,bottom:b.bottom-a.bottom};
 }));
 if(width===1440){expect(new Set(cards.map(c=>c.height)).size).toBe(1);for(const c of cards){expect(c.paddingTop).toBe(c.paddingBottom);expect(c.bottom).toBeLessThanOrEqual(17)}}
 await expect(page.locator('.is-overview .readiness-test')).toHaveCount(0);await overflow(page,width);
 if(width!==768)await shot(page,`after-dashboard-${width}-${theme}`);
 const count=f.reads.filter(s=>s==='runtime').length;f.hold();
 try{await header.getByRole('button',{name:'刷新',exact:true}).click();await expect.poll(()=>f.reads.filter(s=>s==='runtime').length).toBeGreaterThan(count);await expect(page.locator('.dashboard-counters a')).toHaveCount(5);await expect(page.getByText('正在读取工作台…',{exact:true})).toHaveCount(0)}finally{f.release()}
 expect(f.reads.filter(s=>s==='accounts')).toHaveLength(0);expect(f.writes).toEqual([]);expect(f.original.writes).toEqual([]);
 if(capture)writeFileSync(resolve(output,`geometry-home-${width}.json`),JSON.stringify({boxes,cards,title,button},null,2));
});
const service=(page:Page,kind:typeof kinds[number])=>page.locator('.readiness-cards article').filter({has:page.getByRole('heading',{name:{open115:'115 官方授权',clouddrive2:'CloudDrive2',qbittorrent:'qBittorrent'}[kind],exact:true})});
test('I26a all three existing probes disable in flight and write back results; STRM has no probe',async({page})=>{
 const f=await i26aFixture(page);await page.goto('/ops?tab=attention');
 await expect(page.locator('.readiness-test')).toHaveCount(3);
 await expect(page.locator('.readiness-cards article').filter({has:page.getByRole('heading',{name:'STRM 播放入口',exact:true})}).getByRole('button')).toHaveCount(0);
 for(const kind of kinds){
  const card=service(page,kind),button=card.getByRole('button',{name:'立即检测',exact:true});f.hold();
  try{await button.click();await expect(button).toBeDisabled();await expect.poll(()=>f.writes.filter(k=>k===kind).length).toBe(1)}finally{f.release()}
  await expect(button).toBeEnabled();await expect(card).toContainText('2026-10-30 10:01:00');await expect(card).toContainText(kind==='qbittorrent'?'连接失败':'连接成功');
 }
 expect(f.writes).toEqual([...kinds]);expect(f.original.writes).toEqual([]);
});
test('I26a lost CD2 receipt remains locked after reload without locking qB or replaying POST',async({page})=>{
 await page.clock.install();const f=await i26aFixture(page);f.lose();await page.goto('/ops?tab=attention');
 await service(page,'clouddrive2').getByRole('button',{name:'立即检测',exact:true}).click();
 await expect(page.getByText('CloudDrive2：检测结果未知，请先核对，不重复检测。',{exact:true})).toBeVisible();
 await page.reload();await expect(service(page,'clouddrive2').getByRole('button',{name:'立即检测',exact:true})).toBeDisabled();
 await expect(service(page,'qbittorrent').getByRole('button',{name:'立即检测',exact:true})).toBeEnabled();
 await page.getByRole('button',{name:'核对结果',exact:true}).click();await expect(page.getByText(/CloudDrive2：尚未取得新的检测记录/)).toBeVisible();expect(f.writes).toEqual(['clouddrive2']);
});
test('I26a disabled connections never offer a probe',async({page})=>{
 const f=await i26aFixture(page);f.disable();await page.goto('/ops?tab=attention');await expect(page.locator('.readiness-cards article')).toHaveCount(4);
 await expect(page.locator('.readiness-test')).toHaveCount(0);expect(f.writes).toEqual([]);
});
for(const [name,path,width] of [['ops-attention','/ops?tab=attention',1440],['ops-attention','/ops?tab=attention',390],['connections','/connections?tab=storage',390],['trends','/trends?hours=24',390]] as const)test(`I26a header spot check ${name}/${width}`,async({page})=>{
 const f=await i26aFixture(page);await page.setViewportSize({width,height:1000});await page.addInitScript(()=>localStorage.setItem('media-tool:theme-mode:v1','light'));
 await page.goto(path);await expect(page.locator('.page-heading h1')).toBeVisible();await expect(page.locator('.service-readiness .readiness-cards article,.saved-connection,.u6-trend').first()).toBeVisible();
 const header=page.locator('.page-heading').first(),b=await header.boundingBox(),actions=await header.locator('.heading-actions').boundingBox();
 expect(actions!.x).toBeGreaterThanOrEqual(b!.x);expect(actions!.x+actions!.width).toBeLessThanOrEqual(b!.x+b!.width+1);
 await expect(header.getByRole('button',{name:'刷新',exact:true})).toHaveCSS('width','44px');await overflow(page,width);
 await shot(page,`after-${name}-${width}-light`);expect(f.writes).toEqual([]);expect(f.original.writes).toEqual([]);
});
test('I26a homepage cards remain equal-height with a longer failure reason, without hiding it',async({page})=>{
 const f=await i26aFixture(page);f.failCloud();await page.goto('/dashboard');
 await expect(page.getByText('本地模拟：连接检测超时，请检查网络后重试',{exact:true})).toBeVisible();
 const cards=await page.locator('.is-overview .readiness-cards article').evaluateAll(es=>es.map(e=>{
  const b=e.getBoundingClientRect(),a=e.querySelector('a')!.getBoundingClientRect();return {height:b.height,linkBottom:a.bottom};
 }));
 expect(new Set(cards.map(c=>c.height)).size).toBe(1);expect(new Set(cards.map(c=>c.linkBottom)).size).toBe(1);
});
