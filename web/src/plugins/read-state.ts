import {onScopeDispose,reactive} from 'vue'
import {operationError} from './operation-error'

// Unknown is not empty. Keep the last successful value on refresh failure,
// fence late replies (including unmounted dialogs), and never retry writes.
export function useReadState(initialLoading=true){
 const state=reactive({loading:initialLoading,loaded:false,error:''})
 let epoch=0,disposed=false
 function reset(){++epoch;state.loading=false;state.loaded=false;state.error=''}
 onScopeDispose(()=>{disposed=true;reset()})
 async function run<T>(fetch:()=>Promise<T>,commit:(value:T)=>void):Promise<boolean>{
  if(disposed)return false
  const current=++epoch;state.loading=true;state.error=''
  try{const value=await fetch();if(current!==epoch)return false;commit(value);state.loaded=true;return true}
  catch(e){if(current===epoch)state.error=operationError(e).label;return false}
  finally{if(current===epoch)state.loading=false}
 }
 return Object.assign(state,{run,reset})
}
