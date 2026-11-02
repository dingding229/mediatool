import {joinMetadata} from '../presentation/metadata';
import type {TMDBConditions} from '../api/cache';
export function tmdbConditionText(c?:TMDBConditions){
 if(!c)return '旧缓存未记录查询条件';
 return joinMetadata([c.operation==='details'?'作品详情':'关键词搜索',({auto:'自动识别',movie:'电影',series:'剧集'} as Record<string,string>)[c.mediaType||''],c.year,c.language?`语言 ${c.language}`:'语言未记录',c.tmdbId?`TMDB ${c.tmdbId}`:null]);
}
