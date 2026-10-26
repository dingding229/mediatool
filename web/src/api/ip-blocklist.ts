import {request,post} from './client';
export interface IPBlockRule{id:string;network:string;note:string;createdAt:string;updatedAt:string;hitCount:number;lastHitAt:string|null}
export interface IPBlockStatus{available:boolean;message?:string;retrySeconds:number;counterWarning:boolean}
export interface IPBlockPage{items:IPBlockRule[];total:number;revision:number;status:IPBlockStatus;currentIp:string;maxRules:number}
export interface IPBlockInput{network:string;note:string;revision:number;confirm:boolean;confirmSelf:boolean}
export interface IPBlockCheck{matched:boolean;ruleId?:string;network?:string;revision:number;candidateNetwork?:string;affectsCurrent:boolean}
export const listIPBlocks=(page=1,q='')=>request<IPBlockPage>('/player-access/ip-blocklist?'+new URLSearchParams({page:String(page),pageSize:'25',q}));
export const checkIPBlock=(address:string,network='')=>post<IPBlockCheck>('/player-access/ip-blocklist/check',{address,network});
export const saveIPBlock=(input:IPBlockInput,id?:string)=>request<{revision:number}>('/player-access/ip-blocklist'+(id?'/'+encodeURIComponent(id):''),{method:id?'PUT':'POST',body:JSON.stringify(input)});
export const deleteIPBlock=(id:string,revision:number)=>request<{revision:number}>('/player-access/ip-blocklist/'+encodeURIComponent(id),{method:'DELETE',body:JSON.stringify({revision,confirm:true})});
