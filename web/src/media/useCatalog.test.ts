import {beforeEach,afterEach,expect,it,vi} from 'vitest'
import {defineComponent} from 'vue'
import {request,APIError} from '../api/client'
import {setupComponent,flush,deferred} from '../test/setup-component'
import {useCatalog} from './useCatalog'
import {image,originalSTRM,sourceState} from './model'
vi.mock('vue-router',()=>({useRoute:()=>({query:{}}),useRouter:()=>({replace:vi.fn()})}))
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn()}))
let view:ReturnType<typeof setupComponent>
beforeEach(async()=>{vi.mocked(request).mockReset().mockResolvedValue({items:[],total:0});view=setupComponent(defineComponent({setup:useCatalog}));await flush()})
afterEach(()=>view.unmount())
it.each([[401,'unauthorized'],[403,'unauthorized'],[501,'unsupported'],[503,'error']])('maps HTTP %i without confusing an unavailable source with empty data', (status,state)=>{expect(sourceState(new APIError(Number(status),'unavailable','不可用'))).toBe(state)})
it('late catalog data cannot overwrite another source',async()=>{const old=deferred();vi.mocked(request).mockReturnValueOnce(old.promise).mockResolvedValueOnce({items:[{tmdbId:42,mediaType:'movie',title:'插件条目',instanceId:'owned'}],hasMore:true,truncated:true});const pending=view.state.load();view.state.chooseSource('plugins');await flush();old.resolve({items:[{id:'wrong',title:'旧来源'}],total:1});await pending;expect(view.state.items[0].title).toBe('插件条目');expect(view.state.hasMore).toBe(true);expect(request).toHaveBeenCalledWith('/media/library/sources?kind=discover.source&page=1&size=24')})
it('switching source invalidates an in-flight detail response',async()=>{view.state.chooseSource('emby');await flush();const old=deferred();vi.mocked(request).mockReturnValueOnce(old.promise);const first=view.state.inspect({id:'emby-old',source:'emby',title:'旧详情'});view.state.chooseSource('managed');await view.state.inspect({id:'managed',source:'managed',title:'新详情'});old.resolve({id:'emby-old',title:'错误覆盖'});await first;expect(view.state.detail.title).toBe('新详情')})
it('only self-owned present assets get the protected original-STRM link',()=>{expect(originalSTRM({id:'owned',source:'managed',title:'影片',state:'present'})).toBe('/api/v1/media/assets/owned/strm');expect(originalSTRM({id:'upstream',source:'emby',title:'影片'})).toBe('');expect(originalSTRM({id:'missing',source:'managed',title:'影片',state:'missing'})).toBe('')})
it('posters never include upstream credentials or an arbitrary remote origin',()=>{expect(image('https://emby.invalid/poster?api_key=secret')).toBe('');expect(image('/api/v1/media/library/emby/owned/image?api_key=secret')).toBe('');expect(image('/api/v1/media/library/emby/owned/image')).toContain('/api/v1/media/library/emby/')})
