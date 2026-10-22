import {getCurrentInstance,onUnmounted,ref,watch} from 'vue';
export function validPage(value:unknown){const n=Number(value);return Number.isSafeInteger(n)&&n>0&&n<=20000?n:1;}
// Per-list keys avoid a drawer/list or two tabs overwriting one another.
export function usePageNumber(key='page') {
 const router=getCurrentInstance()?.appContext.config.globalProperties.$router;
 const route=router?.currentRoute;
 const scope=key==='page'?String(route?.value.query.tab||'list')+'Page':key;
 const page=ref(validPage(route?.value.query[scope]??route?.value.query.page));
 let timer:ReturnType<typeof setTimeout>;
 watch(page,value=>{clearTimeout(timer);timer=setTimeout(()=>{if(router&&route&&validPage(route.value.query[scope])!==value)void router.replace({query:{...route.value.query,page:undefined,[scope]:String(value)}})},0)});
 onUnmounted(()=>clearTimeout(timer));
 watch(()=>route?.value.query[scope]??route?.value.query.page,value=>{if(route)page.value=validPage(value)});
 return page;
}
