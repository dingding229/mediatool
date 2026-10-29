import {expect,it,vi} from 'vitest';
import type {Router} from 'vue-router';
import {isChunkLoadError,reserveChunkReload,installChunkRecovery} from './chunk-recovery';
const store=()=>{const rows=new Map<string,string>();return {getItem:(k:string)=>rows.get(k)??null,setItem:(k:string,v:string)=>{rows.set(k,v)}}};
it.each(['Failed to fetch dynamically imported module: /assets/old.js','Importing a module script failed.','error loading dynamically imported module','Loading chunk 7 failed.','Unable to preload CSS for /assets/old.css'])('recognizes resource failures: %s',message=>{expect(isChunkLoadError(Error(message))).toBe(true);expect(isChunkLoadError(message)).toBe(true)});
it.each([null,undefined,{},Error('Failed to fetch'),Error('401 invalid credentials'),Error('business request failed')])('never reloads for other errors: %s',error=>expect(isChunkLoadError(error)).toBe(false));
it('allows one reload per full target, including intervening other targets, expires at 30s',()=>{
 const s=store(),a='https://owned.test/ops?tab=jobs#rows',b='https://owned.test/trends?hours=24';
 expect(reserveChunkReload(s,a,1000)).toBe(true);expect(reserveChunkReload(s,b,1001)).toBe(true);expect(reserveChunkReload(s,a,30999)).toBe(false);expect(reserveChunkReload(s,a,31000)).toBe(true);
});
it('storage denial, quota failures and corrupt markers fail closed rather than loop',()=>{
 expect(reserveChunkReload({getItem:()=>{throw Error('blocked')},setItem:()=>{}},'/ops')).toBe(false);
 expect(reserveChunkReload({getItem:()=>null,setItem:()=>{throw Error('quota')}},'/ops')).toBe(false);
 expect(reserveChunkReload({getItem:()=>'{bad',setItem:()=>{}},'/ops')).toBe(false);
});
function harness(session=store()){
 const hooks:any={},removed=vi.fn();const events=new EventTarget(),assign=vi.fn(),reload=vi.fn(),replaceState=vi.fn(),error=vi.fn();
 const router={beforeEach:(fn:any)=>{hooks.before=fn;return removed},afterEach:(fn:any)=>{hooks.after=fn;return removed},onError:(fn:any)=>{hooks.error=fn;return removed},resolve:(path:string)=>({href:'/base'+path}),currentRoute:{value:{fullPath:'/dashboard'}}}as unknown as Router;
 const browser={location:{href:'https://owned.test/base/dashboard',assign,reload},history:{state:{},replaceState},sessionStorage:session,addEventListener:events.addEventListener.bind(events),removeEventListener:events.removeEventListener.bind(events)}as unknown as Window;
 const dispose=installChunkRecovery(router,error,browser);return {hooks,events,assign,reload,replaceState,error,dispose,removed};
}
it('preload and router error share target and marker; the next document failure displays existing error',()=>{
 const s=store(),h=harness(s),e=Object.assign(new Event('vite:preloadError',{cancelable:true}),{payload:Error('Failed to fetch dynamically imported module')});
 h.hooks.before({fullPath:'/ops?tab=jobs#rows'});h.events.dispatchEvent(e);h.hooks.error(e.payload,{fullPath:'/ops?tab=jobs#rows'});
 expect(e.defaultPrevented).toBe(true);expect(h.assign).toHaveBeenCalledExactlyOnceWith('https://owned.test/base/ops?tab=jobs#rows');expect(h.error).not.toHaveBeenCalled();h.dispose();expect(h.removed).toHaveBeenCalledTimes(3);
 const next=harness(s);next.hooks.before({fullPath:'/ops?tab=jobs#rows'});next.hooks.error(e.payload,{fullPath:'/ops?tab=jobs#rows'});expect(next.assign).not.toHaveBeenCalled();expect(next.error).toHaveBeenCalledOnce();next.dispose();
});
it('router-only failure navigates to full URL, while unrelated preload is not suppressed',()=>{
 const h=harness();const e=Object.assign(new Event('vite:preloadError',{cancelable:true}),{payload:Error('business failure')});h.events.dispatchEvent(e);expect(e.defaultPrevented).toBe(false);h.hooks.error(Error('Importing a module script failed'),{fullPath:'/trends?hours=1#sample'});expect(h.assign).toHaveBeenCalledExactlyOnceWith('https://owned.test/base/trends?hours=1#sample');h.dispose();
});
it('same document including hash-only targets uses real reload, not a same-document assign',()=>{
 const h=harness();h.hooks.error(Error('Importing a module script failed'),{fullPath:'/dashboard#card'});expect(h.assign).not.toHaveBeenCalled();expect(h.replaceState).toHaveBeenCalledWith({},'', 'https://owned.test/base/dashboard#card');expect(h.reload).toHaveBeenCalledOnce();h.dispose();
});
