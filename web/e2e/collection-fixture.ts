import type {Route} from '@playwright/test';
const collections=/^\/api\/v1\/(?:downloads(?:\/manual)?|transfers|notifications|media-server\/users|library\/scans|subscriptions(?:\/[^/]+\/history)?)$/;
// Keep legacy non-browser array fixtures explicit while UI fixtures model the
// new opt-in page envelope. Malformed objects/errors and every write are intact.
export function collectionFixture(url:string,method:string,value:unknown){
 const u=new URL(url);if(method!=='GET'||!u.searchParams.has('page')||!collections.test(u.pathname)||!Array.isArray(value))return value;
 const page=Number(u.searchParams.get('page')||1),pageSize=Number(u.searchParams.get('pageSize')||25);
 return {items:value.slice((page-1)*pageSize,page*pageSize),total:value.length,page,pageSize};
}
export function fixtureFulfill(route:Route,options:Parameters<Route['fulfill']>[0]){
 // UI-only fixtures use deterministic times; real parser/security are exercised
 // by HTTP and scheduling tests, not by a second browser cron implementation.
 if(new URL(route.request().url()).pathname==='/api/v1/scheduling/cron/preview'){
  const {expression,purpose}=route.request().postDataJSON(),fields=String(expression).trim().split(/\s+/);
  const minimum:Record<string,number>={generic:0,subscription:300,scan:300,'site-metrics':900,backup:3600,plugin:60};
  if(![5,6,7].includes(fields.length)||!(purpose in minimum)||/^6[0-9] /.test(expression)||expression==='* * * * *'||(purpose==='subscription'&&expression==='*/1 * * * *'))return route.fulfill({status:400,json:{error:{message:'计划分钟、秒或年份格式、范围或最小间隔无效'}}});
  return route.fulfill({json:{data:{timezone:'Asia/Shanghai',serverNow:'2026-10-26T00:00:00+08:00',fieldCount:fields.length,next:[1,2,3,4,5].map(n=>`2026-10-${26+n}T09:00:00+08:00`),exhausted:false,minimumIntervalSeconds:minimum[purpose]}}});
 }
 const body=options?.json;if(body&&Array.isArray(body.data))return route.fulfill({...options,json:{...body,data:collectionFixture(route.request().url(),route.request().method(),body.data)}});
 return route.fulfill(options);
}
