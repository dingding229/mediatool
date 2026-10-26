import {beforeEach,afterEach,expect,it,vi} from 'vitest'
import {defineComponent} from 'vue'
import {request,APIError} from '../api/client'
import {setupComponent,flush,deferred} from '../test/setup-component'
import {useCatalog} from './useCatalog'
import {image,sourceState} from './model'
vi.mock('vue-router',()=>({useRoute:()=>({query:{}}),useRouter:()=>({replace:vi.fn()})}))
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn()}))
let view:ReturnType<typeof setupComponent>
beforeEach(async()=>{vi.mocked(request).mockReset().mockResolvedValue({items:[],total:0});view=setupComponent(defineComponent({setup:useCatalog}));await flush()})
afterEach(()=>view.unmount())
it.each([[401,'unauthorized'],[403,'unauthorized'],[501,'unsupported'],[503,'error']])('maps HTTP %i without confusing an unavailable source with empty data', (status,state)=>{expect(sourceState(new APIError(Number(status),'unavailable','不可用'))).toBe(state)})
it('posters never include upstream credentials or an arbitrary remote origin',()=>{expect(image('https://emby.invalid/poster?api_key=secret')).toBe('');expect(image('/api/v1/media/library/emby/owned/image?api_key=secret')).toBe('');expect(image('/api/v1/media/library/emby/owned/image')).toBe('');expect(image('/api/v1/media/library/managed/owned/images/Primary')).toContain('/api/v1/media/library/managed/')})

it('managed is the default read-only source even when a removed plugin source is supplied',()=>{expect(view.state.source).toBe('managed');expect(vi.mocked(request).mock.calls.some(([p])=>p.includes('/media/library?source=managed'))).toBe(true);expect(vi.mocked(request).mock.calls.every(([,o])=>!o?.method||o.method==='GET')).toBe(true)});
it('managed server parents browse seasons and paged episodes beyond the current file page',async()=>{
 vi.mocked(request).mockResolvedValue({items:[{id:'series_work',source:'managed',title:'剧集',mediaType:'series',type:'Series'}],total:1});await view.state.load();expect(view.state.displayItems).toHaveLength(1);
 vi.mocked(request).mockResolvedValue({items:[{id:'season_work_30',source:'managed',title:'第 30 季',season:30,type:'Season'}],total:30});await view.state.inspect(view.state.displayItems[0]);await flush();expect(view.state.childTotal).toBe(30);expect(vi.mocked(request).mock.calls.some(([p])=>p.includes('parentId=series_work'))).toBe(true);
 const selected=view.state.children[0];vi.mocked(request).mockResolvedValue({items:[{id:'ep97',source:'managed',title:'跨页第97集',season:30,episode:97,type:'Episode'}],total:100});await view.state.chooseSeason(selected);expect(view.state.childTotal).toBe(100);expect(view.state.episodes[0].id).toBe('ep97');expect(vi.mocked(request).mock.calls.some(([p])=>p.includes('parentId=season_work_30'))).toBe(true);
});


it('loads persisted managed detail through its API client and preserves numeric facts',async()=>{
 vi.mocked(request).mockResolvedValue({id:'owned',title:'持久化详情',rating:6.681,size:7960117968,resolutions:[{width:1920,height:872}],category:'电影 / 欧美电影'});
 await view.state.inspect({id:'owned',title:'列表标题',source:'managed'});
 expect(view.state.detail.title).toBe('持久化详情');expect(view.state.detail.rating).toBe(6.681);
 expect(vi.mocked(request)).toHaveBeenCalledWith('/media/library/managed/owned');
 expect(vi.mocked(request).mock.calls.some(([p])=>p.startsWith('/metadata/details'))).toBe(false);
});
it('passes stable category identity before pagination and uses server options',async()=>{
 vi.mocked(request).mockResolvedValue({items:[],total:0,categories:[{id:'r:movie:western',name:'电影 / 欧美电影',group:'电影'}]});
 view.state.category='r:movie:western';await view.state.search();await flush();
 expect(vi.mocked(request).mock.calls.some(([p])=>p.includes('categoryId=r%3Amovie%3Awestern')&&p.includes('page=1'))).toBe(true);
 expect(view.state.categories[0].name).toBe('电影 / 欧美电影');
});

it('T16 no external connection or library is read',()=>{expect(view.state.source).toBe('managed');expect(vi.mocked(request).mock.calls.every(([p])=>!p.includes('emby')&&p!=='/integrations')).toBe(true)})
