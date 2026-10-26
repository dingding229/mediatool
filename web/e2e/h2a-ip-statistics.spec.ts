import {test,expect} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {u5PlaybackFixture,mediaUser} from './u5-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/h2a');mkdirSync(dir,{recursive:true});
const before=process.env.H2A_BASELINE==='1',userId=mediaUser.id;
const since='2026-10-23T09:00:00Z';
const rows=[{ip:'114.114.114.114',label:'中国 江苏省 南京市 电信',location:'中国 江苏省 南京市',country:'中国',province:'江苏省',city:'南京市',isp:'电信',users:1,devices:1,count:12,firstSeenAt:since,lastSeenAt:since,accounts:[{id:userId,name:'家庭成员'}],clients:[{id:'living-room',name:'客厅平板',client:'SenPlayer'}],newIP:true,differentRegion:false},{ip:'240e:3b7:3272::1',label:'中国 广东省 深圳市 电信',location:'中国 广东省 深圳市',country:'中国',province:'广东省',city:'深圳市',isp:'电信',users:1,devices:1,count:3,firstSeenAt:since,lastSeenAt:since,accounts:[{id:userId,name:'家庭成员'}],clients:[{id:'phone',name:'手机',client:'Emby'}],newIP:false,differentRegion:true}];
const summary={uniqueIPs:2,regions:2,activeUsers:1,primaryRegion:'中国 江苏省 南京市',provinces:[{name:'中国 江苏省',count:12},{name:'中国 广东省',count:3}],cities:[{name:'中国 江苏省 南京市',count:12},{name:'中国 广东省 深圳市',count:3}],operators:[{name:'电信',count:15}]};
for(const width of [1440,390])for(const theme of ['light','dark'])test(`H2a IP statistics and settings ${width} ${theme}`,async({page})=>{
 await page.setViewportSize({width,height:950});await u5PlaybackFixture(page);await page.addInitScript(mode=>localStorage.setItem('media-tool:theme-mode:v1',mode),theme);
 await page.route('**/api/v1/system/version',r=>r.fulfill({json:{data:{version:before?'0.29.0':'0.30.0',commit:'a'.repeat(40),buildTime:since,schema:before?79:80,minimumSchema:79,pluginPin:'fixture'}}}));
 await page.route('**/api/v1/settings/application',r=>r.fulfill({json:{data:{revision:1,value:{trustedProxyCidrs:[],searchCacheSeconds:3600,tmdbCacheSeconds:86400,playbackRetentionDays:180}}}}));
 await page.route('**/api/v1/settings/access',r=>r.fulfill({json:{data:{revision:1,value:{managementUrl:'https://management.example.test',playbackUrl:'https://play.example.test'}}}}));
 await page.route('**/api/v1/ip-geolocation?*',r=>r.fulfill({json:{data:rows}}));
 await page.route('**/api/v1/media-server/playback-statistics?*',r=>r.fulfill({json:{data:{count:15,seconds:600,movies:1,series:0,episodes:0,top:[],trend:[]}}}));
 const queries:string[]=[];
 await page.route('**/api/v1/media-server/playback-ips?*',r=>{const q=new URL(r.request().url()).searchParams;queries.push(q.toString());const filtered=q.get('q')?rows.filter(row=>(row.ip+row.location+row.isp).includes(q.get('q')!)):rows;return r.fulfill({json:{data:{items:filtered,total:filtered.length,startIndex:0,limit:25,summary:{...summary,uniqueIPs:filtered.length,regions:filtered.length,activeUsers:filtered.length?1:0,provinces:summary.provinces.filter(region=>filtered.some(row=>row.location.startsWith(region.name))),cities:summary.cities.filter(region=>filtered.some(row=>row.location===region.name)),operators:filtered.length?[{name:'电信',count:filtered.reduce((n,row)=>n+row.count,0)}]:[]}}}})});
 await page.route('**/api/v1/media-server/playback-events?*',r=>r.fulfill({json:{data:{items:[],total:0,startIndex:0,limit:25}}}));
 await page.route('**/api/v1/media-server/users/*/devices?*',r=>r.fulfill({json:{data:{items:[{id:'living-room',name:'客厅平板',client:'SenPlayer'}],total:1}}}));
 const check=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 await page.goto('/playback?tab=ips');await expect(page.locator('.f7-statistics')).toBeVisible();await expect(page.locator('.f7-statistics')).toContainText('114.114.114.114');
 await page.screenshot({path:resolve(dir,`${before?'before':'after'}-overall-${width}-${theme}.png`),fullPage:true,animations:'disabled'});await check();
 if(!before){for(const label of ['独立 IP 数','地区数','活跃用户数','运营商分布','IP 明细'])await expect(page.getByText(label,{exact:true})).toBeVisible();await page.getByRole('textbox',{name:'搜索 IP 统计',exact:true}).fill('深圳');await page.getByRole('button',{name:'搜索 IP',exact:true}).click();await expect.poll(()=>queries.some(q=>new URLSearchParams(q).get('q')==='深圳')).toBe(true);await expect(page.locator('.ip-details')).toContainText('深圳市');await expect(page.locator('.ip-details')).not.toContainText('114.114.114.114');await expect(page.locator('.ip-kpis dd').first()).toHaveText('1');await page.getByRole('textbox',{name:'搜索 IP 统计',exact:true}).fill('不存在');await page.getByRole('button',{name:'搜索 IP',exact:true}).click();await expect(page.getByText('暂无匹配的公网 IP',{exact:true})).toBeVisible();await expect(page.locator('.ip-kpis dd').first()).toHaveText('0');await expect(page.locator('.ip-report .el-empty')).toHaveCount(1);await page.screenshot({path:resolve(dir,`after-empty-${width}-${theme}.png`),fullPage:true,animations:'disabled'});}
 await page.goto('/playback?tab=users');await page.getByRole('button',{name:'用户详情',exact:true}).click();await page.getByRole('button',{name:'播放统计 / IP',exact:true}).click();await page.locator('.media-detail').getByRole('button',{name:'IP 统计',exact:true}).click();await expect(page.locator('.media-detail .f7-statistics')).toContainText('114.114.114.114');
 if(!before){await expect(page.locator('.media-detail')).toContainText('新 IP');await expect(page.locator('.media-detail')).toContainText('异地');await expect(page.locator('.media-detail')).toContainText('SenPlayer');}
 await page.screenshot({path:resolve(dir,`${before?'before':'after'}-user-${width}-${theme}.png`),fullPage:true,animations:'disabled'});await check();
 await page.locator(before?'.media-detail .f7-results':'.media-detail .ip-details').scrollIntoViewIfNeeded();await page.screenshot({path:resolve(dir,`${before?'before':'after'}-user-details-${width}-${theme}.png`),animations:'disabled'});
 await page.goto('/settings?tab=general');await expect(page.getByRole('heading',{name:'设置',exact:true})).toBeVisible();await expect(page.locator('.f7-geo')).toHaveCount(0);
 await page.screenshot({path:resolve(dir,`${before?'before':'after'}-settings-${width}-${theme}.png`),animations:'disabled'});await check();
});
