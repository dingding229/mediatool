import {expect,it} from 'vitest';
import {mediaTitle,playbackDuration} from './media-title';
it('uses only structured episode fields and never adds episode metadata to movies',()=>{
 expect(mediaTitle({name:'地狱大状',type:'episode',season:1,episode:4})).toBe('地狱大状 · 第1季 第4集');
 expect(mediaTitle({name:'剧集',type:'Episode',season:0,episode:2,episodeTitle:'幕后'})).toBe('剧集 · 特别篇 第2集 · 幕后');
 expect(mediaTitle({name:'电影',type:'movie',season:1,episode:4,episodeTitle:'不应出现'})).toBe('电影');
 expect(mediaTitle({name:'标题 s1e4',type:'episode'})).toBe('标题 s1e4');
 expect(playbackDuration(3661)).toBe('1 小时 1 分 1 秒');
});
