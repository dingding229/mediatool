import {request} from './client';
export type CronPurpose='generic'|'subscription'|'scan'|'site-metrics'|'backup'|'plugin';
export interface CronPreview {timezone:string;fieldCount:number;serverNow:string;next:string[];exhausted:boolean;minimumIntervalSeconds:number}
export const previewCron=(expression:string,purpose:CronPurpose,signal?:AbortSignal)=>request<CronPreview>('/scheduling/cron/preview',{method:'POST',body:JSON.stringify({expression,purpose}),signal});
