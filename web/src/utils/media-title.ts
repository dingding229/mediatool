export interface MediaTitle {name?:string;type?:string;season?:number|null;episode?:number|null;episodeTitle?:string}
// Structured metadata only. Never recover season/episode from a key or filename.
export function mediaTitle(item:MediaTitle){
 const name=item.name||'已移除条目';
 if(item.type?.toLowerCase()!=='episode'||item.season==null||item.episode==null)return name;
 const season=item.season===0?'特别篇':`第${item.season}季`;
 return `${name} · ${season} 第${item.episode}集${item.episodeTitle?' · '+item.episodeTitle:''}`;
}
export const playbackDuration=(seconds=0)=>`${Math.floor(seconds/3600)} 小时 ${Math.floor(seconds%3600/60)} 分 ${Math.floor(seconds%60)} 秒`;
