import {request,post} from './client';
export interface BuildInfo{version:string;commit:string;buildTime:string;schema:number;minimumSchema:number;pluginPin:string}
export interface ReleaseInfo{version:string;commit:string;digest:string;minimumSchema:number;schema:number;platform:string;repository:string;tag:string;notes:string;publishedAt:string;url:string}
export interface UpdateState{enabled:boolean;current:BuildInfo;hasUpdate:boolean;intervalHours:number;result:{checkedAt:string;status:string;latest:ReleaseInfo|null}}
function build(value:BuildInfo){if(!value||typeof value.version!=='string'||typeof value.commit!=='string'||typeof value.buildTime!=='string'||!Number.isInteger(value.schema)||typeof value.pluginPin!=='string')throw Error('构建信息不完整');return value}
function state(value:UpdateState){if(!value||typeof value.enabled!=='boolean'||typeof value.hasUpdate!=='boolean'||!value.result||typeof value.result.status!=='string')throw Error('更新检查状态不完整');return value}
export const getBuildInfo=()=>request<BuildInfo>('/system/version').then(build);
export const getUpdateState=()=>request<UpdateState>('/system/updates').then(state);
export const checkUpdates=()=>post<UpdateState>('/system/updates/check',{}).then(state);
export const setUpdateChecks=(enabled:boolean,intervalHours:number)=>request<UpdateState>('/system/updates/settings',{method:'PUT',body:JSON.stringify({enabled,intervalHours})}).then(state);
export const takeUpdateNotice=()=>post<{release:ReleaseInfo|null}>('/system/updates/notice',{});
export interface OnlineUpdateJob{id:string;actor:string;fromVersion:string;fromImage:string;target:ReleaseInfo;phase:string;failure?:string;createdAt:string;updatedAt:string;backup:string;dumpSHA256:string;events:{at:string;phase:string}[]}
export interface OnlineUpdateStatus{recoveryRequired:string[];available:boolean;reason:string;socketMounted:boolean;policyVerified:boolean;job:OnlineUpdateJob|null}
export const getOnlineUpdate=()=>request<OnlineUpdateStatus>('/system/online-update');
export const executeOnlineUpdate=(version:string,digest:string,password:string,confirm:boolean)=>post<OnlineUpdateJob>('/system/online-update',{version,digest,password,confirm});

export const confirmOnlineUpdateRecovery=(jobId:string,password:string,confirm:boolean)=>post<{ok:boolean}>('/system/online-update/control',{action:'confirm_recovery',jobId,password,confirm});

export interface IdleUpdatePolicy{enabled:boolean;startTime:string;endTime:string;quietMinutes:number;revision:number;reason:string}
export interface IdleUpdateCondition{key:string;label:string;passed:boolean;reason:string}
export interface IdleUpdateCheck{ready:boolean;timezone:string;checkedAt:string;conditions:IdleUpdateCondition[];target?:ReleaseInfo}
export const getIdleUpdate=()=>request<IdleUpdatePolicy>('/system/online-update/idle');
export const saveIdleUpdate=(value:IdleUpdatePolicy,password='',confirm=false)=>request<IdleUpdatePolicy>('/system/online-update/idle',{method:'PUT',body:JSON.stringify({...value,password,confirm})});
export const checkIdleUpdate=()=>post<IdleUpdateCheck>('/system/online-update/idle/check',{});
