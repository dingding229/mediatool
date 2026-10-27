import {request} from './client';
export const metadataFields=['title','originalTitle','sortName','year','premiereDate','overview','tagline','genres','tags','communityRating','officialRating'] as const;
export type MetadataField=typeof metadataFields[number];
export type MetadataValue=string|number|string[]|null;
export type MetadataValues=Partial<Record<MetadataField,MetadataValue>>;
export interface MetadataTarget {mediaId:string;kind:'movie'|'series'|'season'|'episode';season:number;episode:number}
export interface MetadataRecord {target:MetadataTarget;targetKey:string;revision:number;base:MetadataValues;effective:MetadataValues;overrides:MetadataValues;matched:boolean;unmatched:MetadataTarget[];choices:MetadataTarget[];unmatchedPage?:number;unmatchedTotal?:number}
export interface MetadataPatch {targetKey:string;revision:number;set:MetadataValues;restore:MetadataField[]}
export const readMediaMetadata=(id:string,unmatchedPage=1,unmatchedPageSize=50)=>request<MetadataRecord>(`/media/library/managed/${encodeURIComponent(id)}/metadata?unmatchedPage=${unmatchedPage}&unmatchedPageSize=${unmatchedPageSize}`);
export const saveMediaMetadata=(id:string,patch:MetadataPatch)=>request<MetadataRecord>(`/media/library/managed/${encodeURIComponent(id)}/metadata`,{method:'PATCH',body:JSON.stringify(patch)});
export const metadataTargetID=(t:MetadataTarget)=>`${t.kind}_${t.mediaId}${t.kind==='season'||t.kind==='episode'?`_${t.season}`:''}${t.kind==='episode'?`_${t.episode}`:''}`;
