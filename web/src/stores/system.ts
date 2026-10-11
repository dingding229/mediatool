import { ref } from 'vue'
import { defineStore } from 'pinia'
import { request } from '../api/client'
import type { ModuleInfo,SystemStatus } from '../api/types'
export const useSystemStore=defineStore('system',()=>{
 const status=ref<SystemStatus|null>(null),modules=ref<ModuleInfo[]>([]),loading=ref(false),error=ref('')
 async function refresh(){loading.value=true;error.value='';try{const [s,m]=await Promise.all([request<SystemStatus>('/system/status'),request<ModuleInfo[]>('/system/modules')]);status.value=s;modules.value=m}catch(e){error.value=e instanceof Error?e.message:'服务状态查询失败'}finally{loading.value=false}}
 return{status,modules,loading,error,refresh}
})
