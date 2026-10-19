import {APIError} from '../api/client'
// Fixed host enums only. Neither Error.message nor an arbitrary server code is a log/display label.
const codes=new Set(['invalid_plugin_request','plugin_conflict','plugin_not_found','plugin_permission_denied','plugins_unavailable','unauthorized','forbidden','plugin_busy','invalid_response'])
export function operationError(error:unknown){
 const known=error instanceof APIError
 const code=known&&codes.has(error.code)?error.code:known?'request_rejected':'transport_unknown'
 const requestId=known&&/^[a-f0-9]{24}$/.test(error.requestId)?error.requestId:''
 const uncertain=!known||error.status>=500||error.status===408||error.code==='invalid_response'
 return {uncertain,label:`${code} · requestId=${requestId||'不可用'}`}
}
