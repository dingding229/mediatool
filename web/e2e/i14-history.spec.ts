import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {organizeFixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i14/screens');
const capture=process.env.I14_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const at='2026-10-28T10:00:00Z';
async function fixture(page:Page,large=false){
 await organizeFixture(page);const reads:URL[]=[],writes:Array<{path:string;body:any}>=[];
 const file={id:'file-1',kind:'file',state:'failed',source:'/115/待整理/海边的故事.2026.mkv',destination:'/115/电影/海边的故事 (2026)/海边的故事.mkv',media:{title:'海边的故事',year:'2026'},category:'华语电影',updatedAt:at,createdAt:at,message:'失败原因仅在详情展示',details:{completedSteps:0}};
 const items=[file,{...file,id:'batch-1',kind:'batch',state:'succeeded',source:'/115/待整理/第二部电影.mkv',media:{title:'第二部电影'},details:{items:[]}},...['pending','conflict'].map(state=>({...file,id:state,kind:'output',state,source:`Movies/海边的故事/${state}.strm`,details:{operation:'write',kind:'strm'}}))];
 if(large)items.push(...Array.from({length:65},(_,i)=>({...file,id:'extra-'+i,state:'succeeded'})));
 await page.route('**/api/v1/library/activity?*',r=>{const u=new URL(r.request().url());reads.push(u);const q=u.searchParams,filtered=items.filter(x=>(!q.get('kind')||x.kind===q.get('kind'))&&(!q.get('state')||x.state===q.get('state')));const size=Number(q.get('pageSize')||25),offset=(Number(q.get('page')||1)-1)*size;return fixtureFulfill(r,{json:{data:{items:filtered.slice(offset,offset+size),total:filtered.length}}})});
 await page.route('**/api/v1/library/artifact-repairs/*/retry',r=>{writes.push({path:new URL(r.request().url()).pathname,body:r.request().postDataJSON()});items.find(x=>r.request().url().includes('/'+x.id+'/'))!.state='applied';return r.fulfill({json:{data:{repaired:true}}})});
 await page.route('**/api/v1/library/activity/remove',r=>{const body=r.request().postDataJSON();writes.push({path:new URL(r.request().url()).pathname,body});for(const row of body.items){const index=items.findIndex(x=>x.kind===row.kind&&x.id===row.id);if(index>=0)items.splice(index,1)}return r.fulfill({json:{data:{removed:true}}})});
 return{reads,writes,items};
}
async function option(page:Page,name:string,value:string){const box=page.getByRole('combobox',{name,exact:true});await page.locator('.el-select').filter({has:box}).click();const id=await box.getAttribute('aria-controls');await page.locator(`[id="${id}"]`).getByRole('option',{name:value,exact:true}).click()}
async function beforeBundle(page:Page){
 const root=resolve(dir,'../before-assets');await page.route('**/organize{,?*}',r=>r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}));
 await page.route('**/assets/*',r=>{const file=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(file)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',file))})});
}
async function layout(page:Page,width:number){
   const row=page.locator('.f5-history-row').first();const geometry=await row.evaluate(el=>{const r=el.getBoundingClientRect(),parent=el.parentElement!.getBoundingClientRect(),actions=el.querySelector('.history-row-actions')!.getBoundingClientRect();return{width:r.width,parent:parent.width,right:Math.abs(r.right-actions.right),border:getComputedStyle(el.parentElement!).borderInlineStartWidth,before:getComputedStyle(el,'::before').content}});
   expect(Math.abs(geometry.width-geometry.parent)).toBeLessThan(1);expect(geometry.right).toBeLessThan(1);expect(geometry.border).toBe('0px');expect(geometry.before).toBe('none');
   const filter=page.locator('.organize-history-filters');expect(await filter.getByRole('combobox').count()).toBe(4);
   if(width===1440){const labels=await filter.locator(':scope > label').evaluateAll(es=>es.map(el=>Math.round(el.getBoundingClientRect().y)));expect(new Set(labels).size).toBe(2)}
   else{
    for(const button of await row.getByRole('button').all()){await expect(button).toHaveCSS('height','44px');await expect(button).toHaveCSS('width','44px');const box=await button.boundingBox();expect(box!.width).toBeGreaterThanOrEqual(44);expect(box!.height).toBeGreaterThanOrEqual(44)}
    const paths=await row.locator('.history-paths').boundingBox(),bounds=await row.boundingBox();expect(paths!.width).toBeGreaterThanOrEqual(bounds!.width-1);
   }
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
}
test('I14 stage, recovery shortcuts, reset and pagination retain exact queries',async({page})=>{
 const f=await fixture(page,true);await page.goto('/organize?tab=history');const list=page.getByRole('list',{name:'整理历史列表',exact:true});await expect(list.getByRole('listitem')).toHaveCount(25);
 await test.step('stage dropdown',async()=>{
 await expect(page.getByRole('navigation',{name:'历史阶段快捷筛选',exact:true})).toHaveCount(0);
 await option(page,'历史阶段','文件整理');await expect.poll(()=>f.reads.at(-1)?.searchParams.get('kind')).toBe('file');
 await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'历史阶段',exact:true})}).click();await expect(page.getByRole('option',{name:'Emby 可见性',exact:true})).toHaveCount(0);await page.keyboard.press('Escape');
 });
 const nav=page.getByRole('navigation',{name:'分页',exact:true});
 await test.step('page and size',async()=>{
 await nav.getByRole('button',{name:'下一页',exact:true}).click();await expect.poll(()=>f.reads.at(-1)?.searchParams.get('page')).toBe('2');expect(f.reads.at(-1)?.searchParams.get('kind')).toBe('file');
 await nav.locator('.el-select').click();const id=await nav.getByRole('combobox').getAttribute('aria-controls');await page.locator(`[id="${id}"]`).getByRole('option',{name:'10',exact:true}).click();
 await expect.poll(()=>f.reads.at(-1)?.searchParams.get('page')).toBe('1');await expect.poll(()=>f.reads.at(-1)?.searchParams.get('pageSize')).toBe('10');
 });
 await test.step('recovery shortcuts',async()=>{
 for(const state of ['pending','conflict']){await page.getByRole('button',{name:state==='pending'?'待恢复输出':'冲突输出',exact:true}).click();await expect.poll(()=>f.reads.at(-1)?.searchParams.get('state')).toBe(state);expect(f.reads.at(-1)?.searchParams.get('kind')).toBe('output');await expect(list.getByRole('listitem')).toHaveCount(1)}
 });
 await test.step('filter then reset',async()=>{
 await page.getByRole('textbox',{name:'历史关键词',exact:true}).fill('样片');await page.getByRole('textbox',{name:'历史批次ID',exact:true}).fill('11111111-1111-4111-8111-111111111111');await page.getByLabel('历史开始时间',{exact:true}).fill('2026-10-01T00:00');await page.getByLabel('历史结束时间',{exact:true}).fill('2026-10-29T00:00');
 await page.getByRole('button',{name:'筛选',exact:true}).click();await expect.poll(()=>f.reads.at(-1)?.searchParams.get('q')).toBe('样片');
 await page.getByRole('button',{name:'重置',exact:true}).click();await expect(list.getByRole('listitem')).toHaveCount(10);
 const q=f.reads.at(-1)!.searchParams;for(const key of ['kind','state','q','batchId','origin','method'])expect(q.get(key)).toBe('');expect(q.has('from')).toBe(false);expect(q.has('to')).toBe(false);expect(q.get('pageSize')).toBe('10');expect(q.get('page')).toBe('1');
 });
 expect(f.writes).toEqual([]);
});
test('I14 keeps recovery retry and bulk removal behind their existing confirmations',async({page})=>{
 const f=await fixture(page);await page.goto('/organize?tab=history');await expect(page.locator('.f5-history-row')).toHaveCount(4);await page.getByRole('button',{name:'待恢复输出',exact:true}).click();await expect(page.locator('.f5-history-row')).toHaveCount(1);await page.getByRole('button',{name:'查看步骤详情',exact:true}).click();
 const detail=page.getByRole('dialog',{name:'整理步骤详情',exact:true});await detail.getByRole('button',{name:'重试恢复',exact:true}).click();expect(f.writes).toEqual([]);
 await page.getByRole('dialog',{name:'重试输出恢复',exact:true}).getByRole('button',{name:'确认重试恢复',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes[0]!.path).toBe('/api/v1/library/artifact-repairs/pending/retry');
 await page.getByRole('button',{name:'重置',exact:true}).click();const first=page.getByRole('list',{name:'整理历史列表',exact:true}).getByRole('listitem').first();await first.getByRole('checkbox').check();await page.getByRole('button',{name:'移除所选历史',exact:true}).click();expect(f.writes).toHaveLength(1);
 await page.getByRole('dialog',{name:'移除整理历史',exact:true}).getByRole('button',{name:'明确移除历史',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(2);expect(f.writes[1]!.body).toEqual({confirm:true,items:[{id:'file-1',kind:'file',updatedAt:at}]});await expect(page.locator('.f5-history-row')).toHaveCount(3);
});
test('I14 unknown historical kinds have a neutral fallback; URL reset removes deep-link filters',async({page})=>{
 const f=await fixture(page);f.items.push({...f.items[0]!,id:'legacy',kind:'emby',media:{title:''},state:'visible'});
 await page.goto('/organize?tab=history&kind=output&state=pending&q=old&keep=yes');await page.getByRole('button',{name:'重置',exact:true}).click();
 await expect.poll(()=>new URL(page.url()).searchParams.has('kind')).toBe(false);expect(new URL(page.url()).searchParams.get('keep')).toBe('yes');
 const legacy=page.getByRole('heading',{name:'其他/历史阶段',exact:true});await expect(legacy).toBeVisible();await legacy.locator('..').locator('..').getByRole('button',{name:'查看步骤详情',exact:true}).click();await expect(page.getByRole('dialog',{name:'整理步骤详情',exact:true}).getByRole('heading',{name:'其他/历史阶段',exact:true})).toBeVisible();
});
test('I14 compact filters and list geometry fit desktop and mobile',async({page})=>{
 await fixture(page);await page.goto('/organize?tab=history');await expect(page.locator('.f5-history-row')).toHaveCount(4);
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});await layout(page,width)}
});
test('I14 history pagination stays independent when another organize tab mounted first',async({page})=>{
 const f=await fixture(page,true);await page.goto('/organize?tab=candidates&candidatesPage=1&candidatesPageSize=50&historyPage=2&historyPageSize=10');
 await page.locator('.media-workspace > .page-tabs').getByRole('button',{name:'整理历史',exact:true}).click();const pager=page.getByRole('navigation',{name:'分页',exact:true});await expect(pager).toContainText('第 2 页');
 await pager.getByRole('button',{name:'下一页',exact:true}).click();await expect.poll(()=>new URL(page.url()).searchParams.get('historyPage')).toBe('3');await expect.poll(()=>f.reads.at(-1)?.searchParams.get('page')).toBe('3');expect(f.reads.at(-1)?.searchParams.get('pageSize')).toBe('10');
 expect(new URL(page.url()).searchParams.get('candidatesPage')).toBe('1');expect(new URL(page.url()).searchParams.get('candidatesPageSize')).toBe('50');
});
test('I14 first history read keeps controls stable with delayed populated results',async({page})=>{
 const f=await fixture(page);
 await page.route('**/api/v1/library/activity?*',async r=>{await new Promise(resolve=>setTimeout(resolve,180));return fixtureFulfill(r,{json:{data:{items:f.items,total:f.items.length}}})});
 await page.addInitScript(()=>{(window as any).__i14cls=0;new PerformanceObserver(list=>{for(const row of list.getEntries() as any[])if(!row.hadRecentInput)(window as any).__i14cls+=row.value}).observe({type:'layout-shift',buffered:true})});
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});await page.goto('/organize?tab=history');await expect(page.locator('.f5-history-row')).toHaveCount(4);
  await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))));
  expect(await page.evaluate(()=>(window as any).__i14cls)).toBeLessThan(.01);
 }
});
test.describe('I14 local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit I14_SCREENSHOTS=1, never CI');
 for(const width of [1440,390])for(const theme of ['light','dark'])test(`history-${width}-${theme}`,async({page})=>{
  mkdirSync(dir,{recursive:true});await fixture(page);const before=process.env.I14_BEFORE==='1';if(before)await beforeBundle(page);
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/organize?tab=history');
  await expect(page.locator('.f5-history-row')).toHaveCount(4);await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  if(!before)await layout(page,width);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  await page.screenshot({path:join(dir,`${before?'before':'after'}-history-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
 });
});
