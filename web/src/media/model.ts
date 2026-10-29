import { APIError } from '../api/client';
import { tmdbImage } from '../utils/tmdb-image';
export interface MediaItem {
  id: string;
  source: string;
  title: string;
  year?: string | number;
  mediaType?: string;
  type?: string;
  overview?: string;
  poster?: string;
  image?: string;
  state?: string;
  filePath?: string;
  season?: number;
  episode?: number;
  progress?: number;
  subtitle?: string;
  tmdbId?: number;
  instanceId?: string;
  groupFiles?: MediaItem[];
  rating?: number | null;
  size?: number | null;
 categoryId?: string;
 category?: string;
 resolutions?: {width:number;height:number}[];
 aggregate?: boolean;
 assetCount?: number;
  resolution?: string;
  genres?: string[];
  episodeDetails?: {
    id: number;
    season: number;
    number: number;
    title: string;
    overview?: string;
  }[];
}
export interface Home {
  users: {
    id: string;
    name: string;
  }[];
  userId: string;
  libraries: MediaItem[];
  playing: MediaItem[];
  resume: MediaItem[];
  recent: MediaItem[];
  latest: MediaItem[];
}
export const rows = (value: any): any[] => Array.isArray(value) ? value : [];
export const mediaType = (value?: string) => ({ movie: '电影', series: '剧集', Movie: '电影', Series: '剧集', Season: '季', Episode: '单集' } as Record<string, string>)[value || ''] || '媒体';
export function image(value?: string) {
  return value && /^\/api\/v1\/(?:media\/(?:library\/(?:managed\/[^/?]+\/images\/(?:Primary|Backdrop|Logo))|tmdb\/image)|metadata\/image\/)/.test(value) && !/[?&](?:api_key|token|key)=/i.test(value) ? value : tmdbImage(value);
}
export const sourceState = (e: unknown) => e instanceof APIError && [401, 403].includes(e.status) ? 'unauthorized' : e instanceof APIError && e.status === 501 ? 'unsupported' : 'error';
export const cloudState = (v?: string) => ({ present: '文件在库', suspected_missing: '待完整对账', missing: '文件缺失', trashed: '已在回收目录' } as Record<string, string>)[v || ''] || '—';

export function displayRating(value?:number|null){return value!=null&&Number.isFinite(value)&&value>0&&value<=10?value.toFixed(1):'—';}
export function displayResolution(values?:{width:number;height:number}[]){
 return values?.filter(v=>Number.isInteger(v.width)&&v.width>0&&Number.isInteger(v.height)&&v.height>0).map(v=>`${v.width>=3800?'4K':v.width>=1900?'1080p':v.width>=1260?'720p':'SD'}（${v.width}×${v.height}）`).join('、')||'—';
}
// Match the owner's media-size convention without changing global capacity units.
export function displayMediaSize(value?:number|null){
 if(value==null||!Number.isFinite(value)||value<0)return '—';
 const units=['B','KB','MB','GB','TB','PB'];let i=0;
 while(value>=1024&&i<units.length-1){value/=1024;i++;}
 return `${value.toFixed(2)} ${units[i]}`;
}
