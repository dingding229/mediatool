import {getCurrentInstance,onUnmounted,ref,watch} from 'vue';
export function validPage(value:unknown){const n=Number(value);return Number.isSafeInteger(n)&&n>0&&n<=20000?n:1;}
// Batch page/size writes so two lists or a resize cannot overwrite each other.
const pending=new WeakMap<object,{patch:Record<string,string|undefined>;timer:ReturnType<typeof setTimeout>}>();
function queryNumber(key:string,initial:number,validate:(value:unknown)=>number,legacy=false){
 const router=getCurrentInstance()?.appContext.config.globalProperties.$router;
 const route=router?.currentRoute;
 const value=ref(validate(route?.value.query[key]??(legacy?route?.value.query.page:undefined)));
 let reading=false;
 const stop=watch(value,n=>{
  if(!router||!route||reading)return;
  const previous=pending.get(router);
  if(previous)clearTimeout(previous.timer);
  const patch={...previous?.patch,[key]:String(n),...(legacy?{page:undefined}:{})};
  pending.set(router,{patch,timer:setTimeout(()=>{pending.delete(router);void router.replace({query:{...route.value.query,...patch}})},0)});
 },{flush:'sync'});
 watch(()=>route?.value.query[key]??(legacy?route?.value.query.page:undefined),v=>{if(route){reading=true;value.value=v==null?initial:validate(v);reading=false}},{flush:'sync'});
 onUnmounted(()=>{
  stop();if(!router)return;
  const job=pending.get(router);if(!job)return;
  delete job.patch[key];if(legacy)delete job.patch.page;
  if(!Object.keys(job.patch).length){clearTimeout(job.timer);pending.delete(router)}
 });
 return value;
}
function scope(key:string){const route=getCurrentInstance()?.appContext.config.globalProperties.$router?.currentRoute;return key==='page'?String(route?.value.query.tab||'list')+'Page':key;}
// Per-list URL keys keep drawers and tabs independent. URL always wins on load/back.
export function usePageNumber(key='page'){return queryNumber(scope(key),1,validPage,true)}
export function usePageSize(initial:number,key='page',options:number[]=[10,25,50,100]){
 const allowed=new Set([...options,initial]);
 return queryNumber(scope(key)+'Size',initial,v=>allowed.has(Number(v))?Number(v):initial);
}
