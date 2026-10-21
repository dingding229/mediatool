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
  rating?: number;
  size?: number;
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
  return value && /^\/api\/v1\/(?:media\/(?:library\/emby\/[^/?]+\/image|tmdb\/image)|metadata\/image\/)/.test(value) && !/[?&](?:api_key|token|key)=/i.test(value) ? value : tmdbImage(value);
}
export function originalSTRM(item?: MediaItem | null) {
  return item?.source === 'managed' && item.id && !['Series','Season'].includes(item.type || '') && !['missing', 'suspected_missing', 'trashed'].includes(item.state || '') ? `/api/v1/media/assets/${encodeURIComponent(item.id)}/strm` : '';
}
export const sourceState = (e: unknown) => e instanceof APIError && [401, 403].includes(e.status) ? 'unauthorized' : e instanceof APIError && e.status === 501 ? 'unsupported' : 'error';
export const cloudState = (v?: string) => ({ present: '文件在库', suspected_missing: '待完整对账', missing: '文件缺失', trashed: '已在回收目录' } as Record<string, string>)[v || ''] || '—';
