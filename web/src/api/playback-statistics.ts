import {request} from './client';
import type {ManagementPage} from './mediaServer';
import type {MediaTitle} from '../utils/media-title';
export interface StatsFilter{userId?:string;deviceId?:string;days:number;startIndex?:number;limit?:number;group?:string;q?:string}
export interface PlayEvent extends MediaTitle{id:string;itemId:string;userId:string;deviceId:string;userName:string;deviceName:string;client:string;userAgent:string;ip:string;state:string;startedAt:string;lastSeenAt:string;endedAt:string|null;durationSeconds:number;positionTicks:number;redirectCount:number}
export interface PlaySummary{count:number;seconds:number;movies:number;series:number;episodes:number;top:Array<MediaTitle&{itemId:string;count:number;seconds:number}>;trend:Array<{date:string;count:number;seconds:number}>}
export interface IPAccount{id:string;name:string}
export interface IPClient{id:string;name:string;client:string}
export interface IPStats{ip:string;users:number;devices:number;count:number;firstSeenAt:string;lastSeenAt:string;location?:string;label?:string;country?:string;province?:string;city?:string;isp?:string;accounts:IPAccount[]|null;clients:IPClient[]|null;newIP:boolean;differentRegion:boolean}
export interface IPDistribution{name:string;count:number}
export interface IPOverview{uniqueIPs:number;regions:number;activeUsers:number;primaryRegion?:string;provinces:IPDistribution[];cities:IPDistribution[];operators:IPDistribution[]}
export interface IPStatsPage extends ManagementPage<IPStats>{summary?:IPOverview}
export interface IPReport extends IPStatsPage{summary:IPOverview}
function query(f:StatsFilter){return '?'+new URLSearchParams(Object.entries(f).filter(([,v])=>v!==undefined&&v!=='').map(([k,v])=>[k,String(v)]))}
export const getPlaybackStatistics=(f:StatsFilter)=>request<PlaySummary>('/media-server/playback-statistics'+query(f));
export const getPlaybackEvents=(f:StatsFilter)=>request<ManagementPage<PlayEvent>>('/media-server/playback-events'+query(f));
export const getPlaybackIPs=(f:StatsFilter)=>request<IPReport>('/media-server/playback-ips'+query(f));

export const getDeviceIPHistory=(f:StatsFilter)=>request<ManagementPage<IPStats>>('/media-server/device-ips'+query(f));
