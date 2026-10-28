import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {at,i13Fixture,i13AccessFixture} from './i13-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i13/screens');
const capture=process.env.I13_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const labels={candidates:'待确认文件列表',scans:'扫描任务列表',assets:'已入库资产列表'};
async function resize(page:Page,nav:ReturnType<Page['getByRole']>,value:string){await nav.locator('.el-select').click();const id=await nav.getByRole('combobox').getAttribute('aria-controls');await page.locator(`[id="${id}"]`).getByRole('option',{name:value,exact:true}).click()}
for(const tab of ['candidates','scans','assets']as const)test(`I13 ${tab} preserves row action, selection and page query`,async({page})=>{
 const f=await i13Fixture(page,true);await page.goto('/organize?tab='+tab);const list=page.getByRole('list',{name:labels[tab],exact:true});const first=list.getByRole('listitem').first();await expect(first).toBeVisible();
 const geometry=await first.evaluate(el=>{const row=el.getBoundingClientRect(),parent=el.parentElement!.getBoundingClientRect(),actions=el.querySelector('.organize-list-actions')!.getBoundingClientRect();return{width:row.width,parentWidth:parent.width,height:row.height,right:Math.abs(row.right-actions.right)}});expect(Math.abs(geometry.width-geometry.parentWidth)).toBeLessThan(1);expect(geometry.right).toBeLessThan(1);expect(geometry.height).toBeLessThanOrEqual(90);
 if(tab==='candidates'){
  await first.getByRole('checkbox').check();await expect(page.getByRole('button',{name:'批量确认作品 · 1',exact:true})).toBeEnabled();await first.getByRole('button',{name:'确认作品',exact:true}).click();const dialog=page.getByRole('dialog',{name:'确认候选媒体',exact:true});await expect(dialog).toBeVisible();await expect.poll(()=>f.writes.some(p=>p.endsWith('/candidate-0/recognize'))).toBe(true);await dialog.getByRole('button',{name:'取消',exact:true}).click();
 }else if(tab==='scans'){
  await expect(first.getByRole('button',{name:'取消后台任务',exact:true})).toBeVisible();await first.getByRole('button',{name:'暂停扫描',exact:true}).click();const dialog=page.getByRole('dialog',{name:'扫描控制',exact:true});expect(f.writes).toHaveLength(0);await dialog.getByRole('button',{name:'确认执行',exact:true}).click();await expect.poll(()=>f.writes.includes('/api/v1/library/scans/scan-0/pause')).toBe(true);
 }else{
  await expect(page.locator('.organize-asset-columns')).toContainText('入库时间');await expect(first.locator('time')).toHaveAttribute('datetime',at);await expect(first.locator('time')).toContainText('2026');
  await first.getByRole('checkbox').check();await expect(page.getByRole('link',{name:'在文件页批量整理 · 1',exact:true})).toBeVisible();await first.getByRole('button',{name:'查看资产详情',exact:true}).click();const dialog=page.getByRole('dialog',{name:'资产与输出详情',exact:true});await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();
 }
 const surface=page.locator(tab==='scans'?'.media-surface:has([aria-label="扫描任务列表"])':`.media-surface:has([aria-label="${labels[tab]}"])`),nav=surface.getByRole('navigation',{name:'分页',exact:true});await nav.getByRole('button',{name:'下一页',exact:true}).click();const key=tab==='assets'?'media/assets':`library/${tab}`;await expect.poll(()=>f.reads.some(u=>u.pathname.endsWith(key)&&u.searchParams.get('page')==='2')).toBe(true);
 await resize(page,nav,'10');await expect(nav).toContainText('第 1 页');await expect.poll(()=>f.reads.some(u=>u.pathname.endsWith(key)&&u.searchParams.get('page')==='1'&&u.searchParams.get('pageSize')==='10')).toBe(true);expect(new URL(page.url()).searchParams.get(tab+'PageSize')).toBe('10');
});
async function beforeAssets(page:Page){
 const root=resolve(dir,'../before-assets');for(const name of ['organize','playback'])await page.route(`**/${name}{,?*}`,r=>r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}));
 await page.route('**/assets/*',r=>{const file=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(file)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',file))})});
}
test.describe('I13 local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit I13_SCREENSHOTS=1, never CI');
 for(const tab of ['candidates','scans','assets']as const)for(const width of [1440,390])for(const theme of ['light','dark'])test(`${tab}-${width}-${theme}`,async({page})=>{
  mkdirSync(dir,{recursive:true});await i13Fixture(page);const before=process.env.I13_BEFORE==='1';if(before)await beforeAssets(page);await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/organize?tab='+tab);
  await expect(page.getByRole('heading',{name:tab==='candidates'?'待确认文件':tab==='scans'?'扫描目录':'已入库资产',exact:true})).toBeVisible();const row=page.locator(before?tab==='assets'?'.asset-card':tab==='scans'?'.f5-scan-row':'.organize-record:visible':tab==='assets'?'.organize-asset-row':tab==='scans'?'.organize-scan-row':'.organize-candidate-row');await expect(row).toHaveCount(2);
  if(!before){for(const action of await row.first().getByRole('button').all()){const r=await action.boundingBox();if(r)expect(r.height).toBeGreaterThanOrEqual(width<1241?44:32)}}
  if(tab==='assets'&&!before){
   const time=row.first().locator('time');await expect(time).toHaveAttribute('datetime',at);await expect(time).toBeVisible();await expect(time).toContainText('2026');
   if(width===390){const path=await row.first().locator('.organize-list-path').boundingBox(),stamp=await time.boundingBox();expect(stamp!.y).toBeGreaterThanOrEqual(path!.y+path!.height);expect(Math.abs(stamp!.x-path!.x)).toBeLessThan(1)}
  }
  if(tab==='assets')await expect.poll(()=>row.first().locator('img').evaluate(img=>(img as HTMLImageElement).naturalWidth)).toBeGreaterThan(0);expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);await page.screenshot({path:join(dir,`${before?'before':'after'}-${tab}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
 });
 for(const width of [1440,390])for(const theme of ['light','dark'])test(`AccessPanel pixel regression ${width}-${theme}`,async({page})=>{
  mkdirSync(dir,{recursive:true});await i13AccessFixture(page);const before=process.env.I13_BEFORE==='1';if(before)await beforeAssets(page);await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/playback?tab=players');await expect(page.locator('.ua-rule-row')).toHaveCount(2);const shot=await page.locator('.ua-list').first().screenshot({animations:'disabled'});const name=`access-${width}-${theme}.png`;writeFileSync(join(dir,`${before?'before':'after'}-${name}`),shot);if(!before)expect(shot.equals(readFileSync(join(dir,'before-'+name))),'I9 rules list PNG bytes must be identical').toBe(true);if(width===1440){const header=await page.locator('.ua-rule-columns').screenshot({animations:'disabled'}),headerName=`access-header-${theme}.png`;writeFileSync(join(dir,`${before?'before':'after'}-${headerName}`),header);if(!before)expect(header.equals(readFileSync(join(dir,'before-'+headerName))),'I9 column header PNG bytes must be identical').toBe(true)}
 });
});
