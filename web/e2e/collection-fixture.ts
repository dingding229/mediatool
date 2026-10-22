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
 const body=options?.json;if(body&&Array.isArray(body.data))return route.fulfill({...options,json:{...body,data:collectionFixture(route.request().url(),route.request().method(),body.data)}});
 return route.fulfill(options);
}
