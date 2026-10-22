import {expect,it} from 'vitest';
import {tmdbImage,cachedTMDBImage} from './images';
it('keeps poster requests in the authenticated API and normalizes existing cached paths',()=>{const expected='/api/v1/metadata/image/w185/owned.jpg';expect(cachedTMDBImage('/owned.jpg')).toBe(expected);expect(cachedTMDBImage('/api/v1/media/tmdb/image?path=%2Fowned.jpg')).toBe(expected);expect(cachedTMDBImage('/api/v1/metadata/image/w500/owned.jpg')).toBe(expected)});
it.each(['https://image.tmdb.org/t/p/w500/private.jpg','http://127.0.0.1/a.jpg','/../secret.jpg','/a.svg','/a.jpg?token=secret'])('rejects non-local or untrusted image path %s',value=>expect(cachedTMDBImage(value)).toBe(''));
it('rejects unsupported image sizes',()=>expect(tmdbImage('/owned.jpg','original')).toBe(''));
