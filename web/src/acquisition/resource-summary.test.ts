import {it,expect} from 'vitest';
import {resourceKind,resourceSummary} from './resource-summary';
const row:any={title:'Where.the.Mask.Ends.2026.S01E06.2160p.WEB-DL.mkv',siteName:'本地站点',metadata:{subtitle:'电影资源',attributes:{codec:'h265',resolution:'2160p',source:'web-dl'}}};
it('never treats the tracker subtitle as the type label; episodic markers win for display only',()=>{expect(resourceKind(row,{mediaType:'movie'} as any)).toBe('剧集资源');expect(resourceSummary(row,{mediaType:'series'} as any)).toBe('剧集资源 · 本地站点 · H265 · WEB-DL · 2160p');expect(row.metadata.subtitle).toBe('电影资源')});
it('uses selected type without markers and preserves meaningful tracker descriptions in the same metadata line',()=>{const r={...row,title:'Release.2026.mkv',metadata:{...row.metadata,subtitle:'国语中字'}};expect(resourceKind(r,{mediaType:'series'} as any)).toBe('剧集资源');expect(resourceSummary(r,{mediaType:'movie'} as any)).toContain('电影资源 · 国语中字')});
