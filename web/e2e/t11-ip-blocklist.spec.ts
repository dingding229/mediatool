import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync,writeFileSync,appendFileSync,existsSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {u5PlaybackFixture,mediaUser} from './u5-fixture';
import {fixtureFulfill} from './collection-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i11/screens'),userId=mediaUser.id;
const screenshots=process.env.I11_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
mkdirSync(dir,{recursive:true});
const since='2026-10-23T09:00:00Z';
const rows=[{ip:'114.114.114.114',label:'中国 江苏省 南京市 电信',location:'中国 江苏省 南京市',country:'中国',province:'江苏省',city:'南京市',isp:'电信',users:1,devices:1,count:12,firstSeenAt:since,lastSeenAt:since,accounts:[{id:userId,name:'家庭成员'}],clients:[{id:'living-room',name:'客厅平板',client:'SenPlayer'}],newIP:true,differentRegion:false},{ip:'240e:3b7:3272::1',label:'中国 广东省 深圳市 电信',location:'中国 广东省 深圳市',country:'中国',province:'广东省',city:'深圳市',isp:'电信',users:1,devices:1,count:3,firstSeenAt:since,lastSeenAt:since,accounts:[{id:userId,name:'家庭成员'}],clients:[{id:'phone',name:'手机',client:'Emby'}],newIP:false,differentRegion:true}];
const summary={uniqueIPs:2,regions:2,activeUsers:1,primaryRegion:'中国 江苏省 南京市',provinces:[{name:'中国 江苏省',count:12},{name:'中国 广东省',count:3}],cities:[{name:'中国 江苏省 南京市',count:12},{name:'中国 广东省 深圳市',count:3}],operators:[{name:'电信',count:15}]};

const runtimeErrors=new WeakMap<Page,string[]>(),expectedBadRequest=new WeakSet<Page>();
test.afterEach(async({page})=>{expect(runtimeErrors.get(page)||[]).toEqual([])});
async function fixture(page:Page){
 const errors:string[]=[];runtimeErrors.set(page,errors);page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='error'&&!m.text().includes('Service Worker registration blocked')&&!(expectedBadRequest.has(page)&&/^Failed to load resource:.*400 \(Bad Request\)/.test(m.text())))errors.push(m.text().slice(0,800))});
 page.setDefaultTimeout(7000);page.on('console',m=>{if(m.type()==='error'||m.type()==='warning')appendFileSync(join(dir,'console-errors.log'),m.text().slice(0,800)+'\n')});
 await u5PlaybackFixture(page);
 await page.route('**/api/v1/media-server/users/*/devices?*',r=>r.fulfill({json:{data:{items:[],total:0,startIndex:0,limit:50}}}));
 await page.route('**/api/v1/media-server/playback-statistics?*',r=>r.fulfill({json:{data:{count:0,seconds:0,movies:0,series:0,episodes:0,top:[],trend:[]}}}));
 await page.route('**/api/v1/media-server/playback-events?*',r=>r.fulfill({json:{data:{items:[],total:0,startIndex:0,limit:25}}}));
 const writes:any[]=[];let revision=1,available=true;
 let rules=[{id:'11111111-0000-4000-8000-000000000001',network:'203.0.113.9/32',note:'文档示例，仅测试',createdAt:since,updatedAt:since,hitCount:0,lastHitAt:null}];
 await page.route('**/api/v1/player-access/ip-blocklist**',async r=>{
  const req=r.request(),path=new URL(req.url()).pathname,method=req.method();
  if(method==='GET')return r.fulfill({json:{data:{items:rules,total:rules.length,revision,status:{available,retrySeconds:available?0:1,message:available?'':'IP 黑名单策略暂不可用，媒体请求返回 503；后台自动重试',counterWarning:false},currentIp:'8.8.8.8',maxRules:10000}}});
  const v=req.postDataJSON();
  if(path.endsWith('/check')){
   if(v.network?.includes('/0')||v.network==='bad'||v.address==='bad')return r.fulfill({status:400,json:{error:{code:'invalid_player_ip',message:'请输入有效 IP 或规范 CIDR，不能使用 /0'}}});
   const network=v.network?(v.network.includes('/')?v.network:v.network.includes(':')?v.network.replace(/::[^:]*$/,'::')+'/64':v.network.split('.').slice(0,3).join('.')+'.0/24'):'';
   const coveredBy=rules.find(x=>x.id!==v.excludeId&&x.network===network)?.network;
   const match=rules.find(x=>x.network===v.address+(v.address.includes(':')?'/128':'/32'));
   return r.fulfill({json:{data:{matched:!!match,ruleId:match?.id,network:match?.network,revision,candidateNetwork:network,coveredBy,affectsCurrent:network==='8.8.8.0/24'}}});
  }
  if(v.revision!==revision)return r.fulfill({status:409,json:{error:{code:'player_ip_conflict',message:'IP 黑名单规则已修改，请刷新后重试'}}});
  writes.push({path,method,...v});revision++;
  if(method==='DELETE')rules=rules.filter(x=>!path.endsWith(x.id));
  else if(method==='PUT')rules=rules.map(x=>path.endsWith(x.id)?{...x,network:v.network,note:v.note}:x);
  else rules.push({...rules[0]!,id:'11111111-0000-4000-8000-'+String(revision).padStart(12,'0'),network:v.network,note:v.note});
  return r.fulfill({json:{data:{revision}}});
 });

 await page.route('**/api/v1/media-server/playback-ips?*',r=>fixtureFulfill(r,{json:{data:{items:rows,total:2,startIndex:0,limit:25,summary}}}));
 await page.route('**/api/v1/ip-geolocation?*',r=>fixtureFulfill(r,{json:{data:rows}}));
 return {writes,setAvailable:(v:boolean)=>available=v};
}
test.describe('I11 screenshots @local-screenshots',()=>{
 test.skip(!screenshots,'Explicit local-only I11_SCREENSHOTS=1');
 for(const kind of ['blocklist','ips','user-ips'])for(const width of [1440,390])for(const theme of ['light','dark'])test(`${kind}-${width}-${theme}`,async({page})=>{
  await fixture(page);const before=process.env.I11_BEFORE==='1';
  if(before){const root=resolve(dir,'../before-assets');await page.route('**/playback?*',r=>r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}));await page.route('**/assets/*',r=>{const n=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(n)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',n))})})}
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);
  await page.goto('/playback?tab='+(kind==='blocklist'?'ip-blocklist':kind==='ips'?'ips':'users'));
  if(kind==='user-ips'){await page.getByRole('button',{name:'用户详情',exact:true}).click();await page.getByRole('button',{name:'播放统计 / IP',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'IP 统计',exact:true}).click()}
  if(kind!=='blocklist')await expect(page.locator('.ip-details tbody tr')).toHaveCount(2);else await expect(page.getByRole('heading',{name:before?'IP 拉黑':'IP 黑名单',exact:true})).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  await page.screenshot({path:join(dir,`${before?'before':'after'}-${kind}-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
 });
});

test('T11 CRUD, read-only preview, validation and self-impact confirmation',async({page})=>{
 const f=await fixture(page);expectedBadRequest.add(page);await page.goto('/playback?tab=ip-blocklist');await expect(page.getByText('203.0.113.9/32',{exact:true})).toBeVisible();
 await page.getByLabel('预检 IP 地址',{exact:true}).fill('203.0.113.9');await page.getByRole('button',{name:'检查是否命中',exact:true}).click();await expect(page.getByRole('status').filter({hasText:'已命中规则'})).toBeVisible();expect(f.writes).toHaveLength(0);
 await page.getByRole('button',{name:'添加 IP 规则',exact:true}).click();let dialog=page.getByRole('dialog',{name:'添加 IP 黑名单规则',exact:true});await dialog.getByLabel('黑名单 IP 或 CIDR',{exact:true}).fill('0.0.0.0/0');await dialog.getByRole('button',{name:'核对并继续',exact:true}).click();await expect(dialog).toContainText('不能使用 /0');expect(f.writes).toHaveLength(0);
 await dialog.getByLabel('黑名单 IP 或 CIDR',{exact:true}).fill('8.8.8.8');await dialog.getByLabel('IP 黑名单备注',{exact:true}).fill('自身风险验证');await dialog.getByRole('button',{name:'核对并继续',exact:true}).click();const confirmation=page.getByRole('dialog',{name:'确认 IP 黑名单范围',exact:true});await expect(confirmation).toContainText('覆盖当前管理员访问 IP');await expect(confirmation.getByRole('button',{name:'确认保存黑名单规则',exact:true})).toBeDisabled();await confirmation.getByText('我已确认自身媒体访问将受限，仍要加入 IP 黑名单',{exact:true}).click();await expect(confirmation.getByRole('checkbox')).toBeChecked();await confirmation.getByRole('button',{name:'确认保存黑名单规则',exact:true}).click();await expect(page.locator('.ip-block-row').filter({hasText:'8.8.8.0/24'})).toBeVisible();expect(f.writes).toHaveLength(1);expect(f.writes[0].confirmSelf).toBe(true);
 const row=page.locator('.ip-block-row').filter({hasText:'8.8.8.0/24'});await row.getByRole('button',{name:'编辑',exact:true}).click();dialog=page.getByRole('dialog',{name:'编辑 IP 黑名单规则',exact:true});await dialog.getByLabel('IP 黑名单备注',{exact:true}).fill('已编辑');await dialog.getByRole('button',{name:'核对并继续',exact:true}).click();await confirmation.getByText('我已确认自身媒体访问将受限，仍要加入 IP 黑名单',{exact:true}).click();await expect(confirmation.getByRole('checkbox')).toBeChecked();await confirmation.getByRole('button',{name:'确认保存黑名单规则',exact:true}).click();await expect(row).toContainText('已编辑');
 await row.getByRole('button',{name:'删除',exact:true}).click();const del=page.getByRole('dialog',{name:'删除 IP 黑名单规则',exact:true});await del.getByRole('button',{name:'取消',exact:true}).click();expect(f.writes).toHaveLength(2);await row.getByRole('button',{name:'删除',exact:true}).click();await del.getByRole('button',{name:'确认删除规则',exact:true}).click();await expect(row).toHaveCount(0);expect(f.writes).toHaveLength(3);
});
test('T11 IP detail actions require confirmation, including single user',async({page})=>{
 const f=await fixture(page);await page.goto('/playback?tab=ips');await page.locator('.ip-details tbody tr').first().getByRole('button',{name:'加入 IP 黑名单',exact:true}).click();let dialog=page.getByRole('dialog',{name:'添加 IP 黑名单规则',exact:true});await expect(dialog.getByLabel('黑名单 IP 或 CIDR',{exact:true})).toHaveValue(rows[0]!.ip);expect(f.writes).toHaveLength(0);await dialog.getByRole('button',{name:'取消',exact:true}).click();
 await page.goto('/playback?tab=users');await page.getByRole('button',{name:'用户详情',exact:true}).click();await page.getByRole('button',{name:'播放统计 / IP',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'IP 统计',exact:true}).click();await page.locator('.ip-details tbody tr').last().getByRole('button',{name:'加入 IP 黑名单',exact:true}).click();dialog=page.getByRole('dialog',{name:'添加 IP 黑名单规则',exact:true});await expect(dialog.getByLabel('黑名单 IP 或 CIDR',{exact:true})).toHaveValue(rows[1]!.ip);expect(f.writes).toHaveLength(0);await dialog.getByRole('button',{name:'核对并继续',exact:true}).click();await page.getByRole('dialog',{name:'确认 IP 黑名单范围',exact:true}).getByRole('button',{name:'确认保存黑名单规则',exact:true}).click();expect(f.writes).toHaveLength(1);
});
test('T11 unavailable cache warning disappears automatically after recovery',async({page})=>{const f=await fixture(page);f.setAvailable(false);await page.goto('/playback?tab=ip-blocklist');await expect(page.getByRole('alert')).toContainText('503');await expect(page.getByRole('button',{name:'添加 IP 规则',exact:true})).toBeDisabled();f.setAvailable(true);await expect(page.getByRole('button',{name:'添加 IP 规则',exact:true})).toBeEnabled({timeout:10000});});

test('T11 nested editor and confirmation fit both widths and themes',async({page})=>{
 await fixture(page);
 for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/playback?tab=users');await page.getByRole('button',{name:'用户详情',exact:true}).click();await page.getByRole('button',{name:'播放统计 / IP',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'IP 统计',exact:true}).click();await page.locator('.ip-details tbody tr').first().getByRole('button',{name:'加入 IP 黑名单',exact:true}).click();const editor=page.getByRole('dialog',{name:'添加 IP 黑名单规则',exact:true});await expect(editor.getByRole('button',{name:'核对并继续',exact:true})).toBeEnabled();if(width===390&&theme==='dark')await editor.getByLabel('黑名单 IP 或 CIDR',{exact:true}).fill('8.8.8.8');await expect.poll(()=>editor.evaluate(e=>{const r=e.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&getComputedStyle(e).opacity==='1'&&getComputedStyle(e).transform==='none'})).toBe(true);if(screenshots)await page.screenshot({path:join(dir,`after-editor-${width}-${theme}.png`),fullPage:true,animations:'disabled'});await editor.getByRole('button',{name:'核对并继续',exact:true}).click();const confirm=page.getByRole('dialog',{name:'确认 IP 黑名单范围',exact:true});await expect.poll(()=>confirm.evaluate(e=>{const r=e.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&getComputedStyle(e).opacity==='1'&&getComputedStyle(e).transform==='none'})).toBe(true);if(screenshots)await page.screenshot({path:join(dir,`after-confirm-${width}-${theme}.png`),fullPage:true,animations:'disabled'});await confirm.getByRole('button',{name:'返回修改',exact:true}).click();await editor.getByRole('button',{name:'取消',exact:true}).click();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 }
});

test('I11 covered candidate is not submitted and explicit CIDR remains editable',async({page})=>{
 const f=await fixture(page);await page.goto('/playback?tab=ip-blocklist');await page.getByRole('button',{name:'添加 IP 规则',exact:true}).click();
 const editor=page.getByRole('dialog',{name:'添加 IP 黑名单规则',exact:true});await editor.getByLabel('黑名单 IP 或 CIDR',{exact:true}).fill('192.0.2.9');await editor.getByRole('button',{name:'核对并继续',exact:true}).click();
 const confirmation=page.getByRole('dialog',{name:'确认 IP 黑名单范围',exact:true});await expect(confirmation).toContainText('将拉黑网段：192.0.2.0/24');expect(f.writes).toHaveLength(0);await confirmation.getByRole('button',{name:'确认保存黑名单规则',exact:true}).click();
 await page.getByRole('button',{name:'添加 IP 规则',exact:true}).click();await editor.getByLabel('黑名单 IP 或 CIDR',{exact:true}).fill('192.0.2.99');await editor.getByRole('button',{name:'核对并继续',exact:true}).click();await expect(editor).toContainText('已在 192.0.2.0/24 中');expect(f.writes).toHaveLength(1);
 await editor.getByLabel('黑名单 IP 或 CIDR',{exact:true}).fill('192.0.3.9/32');await editor.getByRole('button',{name:'核对并继续',exact:true}).click();await expect(confirmation).toContainText('192.0.3.9/32');await confirmation.getByRole('button',{name:'返回修改',exact:true}).click();await editor.getByRole('button',{name:'取消',exact:true}).click();expect(f.writes).toHaveLength(1);
});
