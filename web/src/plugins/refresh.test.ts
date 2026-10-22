import {afterEach,expect,test,vi} from 'vitest'
import {markRaw} from 'vue'
import {setupComponent,flush,deferred} from '../test/setup-component'
import {pluginUI} from '../api/plugins'
import {APIError} from '../api/client'
import Surface from './PluginSurface.vue'
vi.mock('../api/plugins',()=>({pluginUI:vi.fn()}));vi.mock('./esm',()=>({loadPluginUI:vi.fn()}))
let view:ReturnType<typeof setupComponent>
afterEach(()=>view?.unmount())
test('plugin refresh retains successful data, fences late replies, and stops executable UI on authorization failure',async()=>{
 const data={content:[{component:'Text',props:{text:'last successful value'}}]};vi.mocked(pluginUI).mockResolvedValueOnce(data as any)
 view=setupComponent(Surface,{instanceId:'owned',pluginId:'absent',pageKey:'main',refresh:15});await flush();expect(view.state.data).toEqual(data)
 vi.mocked(pluginUI).mockRejectedValueOnce(new APIError(503,'unavailable','offline'));view.state.retry();await flush();expect(view.state.data).toEqual(data);expect(view.state.loading).toBe(false)
 view.state.esm=markRaw({render:()=>null});view.state.mode='vue';vi.mocked(pluginUI).mockRejectedValueOnce(new APIError(403,'permission_denied','changed'));view.state.retry();await flush();expect(view.state.esm).toBeUndefined();expect(view.state.mode).toBe('json');expect(view.state.data).toEqual(data)
 const pending=deferred();vi.mocked(pluginUI).mockReturnValueOnce(pending.promise);view.state.retry();view.unmount();pending.resolve({content:[]});await flush();expect(view.state.data).toEqual(data)
})
