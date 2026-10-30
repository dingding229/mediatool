import {onBeforeUnmount,watch,type WatchSource} from 'vue';
/** Coalesce edits, invalidate stale reads via the caller's resource, cancel on unmount. */
export function useDebouncedFilter<T>(source:WatchSource<T>,apply:(value:T)=>void|Promise<void>,delay=350){
 let timer:ReturnType<typeof setTimeout>|undefined;
 const cancel=()=>clearTimeout(timer);
 const stop=watch(source,value=>{cancel();timer=setTimeout(()=>void apply(value),Math.max(300,delay));});
 onBeforeUnmount(()=>{stop();cancel()});
 return {cancel};
}
