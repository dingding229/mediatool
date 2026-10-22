import { post, request } from './client'
import inventory from '../../../internal/modules/plugins/v2/library.json'
export const pluginPin = inventory.pin
export interface Permission { scope:string; resources?:Record<string,string[]> }
export interface PluginMetadata { id:string;name:string;version:string;description:string;author?:string;icon?:string;level?:number;history?:Record<string,string>;permissions:Permission[] }
export interface PluginDefinition { manifest:PluginMetadata; fingerprint:string;installedCount?:number }
export interface PluginInstance { id:string;pluginId:string;desiredState:'enabled'|'disabled'|'absent';state:string;revision:number;reason:string;metadata?:PluginMetadata;availability?:string }
export interface PluginOperation { operationKey:string;kind:'install'|'enable'|'disable'|'clone'|'configure'|'review'|'remove';instanceId?:string;pluginId?:string;fingerprint:string;expectedRevision:number;confirm:boolean;scopes:string[];config?:unknown }
export interface Inspection { instance:PluginInstance;config:{value:unknown;revision:number};fingerprint:string }
export const dangerousScope=(scope:string)=>inventory.hostMethods.some(m=>m.scope===scope&&m.level==='D')||/^(auth\.provider$|agent\.tools$|system\.exec|downloads\.add$|sites\.request$|sites\.templates\.select$|sites\.content\.raw$|network|fs\.media|ui\.trusted|api\.public|settings\.write|events\.chain\.|module\.)/.test(scope)||/\.(write|delete|execute)$/.test(scope)
export const catalog=(page=1)=>request<{items:PluginDefinition[];total:number}>(`/plugins/catalog?page=${page}&pageSize=200`)
export interface ReviewPlan { instance:PluginInstance;current:PluginMetadata;availability:string;fingerprint:string;approvedVersion:string;baselineKnown:boolean;approvedScopes:string[];added:string[];removed:string[];changed:string[];needsReview:boolean }
export const reviewPlan=(id:string)=>request<ReviewPlan>(`/plugins/${encodeURIComponent(id)}/review`)
export const instances=(page=1)=>request<PluginInstance[]>(`/plugins?page=${page}&pageSize=50`)
export const inspect=(id:string)=>request<Inspection>(`/plugins/${encodeURIComponent(id)}`)
export const operate=(input:PluginOperation)=>post<PluginInstance>('/plugins/operations',input)
export interface ExtensionDescriptor {kind:string;id:string;handler:string;name?:string;cmd?:string;description?:string;trigger?:string;cron?:string;intervalMs?:number;auth?:string;path?:string;priority?:number;requiredScopes:string[]}
export interface ExtensionSnapshot {id:string;instanceId:string;hash:string;metadata:{version:number;descriptors:ExtensionDescriptor[]}}
export interface PluginRun {id:string;kind:string;handler:string;state:string;reason:string;createdAt:string;finishedAt?:string}
export const extensions=(id:string)=>request<ExtensionSnapshot>(`/plugins/${encodeURIComponent(id)}/extensions`)
export const runHistory=(id:string,page=1)=>request<PluginRun[]>(`/plugins/${encodeURIComponent(id)}/runs?page=${page}&pageSize=50`)
export const runExtension=(id:string,input:{kind:string;id:string;operationKey:string;confirm:boolean;args?:unknown})=>post<{runId?:string;jobId?:string}>(`/plugins/${encodeURIComponent(id)}/run`,input)
export const staticPluginForm=(id:string):PluginPage|undefined=>{const form=(inventory as unknown as {forms?:Record<string,PluginPage>}).forms?.[id];return form?JSON.parse(JSON.stringify(form)):undefined}
export interface PluginPage {content:import('../plugins/PluginJson').PluginNode[];defaults?:Record<string,unknown>}
export interface DashboardMeta {key:string;title:string;refresh:number;border:boolean;cols:number}
export const pluginUI=<T=PluginPage>(id:string,kind:'form'|'page'|'dashboard'|'meta',key='')=>request<T>(`/plugins/${encodeURIComponent(id)}/ui/${kind}?key=${encodeURIComponent(key)}`)

export const uiSurfaces=()=>request<Array<{instanceId:string;pluginId:string}>>('/plugins/ui-surfaces')
export interface WorkflowSummary {id:string;revision:number;fingerprint:string;approved:string;steps:number}
export interface WorkflowDefinition {dag:{id:string;steps:Array<{id:string;instanceId:string;action:string;dependsOn:string[];input:{data:Record<string,unknown>}}>};revision:number;fingerprint:string;approved:string}
export interface WorkflowRunView {id:string;workflowId:string;state:string;step?:string;steps:Record<string,string>;reasons:Record<string,string>}
export const workflows=(page=1)=>request<WorkflowSummary[]>(`/plugins/workflows?page=${page}&pageSize=20`)
export const workflowDefinition=(id:string)=>request<WorkflowDefinition>(`/plugins/workflows?workflowId=${encodeURIComponent(id)}&view=definition`)
export const workflowRuns=(id:string,page=1)=>request<WorkflowRunView[]>(`/plugins/workflows?workflowId=${encodeURIComponent(id)}&view=runs&page=${page}&pageSize=20`)
export const workflowOperation=(input:Record<string,unknown>)=>post<WorkflowDefinition|WorkflowRunView>('/plugins/workflows',input)

export interface CapabilityStatus {kind:string;id:string;state:'unsupported'|'authorization_required'|'disabled'|'runtime_failed'|'starting'|'ready';nextAction:string;reason:string}
export const capabilities=(id:string)=>request<CapabilityStatus[]>(`/plugins/${encodeURIComponent(id)}/capabilities`)
