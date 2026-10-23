import {expect,it,vi} from 'vitest';
import {getPlaybackEvents,getPlaybackIPs,getPlaybackStatistics} from '../api/playback-statistics';
it('keeps range user device and pagination in registered API paths',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,status:200,json:async()=>({data:{items:[],total:0}})})));
 const f={userId:'user',deviceId:'device',days:7,startIndex:25,limit:25};await getPlaybackEvents(f);await getPlaybackIPs({...f,group:'province'});await getPlaybackStatistics(f);
 for(const [url] of vi.mocked(fetch).mock.calls){expect(String(url)).toContain('userId=user');expect(String(url)).toContain('deviceId=device');expect(String(url)).toContain('days=7');expect(String(url)).toContain('startIndex=25')}
});
