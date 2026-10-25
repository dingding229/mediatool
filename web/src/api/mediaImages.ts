import {request} from './client';
import type {MetadataTarget} from './mediaMetadata';
export type ImageKind='Primary'|'Backdrop'|'Logo';
export const imageKinds:ImageKind[]=['Primary','Backdrop','Logo'];
export interface MediaImage {id?:string;tag?:string;url:string}
export interface ImageRecord {target:MetadataTarget;targetKey:string;revision:number;base:Partial<Record<ImageKind,MediaImage>>;effective:Partial<Record<ImageKind,MediaImage>>;overrides:Partial<Record<ImageKind,string>>;matched:boolean;unmatched:MetadataTarget[];choices:MetadataTarget[]}
export interface ImageChange {targetKey:string;revision:number;kind:ImageKind}
export type ImageCandidates=Partial<Record<ImageKind,{path:string;url:string}[]>>;
const path=(id:string)=>`/media/library/managed/${encodeURIComponent(id)}/images`;
export const readMediaImages=(id:string)=>request<ImageRecord>(path(id));
export const readImageCandidates=(id:string)=>request<ImageCandidates>(path(id)+'/candidates');
export const changeMediaImage=(id:string,change:ImageChange,imagePath='')=>request<ImageRecord>(path(id),{method:'PATCH',body:JSON.stringify({...change,path:imagePath})});
export function uploadMediaImage(id:string,change:ImageChange,file:File){const body=new FormData();body.set('targetKey',change.targetKey);body.set('revision',String(change.revision));body.set('kind',change.kind);body.set('file',file);return request<ImageRecord>(path(id),{method:'POST',body});}
