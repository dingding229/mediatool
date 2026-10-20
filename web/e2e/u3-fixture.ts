import type { Page } from '@playwright/test'
export const u3Media={id:42,mediaType:'movie',title:'海边的故事',originalTitle:'A Story by the Sea',year:'2026',overview:'用于界面验收的影视条目。',genres:['剧情'],posterPath:''}
export const u3Resource={id:'resource-u3',title:'A.Story.by.the.Sea.2026.1080p.WEB-DL.H264',siteName:'示例站点',size:1073741824,seeders:12,leechers:0,downloadFactor:0,metadata:{subtitle:'电影资源',attributes:{resolution:'1080p',source:'web-dl',codec:'h264'}}}
export const u3Run={id:'run-u3',keyword:u3Media.title,media:u3Media,state:'succeeded',sites:[{accountId:'site-u3',name:'示例站点',state:'succeeded',count:1,message:''}],resources:[u3Resource],total:1,excluded:0,createdAt:'2026-10-21T00:00:00Z'}
export async function u3Fixture(page:Page){
 await page.route('**/api/v1/**',r=>r.fulfill({json:{data:[]}}))
 await page.route('**/api/v1/auth/session',r=>r.fulfill({json:{data:{authenticated:true,account:{id:'u3-admin',username:'fixture'}}}}))
 await page.route('**/api/v1/system/status',r=>r.fulfill({json:{data:{version:'UI mock',environment:'development',dependencies:[]}}}))
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{id:'site-u3',kind:'ptsite',name:'示例站点',enabled:true}]}}))
 await page.route('**/api/v1/search',r=>r.request().method()==='POST'?r.fulfill({status:202,json:{data:{id:'run-u3',cached:false}}}):r.fulfill({json:{data:[u3Run]}}))
 await page.route('**/api/v1/search/run-u3?*',r=>r.fulfill({json:{data:u3Run}}))
 await page.route('**/api/v1/search/media?*',r=>r.fulfill({json:{data:{items:[u3Media],cached:false}}}))
 await page.route('**/api/v1/metadata/details?*',r=>r.fulfill({json:{data:{item:u3Media}}}))
 await page.route('**/api/v1/metadata/search?*',r=>r.fulfill({json:{data:[u3Media]}}))
 await page.route('**/api/v1/cache/search?*',r=>r.fulfill({json:{data:{items:[{id:'run-u3',query:u3Media.title,count:1,state:'succeeded',createdAt:u3Run.createdAt}],total:1,truncated:false}}}))
 await page.route('**/api/v1/cache/search/run-u3',r=>r.fulfill({json:{data:{id:'run-u3',query:u3Media.title,state:'succeeded',data:u3Run}}}))
}
