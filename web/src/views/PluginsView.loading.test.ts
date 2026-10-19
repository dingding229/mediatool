import {beforeEach,expect,it,vi} from 'vitest'
import {setupComponent,deferred,flush} from '../test/setup-component'
import PluginsView from './PluginsView.vue'
import {catalog,instances} from '../api/plugins'
vi.mock('../api/plugins',()=>({catalog:vi.fn(),instances:vi.fn(),inspect:vi.fn(),operate:vi.fn(),dangerousScope:()=>false,reviewPlan:vi.fn(),extensions:vi.fn(),runHistory:vi.fn(),runExtension:vi.fn()}))
vi.mock('element-plus',()=>({ElMessageBox:{confirm:vi.fn()}}))
beforeEach(()=>{vi.clearAllMocks();vi.mocked(instances).mockResolvedValue([])})
it('initial list is unknown while either response is pending, not an empty result',async()=>{
 const response=deferred<any>();vi.mocked(catalog).mockReturnValue(response.promise)
 const v=setupComponent(PluginsView)
 try{expect(v.state.listRead).toMatchObject({loading:true,loaded:false});response.resolve({items:[],total:0});await flush();expect(v.state.listRead).toMatchObject({loading:false,loaded:true,error:''})}finally{v.unmount()}
})
it('failed first read remains unknown; retry can establish a genuine empty result',async()=>{
 vi.mocked(catalog).mockRejectedValueOnce(new Error('unavailable')).mockResolvedValueOnce({items:[],total:0})
 const v=setupComponent(PluginsView)
 try{await flush();expect(v.state.listRead).toMatchObject({loading:false,loaded:false});expect(v.state.listRead.error).not.toBe('');await v.state.load();expect(v.state.listRead).toMatchObject({loading:false,loaded:true,error:''})}finally{v.unmount()}
})
