import type {Page} from '@playwright/test';
import {organizeFixture} from './u4-fixture';
import {u5PlaybackFixture} from './u5-fixture';
import {fixtureFulfill} from './collection-fixture';
const posterPNG=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAHgAAAC0CAIAAADQLH9KAAACWklEQVR4nO3QBW5bARQF0bukMjNjysxmh5nBTsyMWW//Km6l15HOAkajM6kSDPTPC/4TjHaNPpsqwYDRjI5F59IlGDDaN7oMA51Pl2HAaEbHoguZMgwYzehYdDFThkEy+hgGjHaNvpQ9hgGjGR2LLmePYcBoRseiK7kTGDCa0bHoau4EBoxmdCy6lj+BAaN9oysw0PV8BQaMZnQsulGowIDRjI5FNwsVGCSjqzBgtGv0rWIVBoxmdCy6XazCgNGMjkV3ZmswYDSjY9Hd2RoMGM3oWHRvrgYDRvtG12Gg+3N1GDCa0bHowXwdBoxmdCx6OF+HQTK6AQNGu0Y/WmjAgNGMjkWPFxowYDSjY9GTxSYMGM3oWPR0sQkDRjM6Fj1basKA0b7RLRjo+VILBoxmdCx6sdyCAaMZHYteLrdgkIxuw4DRrtEzK20YMJrRsejVShsGjGZ0LHq92oEBoxkdi96sdmDAaEbHordrHRgw2je6CwO9W+vCgNGMjkXv17swYDSjY9GH9S4MktE9GDDaNfrjRg8GjGZ0LPq00YMBoxkdiz5v9mHAaEbHoi+bfRgwmtGx6OtWHwaM9o0ewEDftgYwYDSjY9H37QEMGM3oWPRjewCDZPQQBox2jf65M4QBoxkdi37tDGHAaEbHot+7IxgwmtGx6M/uCAaMZnQsSu2NYMBo3+gxDJTeG8OA0YyORZn9MQwYzehYlN0fwyAZPYEBo12jcwcTGDCa0bEofzCBAaMZHYsKh1MYMJrRsah4OIUBoxkdi2aPpjBgtG/0KQw0d3QKA0YzOpa/T4NhFbVOdBgAAAAASUVORK5CYII=','base64');
export const at='2026-10-28T10:00:00Z';
export async function i13Fixture(page:Page,large=false){
 await organizeFixture(page);const reads:URL[]=[],writes:string[]=[];
 const candidates=Array.from({length:large?110:2},(_,i)=>({id:`candidate-${i}`,name:`海边的故事 ${i+1}.2026.mkv`,path:`/115/电影/海边的故事 ${i+1}.2026.mkv`,state:'needs_confirmation',cloudState:'present',size:5368709120,revision:1,updatedAt:at}));
 const assets=Array.from({length:large?110:2},(_,i)=>({id:`asset-${i}`,title:`海边的故事 ${i+1}`,mediaType:'movie',path:`/115/电影/海边的故事 ${i+1}.2026.mkv`,poster:'/api/v1/media/tmdb/image?path=%2Fi13-poster.png',state:'present',artifactsState:'ready',fileScoped:true,revoked:false,revision:1,createdAt:at}));
 const scans=Array.from({length:large?18:2},(_,i)=>({id:`scan-${i}`,root:`/115/媒体/目录 ${i+1}`,state:'running',files:24,directories:8,completedDirectories:3,createdAt:at,updatedAt:at,revision:1}));
 for(const [endpoint,source,initial]of [['library/candidates',candidates,50],['media/assets',assets,50],['library/scans',scans,8]]as const)await page.route(`**/api/v1/${endpoint}?*`,r=>{const u=new URL(r.request().url());reads.push(u);const size=Number(u.searchParams.get('pageSize')||initial),offset=(Number(u.searchParams.get('page')||1)-1)*size;return fixtureFulfill(r,{json:{data:{items:source.slice(offset,offset+size),total:source.length}}})});
 await page.route('**/api/v1/library/candidates/*/recognize',r=>{writes.push(new URL(r.request().url()).pathname);return r.fulfill({json:{data:{fileDigest:'a'.repeat(64),hint:{title:'海边的故事',mediaType:'movie'},matches:[],evidence:['本地测试文件名']}}})});
 await page.route('**/api/v1/library/scans/*/pause',r=>{writes.push(new URL(r.request().url()).pathname);scans.find(s=>r.request().url().includes(s.id))!.state='paused';return r.fulfill({json:{data:{accepted:true}}})});
 await page.route('**/api/v1/library/scans/*?*',r=>r.fulfill({json:{data:{scan:scans[0],directories:[]}}}));
 await page.route('**/api/v1/library/outputs?*',r=>fixtureFulfill(r,{json:{data:{items:assets.map(a=>({...a,outputs:[{path:'Movie.strm',kind:'strm'}]})),total:assets.length}}}));
 await page.route('**/api/v1/media/assets/*/images',r=>r.fulfill({json:{data:{revision:1,base:{},effective:{},locked:{}}}}));
 await page.route('**/api/v1/media/tmdb/image?*',r=>r.fulfill({contentType:'image/png',body:posterPNG}));
 return {reads,writes,candidates,assets,scans};
}
export async function i13AccessFixture(page:Page){
 await u5PlaybackFixture(page);
 await page.route('**/api/v1/player-access/policy',r=>r.fulfill({json:{data:{enabled:true,mode:'whitelist',revision:1,rules:[{id:'one',label:'客厅播放器',userAgent:'FamilyPlayer/',matchType:'prefix',enabled:true},{id:'two',label:'备用播放器',userAgent:'Family/1.0',matchType:'exact',enabled:false}],blacklistRules:[]}}}));
 for(const endpoint of ['observations','events'])await page.route(`**/api/v1/player-access/${endpoint}?*`,r=>r.fulfill({json:{data:{items:[],total:0}}}));
}
