import {expect,it,vi} from 'vitest'
import {setupComponent,flush} from '../test/setup-component'
import {pluginUI} from '../api/plugins'
import PluginForm from './PluginForm.vue'
vi.mock('../api/plugins',()=>({pluginUI:vi.fn()}))
it('disabled forms do not invoke RPC or start a worker and explicitly allow JSON configuration',async()=>{
 const view=setupComponent(PluginForm,{instanceId:'disabled',modelValue:'{}',running:false});await flush()
 expect(pluginUI).not.toHaveBeenCalled();expect(view.state.error).toContain('可直接保存 JSON');expect(view.state.error).toContain('不会自动启用');view.unmount()
})
