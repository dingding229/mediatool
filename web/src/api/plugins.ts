import { post, request } from './client'
import inventory from '../../../internal/modules/plugins/v2/library.json'
export const pluginPin = inventory.pin
export interface Permission { scope:string; resources?:Record<string,string[]> }
export interface PluginDefinition { manifest:{id:string;name:string;version:string;description:string;permissions:Permission[]}; fingerprint:string }
export interface PluginInstance { id:string;pluginId:string;desiredState:'enabled'|'disabled'|'absent';state:string;revision:number;reason:string }
export interface PluginOperation { operationKey:string;kind:'install'|'enable'|'disable'|'clone'|'configure'|'review'|'remove';instanceId?:string;pluginId?:string;fingerprint:string;expectedRevision:number;confirm:boolean;scopes:string[];config?:unknown }
export interface Inspection { instance:PluginInstance;config:{value:unknown;revision:number};entries:Array<{key:string;value:unknown;revision:number}>;next:string;fingerprint:string }
export const dangerousScope=(scope:string)=>/^(system\.exec|network|fs\.media|ui\.trusted|settings\.write|events\.chain\.|module\.)/.test(scope)||/\.(write|delete|execute)$/.test(scope)
export const catalog=()=>request<{items:PluginDefinition[];total:number}>('/plugins/catalog?pageSize=200')
export const instances=(page=1)=>request<PluginInstance[]>(`/plugins?page=${page}&pageSize=50`)
export const inspect=(id:string,after='')=>request<Inspection>(`/plugins/${encodeURIComponent(id)}?after=${encodeURIComponent(after)}`)
export const operate=(input:PluginOperation)=>post<PluginInstance>('/plugins/operations',input)
