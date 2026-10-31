import type {Page} from '@playwright/test';
import {u5PlaybackFixture,mediaUser} from './u5-fixture';
import {compiledStaticFixture} from './compiled-static-fixture';
export const at='2026-10-30T12:00:00Z';
export async function playbackFixture(page:Page){
 await compiledStaticFixture(page,'http://localhost:18084');await u5PlaybackFixture(page);
 await page.addInitScript(()=>localStorage.setItem('media-tool:theme-mode:v1','light'));
 const reads:URL[]=[],writes:string[]=[],rules=[{id:'family',label:'家庭播放器',userAgent:'SyntheticPlayer/',matchType:'prefix',enabled:true},{id:'mobile',label:'手机播放器',userAgent:'SyntheticMobile/',matchType:'prefix',enabled:true}];
 page.on('request',r=>{if(r.url().includes('/api/v1/')){if(r.method()==='GET')reads.push(new URL(r.url()));else writes.push(new URL(r.url()).pathname)}});
 const observe=Array.from({length:16},(_,i)=>({id:'ua-'+i,userAgent:i?'SyntheticMobile/'+i:'SyntheticPlayer/1.0',requestCount:20+i,allowedCount:20+i,deniedCount:0,whitelisted:i%4===0,blacklisted:i%3===0,lastClientIp:'8.8.8.8',lastSeenAt:at,lastReason:'restriction_disabled'}));
 const visits=Array.from({length:625},(_,i)=>({id:'visit-'+i,userAgent:'SyntheticPlayer/'+i,allowed:true,status:302,reason:'restriction_disabled',clientIp:'8.8.8.8',connectionIp:i%2?'1.1.1.1':'8.8.8.8',ipSource:i%2?'trusted_proxy':'direct',ipPublic:true,method:'GET',createdAt:at}));
 await page.route('**/api/v1/player-access/policy',r=>r.fulfill({json:{data:{mode:'off',enabled:false,revision:4,rules,blacklistRules:[{...rules[0],id:'crawler',label:'异常客户端',userAgent:'Crawler',matchType:'contains'}]}}}));
 for(const [endpoint,items] of [['observations',observe],['events',visits]] as const)await page.route(`**/api/v1/player-access/${endpoint}?*`,r=>{const q=new URL(r.request().url()).searchParams,page=Number(q.get('page')||1),size=Number(q.get('pageSize')||25);return r.fulfill({json:{data:{items:items.slice((page-1)*size,page*size),total:items.length,page,pageSize:size}}})});
 const libraries=[['anime','动漫','日漫','tvshows'],['anime2','动漫','国漫','tvshows'],['movie','电影','华语电影','movies'],['movie2','电影','外语电影','movies'],['series','电视剧','国产剧','tvshows'],['series2','电视剧','日韩剧','tvshows'],['f:movie','','未分类电影','movies'],['f:series','','未分类剧集','tvshows']].map(([id,groupName,name,collectionType],i)=>({id,groupId:id!.startsWith('f:')?'':id,name,groupName,ruleName:groupName?name:'',collectionType,hidden:false,assetCount:i===6?1:i===2?3:0}));
 await page.route('**/api/v1/media-server/native/library-mappings?*',r=>r.fulfill({json:{data:{items:libraries,total:libraries.length}}}));
 await page.route('**/api/v1/media-server/native/settings',r=>r.fulfill({json:{data:{serverName:'家庭媒体库',enabled:true,revision:5,publicUrl:'https://media.fixture.invalid',listenPort:'8080'}}}));
 const users=[{...mediaUser,username:'family',displayName:'家庭成员',maxStreams:2,lastPlayedAt:at,lastIp:'8.8.8.8',deviceCount:2,activeSessions:1}];
 await page.route('**/api/v1/media-server/users?*',r=>r.fulfill({json:{data:{items:users,total:1}}}));
 await page.route('**/api/v1/media-server/users/*/devices?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));
 await page.route('**/api/v1/media-server/native/processing',r=>r.fulfill({json:{data:{settings:{enabled:true,probeEnabled:true,stillsEnabled:true,revision:2},probe:[{state:'pending',count:0},{state:'probing',count:0},{state:'ready',count:3},{state:'failed',count:0},{state:'needs_action',count:0}],still:[{state:'ready',count:2}],capacity:{usedBytes:1048576,reservedBytes:0,limitBytes:1073741824,unknownSizes:0}}}}));
 await page.route('**/api/v1/media-server/native/processing/tasks?*',r=>{const state=new URL(r.request().url()).searchParams.get('state'),items=state&&state!=='ready'?[]:Array.from({length:3},(_,i)=>({id:'probe-'+i,name:`海边的故事 ${i+1}.2026.mkv`,state:'ready',errorReason:'',attempts:1,nextAttemptAt:null}));return r.fulfill({json:{data:{items,total:items.length}}})});
 await page.route('**/api/v1/ip-geolocation?*',r=>r.fulfill({json:{data:['8.8.8.8','1.1.1.1'].map(ip=>({ip,label:'测试地区 · 测试运营商'}))}}));
 await page.route('**/api/v1/media-server/playback-ips?*',r=>r.fulfill({json:{data:{items:['8.8.8.8','1.1.1.1'].map(ip=>({ip,count:1,users:1,devices:1,firstSeenAt:at,lastSeenAt:at,location:'测试国家 测试省份 测试城市',country:'测试国家',province:'测试省份',city:'测试城市',isp:'测试运营商',accounts:[{id:mediaUser.id,name:'家庭成员'}],clients:[{id:'device',name:'客厅播放器',client:'SyntheticPlayer'}]})),total:2,summary:{uniqueIPs:2,regions:1,activeUsers:1,primaryRegion:'测试省份',provinces:[{name:'测试省份',count:2}],cities:[{name:'测试城市',count:2}],operators:[{name:'测试运营商',count:2}]}}}}));
 await page.route('**/api/v1/player-access/ip-blocklist?*',r=>r.fulfill({json:{data:{items:[],total:0,maxRules:1000,revision:1,status:{available:true}}}}));
 return {reads,writes};
}
