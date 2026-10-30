import type {Page} from '@playwright/test';
import {organizeFixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
export const name='Where.the.Mask.Ends.嫁金钗.2026.S01E01.2160p.WEB-DL.H.265.DDP5.1.Atmos-HHWEB.mkv';
export const source='/115/媒体归档/电视剧/国产剧/2026/待整理/嫁金钗 第一季/'+name;
export const destination='/115/媒体归档/已整理/电视剧/国产剧/2026/嫁金钗/Season 01/'+name;
export const root='/115/媒体归档/电视剧/国产剧/2026/待整理/'+name.replace(/\.mkv$/,'');
export async function fixture(page:Page){
 await organizeFixture(page);const at='2026-10-30T10:00:00Z';
 const candidate={id:'i34-candidate',name,path:source,state:'needs_confirmation',cloudState:'present',size:5368709120,revision:1,updatedAt:at};
 const asset={id:'i34-asset',title:'嫁金钗',path:source,mediaType:'series',season:1,episodes:[1],state:'present',artifactsState:'ready',fileScoped:true,revision:1,createdAt:at};
 const scan={id:'i34-scan',root,state:'running',files:1,directories:1,completedDirectories:1,currentDirectory:root+'/额外的完整目录名',createdAt:at,updatedAt:at};
 const history={id:'i34-history',kind:'file',state:'completed',source,destination,createdAt:at,updatedAt:at,media:{title:'嫁金钗',year:'2026'},details:{completedSteps:1,plan:{steps:[{action:'rename',source:{path:source},target:destination}]},items:[{ordinal:1,source,state:'completed'}]}};
 for(const [endpoint,data]of [['library/candidates',[candidate]],['media/assets',[asset]],['library/scans',[scan]],['library/activity',[history]]]as const)await page.route(`**/api/v1/${endpoint}?*`,r=>fixtureFulfill(r,{json:{data:{items:data,total:1}}}));
 await page.route('**/api/v1/library/scan-schedules',r=>fixtureFulfill(r,{json:{data:[{id:'i34-schedule',root,cron:'0 * * * *',enabled:false,watchChanges:false,watchState:'disabled',revision:1}]}}));
 await page.route('**/api/v1/library/scans/i34-scan?*',r=>fixtureFulfill(r,{json:{data:{scan,directories:[{path:root+'/目录覆盖检查',state:'completed',updatedAt:at}]}}}));
 await page.route('**/api/v1/library/outputs?*',r=>fixtureFulfill(r,{json:{data:{items:[{...asset,outputs:[{kind:'strm',path:destination.replace(/\.mkv$/,'.strm')}]}],total:1}}}));
 await page.route('**/api/v1/library/candidates/*/recognize',r=>fixtureFulfill(r,{json:{data:{fileDigest:'a'.repeat(64),hint:{title:'嫁金钗',mediaType:'series',season:1,episodes:[1]},matches:[{id:300146,title:'嫁金钗',mediaType:'series',year:'2026'}],selected:{id:300146,title:'嫁金钗',mediaType:'series',year:'2026'},evidence:['发布名含季集，待核对']}}}));
 await page.route('**/api/v1/metadata/search?*',r=>fixtureFulfill(r,{json:{data:[{id:300146,title:'嫁金钗',mediaType:'series',year:'2026'}]}}));
 await page.route('**/api/v1/media/assets/*/identity-preview',r=>fixtureFulfill(r,{json:{data:{planDigest:'b'.repeat(64),operations:[{operation:'archive',path:source.replace(/\.mkv$/,'.strm')},{operation:'create',path:destination.replace(/\.mkv$/,'.strm')}]}}}));
 await page.route('**/api/v1/files/roots',r=>fixtureFulfill(r,{json:{data:[{id:'i34-root',name:'深层媒体目录',kind:'cloud',path:root}]}}));
 return {candidate,asset,scan,history};
}
