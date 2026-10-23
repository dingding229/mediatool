import {ref} from 'vue';
import {defineStore} from 'pinia';
import {getBuildInfo,getUpdateState,takeUpdateNotice,type BuildInfo,type UpdateState,type ReleaseInfo} from '../api/updates';
export const useUpdateStore=defineStore('updates',()=>{
 const build=ref<BuildInfo>(),state=ref<UpdateState>(),notice=ref<ReleaseInfo>(),error=ref('');let flight:Promise<void>|undefined;
 function accept(value:UpdateState){state.value=value;if(!value.hasUpdate||notice.value?.version!==value.result.latest?.version)notice.value=undefined}
 async function refresh(){if(flight)return flight;flight=(async()=>{error.value='';const results=await Promise.allSettled([getBuildInfo(),getUpdateState()]);if(results[0].status==='fulfilled')build.value=results[0].value;
 if(results[1].status==='fulfilled'){accept(results[1].value);if(state.value?.hasUpdate){try{const claimed=await takeUpdateNotice();if(claimed.release)notice.value=claimed.release}catch{error.value='更新提示暂不可用'}}}else error.value='版本检查状态暂不可用'})().finally(()=>{flight=undefined});return flight}
 function dismiss(){notice.value=undefined}
 return{build,state,notice,error,refresh,accept,dismiss};
});
