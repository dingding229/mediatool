import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {defineComponent} from 'vue'
import {request,post} from '../api/client'
import {setupComponent,flush,deferred} from '../test/setup-component'
import {useSearch} from './useSearch'
vi.mock('vue-router',()=>({useRouter:()=>({push:vi.fn()})}))
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn(),post:vi.fn()}))
let view:ReturnType<typeof setupComponent>
beforeEach(async()=>{const store=new Map<string,string>();vi.stubGlobal('sessionStorage',{getItem:(k:string)=>store.get(k)||null,setItem:(k:string,v:string)=>store.set(k,v),removeItem:(k:string)=>store.delete(k)});vi.mocked(request).mockReset().mockImplementation(async p=>p==='/integrations'?[{id:'owned-site',kind:'ptsite',name:'站点',enabled:true}]:[]);vi.mocked(post).mockReset();view=setupComponent(defineComponent({setup:useSearch}));await flush()})
afterEach(()=>{view.unmount();vi.unstubAllGlobals()})
it('site failure does not erase recent search records',async()=>{vi.mocked(request).mockImplementation(async p=>{if(p==='/integrations')throw Error('denied');return[{id:'history'}]});await view.state.load();expect(view.state.accountsError).toBeTruthy();expect(view.state.history).toHaveLength(1)})
it('TMDB results never silently start a site search',async()=>{view.state.query='作品';await flush();vi.mocked(request).mockResolvedValue({items:[{id:1,mediaType:'movie',title:'作品'}],cached:true});await view.state.findMedia();expect(post).not.toHaveBeenCalled();expect(view.state.cached).toBe(true)})
it('changing query invalidates late TMDB results',async()=>{view.state.query='旧词';await flush();const pending=deferred();vi.mocked(request).mockReturnValueOnce(pending.promise);const find=view.state.findMedia();view.state.query='新词';await flush();pending.resolve({items:[{id:1,title:'旧结果'}]});await find;expect(view.state.media).toEqual([])})
it('selected media type and site ids are preserved in the accepted request',async()=>{view.state.selected={id:7,mediaType:'series',title:'作品'};view.state.accountIds=['owned-site'];vi.mocked(post).mockResolvedValue({id:'accepted'});await view.state.start();expect(post).toHaveBeenCalledWith('/search',expect.objectContaining({media:{id:7,mediaType:'series'},accountIds:['owned-site']}));expect(view.state.intent.phase).toBe('queued')})
