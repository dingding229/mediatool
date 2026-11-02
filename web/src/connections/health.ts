import type {Connection} from './model';
export const hasCheckTime=(value?:string)=>Boolean(value&&Number.isFinite(Date.parse(value))&&Date.parse(value)>0);
export function connectionState(row:Connection){
 if(!row.enabled)return 'disabled';
 const state=row.readiness?.state||row.lastCheckState;
 if(row.readiness?.reason==='probe_missing_or_stale'||state==='ready'&&!hasCheckTime(row.lastCheckedAt))return 'check_required';
 return state==='unchecked'?'check_required':state||'check_required';
}
export function healthState(state:string,checkedAt?:string,message=''){
 return !hasCheckTime(checkedAt)&&(state==='ready'||/尚无有效检测|尚未检测/.test(message))?'check_required':state;
}
export const checkLabel=(state:string)=>({ready:'连接成功',disabled:'已停用',check_required:'待检测',unchecked:'尚未检测',not_configured:'未配置',check_failed:'连接失败',unavailable:'连接失败',failed:'连接失败'}[state]||'待检测');

export function connectionMessage(row:Connection){return connectionState(row)==='check_required'?(hasCheckTime(row.lastCheckedAt)?'检测记录已过期或配置已更新，请重新检测。':'尚未检测当前连接，请先检测。'):row.lastCheckMessage}
