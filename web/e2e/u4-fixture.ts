import type {Page} from '@playwright/test'
export const libraryItem={id:'owned-asset',source:'managed',title:'海边的故事',year:'2026',mediaType:'movie',overview:'一段与故乡重逢的故事。',poster:'',filePath:'/115/电影/海边的故事 (2026).mkv',state:'present'}
export async function u4Fixture(page:Page){

 await page.route('**/api/v1/**',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{id:'fixture-emby',kind:'emby',enabled:true}]}}));await page.route('**/api/v1/auth/session',r=>r.fulfill({json:{data:{authenticated:true,account:{id:'u4-admin',username:'fixture'}}}}));await page.route('**/api/v1/system/status',r=>r.fulfill({json:{data:{version:'UI mock',environment:'development',dependencies:[]}}}));await page.route('**/api/v1/media/library?*',r=>r.fulfill({json:{data:{items:[libraryItem],total:1,page:1,pageSize:24,source:'managed'}}}));await page.route('**/api/v1/media/library/sources?*',r=>r.fulfill({json:{data:{items:[{instanceId:'plugin-owned',source:'plugin:plugin-owned',kind:'discover.source',tmdbId:42,mediaType:'movie',title:'插件发现条目',year:2026}],hasMore:false,truncated:false,page:1,size:24}}}));await page.route('**/api/v1/metadata/details?*',r=>r.fulfill({json:{data:{item:{id:42,mediaType:'movie',title:'插件发现条目',overview:'只读媒体信息'}}}}));await page.route('**/api/v1/media/library/emby/home?*',r=>r.fulfill({json:{data:{users:[{id:'media-user',name:'媒体用户'}],userId:'media-user',libraries:[{id:'library-root',title:'电影',type:'CollectionFolder',image:''}],playing:[],resume:[],recent:[],latest:[{id:'emby-item',title:'Emby影片',type:'Movie',image:'',progress:0}]}}}));await page.route('**/api/v1/media/emby/items/emby-item',r=>r.fulfill({json:{data:{...libraryItem,id:'emby-item',source:'emby'}}}))
 await page.route('**/api/v1/auth/sessions?*',r=>r.fulfill({json:{data:{items:[],hasNext:false}}}));
}
export async function organizeFixture(page:Page){
 await u4Fixture(page)
 const candidate={id:'candidate-owned',name:'海边的故事.2026.mkv',path:'/115/电影/海边的故事.2026.mkv',state:'needs_confirmation',cloudState:'present',size:5368709120,revision:1}
 await page.route('**/api/v1/library/candidates?*',r=>r.fulfill({json:{data:{items:[candidate],total:1}}}))
 await page.route('**/api/v1/media/assets?*',r=>r.fulfill({json:{data:{items:[],total:0}}}))
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[],total:0}}}))
 await page.route('**/api/v1/library/archives?*',r=>r.fulfill({json:{data:{items:[],total:0}}}))
 await page.route('**/api/v1/library/archive-policy',r=>r.fulfill({json:{data:{retentionDays:30,revision:1,automaticCleanup:false}}}))
 await page.route('**/api/v1/files/roots',r=>r.fulfill({json:{data:[{id:'owned-cloud',name:'我的云盘',kind:'cloud',path:'/115/媒体',capabilities:{read:true,write:true,move:true,recycle:true}}]}}))
 await page.route('**/api/v1/library/scans',r=>r.fulfill({json:{data:[{id:'scan-owned',root:'/115/媒体',state:'running',files:24,directories:8,completedDirectories:3,currentDirectory:'/115/媒体/电影',message:'正在读取目录，完整覆盖后核对缺失文件',revision:1,generation:1}]}}))
 await page.route('**/api/v1/library/scan-schedules',r=>r.fulfill({json:{data:[{id:'schedule-owned',root:'/115/媒体',enabled:false,watchChanges:false,watchState:'disabled',cron:'0 * * * *',revision:1}]}}))
}
