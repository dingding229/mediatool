import {afterEach,expect,it,vi} from 'vitest';
import {managedCatalog,managedDetail,listAssets,type MediaAsset} from './mediaCatalog';
afterEach(()=>vi.unstubAllGlobals());
it('keeps managed details and category filtering in the same-origin API transport',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,status:200,json:async()=>({data:{items:[],total:0}})});vi.stubGlobal('fetch',fetch);
 await managedDetail('asset/a');expect(fetch.mock.calls[0]![0]).toBe('/api/v1/media/library/managed/asset%2Fa');
 await managedCatalog('电影 /','series_parent','r:movie:west',2);
 expect(fetch.mock.calls[1]![0]).toContain('categoryId=r%3Amovie%3Awest');expect(fetch.mock.calls[1]![0]).toContain('page=2');
 expect(fetch.mock.calls.every(([,o])=>o.credentials==='same-origin')).toBe(true);
});
it('reads creation time in the existing paged asset request and preserves the filter',async()=>{
 const asset:MediaAsset={id:'asset',revision:1,artifactsState:'ready',title:'电影',mediaType:'movie',path:'/movie.mkv',state:'present',revoked:false,fileScoped:false,poster:'',createdAt:'2026-10-28T10:00:00Z'};
 const fetch=vi.fn().mockResolvedValue({ok:true,status:200,json:async()=>({data:{items:[asset],total:1,page:1,pageSize:50}})});vi.stubGlobal('fetch',fetch);
 expect((await listAssets(1,50)).items[0]?.createdAt).toBe(asset.createdAt);
 expect(fetch.mock.calls[0]![0]).toBe('/api/v1/media/assets?page=1&pageSize=50');
 await listAssets(2,10,'id/filter');
 expect(fetch.mock.calls[1]![0]).toBe('/api/v1/media/assets?page=2&pageSize=10&assetId=id%2Ffilter');
 expect(fetch).toHaveBeenCalledTimes(2);
});
