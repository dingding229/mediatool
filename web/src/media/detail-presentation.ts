import {displayRating,displayResolution,displayMediaSize,type MediaItem} from './model';
export const joinMediaFacts=(values:unknown[])=>values.filter(v=>v!=null&&String(v).trim()!==''&&v!=='—').join(' · ');
export function mediaFacts(item:MediaItem){return [{key:'rating',label:'评分',value:displayRating(item.rating)},{key:'resolution',label:'清晰度',value:displayResolution(item.resolutions)!=='—'?displayResolution(item.resolutions):item.resolution},{key:'size',label:'文件大小',value:displayMediaSize(item.size??item.fileSize)},{key:'category',label:'分类',value:item.category}]}
export function mediaFileFacts(item:MediaItem){return joinMediaFacts([displayMediaSize(item.size??item.fileSize),item.resolution||displayResolution(item.resolutions)])}
