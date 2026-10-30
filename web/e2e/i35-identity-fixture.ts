import type {Page} from '@playwright/test';
import {organizeFixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
export const release=(i=1)=>`Where.the.Mask.Ends.嫁金钗.2026.S01E${String(i).padStart(2,'0')}.2160p.WEB-DL.H.265.DDP5.1.Atmos-HHWEB.mkv`;
export const base='/115/媒体归档/电视剧/国产剧/2026/待整理/嫁金钗 第一季/';
export async function fixture(page:Page,mode:'batch'|'movie'|'migration'){
 await organizeFixture(page);const mediaType=mode==='movie'?'movie':'series',title=mode==='movie'?'海边的故事':'嫁金钗';
 const candidates=Array.from({length:8},(_,i)=>({id:'i35-'+i,name:release(i+1),path:base+release(i+1),state:'needs_confirmation',cloudState:'present',revision:1,size:5368709120}));
 const asset={id:'i35-asset',title,mediaType,path:base+release(),revision:2,cloudState:'present',state:'present',artifactsState:'ready',fileScoped:true};
 const matches=Array.from({length:8},(_,i)=>({id:300146+i,title:i?title+' '+(i+1):title,originalTitle:i?'Alternative '+(i+1):'Where the Mask Ends',mediaType,year:String(2026-i),posterPath:i%2?'':'/owned-poster.jpg'}));
 const operations=Array.from({length:8},(_,i)=>({kind:i%2?'nfo':'strm',operation:i<4?'archive':'write',path:(i<4?'/115/托管输出/旧目录/电视剧/国产剧/2026/第一季/':'/115/托管输出/新目录/电视剧/国产剧/2026/第一季/')+release(i+1).replace(/\.mkv$/,i%2?'.nfo':'.strm')}));
 const calls:{path:string;body:any}[]=[];
 await page.route('**/api/v1/library/candidates?*',r=>fixtureFulfill(r,{json:{data:{items:candidates,total:8}}}));
 await page.route('**/api/v1/media/assets?*',r=>fixtureFulfill(r,{json:{data:{items:[asset],total:1}}}));
 await page.route('**/api/v1/library/candidates/*/recognize',r=>{calls.push({path:new URL(r.request().url()).pathname,body:r.request().postDataJSON()});return fixtureFulfill(r,{json:{data:{fileDigest:'a'.repeat(64),hint:{title,mediaType,season:mediaType==='series'?1:0,episodes:mediaType==='series'?[Number(/i35-(\d+)/.exec(r.request().url())?.[1]||0)+1]:[]},matches,selected:matches[0],evidence:['本地长发布名样本']}}})});
 await page.route('**/api/v1/metadata/search?*',r=>fixtureFulfill(r,{json:{data:matches}}));
 await page.route('**/api/v1/metadata/image/*/*',r=>r.fulfill({contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="160" height="240"><rect width="160" height="240" fill="#52777a"/><circle cx="80" cy="75" r="48" fill="#e9dac2"/></svg>'}));
 await page.route('**/api/v1/media/assets/*/identity-preview',r=>{calls.push({path:new URL(r.request().url()).pathname,body:r.request().postDataJSON()});return fixtureFulfill(r,{json:{data:{assetId:asset.id,revision:2,planDigest:'b'.repeat(64),operations,cloudFileUnchanged:true,playbackGrantUnchanged:true}}})});
 await page.route('**/api/v1/media/assets/*/identity',r=>{calls.push({path:new URL(r.request().url()).pathname,body:r.request().postDataJSON()});return fixtureFulfill(r,{json:{data:{corrected:true}}})});
 await page.route('**/api/v1/library/candidates/batch-confirm',r=>{calls.push({path:new URL(r.request().url()).pathname,body:r.request().postDataJSON()});return fixtureFulfill(r,{json:{data:candidates.map(c=>({id:c.id,succeeded:true}))}})});
 return {candidates,asset,matches,operations,calls};
}
export async function open(page:Page,mode:'batch'|'movie'|'migration'){
 if(mode==='batch'){
  await page.goto('/organize?tab=candidates');await page.locator('.organize-candidate-row').first().waitFor();for(const box of await page.locator('.organize-candidate-row input[type=checkbox]').all())await box.check();await page.getByRole('button',{name:'批量确认作品 · 8',exact:true}).click();
 }else{await page.goto('/organize?tab=assets');await page.getByRole('button',{name:'修正身份 / 季集',exact:true}).click();}
 const dialog=page.getByRole('dialog',{name:mode==='batch'?'确认候选媒体':'修正媒体身份与季集',exact:true});if(mode==='batch'){await dialog.locator('.el-select').filter({has:page.getByRole('combobox',{name:'媒体类型',exact:true})}).click();await page.getByRole('option',{name:'剧集',exact:true}).click();}await dialog.getByRole('textbox',{name:'媒体关键词',exact:true}).fill(mode==='movie'?'海边的故事':'嫁金钗');await dialog.getByRole('button',{name:'查询TMDB',exact:true}).click();await dialog.getByRole('button',{name:(mode==='movie'?'海边的故事':'嫁金钗')+' · 2026 · '+(mode==='movie'?'电影':'剧集'),exact:true}).click();return dialog;
}
