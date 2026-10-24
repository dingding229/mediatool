import {request} from './client';
import {requestUUID} from '../utils/request-uuid';
export interface SubtitleMetadata{language:string;title:string;default:boolean;forced:boolean;hidden:boolean;revision:number}
export interface Subtitle extends SubtitleMetadata{id:string;assetId:string;source:'automatic'|'embedded'|'uploaded';index:number;filename:string;extension:string;size:number;deleting:boolean;encoding?:string}
export interface SubtitlePage{assetId:string;versions:{id:string;name:string}[];items:Subtitle[]}
const root=(asset:string)=>`/media/assets/${encodeURIComponent(asset)}/subtitles`;
export const listSubtitles=(asset:string)=>request<SubtitlePage>(root(asset));
export const uploadSubtitle=(asset:string,file:File,encoding='auto',uploadId:string=requestUUID())=>{const body=new FormData();body.append('file',file);body.append('encoding',encoding);body.append('uploadId',uploadId);return request<Subtitle>(root(asset),{method:'POST',body})};
export const saveSubtitle=(asset:string,id:string,metadata:SubtitleMetadata)=>request<Subtitle>(`${root(asset)}/${encodeURIComponent(id)}`,{method:'PUT',body:JSON.stringify({language:metadata.language,title:metadata.title,default:metadata.default,forced:metadata.forced,hidden:metadata.hidden,revision:metadata.revision})});
export const deleteSubtitle=(asset:string,id:string,revision:number)=>request<{deleted:boolean}>(`${root(asset)}/${encodeURIComponent(id)}`,{method:'DELETE',body:JSON.stringify({revision,confirm:true})});
export const subtitleDownload=(asset:string,id:string)=>`/api/v1${root(asset)}/${encodeURIComponent(id)}/download`;
