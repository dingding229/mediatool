import type {Page} from '@playwright/test';
import {waitForLayout} from './layout-stability';
import {u5PlaybackFixture,mediaUser} from './u5-fixture';
export async function i26bPlaybackFixture(page:Page){
 await u5PlaybackFixture(page);const writes:unknown[]=[];
 await page.route('**/api/v1/media-server/users?*',r=>r.fulfill({json:{data:{items:[mediaUser],total:1}}}));
 await page.route('**/api/v1/player-access/events?*',r=>r.fulfill({json:{data:{items:[{id:'event',allowed:true,status:302,reason:'restriction_disabled',method:'GET',createdAt:'2026-10-30T02:00:00Z',clientIp:'203.0.113.9',connectionIp:'203.0.113.9',userAgent:'LocalPlayer/1.0'}],total:1}}}));
 await page.route('**/api/v1/player-access/ip-blocklist**',r=>{
  if(r.request().method()==='GET')return r.fulfill({json:{data:{items:[],total:0,revision:1,status:{available:true,retrySeconds:0,counterWarning:false},currentIp:'8.8.8.8',maxRules:10000}}});
  if(new URL(r.request().url()).pathname.endsWith('/check'))return r.fulfill({json:{data:{revision:1,candidateNetwork:'8.8.8.0/24',affectsCurrent:true}}});
  writes.push(r.request().postDataJSON());return r.fulfill({status:405,json:{error:{code:'fixture_write_blocked',message:'本地测试不保存规则'}}});
 });return {writes};
}
export async function stableLayout(page:Page){
 await waitForLayout(page);
}
