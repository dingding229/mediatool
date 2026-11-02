import {inject,provide,ref,watch,onUnmounted,type Ref,type InjectionKey,type ComputedRef} from 'vue';
export interface PageCrumb{label:string;to?:string}
interface Context{items:Ref<PageCrumb[]>;owner?:symbol}
const key:InjectionKey<Context>=Symbol('page-breadcrumbs');
export function providePageBreadcrumbs(){const context:Context={items:ref([])};provide(key,context);return context.items}
export function usePageBreadcrumbs(items:ComputedRef<PageCrumb[]>){const context=inject(key,undefined),owner=Symbol();if(!context)return;watch(items,value=>{context.owner=owner;context.items.value=value},{immediate:true});onUnmounted(()=>{if(context.owner===owner){context.items.value=[];context.owner=undefined}})}
