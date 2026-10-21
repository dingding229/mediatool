import type {Page} from '@playwright/test';
import {u4Fixture,organizeFixture,libraryItem} from './u4-fixture';
import {u3Fixture,u3DownloadsFixture} from './u3-fixture';
import {u5PlaybackFixture,u5SitesFixture} from './u5-fixture';
import {u6PluginsFixture,u6OpsFixture,opsSnapshot} from './u6-fixture';
export const settingsRecord={revision:3,value:{movieTemplate:'{title} ({year})/{title}',seriesTemplate:'{title}/Season {season}/{title} - {season_episode}',trustedProxyCidrs:[],searchCacheSeconds:600,tmdbCacheSeconds:86400,subscriptionCron:'*/30 * * * *',cleanupAfterImport:false,embyUserId:'',scraping:{movie:{nfo:true,poster:true},series:{nfo:true,poster:true},season:{nfo:true},episode:{nfo:true}}}};
const connection={id:'owned-emby',kind:'emby',name:'家庭媒体服务',enabled:true,revision:2,settings:{endpoint:'https://emby.example.test'},secretFields:['apiKey'],lastCheckState:'ready'};
export async function u7Fixture(page:Page,path:string){
 if(path==='/search')await u3Fixture(page);
 else if(path==='/downloads')await u3DownloadsFixture(page);
 else if(path==='/organize')await organizeFixture(page);
 else if(path==='/playback')await u5PlaybackFixture(page);
 else if(path==='/sites')await u5SitesFixture(page);
 else if(path==='/plugins')await u6PluginsFixture(page);
 else if(path==='/ops')await u6OpsFixture(page);
 else await u4Fixture(page);
 if(['/dashboard','/connections'].includes(path)){
  await page.route('**/api/v1/readiness',r=>r.fulfill({json:{data:{components:{open115:{state:'ready',reason:'fresh_saved_probe'},clouddrive2:{state:'ready',reason:'fresh_saved_probe'},emby:{state:'ready',reason:'fresh_saved_probe'},strm:{state:'configured',nextAction:'verify_player_302',reason:'stable_resolver_configured_not_player_verified'}}}}}));
  await page.route('**/api/v1/system/runtime',r=>r.fulfill({json:{data:{server:{loadAverage:[.4,.5,.6],memoryUsed:2147483648,memoryTotal:8589934592,diskAvailable:107374182400,diskTotal:214748364800,uptimeSeconds:86400,sampledAt:'2026-10-21T00:00:00Z'},daily:{sites:2,subscriptions:3,downloads:1,media:50,todaySearches:4}}}}));
  await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[connection,{...connection,id:'site',kind:'ptsite',name:'家庭站点'}]}}));
  await page.route('**/api/v1/ops',r=>r.fulfill({json:{data:{...opsSnapshot,alerts:[]}}}));
  await page.route('**/api/v1/jobs/browse?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));
  await page.route('**/api/v1/library/candidates?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));
  await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));
 }
 if(path==='/settings'){
  await page.route('**/api/v1/settings/application',r=>r.fulfill({json:{data:settingsRecord}}));
  await page.route('**/api/v1/settings/access',r=>r.fulfill({json:{data:{revision:3,value:{managementUrl:'',playbackUrl:'',memberUrl:'',embyUrl:''},effective:{managementUrl:'https://admin.example.test',playbackUrl:'https://play.example.test',memberUrl:'',embyUrl:''},listeners:{management:':8080',member:':8097',emby:':8096'}}}}));
  await page.route('**/api/v1/settings/naming-preview',r=>r.fulfill({json:{data:{movie:'影片 (2026)/影片.mkv',series:'剧集/Season 01/剧集 - S01E01.mkv'}}}));
  await page.route('**/api/v1/classification',r=>r.fulfill({json:{data:{revision:2,value:{groups:[{id:'movies',name:'电影',directory:'电影',mediaType:'movie',enabled:true,rules:[]}]}}}}));
 }
 if(path==='/subscriptions')await page.route('**/api/v1/subscriptions?*',r=>r.fulfill({json:{data:[{id:'owned-sub',title:'海边的故事',media:{id:42,mediaType:'movie',title:'海边的故事',year:'2026'},enabled:true,source:'both',revision:3,rules:{accountIds:[],include:[],exclude:[],freeOnly:true,rejectHR:true},lastState:'succeeded',lastMessage:'已覆盖的作品不会重复下载'}]}}));
 if(path==='/files'){
  const capabilities={read:true,write:true,move:true,recycle:true,readOnly:false,role:'workspace'};
  await page.route('**/api/v1/files/roots',r=>r.fulfill({json:{data:[{id:'owned-root',name:'媒体工作区',path:'/owned',kind:'local',capabilities}]}}));
  await page.route('**/api/v1/files?*',r=>r.fulfill({json:{data:{items:[{id:'folder',name:'电影',path:'电影',directory:true,capabilities},{id:'file',name:'海边的故事.mkv',path:'海边的故事.mkv',size:5368709120,directory:false,capabilities}],total:2,capabilities}}}));
 }
 if(path==='/library')await page.route('**/api/v1/media/library?*',r=>r.fulfill({json:{data:{items:[libraryItem,{...libraryItem,id:'owned-2',title:'山间来信',year:'2025'},{...libraryItem,id:'owned-3',title:'春日旅程'}],total:72,page:1,pageSize:24,source:'managed'}}}));
}
