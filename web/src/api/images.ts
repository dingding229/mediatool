// Image transports are same-origin authenticated API paths, never public TMDB URLs.
export function tmdbImage(path?:string,size='w500'){
 return path&&/^\/[A-Za-z0-9_-]{1,120}\.(?:jpg|png|webp)$/.test(path)&&['w185','w342','w500','w1280'].includes(size)?`/api/v1/metadata/image/${size}/${path.slice(1)}`:'';
}
export function cachedTMDBImage(value?:string,size='w185'){
 if(!value)return '';
 const match=/^\/api\/v1\/metadata\/image\/w(?:185|342|500|1280)\/([A-Za-z0-9_-]{1,120}\.(?:jpg|png|webp))$/.exec(value);
 if(match)return tmdbImage('/'+match[1],size);
 if(value.startsWith('/api/v1/media/tmdb/image?'))return tmdbImage(new URLSearchParams(value.split('?')[1]).get('path')||'',size);
 return tmdbImage(value,size);
}
