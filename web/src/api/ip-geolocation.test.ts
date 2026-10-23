import {beforeEach,expect,it,vi} from 'vitest';
import {lookupIP,getGeoStatus,saveGeoSettings} from './ip-geolocation';
beforeEach(()=>{vi.stubGlobal('fetch',vi.fn(async(url:string)=>({ok:true,status:200,json:async()=>({data:url.includes('?')?[{ip:'8.8.8.8',label:'中国 广东 深圳 电信'},{ip:''}]:{configured:false,ipv4:false,ipv6:false}})})))});
it('coalesces lookups through administrator API and never sends IPs to CZ88',async()=>{
 const [a,b]=await Promise.all([lookupIP('8.8.8.8'),lookupIP('10.0.0.1')]);expect(a.label).toBe('中国 广东 深圳 电信');expect(b.ip).toBe('');expect(fetch).toHaveBeenCalledTimes(1);expect(vi.mocked(fetch).mock.calls[0][0]).toContain('/api/v1/ip-geolocation?');
});
it('keys use JSON body rather than query string',async()=>{await getGeoStatus();await saveGeoSettings('PRIVATE_KEY','PRIVATE_AES');const [url,options]=vi.mocked(fetch).mock.calls.at(-1)!;expect(url).toBe('/api/v1/ip-geolocation/settings');expect(options?.body).toContain('PRIVATE_KEY');expect(url).not.toContain('PRIVATE_KEY')});
