import type {Router} from 'vue-router';
const key='media-tool:chunk-recovery:v1',cooldown=30000;
type SessionStore=Pick<Storage,'getItem'|'setItem'>;
export function isChunkLoadError(error:unknown){
 const message=typeof error==='string'?error:(error as {message?:unknown})?.message;
 return typeof message==='string'&&/failed to fetch dynamically imported module|importing a module script failed|error loading dynamically imported module|failed to load module script|loading (?:css )?chunk .+ failed|unable to preload css/i.test(message);
}
export function reserveChunkReload(storage:SessionStore,target:string,now=Date.now()){
 try{
  const raw=storage.getItem(key),saved=raw?JSON.parse(raw):{};
  const entries=Object.fromEntries(Object.entries(saved&&typeof saved==='object'&&!Array.isArray(saved)?saved:{}).filter(([,at])=>typeof at==='number'&&Number.isFinite(at)&&now-at<cooldown));
  if(Object.hasOwn(entries,target))return false;
  entries[target]=now;storage.setItem(key,JSON.stringify(entries));return true;
 }catch{return false} // No persistent marker means no safe automatic reload.
}
export function installChunkRecovery(router:Router,showError:()=>void,browser:Window=window){
 let pending='',reloading='';
 const before=router.beforeEach(to=>{pending=to.fullPath;reloading=''});
 const after=router.afterEach(()=>{pending=''});
 function recover(error:unknown,path:string){
  if(!isChunkLoadError(error))return false;
  const target=new URL(router.resolve(path).href,browser.location.href).href;
  if(reloading===target)return true; // preloadError and onError may report the same failure.
  try{
   if(reserveChunkReload(browser.sessionStorage,target)){
    reloading=target;
    const current=new URL(browser.location.href),next=new URL(target);
    if(current.pathname===next.pathname&&current.search===next.search){browser.history.replaceState(browser.history.state,'',target);browser.location.reload()}
    else browser.location.assign(target);
    return true;
   }
  }catch{reloading=''}
  showError();return true;
 }
 const onError=router.onError((error,to)=>{recover(error,to.fullPath)});
 const preload=(event:Event)=>{if(recover((event as Event&{payload?:unknown}).payload,pending||router.currentRoute.value.fullPath))event.preventDefault()};
 browser.addEventListener('vite:preloadError',preload);
 return ()=>{before();after();onError();browser.removeEventListener('vite:preloadError',preload)};
}
