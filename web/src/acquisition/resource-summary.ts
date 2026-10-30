import type {Media,Resource} from './model';
import {sourceLabel} from '../utils/resource-source';
// The site's subtitle is not a media-type field. Season/episode release markers
// are presentation evidence only; delivery still uses the server identity gate.
export function resourceKind(row:Resource,media?:Media|null){
 const episodic=/(?:^|[.\s_\-])(?:S\d+(?:E\d+(?:-E?\d+)?)?|E\d+|\d+x\d+)(?=$|[.\s_\-])|第[\d一二三四五六七八九十百]+[季集]/i.test(row.title);
 return episodic||media?.mediaType==='series'?'剧集资源':media?.mediaType==='movie'?'电影资源':'资源';
}
export function resourceSummary(row:Resource,media?:Media|null){
 const subtitle=row.metadata?.subtitle?.trim();
 return [resourceKind(row,media),subtitle&&!/^(电影|剧集|电视剧)资源$/.test(subtitle)?subtitle:'',row.siteName,row.metadata?.attributes?.codec?.toUpperCase(),sourceLabel(row.metadata?.attributes?.source),row.metadata?.attributes?.resolution].filter(Boolean).join(' · ');
}
