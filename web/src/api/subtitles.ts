import {request} from './client';
import {requestUUID} from '../utils/request-uuid';
export interface SubtitleLanguage{language:string;languageTags?:string[]}
export interface SubtitleMetadata extends SubtitleLanguage{title:string;default:boolean;forced:boolean;hidden:boolean;revision:number}
export interface Subtitle extends SubtitleMetadata{id:string;assetId:string;source:'automatic'|'embedded'|'uploaded';index:number;filename:string;extension:string;size:number;deleting:boolean;encoding?:string}
export interface SubtitleVersion{id:string;name:string;season?:number;episode?:number}
export interface SubtitlePage{assetId:string;versions:SubtitleVersion[];items:Subtitle[]}
export interface SubtitleCatalog{selectedAssetId:string;versions:SubtitleVersion[];groups:SubtitlePage[]}
export interface LanguageDetection extends SubtitleLanguage{label:string;evidence:string;encoding:string;known:boolean}
const root=(asset:string)=>`/media/assets/${encodeURIComponent(asset)}/subtitles`;
export const listSubtitles=(asset:string)=>request<SubtitlePage>(root(asset));
export const catalogSubtitles=(item:string,asset='')=>request<SubtitleCatalog>(`/media/library/managed/${encodeURIComponent(item)}/subtitles?assetId=${encodeURIComponent(asset)}`);
export function identifySubtitle(file:File,encoding='auto',signal?:AbortSignal){const body=new FormData();body.set('file',file);body.set('encoding',encoding);return request<LanguageDetection>('/media/subtitles/identify',{method:'POST',body,signal})}
export const uploadSubtitle=(asset:string,file:File,encoding='auto',uploadId:string=requestUUID(),selected?:SubtitleLanguage)=>{const body=new FormData();body.append('file',file);body.append('encoding',encoding);body.append('uploadId',uploadId);if(selected){body.set('language',selected.language);body.set('languageTags',JSON.stringify(selected.languageTags||[]))}return request<Subtitle>(root(asset),{method:'POST',body})};
export const saveSubtitle=(asset:string,id:string,metadata:SubtitleMetadata)=>request<Subtitle>(`${root(asset)}/${encodeURIComponent(id)}`,{method:'PUT',body:JSON.stringify({language:metadata.language,languageTags:metadata.languageTags||[],title:metadata.title,default:metadata.default,forced:metadata.forced,hidden:metadata.hidden,revision:metadata.revision})});
export const deleteSubtitle=(asset:string,id:string,revision:number)=>request<{deleted:boolean}>(`${root(asset)}/${encodeURIComponent(id)}`,{method:'DELETE',body:JSON.stringify({revision,confirm:true})});
export const subtitleDownload=(asset:string,id:string)=>`/api/v1${root(asset)}/${encodeURIComponent(id)}/download`;
