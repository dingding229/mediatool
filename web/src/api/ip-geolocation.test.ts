import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {lookupIP,getGeoStatus} from './ip-geolocation';
beforeEach(()=>{vi.stubGlobal('fetch',vi.fn(async(url:string)=>({ok:true,status:200,json:async()=>({data:url.includes('?')?[{ip:'8.8.8.8',label:'United States California Google LLC'},{ip:'',label:'局域网/保留地址'}]:{source:'ip2region',version:'f8b0792b0cd7',ipv4:true,ipv6:true,cacheStrategy:'content',memoryBytes:48400455}})})))});
afterEach(()=>vi.unstubAllGlobals());
it('coalesces administrator-only lookups without any external browser request',async()=>{
 const [a,b]=await Promise.all([lookupIP('8.8.8.8'),lookupIP('10.0.0.1')]);expect(a.label).toContain('California');expect(b.ip).toBe('');expect(b.label).toBe('局域网/保留地址');expect(fetch).toHaveBeenCalledTimes(1);expect(vi.mocked(fetch).mock.calls[0][0]).toContain('/api/v1/ip-geolocation?');
});
it('reads bundled dual-stack metadata without credentials or writes',async()=>{const status=await getGeoStatus();expect(status.source).toBe('ip2region');expect(status.ipv4&&status.ipv6).toBe(true);const [url,options]=vi.mocked(fetch).mock.calls.at(-1)!;expect(url).toBe('/api/v1/ip-geolocation/settings');expect(options?.method).toBeUndefined();expect(options?.body).toBeUndefined()});
