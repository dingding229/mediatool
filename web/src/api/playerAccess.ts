import {post,request} from './client';
export type AccessMode='off'|'whitelist'|'blacklist';
export type RuleList='rules'|'blacklistRules';
export interface UARule{id:string;label:string;matchType:'exact'|'prefix'|'contains';userAgent:string;enabled:boolean}
export interface UAPolicy{enabled:boolean;mode:AccessMode;revision:number;rules:UARule[];blacklistRules:UARule[]}
export const getUAPolicy=()=>request<UAPolicy>('/player-access/policy');
export const saveUAPolicy=(value:UAPolicy)=>request<UAPolicy>('/player-access/policy',{method:'PUT',body:JSON.stringify(value)});
export const addUAObservation=(id:string,list:RuleList,revision:number,label:string)=>post(`/player-access/observations/${encodeURIComponent(id)}/${list==='blacklistRules'?'block':'allow'}`,{revision,label,confirm:true});
