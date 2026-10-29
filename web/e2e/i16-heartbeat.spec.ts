import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {u3Fixture} from './u3-fixture';
import {fixtureFulfill} from './collection-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i16/screens');
const capture=process.env.I16_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS,before=process.env.I16_BEFORE==='1';
const checked='2026-10-29T03:00:00Z';
const rows=[
 {id:'115-fixture',kind:'open115',name:'115 官方授权',enabled:true,lastCheckState:'ready',lastCheckMessage:'连接成功',lastCheckedAt:checked},
 {id:'cd2-fixture',kind:'clouddrive2',name:'CloudDrive2',enabled:true,lastCheckState:'unavailable',lastCheckMessage:'连接检测超时，请检查网络后重试',lastCheckedAt:checked},
 {id:'qb-fixture',kind:'qbittorrent',name:'qBittorrent',enabled:true,lastCheckState:'ready',lastCheckMessage:'连接成功',lastCheckedAt:checked},
 {id:'off-fixture',kind:'clouddrive2',name:'停用的备用连接',enabled:false,lastCheckState:'disabled',lastCheckMessage:'已停用'},
].map(r=>({...r,settings:{},secretFields:['token'],revision:1,readiness:{state:r.lastCheckState,nextAction:'none',reason:'fresh_saved_probe'}}));
async function fixture(page:Page){
 await u3Fixture(page);let probes=0;
 const data=structuredClone(rows);
 await page.route('**/api/v1/integrations',r=>fixtureFulfill(r,{json:{data}}));
 await page.route('**/api/v1/readiness',r=>fixtureFulfill(r,{json:{data:{components:{...Object.fromEntries(data.filter(r=>r.enabled).map(r=>[r.kind,{state:r.lastCheckState,nextAction:'test_connection',reason:'fresh_saved_probe',connectionId:r.id,lastCheckedAt:r.lastCheckedAt,message:r.lastCheckMessage}])),strm:{state:'configured',reason:'stable_resolver_configured_not_player_verified',nextAction:'verify_player_302'}}}}}));
 await page.route('**/api/v1/integrations/*/test',r=>{probes++;const row=data.find(x=>r.request().url().includes(x.id))!;row.lastCheckState='ready';row.lastCheckMessage='连接成功';row.lastCheckedAt='2026-10-29T03:01:00Z';return fixtureFulfill(r,{json:{data:{passed:true,state:'ready',message:'连接成功'}}})});
 return ()=>probes;
}
test('I16 connections expose only binary results, disabled never probes, manual detection refreshes time',async({page})=>{
 const probes=await fixture(page);await page.goto('/connections?tab=storage');
 const off=page.locator('.saved-connection').filter({hasText:'停用的备用连接'}),cd2=page.locator('.saved-connection').filter({has:page.getByRole('heading',{name:'CloudDrive2',exact:true})});
 await expect(off.getByText('已停用',{exact:true})).toBeVisible();await expect(off.getByRole('button',{name:'测试连接',exact:true})).toBeDisabled();expect(probes()).toBe(0);
 await expect(cd2.getByText('连接失败',{exact:true})).toBeVisible();await expect(cd2.getByText('连接检测超时，请检查网络后重试')).toBeVisible();
 await cd2.getByRole('button',{name:'测试连接',exact:true}).click();await expect(cd2.getByText('连接成功',{exact:true})).toBeVisible();await expect(cd2.getByText(/最近检测：.*11:01/)).toBeVisible();expect(probes()).toBe(1);
 await expect(page.getByText('状态待确认',{exact:true})).toHaveCount(0);
 await page.goto('/connections?tab=downloader');await expect(page.locator('.saved-connection').getByText('连接成功',{exact:true})).toBeVisible();
});
test('I16 default homepage has three connection results with dates and failure reasons',async({page})=>{
 await fixture(page);await page.goto('/');const panel=page.getByRole('region',{name:'服务就绪状态'});
 await expect(panel.getByRole('heading',{name:'qBittorrent',exact:true})).toBeVisible();await expect(panel.getByText('连接成功',{exact:true})).toHaveCount(2);await expect(panel.getByText('连接失败',{exact:true})).toHaveCount(1);
 await expect(panel.getByText(/最近检测：/)).toHaveCount(3);await expect(panel.getByText('连接检测超时，请检查网络后重试')).toBeVisible();await expect(panel.getByText('STRM 播放入口',{exact:true})).toHaveCount(0);
});
test.describe('I16 local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit I16_SCREENSHOTS=1 only; never CI');
 for(const width of [1440,390])for(const theme of ['light','dark'])for(const view of ['connections','home'])test(`${view}-${width}-${theme}`,async({page})=>{
  mkdirSync(dir,{recursive:true});await fixture(page);
  if(before){const root=resolve(dir,'../before-assets');await page.route('**/*',async r=>{const u=new URL(r.request().url());if(u.origin!==new URL(process.env.E2E_BASE_URL??'http://localhost:18084').origin||u.pathname.startsWith('/api/'))return r.fallback();if(u.pathname.startsWith('/assets/'))return r.fulfill({contentType:extname(u.pathname)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',basename(u.pathname)))});if(['/', '/connections'].includes(u.pathname))return r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))});return r.fallback()})}
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto(view==='home'?'/':'/connections?tab=storage');
  await expect(page.getByRole('heading',{name:view==='home'?'首页':'115 官方授权',exact:true})).toBeVisible();await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  await page.screenshot({path:join(dir,`${before?'before':'after'}-${view}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
 });
});
