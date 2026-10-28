import {chooseUASelect,expectUASelect} from './ua-select';
import {test,expect,type Page,type Locator} from '@playwright/test';
import {mkdirSync,readFileSync} from 'node:fs';
import {basename,extname,resolve} from 'node:path';
import {u5PlaybackFixture} from './u5-fixture';
const out=resolve(process.env.I9_SCREENSHOT_DIR||resolve(import.meta.dirname,'../../scratchpad/i-phase/i9/screens'));
const longUA='SyntheticPlayer/1.0 '+('long UA value <> & / + '.repeat(25));
const rules=[{id:'prefix',label:'客厅播放器',userAgent:'Synthetic/',matchType:'prefix',enabled:true},{id:'exact',label:'完整匹配设备',userAgent:'Synthetic/Exact',matchType:'exact',enabled:false},{id:'contains',label:'包含匹配设备',userAgent:'Part',matchType:'contains',enabled:true}];
const observations=[{id:'long',userAgent:longUA,requestCount:12,allowedCount:10,deniedCount:2,whitelisted:false,blacklisted:true,lastClientIp:'2001:db8::1234',lastSeenAt:'2026-10-28T00:00:00Z',lastReason:'blacklisted'},{id:'empty',userAgent:'',requestCount:1,allowedCount:0,deniedCount:1,whitelisted:false,blacklisted:false,lastClientIp:'8.8.8.8',lastSeenAt:'2026-10-27T00:00:00Z',lastReason:'ua_not_whitelisted'}];
const visits=[{id:'visit-1',userAgent:longUA,allowed:false,status:403,reason:'blacklisted',clientIp:'2001:db8::1234',connectionIp:'8.8.8.8',ipSource:'trusted_proxy',ipPublic:false,method:'GET',createdAt:'2026-10-28T00:00:00Z'}];
async function fixture(page:Page,large=false){
 await u5PlaybackFixture(page);const calls:URL[]=[],writes:string[]=[],policy={enabled:true,mode:'whitelist',revision:4,rules,blacklistRules:[{...rules[0],id:'black',label:'黑名单设备'}]};let fail=false;
 page.on('request',r=>{if(r.url().includes('/api/')&&r.method()!=='GET')writes.push(r.url())});
 await page.route('**/api/v1/player-access/policy',r=>r.fulfill({json:{data:policy}}));
 for(const [endpoint,source]of [['observations',observations],['events',visits]]as const)await page.route(`**/api/v1/player-access/${endpoint}?*`,r=>{const url=new URL(r.request().url());calls.push(url);if(fail)return r.fulfill({status:503,json:{error:{message:'读取失败'}}});const all=large?Array.from({length:61},(_,i)=>({...source[0],id:`${endpoint}-${i}`})):source,p=Number(url.searchParams.get('page')),size=Number(url.searchParams.get('pageSize'));return r.fulfill({json:{data:{items:all.slice((p-1)*size,p*size),total:all.length,page:p,pageSize:size}}})});
 await page.route('**/api/v1/ip-geolocation?*',r=>r.fulfill({json:{data:new URL(r.request().url()).searchParams.get('ips')!.split(',').map(ip=>({ip,label:'测试地区 长归属地 运营商'}))}}));
 return{calls,writes,policy,fail:()=>{fail=true}};
}
async function resize(nav:Locator,page:Page,value:string){await nav.locator('.el-select').click();const id=await nav.getByRole('combobox').getAttribute('aria-controls');expect(id).toBeTruthy();await page.locator(`[id="${id}"]`).getByRole('option',{name:value,exact:true}).click()}
async function layout(page:Page,width:number){
 await expectUASelect(page,'UA记录筛选','');await expect(page.locator('.ua-rule-row')).toHaveCount(3);await expect(page.locator('.ua-observation-row')).toHaveCount(2);await expect(page.locator('.ua-visit-row')).toHaveCount(1);
 const reports=await page.locator('.ua-list-row').evaluateAll(els=>els.map(el=>{const r=el.getBoundingClientRect(),list=el.parentElement!.getBoundingClientRect(),actions=el.querySelector('.ua-row-actions')!.getBoundingClientRect(),identity=el.querySelector('.ua-rule-identity,.ua-observation-identity,.ua-visit-result')!.getBoundingClientRect();return{rowWidth:r.width,rowHeight:r.height,isRule:el.classList.contains('ua-rule-row'),listWidth:list.width,rightDelta:Math.abs(r.right-actions.right),below:actions.top>=identity.bottom,buttons:[...el.querySelectorAll('button')].map(b=>{const r=b.getBoundingClientRect();return{width:r.width,height:r.height,label:b.getAttribute('aria-label')}})}}));
 for(const r of reports){expect(Math.abs(r.rowWidth-r.listWidth)).toBeLessThan(1);expect(r.rightDelta).toBeLessThan(1);for(const b of r.buttons){expect(b.width).toBeGreaterThanOrEqual(width<1241?44:32);expect(b.height).toBeGreaterThanOrEqual(width<1241?44:32);expect(b.label).toBeTruthy()}if(width<1241)expect(r.below).toBe(true);else if(r.isRule){expect(r.rowHeight).toBeGreaterThanOrEqual(56);expect(r.rowHeight).toBeLessThanOrEqual(72);}}
 const controls=await page.evaluate(()=>{
  const geometry=(el:Element)=>{const r=el.getBoundingClientRect(),s=getComputedStyle(el);return{height:r.height,radius:s.borderTopLeftRadius}};
  const reference=geometry(document.querySelector('.page-pagination .el-select__wrapper,.el-pagination .el-select__wrapper,nav[aria-label="分页"] .el-select__wrapper')!);
  const selects=['UA 策略模式','UA 规则名单','UA记录筛选'].map(name=>geometry(document.querySelector(`[aria-label="${name}"]`)!.closest('.el-select')!.querySelector('.el-select__wrapper')!));
  const note=document.querySelector('.ua-policy-summary')!,field=note.nextElementSibling!;
  return{reference,selects,noteGap:field.getBoundingClientRect().top-note.getBoundingClientRect().bottom,noteHeight:note.getBoundingClientRect().height};
 });
 for(const select of controls.selects){expect(select.height).toBe(controls.reference.height);expect(select.height).toBe(44);expect(select.radius).toBe(controls.reference.radius);expect(parseFloat(select.radius)).toBeGreaterThan(0)}
 expect(controls.noteGap).toBeGreaterThanOrEqual(0);expect(controls.noteGap).toBeLessThanOrEqual(16);if(width===390)expect(controls.noteHeight).toBeLessThan(70);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 if(width===1440){
  for(const [header,row,cells]of [['.ua-rule-columns','.ua-rule-row','.ua-rule-identity>h3,.ua-rule-identity>.ua-match,.ua-rule-value,.ua-rule-state,.ua-row-actions'],['.ua-observation-columns','.ua-observation-row','.ua-observation-identity>.ua-summary-value,.ua-counts,.ua-recent,.ua-row-actions'],['.ua-visit-columns','.ua-visit-row','.ua-visit-result,.ua-addresses,.ua-visit-context>.ua-summary-value,.ua-row-actions']]){
   const delta=await page.evaluate(({header,row,cells})=>{const heads=[...document.querySelectorAll(header+'>span')].map(e=>e.getBoundingClientRect().x);return [...document.querySelector(row)!.querySelectorAll(cells)].map((el,i)=>Math.abs(el.getBoundingClientRect().x-heads[i]!))},{header:header!,row:row!,cells:cells!});expect(delta.every(d=>d<1)).toBe(true);
  }
 }else{await expect(page.locator('.ua-rule-columns')).toBeHidden();await expect(page.locator('.ua-action-text').first()).toBeHidden();await expect(page.locator('.ua-action-icon').first()).toBeVisible();}
}
for(const width of [1440,390])for(const theme of ['light','dark'])test(`I9 list geometry, read-only details and existing operation entries ${width} ${theme}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);const f=await fixture(page);await page.goto('/playback?tab=players');await layout(page,width);
 const rule=page.locator('.ua-rule-row').first();await rule.getByRole('button',{name:'编辑规则',exact:true}).click();const edit=page.getByRole('dialog',{name:'编辑白名单规则',exact:true});await expect(edit.getByLabel('完整 User-Agent',{exact:true})).toHaveValue('Synthetic/');await edit.getByRole('button',{name:'取消',exact:true}).click();await expect(edit).toBeHidden();
 await rule.getByRole('button',{name:'删除规则',exact:true}).click();const remove=page.getByRole('dialog',{name:'删除 UA 白名单规则',exact:true});await remove.getByRole('button',{name:'取消',exact:true}).click();await expect(remove).toBeHidden();
 const row=page.locator('.ua-observation-row').first();await expect(row.getByRole('button',{name:'加入黑名单',exact:true})).toBeDisabled();await expect(page.locator('.ua-observation-row').nth(1).getByRole('button',{name:'加入白名单',exact:true})).toBeDisabled();
 await row.getByRole('button',{name:'查看完整 UA',exact:true}).click();let detail=page.getByRole('dialog',{name:'完整 UA 与访问详情',exact:true});await expect(detail.getByLabel('实际收到的完整 UA',{exact:true})).toHaveValue(longUA);await detail.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(detail).toBeHidden();
 await row.getByRole('button',{name:'加入白名单',exact:true}).click();const add=page.getByRole('dialog',{name:'从访问记录加入白名单',exact:true});await expect(add.getByLabel('完整 User-Agent',{exact:true})).toHaveAttribute('readonly','');await add.getByRole('button',{name:'取消',exact:true}).click();
 await page.locator('.ua-visit-row').getByRole('button',{name:'访问详情',exact:true}).click();detail=page.getByRole('dialog',{name:'完整 UA 与访问详情',exact:true});await expect(detail.getByLabel('实际收到的完整 UA',{exact:true})).toHaveValue(longUA);await detail.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(detail).toBeHidden();
 await chooseUASelect(page,'UA 规则名单','blacklistRules');await expect(page.getByRole('list',{name:'黑名单规则列表',exact:true})).toContainText('黑名单设备');expect(f.policy.rules).toHaveLength(3);expect(f.writes).toEqual([]);
});
test('I9 intermediate width remains compact without overflow',async({page})=>{await page.setViewportSize({width:1180,height:1000});await fixture(page);await page.goto('/playback?tab=players');await layout(page,1180)});
test('I9 summary and visit pagination retain independent URL sizes and search resets both pages',async({page})=>{
 const f=await fixture(page,true);await page.goto('/playback?tab=players&playersPage=2&visitPage=2');await expect(page.locator('.ua-observation-row')).toHaveCount(25);await expect(page.locator('.ua-visit-row')).toHaveCount(25);
 const nav=page.getByRole('navigation',{name:'分页',exact:true});await resize(nav.first(),page,'10');await expect(page.locator('.ua-observation-row')).toHaveCount(10);await expect(page).toHaveURL(/playersPageSize=10/);await expect(nav.nth(1)).toContainText('第 2 页');
 await resize(nav.nth(1),page,'50');await expect(page.locator('.ua-visit-row')).toHaveCount(50);await expect(page).toHaveURL(/visitPageSize=50/);await page.reload();await expect(page.locator('.ua-observation-row')).toHaveCount(10);await expect(page.locator('.ua-visit-row')).toHaveCount(50);
 await nav.first().getByRole('button',{name:'下一页',exact:true}).click();await nav.nth(1).getByRole('button',{name:'下一页',exact:true}).click();await page.getByLabel('搜索UA记录',{exact:true}).fill('Synthetic');await page.getByRole('button',{name:'搜索记录',exact:true}).click();for(const n of await nav.all())await expect(n).toContainText('第 1 页');expect(f.writes).toEqual([]);
});
test('I9 native-select replacement preserves list/filter selection and read-only queries',async({page})=>{
 const f=await fixture(page,true);await page.goto('/playback?tab=players&playersPage=2&visitPage=2');await expect(page.locator('.ua-observation-row')).toHaveCount(25);
 await chooseUASelect(page,'UA记录筛选','blacklisted');await expectUASelect(page,'UA记录筛选','blacklisted');await expect(page.getByRole('navigation',{name:'分页',exact:true}).first()).toContainText('第 1 页');await expect.poll(()=>f.calls.some(url=>url.pathname.endsWith('/observations')&&url.searchParams.get('filter')==='blacklisted'&&url.searchParams.get('page')==='1')).toBe(true);expect(f.writes).toEqual([]);
});
for(const width of [1440,390])test(`I9 full-length rule values remain visible text and wrap without horizontal overflow ${width}`,async({page})=>{await page.setViewportSize({width,height:1000});const f=await fixture(page);f.policy.rules=[{...rules[0]!,userAgent:'R'.repeat(4096)}];await page.goto('/playback?tab=players');const row=page.locator('.ua-rule-row');await expect(row).toHaveCount(1);await expect(row.locator('.ua-rule-value')).toHaveText('R'.repeat(4096));expect(await row.evaluate(el=>el.getBoundingClientRect().height)).toBeGreaterThan(72);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);expect(f.writes).toEqual([])});
test('I9 failed list reads preserve visible rows and allow explicit retry',async({page})=>{const f=await fixture(page);await page.goto('/playback?tab=players');await expect(page.locator('.ua-observation-row')).toHaveCount(2);f.fail();await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.getByText('读取失败',{exact:true}).first()).toBeVisible();await expect(page.locator('.ua-observation-row')).toHaveCount(2);await expect(page.locator('.ua-visit-row')).toHaveCount(1);expect(f.writes).toEqual([])});
const capture=process.env.I9_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS,before=process.env.I9_BEFORE==='1';
test.describe('I9 local visual evidence @screenshots',()=>{
 test.skip(!capture,'Local explicit opt-in only: I9_SCREENSHOTS=1');
 for(const width of [1440,390])for(const theme of ['light','dark'])test(`${width} ${theme}`,async({page})=>{
  await fixture(page);await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  if(before){const root=resolve(out,'../before-assets');await page.route('**/playback?*',r=>r.fulfill({contentType:'text/html',body:readFileSync(resolve(root,'index.html'))}));await page.route('**/assets/*',r=>{const name=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(name)==='.js'?'application/javascript':'text/css',body:readFileSync(resolve(root,'assets',name))})});}
  await page.goto('/playback?tab=players');await expect(page.getByRole('button',{name:'查看完整 UA',exact:true}).first()).toBeVisible();await expect(page.locator('.f7-ip[aria-busy=true]')).toHaveCount(0);if(!before)await layout(page,width);mkdirSync(out,{recursive:true});
  for(const [id,heading]of [['rules','播放器准入'],['summary','播放器访问汇总'],['visits','访问明细']]){await page.getByRole('heading',{name:heading,exact:true}).scrollIntoViewIfNeeded();await page.screenshot({path:resolve(out,`${before?'before':'after'}-${id}-${width}-${theme}.png`),animations:'disabled'});}
 });
});
