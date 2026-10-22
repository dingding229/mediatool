import {readonly,ref} from 'vue';
const active=ref(0);
export const pendingRequests=readonly(active);
export function beginActivity(){active.value++;let finished=false;return ()=>{if(!finished){finished=true;active.value=Math.max(0,active.value-1)}}}
