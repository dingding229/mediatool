import {describe,it,expect} from 'vitest'
import {readFileSync} from 'node:fs'
import {runInNewContext} from 'node:vm'

function harness(){
 const listeners:Record<string,(event:any)=>void>={},stored=new Map<string,any>(),deleted:string[]=[],added:string[][]=[]
 const cache={addAll:async(paths:string[])=>{added.push(paths);for(const path of paths)stored.set(path,{offline:true})},put:async(request:any,response:any)=>{stored.set(typeof request==='string'?request:request.url,response)}}
 let fail=false,skips=0,claims=0
 const context={URL,self:{location:{origin:'https://owned.example'},addEventListener:(kind:string,listener:any)=>listeners[kind]=listener,skipWaiting:()=>{skips++},clients:{claim:()=>{claims++}}},caches:{open:async()=>cache,keys:async()=>['media-tool-public-old','unrelated'],delete:async(key:string)=>{deleted.push(key)},match:async(request:any)=>stored.get(typeof request==='string'?request:request.url)},fetch:async()=>{if(fail)throw new Error('offline');return {status:200,type:'basic',headers:{get:()=>''},clone:()=>({static:true})}}}
 runInNewContext(readFileSync(new URL('../../public/sw.js',import.meta.url),'utf8'),context)
 async function dispatch(kind:string,request?:any){let response:Promise<any>|undefined,work:Promise<any>|undefined;listeners[kind]({request,respondWith:(p:Promise<any>)=>response=p,waitUntil:(p:Promise<any>)=>work=p});if(work)await work;return response?await response:undefined}
 return {context,stored,deleted,added,listeners,dispatch,offline:()=>{fail=true},counts:()=>({skips,claims})}
}
describe('public-only PWA service worker',()=>{
 it('installs only a generic offline page and icons, upgrades its own cache only',async()=>{const h=harness();await h.dispatch('install');expect(h.added[0]).toEqual(['/offline.html','/pwa/icon-192.png','/pwa/icon-512.png']);await h.dispatch('activate');expect(h.deleted).toEqual(['media-tool-public-old']);h.listeners.message({data:{type:'ACTIVATE_UPDATE'}});expect(h.counts()).toEqual({skips:1,claims:1})})
 it('never handles APIs, resolver, gateway, authenticated data or cross-origin media',async()=>{const h=harness();for(const path of ['/api/v1/data/postgres/tables','/api/v1/logs','/api/v1/auth/me','/api/v1/cloudStorage/strm302?apiKey=owned','/emby/Videos/1/stream','/Videos/1/stream','/strm/owned','/login','/assets/app.js?token=owned','https://cdnfhnfile.115cdn.net/owned']){const url=path.startsWith('https:')?path:'https://owned.example'+path;expect(await h.dispatch('fetch',{url,method:'GET',mode:'cors'})).toBeUndefined()}expect(h.stored.size).toBe(0)})
 it('uses only public static cache and generic offline navigation, not user pages',async()=>{const h=harness();await h.dispatch('install');const staticRequest={url:'https://owned.example/assets/public.js',method:'GET',mode:'cors'};await h.dispatch('fetch',staticRequest);expect(h.stored.has(staticRequest.url)).toBe(true);await h.dispatch('fetch',{url:'https://owned.example/data',method:'GET',mode:'navigate'});expect(h.stored.has('https://owned.example/data')).toBe(false);h.offline();expect(await h.dispatch('fetch',staticRequest)).toEqual({static:true});expect(await h.dispatch('fetch',{url:'https://owned.example/data',method:'GET',mode:'navigate'})).toEqual({offline:true})})
 it('does not cache private/no-store or failed responses',async()=>{const h=harness();for(const directive of ['private','no-store']){h.context.fetch=async()=>({status:200,type:'basic',headers:{get:()=>directive},clone:()=>({static:true})});await h.dispatch('fetch',{url:'https://owned.example/assets/private.js',method:'GET',mode:'cors'});expect(h.stored.size).toBe(0)}})
})
