import {APIError} from '../api/client'
// Fixed host enums only. Neither Error.message nor an arbitrary server code is a log/display label.
const accessGuidance=new Set(['插件处于只读状态（权限待确认），请在插件页确认权限后重试','插件已停用，无法执行写操作，请在插件页启用后重试','插件处于只读状态，当前调用未获写权限，请在插件页核对授权后重试','插件扩展缺少已确认的权限，请在插件页核对并确认权限后重试'])
const codes=new Set(['invalid_plugin_request','plugin_conflict','plugin_not_found','plugin_permission_denied','plugins_unavailable','unauthorized','forbidden','plugin_busy','invalid_response'])
export function operationError(error:unknown){
 const known=error instanceof APIError
 const code=known&&codes.has(error.code)?error.code:known?'request_rejected':'transport_unknown'
 const requestId=known&&/^[a-f0-9]{24}$/.test(error.requestId)?error.requestId:''
 const uncertain=!known||error.status<400||error.status>=500||error.status===408||error.code==='invalid_response'
 const label=known&&code==='plugin_permission_denied'&&accessGuidance.has(error.message)?error.message:code
 return {uncertain,label:`${label} · requestId=${requestId||'不可用'}`}
}
