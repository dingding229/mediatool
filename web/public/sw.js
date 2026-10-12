/* Only public same-origin static assets are eligible. No account/API/media data. */
const CACHE='media-tool-public-__BUILD_ID__';
const PUBLIC=['/offline.html','/pwa/icon-192.png','/pwa/icon-512.png'];
function eligible(url,request){return url.origin===self.location.origin&&!url.search&&request.method==='GET'&&(PUBLIC.includes(url.pathname)||url.pathname.startsWith('/assets/'));}
self.addEventListener('install',event=>{event.waitUntil(caches.open(CACHE).then(cache=>cache.addAll(PUBLIC)));});
self.addEventListener('activate',event=>{event.waitUntil(caches.keys().then(keys=>Promise.all(keys.filter(key=>key.startsWith('media-tool-public-')&&key!==CACHE).map(key=>caches.delete(key)))).then(()=>self.clients.claim()));});
self.addEventListener('message',event=>{if(event.data?.type==='ACTIVATE_UPDATE')self.skipWaiting();});
self.addEventListener('fetch',event=>{
 const request=event.request,url=new URL(request.url);
 if(request.method!=='GET'||url.origin!==self.location.origin)return;
 // Resolver, gateway and protected APIs never enter Cache Storage.
 if(url.pathname.startsWith('/api/')||url.pathname.startsWith('/emby')||url.pathname.startsWith('/Videos/')||url.pathname.startsWith('/strm'))return;
 if(request.mode==='navigate'){event.respondWith(fetch(request).catch(()=>caches.match('/offline.html')));return;}
 if(!eligible(url,request))return;
 event.respondWith(fetch(request).then(async response=>{
  if(response.status===200&&response.type==='basic'&&!/no-store|private/i.test(response.headers.get('cache-control')||'')){const cache=await caches.open(CACHE);await cache.put(request,response.clone());}
  return response;
 }).catch(()=>caches.match(request)));
});
