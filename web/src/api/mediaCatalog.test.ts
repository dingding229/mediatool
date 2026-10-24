import {afterEach,expect,it,vi} from 'vitest';
import {managedCatalog,managedDetail} from './mediaCatalog';
afterEach(()=>vi.unstubAllGlobals());
it('keeps managed details and category filtering in the same-origin API transport',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,status:200,json:async()=>({data:{items:[],total:0}})});vi.stubGlobal('fetch',fetch);
 await managedDetail('asset/a');expect(fetch.mock.calls[0]![0]).toBe('/api/v1/media/library/managed/asset%2Fa');
 await managedCatalog('电影 /','series_parent','r:movie:west',2);
 expect(fetch.mock.calls[1]![0]).toContain('categoryId=r%3Amovie%3Awest');expect(fetch.mock.calls[1]![0]).toContain('page=2');
 expect(fetch.mock.calls.every(([,o])=>o.credentials==='same-origin')).toBe(true);
});
