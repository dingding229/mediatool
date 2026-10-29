import {test,expect} from '@playwright/test';
import {readFile} from 'node:fs/promises';
import {logText} from '../src/logs/format';
import {logConsoleFixture,logRows,rawRequestURL,rawRedirectURL} from './i19-5-fixture';
const luminance=(rgb:number[])=>rgb.map(c=>{c/=255;return c<=.04045?c/12.92:((c+.055)/1.055)**2.4}).reduce((s,c,i)=>s+c*[.2126,.7152,.0722][i]!,0);
const contrast=(a:string,b:string)=>{const l=[a,b].map(c=>luminance(c.match(/\d+(?:\.\d+)?/g)!.slice(0,3).map(Number))).sort((x,y)=>y-x);return (l[0]!+.05)/(l[1]!+.05)};

test('I19-5 console follows both themes, keeps four cells, and loaded height follows short content',async({page})=>{
 await logConsoleFixture(page);await page.goto('/ops?tab=logs');await expect(page.locator('.u6-log')).toHaveCount(4);
 for(const theme of ['light','dark']){
  await page.evaluate(t=>{document.documentElement.dataset.theme=t;document.documentElement.classList.toggle('dark',t==='dark')},theme);
  const styles=await page.locator('.f3-log-table').evaluate(el=>{const css=getComputedStyle(el);return {background:css.backgroundColor,text:css.color,colors:[...el.querySelectorAll('.u6-log .f3-log-level,.f3-log-source,.f3-log-message small,.u6-raw')].map(e=>getComputedStyle(e).color)}});
  expect(styles.background).toBe(theme==='light'?'rgb(255, 255, 255)':'rgb(24, 28, 36)');expect(contrast(styles.background,styles.text)).toBeGreaterThanOrEqual(4.5);for(const color of styles.colors)expect(contrast(styles.background,color),color).toBeGreaterThanOrEqual(4.5);
 }
 for(const row of await page.locator('.u6-log').all())await expect(row.getByRole('cell')).toHaveCount(4);
 await expect(page.getByRole('switch',{name:'清爽模式',exact:true})).not.toBeChecked();await expect(page.locator('.f3-log-message').first()).toContainText(rawRequestURL);await expect(page.locator('.f3-log-message').first()).toContainText(rawRedirectURL);
 const geometry=await page.locator('.f3-log-table').evaluate(el=>{const table=el.getBoundingClientRect(),last=el.querySelector('.u6-log:last-child')!.getBoundingClientRect();return {tail:table.bottom-last.bottom,height:table.height}});expect(geometry.tail).toBeLessThanOrEqual(2);expect(geometry.height).toBeLessThan(400);
});

test('I19-5 mobile message and source span the row, controls are 44px and no original text is clipped',async({page})=>{
 await logConsoleFixture(page);await page.setViewportSize({width:390,height:1000});await page.goto('/ops?tab=logs');await expect(page.locator('.u6-log')).toHaveCount(4);
 for(const row of await page.locator('.u6-log').all()){
  const cells=await row.getByRole('cell').all(),message=cells[2]!,source=cells[3]!;const grid=await row.evaluate(el=>{const b=el.getBoundingClientRect(),s=getComputedStyle(el);return {width:b.width,left:parseFloat(s.paddingLeft),right:parseFloat(s.paddingRight)}});
  for(const cell of [message,source]){const b=await cell.boundingBox();expect(Math.abs(b!.width-(grid.width-grid.left-grid.right))).toBeLessThanOrEqual(1)}
  const control=source.getByRole('button');const b=await control.boundingBox();expect(b!.height).toBeGreaterThanOrEqual(44);expect(b!.width).toBeGreaterThanOrEqual(44);
 }
 await expect(page.locator('.f3-log-message').first()).toContainText('END-REQUEST');await expect(page.locator('.f3-log-message').first()).toContainText('END-302');
 const buttons=await page.locator('.f3-log-tools>.el-button').all();expect(buttons).toHaveLength(5);for(const button of buttons)await expect(button).toHaveCSS('height','44px');
 const tops=await page.locator('.f3-log-tools>.el-button').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().top));expect(Math.max(...tops)-Math.min(...tops)).toBeLessThanOrEqual(1);expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
 await page.locator('.u6-log').last().getByRole('button',{name:'完整原始记录',exact:true}).click();const detail=page.getByRole('dialog',{name:'完整原始日志',exact:true});await expect(detail).toContainText('original-file.go:88');await detail.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(detail).not.toBeVisible();
});

test('I19-5 full request/302 and original JSON stay available; clipboard fallback and export preserve exact values',async({page})=>{
 const f=await logConsoleFixture(page);await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{value:{writeText:async()=>{throw Error('fixture clipboard denied')}}}));await page.goto('/ops?tab=logs');
 await page.getByRole('button',{name:'查看完整链接',exact:true}).first().click();const detail=page.getByRole('dialog',{name:'完整原始日志',exact:true});await expect(detail.getByRole('textbox',{name:'完整请求URL',exact:true})).toHaveValue(rawRequestURL);await expect(detail.getByRole('textbox',{name:'完整302目标URL',exact:true})).toHaveValue(rawRedirectURL);await expect(detail.locator('pre')).toContainText('trace-original');
 await detail.getByRole('button',{name:'复制请求地址',exact:true}).click();let copy=page.getByRole('dialog',{name:'复制原始日志',exact:true});await expect(copy.getByRole('textbox',{name:'待复制日志',exact:true})).toHaveValue(rawRequestURL);await copy.getByRole('button',{name:'关闭',exact:true}).click();await expect(copy).not.toBeVisible();await detail.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(detail).not.toBeVisible();
 await page.getByRole('button',{name:'复制当前页日志',exact:true}).click();copy=page.getByRole('dialog',{name:'复制原始日志',exact:true});await expect(copy.getByRole('textbox',{name:'待复制日志',exact:true})).toHaveValue(logText(logRows,false));await copy.getByRole('button',{name:'关闭',exact:true}).click();await expect(copy).not.toBeVisible();
 const downloaded=page.waitForEvent('download');await page.getByRole('button',{name:'下载原始日志',exact:true}).click();const download=await downloaded;expect(await readFile((await download.path())!,'utf8')).toBe(logText(logRows,false));expect(f.writes).toEqual([]);
});

test('I19-5 filters and pagination retain original request fields, 200 default, before anchor and instance scope',async({page})=>{
 const f=await logConsoleFixture(page,true);await page.goto('/ops?tab=logs&module=library&instance=owned-instance');await expect(page.locator('.u6-log')).toHaveCount(200);expect(f.reads[0]!.get('pageSize')).toBe('200');expect(f.reads[0]!.get('module')).toBe('library');expect(f.reads[0]!.get('instance')).toBe('owned-instance');
 await page.getByRole('navigation',{name:'日志分页',exact:true}).getByRole('button',{name:'下一页',exact:true}).click();await expect(page.locator('.u6-log')).toHaveCount(1);expect(f.reads.at(-1)!.get('page')).toBe('2');expect(f.reads.at(-1)!.get('before')).toBe('2026-10-29T00:00:00Z');
 await page.getByRole('textbox',{name:'搜索日志',exact:true}).fill('original-value');await expect.poll(()=>f.reads.at(-1)!.get('keyword')).toBe('original-value');expect(f.reads.at(-1)!.get('page')).toBe('1');expect(f.reads.at(-1)!.has('before')).toBe(false);
 await page.locator('.f3-log-filters .el-select__wrapper').first().click();await page.getByRole('option',{name:'ERROR',exact:true}).click();await expect.poll(()=>f.reads.at(-1)!.get('level')).toBe('error');expect(f.reads.at(-1)!.get('instance')).toBe('owned-instance');
 const before=f.reads.length;await page.getByRole('button',{name:'刷新日志',exact:true}).click();await expect.poll(()=>f.reads.length).toBeGreaterThan(before);expect(f.reads.at(-1)!.get('level')).toBe('error');expect(f.writes).toEqual([]);
});

test('I19-5 many records remain scrollable and clean mode is optional, never imposed or used for export',async({page})=>{
 await logConsoleFixture(page,true);await page.goto('/ops?tab=logs');await expect(page.locator('.u6-log')).toHaveCount(200);
 const box=await page.locator('.f3-log-table').evaluate(el=>({height:el.clientHeight,scroll:el.scrollHeight,overflow:getComputedStyle(el).overflow}));expect(box.scroll).toBeGreaterThan(box.height);expect(box.height).toBeLessThanOrEqual(672);expect(box.overflow).toBe('auto');
 await page.getByRole('switch',{name:'清爽模式',exact:true}).press('Space');await expect(page.locator('.f3-log-message .u6-raw')).toHaveCount(0);await page.getByRole('switch',{name:'清爽模式',exact:true}).press('Space');await expect(page.locator('.f3-log-message').first()).toContainText(rawRedirectURL);
});

for(const mode of ['empty','failed']as const)test(`I19-5 ${mode} remains distinct with compact state and no automatic writes`,async({page})=>{
 const f=await logConsoleFixture(page);if(mode==='empty')f.empty();else f.fail();await page.goto('/ops?tab=logs');
 if(mode==='empty'){await expect(page.getByText('没有符合筛选条件的日志',{exact:true})).toBeVisible();const b=await page.locator('.f3-log-table').boundingBox();expect(b!.height).toBeLessThan(260)}
 else{await expect(page.getByText('暂时无法读取',{exact:true})).toBeVisible();await expect(page.getByText('没有符合筛选条件的日志',{exact:true})).toHaveCount(0)}
 expect(f.writes).toEqual([]);
});

test('I19-5 first-read reservation is removed after load and refresh keeps rows; stale errors are readable',async({page})=>{
 const f=await logConsoleFixture(page);f.hold();try{await page.goto('/ops?tab=logs');await expect(page.locator('.f3-log-table>.state-view.state-loading')).toBeVisible();const b=await page.locator('.f3-log-table').boundingBox();expect(b!.height).toBeGreaterThanOrEqual(400);await expect(page.getByText('没有符合筛选条件的日志',{exact:true})).toHaveCount(0)}finally{f.release()}
 await expect(page.locator('.u6-log')).toHaveCount(4);const before=await page.locator('.f3-log-table').boundingBox();
 f.hold();try{await page.getByRole('button',{name:'刷新日志',exact:true}).click();await expect(page.locator('.u6-log')).toHaveCount(4);const during=await page.locator('.f3-log-table').boundingBox();expect(during!.height).toBe(before!.height);await expect(page.locator('.f3-log-table>.state-view.state-loading')).toHaveCount(0)}finally{f.release()}
 f.fail();await page.getByRole('button',{name:'刷新日志',exact:true}).click();await expect(page.getByText(/刷新失败，保留上次数据/)).toBeVisible();await expect(page.locator('.u6-log')).toHaveCount(4);expect(f.writes).toEqual([]);
});

for(const lost of [false,true])test(`I19-5 log clearing still requires irreversible-scope confirmation; ${lost?'unknown receipt stays locked':'confirmed clear is scoped'}`,async({page})=>{
 const f=await logConsoleFixture(page);if(lost)f.lose();await page.goto('/ops?tab=logs');await page.getByRole('button',{name:'清理日志',exact:true}).click();const dialog=page.getByRole('dialog',{name:'清理日志',exact:true});await expect(dialog).toContainText('现有文件日志将不可恢复');await expect(dialog).toContainText('不修改媒体');expect(f.writes).toEqual([]);
 await dialog.getByRole('button',{name:'确认清理',exact:true}).click();await expect.poll(()=>f.writes.length).toBe(1);expect(f.writes).toEqual([{path:'/api/v1/logs/clear',body:{confirm:true}}]);
 if(lost){await expect(page.getByRole('button',{name:'清理日志',exact:true})).toBeDisabled();await page.getByRole('button',{name:'核对结果',exact:true}).click();await page.reload();await expect(page.getByRole('button',{name:'清理日志',exact:true})).toBeDisabled();expect(f.writes).toHaveLength(1)}else await expect(page.getByText('没有符合筛选条件的日志',{exact:true})).toBeVisible();
});
