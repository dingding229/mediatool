import {post,request} from './client';
export interface ExternalKey{id:string;name:string;scopes:string[];createdAt:string;lastUsedAt:string|null;lastIP:string;lastIPSource:string;revokedAt:string|null}
export interface ExternalDisclosure extends ExternalKey{token:string}
export interface ExternalCall{keyId:string;name:string;method:string;path:string;status:number;clientIP:string;ipSource:string;createdAt:string}
export const publicExternalKey=({id,name,scopes,createdAt,lastUsedAt,lastIP,lastIPSource,revokedAt}:ExternalKey):ExternalKey=>({id,name,scopes,createdAt,lastUsedAt,lastIP,lastIPSource,revokedAt});
export const listExternalKeys=()=>request<ExternalKey[]>('/external-api/keys').then(v=>v.map(publicExternalKey));
export const createExternalKey=(name:string,scopes:string[])=>post<ExternalDisclosure>('/external-api/keys',{name,scopes,confirm:true});
export const revealExternalKey=(id:string,action:'view'|'copy')=>post<ExternalDisclosure>(`/external-api/keys/${encodeURIComponent(id)}/reveal`,{action});
export const revokeExternalKey=(id:string)=>post<{revoked:boolean}>(`/external-api/keys/${encodeURIComponent(id)}/revoke`,{confirm:true});
export const deleteExternalKey=(id:string)=>request<{deleted:boolean}>(`/external-api/keys/${encodeURIComponent(id)}`,{method:'DELETE',body:JSON.stringify({confirm:true})});
export const externalCalls=(id='')=>request<ExternalCall[]>(`/external-api/calls?keyId=${encodeURIComponent(id)}&limit=50`);
