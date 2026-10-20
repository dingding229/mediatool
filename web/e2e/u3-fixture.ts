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
export const u3Task={id:'task-u3',resourceId:'resource-u3',title:'A.Story.by.the.Sea.2026.1080p.WEB-DL.H264',state:'imported',seedingState:'seeding',progress:1,qbState:'uploading',downloadSpeed:0,uploadSpeed:1024,seeds:4,peers:1,syncedAt:'2026-10-21T00:00:00Z',media:u3Media,cleanupState:'disabled',flow:{retryable:false},message:'文件已入库，原下载保留做种。'}
export const u3Transfer={id:'upload-u3',destination:'/115/电影/海边的故事 (2026).mkv',state:'uncertain',phase:'uploading',bytes:0,hashBytes:1073741824,total:1073741824,processingProgress:50,processingStage:'等待远端核对',controlRevision:2,flow:{retryable:false},message:'远端回执待核对'}
export async function u3DownloadsFixture(page:Page){
 await u3Fixture(page)
 await page.route('**/api/v1/downloads',r=>r.fulfill({json:{data:[u3Task]}}))
 await page.route('**/api/v1/transfers',r=>r.fulfill({json:{data:[u3Transfer]}}))
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[{id:'batch-u3',kind:'batch',batchId:'batch-u3',state:'needs_reconcile',source:'待整理目录',message:'批次结果待核对'}],total:1}}}))
 await page.route('**/api/v1/downloads/manual',r=>r.fulfill({json:{data:[{id:'manual-u3',title:'手动添加的种子',state:'uncertain',submitAttempted:true,progress:null,message:'需先在下载器核对提交结果'}]}}))
}
